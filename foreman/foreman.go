// The foreman is a batch proces which scans all clients and ensure
// they are assigned all their hunts and are up to date with their
// event tables.

package foreman

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

const (
	clientsLaterThanHuntQuery = `
{
  "query": {"bool": {"must": [
    {
     "range": {
        "last_hunt_timestamp": {"lt": %q}
    }
  }]}},
  "_source": {"includes": ["client_id"]}
}
`

	updateAllClientHuntId = `
{
 "script": {
   "source": "ctx._source.last_hunt_timestamp = params.last_hunt_timestamp;",
   "lang": "painless",
   "params": {
     "last_hunt_timestamp": %q
   }
  },
  "query": {
     "terms": {
        "client_id": %q
     }
  }
}
`
)

type Foreman struct{}

func (self Foreman) scheduleHuntOnClients(
	ctx context.Context,
	org_config_obj *config_proto.Config,
	hunt *api_proto.Hunt, clients []string) error {
	// Try to schedule the hunt efficiently
	laucher_manager, err := services.GetLauncher(org_config_obj)
	if err != nil {
		return err
	}

	multi_launcher, ok := laucher_manager.(cvelo_services.MultiLauncher)
	if ok {
		request := proto.Clone(hunt.StartRequest).(*flows_proto.ArtifactCollectorArgs)
		request.Creator = hunt.HuntId
		err := multi_launcher.ScheduleVQLCollectorArgsOnMultipleClients(
			ctx, org_config_obj, request, clients)
		if err != nil {
			return err
		}
	} else {
		for _, client_id := range clients {
			request := proto.Clone(
				hunt.StartRequest).(*flows_proto.ArtifactCollectorArgs)
			request.Creator = hunt.HuntId
			request.ClientId = client_id
			_, err := laucher_manager.ScheduleArtifactCollectionFromCollectorArgs(
				ctx, org_config_obj, request, request.CompiledCollectorArgs,
				func() {})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (self Foreman) stopHunt(
	ctx context.Context,
	org_config_obj *config_proto.Config, hunt *api_proto.Hunt) error {
	stopHuntQuery := `
{
  "script": {
     "source": "ctx._source.state='STOPPED';",
     "lang": "painless"
  }
}
`

	return cvelo_services.UpdateIndex(
		ctx, org_config_obj.OrgId, "hunts", hunt.HuntId, stopHuntQuery)
}

// Query the backend for the list of all clients which have not
// received this hunt.
func (self Foreman) GetAllHuntClients(
	ctx context.Context,
	org_config_obj *config_proto.Config,
	hunts []*api_proto.Hunt) (map[string][]*api_proto.Hunt, error) {

	logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)

	hunts_by_hunt_id := make(map[string]*api_proto.Hunt)

	// Prepare a plan of all the hunts we are going to launch right
	// now.
	last_hunt_timestamp := uint64(0)
	plan := make(map[string][]*api_proto.Hunt)
	for _, hunt := range hunts {
		if hunt.State != api_proto.Hunt_RUNNING {
			continue
		}

		fmt.Printf("Expired %v now %v\n", hunt.Expires, time.Now().UnixNano())
		if hunt.Expires > uint64(time.Now().UnixNano()) {
			err := self.stopHunt(ctx, org_config_obj, hunt)
			if err != nil {
				return nil, err
			}
			continue
		}

		if last_hunt_timestamp < hunt.CreateTime {
			last_hunt_timestamp = hunt.CreateTime
		}

		hunts_by_hunt_id[hunt.HuntId] = hunt

		logger.Debug("%v: Getting clients older than %v", hunt.HuntId, hunt.CreateTime)
		hits_chan, err := cvelo_services.QueryChan(ctx, org_config_obj,
			1000, org_config_obj.OrgId, "clients",
			json.Format(clientsLaterThanHuntQuery, hunt.CreateTime), "client_id")
		if err != nil {
			return nil, err
		}

		for hit := range hits_chan {
			client_info := &actions_proto.ClientInfo{}
			err := json.Unmarshal(hit, client_info)
			if err != nil {
				continue
			}

			planned_hunts, _ := plan[client_info.ClientId]
			plan[client_info.ClientId] = append(planned_hunts, hunt)
		}
	}

	// Create an inverse mapping from hunts to clients to run on each
	// hunt.
	hunts_to_clients := make(map[string][]string)

	all_clients := make([]string, 0, len(plan))
	for client_id, hunts := range plan {
		all_clients = append(all_clients, client_id)

		for _, h := range hunts {
			clients, _ := hunts_to_clients[h.HuntId]
			hunts_to_clients[h.HuntId] = append(clients, client_id)
		}
	}

	for hunt_id, clients := range hunts_to_clients {
		if len(clients) == 0 {
			continue
		}

		hunt, pres := hunts_by_hunt_id[hunt_id]
		if !pres {
			continue
		}

		logger.Info("Scheduling hunt %v on %v clients: %v", hunt.HuntId,
			len(clients), slice(clients, 10))
		err := self.scheduleHuntOnClients(ctx, org_config_obj, hunt, clients)
		if err != nil {
			return nil, err
		}
	}

	// Update all the client records to the latest hunt timestamp.
	err := cvelo_services.UpdateByQuery(ctx, org_config_obj.OrgId,
		"clients", json.Format(updateAllClientHuntId, last_hunt_timestamp, all_clients))
	if err != nil {
		return nil, err
	}

	return plan, nil
}

func (self Foreman) RunOnce(
	ctx context.Context,
	org_config_obj *config_proto.Config) error {

	hunt_dispatcher, err := services.GetHuntDispatcher(org_config_obj)
	if err != nil {
		return err
	}

	hunts, err := hunt_dispatcher.ListHunts(
		ctx, org_config_obj, &api_proto.ListHuntsRequest{
			Count: 1000,
		})
	if err != nil {
		return err
	}

	client_monitoring_service, err := services.ClientEventManager(org_config_obj)
	if err != nil {
		return err
	}

	monitoring_table := client_monitoring_service.GetClientMonitoringState()

	indexer, err := services.GetIndexer(org_config_obj)
	if err != nil {
		return err
	}

	scope := vql_subsystem.MakeScope()
	clients_chan, err := indexer.SearchClientsChan(
		ctx, scope, org_config_obj, "all", "")
	if err != nil {
		return err
	}

	logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
	for client := range clients_chan {
		err := self.checkHunts(ctx, org_config_obj, hunts.Items, client)
		if err != nil {
			logger.Error("While processing hunts on %v: %v\n",
				client.ClientId, err)
		}

		err = self.checkClientEventTable(
			ctx, org_config_obj, monitoring_table,
			client_monitoring_service, client)
		if err != nil {
			logger.Error("While processing monitoring tables on %v: %v\n",
				client.ClientId, err)
		}
	}

	return nil
}

func (self Foreman) checkClientEventTable(
	ctx context.Context,
	config_obj *config_proto.Config,
	table *flows_proto.ClientEventTable,
	client_monitoring_service services.ClientEventTable,
	client *api_proto.ApiClient) error {

	// The client is up to date if both its table event version is
	// newer than both the actual verion and any labels applied on the
	// client. Since any label update may change the client's specific
	// table we need to recalcuelate the table each time the labels
	// are changed.
	if client.LastEventTableVersion > table.Version &&
		client.LastEventTableVersion >= client.LastLabelTimestamp {
		return nil
	}

	// Update the client stats
	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	// Recalculate the table for the client and schedule an client update.
	update_message := client_monitoring_service.GetClientUpdateEventTableMessage(
		ctx, config_obj, client.ClientId)

	if update_message.UpdateEventTable == nil {
		return errors.New("Invalid event update")
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Updating client monitoring table for %v from %v to %v",
		client.ClientId, client.LastEventTableVersion,
		update_message.UpdateEventTable.Version)

	err = client_info_manager.QueueMessageForClient(
		ctx, client.ClientId, update_message, true, nil)
	if err != nil {
		return err
	}

	// Now update the client stats
	client_info, err := client_info_manager.Get(ctx, client.ClientId)
	if err != nil {
		return err
	}

	client_info.LastEventTableVersion = update_message.UpdateEventTable.Version
	err = client_info_manager.Set(ctx, client_info)
	if err != nil {
		return err
	}

	return nil
}

func (self Foreman) checkHunts(
	ctx context.Context,
	config_obj *config_proto.Config,
	hunts []*api_proto.Hunt,
	client *api_proto.ApiClient) error {

	_, err := self.GetAllHuntClients(ctx, config_obj, hunts)
	return err

	for _, hunt := range hunts {
		// Ignore stopped hunts.
		if hunt.StartRequest == nil ||
			hunt.State != api_proto.Hunt_RUNNING {
			continue
		}

		if hunt.CreateTime > client.LastHuntTimestamp {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Info("Starting flow on %v for hunt %v: %v\n",
				client.ClientId, hunt.HuntId, hunt.StartRequest.Artifacts)

			// Update the client's last hunt timestamp
			client_info_manager, err := services.GetClientInfoManager(config_obj)
			if err != nil {
				return err
			}

			client_info, err := client_info_manager.Get(ctx, client.ClientId)
			if err != nil {
				return err
			}

			client_info.LastHuntTimestamp = hunt.CreateTime
			err = client_info_manager.Set(ctx, client_info)
			if err != nil {
				return err
			}

			// Schedule the hunt on this client.
			laucher_manager, err := services.GetLauncher(config_obj)
			if err != nil {
				return err
			}

			request := proto.Clone(
				hunt.StartRequest).(*flows_proto.ArtifactCollectorArgs)

			request.Creator = hunt.HuntId

			// Schedule the collection on each client
			request.ClientId = client.ClientId
			_, err = laucher_manager.ScheduleArtifactCollectionFromCollectorArgs(
				ctx, config_obj, request,
				hunt.StartRequest.CompiledCollectorArgs, utils.SyncCompleter)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (self Foreman) Start(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Second):
				err := self.RunOnce(ctx, config_obj)
				if err != nil {
					logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
					logger.Error("Foreman: %v", err)
				}
				return
			}
		}
	}()

	return nil
}

func StartForemanService(ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	return Foreman{}.Start(ctx, wg, config_obj)
}

func slice(a []string, length int) []string {
	if length > len(a) {
		length = len(a)
	}
	return a[:length]
}
