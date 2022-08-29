// The foreman is a batch proces which scans all clients and ensure
// they are assigned all their hunts and are up to date with their
// event tables.

package foreman

import (
	"context"
	"errors"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

type Foreman struct{}

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

			utils.Debug(client)
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
