package foreman

import (
	"context"
	"errors"

	"google.golang.org/protobuf/proto"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

const (
	NOTIFY = true

	updateAllClientEventVersion = `
{
 "query": {
   "terms": {
    "_id": %q
 }},
 "script": {
   "source": "ctx._source.last_event_table_version = params.timestamp;",
   "lang": "painless",
   "params": {
     "timestamp": %q
   }
  }
 }
}
`
)

type Plan struct {
	HuntsToClients  map[string][]string
	HuntsByHuntId   map[string]*api_proto.Hunt
	ClientIdToHunts map[string][]*api_proto.Hunt

	// The below is used to manage updating the client monitoring
	// tables. Client Monitoring applies to label groups. Depending on
	// the client's label assignment, a different set of monitoring
	// queries are issued. As clients lose and gain labels, this needs
	// to be recalculated to see if the client monitoring set needs to
	// change.

	// Mapping between unique update key and update message - this
	// avoids having to recompile messages for each label combination.
	MonitoringTables map[string]*crypto_proto.VeloMessage

	// Mapping beterrn unique update key and list of clients.
	MonitoringTablesToClients map[string][]string
}

func (self *Plan) scheduleRequestOnClients(
	ctx context.Context,
	org_config_obj *config_proto.Config,
	request *flows_proto.ArtifactCollectorArgs, clients []string) error {
	// Try to schedule the hunt efficiently
	laucher_manager, err := services.GetLauncher(org_config_obj)
	if err != nil {
		return err
	}

	multi_launcher, ok := laucher_manager.(cvelo_services.MultiLauncher)
	if ok {
		request_copy := proto.Clone(request).(*flows_proto.ArtifactCollectorArgs)
		if request_copy == nil {
			return errors.New("Invalid hunt: no StartRequest")
		}

		return multi_launcher.ScheduleVQLCollectorArgsOnMultipleClients(
			ctx, org_config_obj, request_copy, clients)

	}

	for _, client_id := range clients {
		request_copy := proto.Clone(request).(*flows_proto.ArtifactCollectorArgs)
		request_copy.ClientId = client_id
		_, err := laucher_manager.ScheduleArtifactCollectionFromCollectorArgs(
			ctx, org_config_obj, request_copy,
			request_copy.CompiledCollectorArgs, func() {})
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *Plan) sendMessageToClients(
	ctx context.Context,
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage, clients []string) error {

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	multi_launcher, ok := client_info_manager.(cvelo_services.MultiClientMessageQueuer)
	if ok {
		return multi_launcher.QueueMessageForMultipleClients(
			ctx, clients, message, !NOTIFY)
	}

	// Otherwise send messages one at a time.
	for _, client_id := range clients {
		err := client_info_manager.QueueMessageForClient(
			ctx, client_id, message, !NOTIFY, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *Plan) ExecuteClientMonitoringUpdate(
	ctx context.Context,
	org_config_obj *config_proto.Config) error {
	logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)

	for key, clients := range self.MonitoringTablesToClients {
		message, pres := self.MonitoringTables[key]
		if !pres {
			continue
		}

		logger.Info("Update Client Monitoring Tables %v on %v clients: %v",
			key, len(clients), slice(clients, 10))

		err := self.sendMessageToClients(ctx, org_config_obj, message, clients)
		if err != nil {
			return err
		}

		// Update all the client records to the latest event table
		// versiono
		query := json.Format(updateAllClientEventVersion, clients,
			Clock.Now().UnixNano())

		err = cvelo_services.UpdateByQuery(ctx, org_config_obj.OrgId,
			"clients", query)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *Plan) ExecuteHuntUpdate(
	ctx context.Context,
	org_config_obj *config_proto.Config) error {
	logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)

	// Create an inverse mapping from hunts to clients to run on each
	// hunt: key->hunt id, value->list of client ids
	for client_id, hunts := range self.ClientIdToHunts {
		for _, h := range hunts {
			clients, _ := self.HuntsToClients[h.HuntId]
			self.HuntsToClients[h.HuntId] = append(clients, client_id)
			self.HuntsByHuntId[h.HuntId] = h
		}
	}

	for hunt_id, clients := range self.HuntsToClients {
		if len(clients) == 0 {
			continue
		}

		hunt, pres := self.HuntsByHuntId[hunt_id]
		if !pres {
			continue
		}

		if hunt == nil || hunt.StartRequest == nil {
			continue
		}

		request := hunt.StartRequest
		request.Creator = hunt.HuntId

		logger.Info("Scheduling hunt %v on %v clients: %v", hunt.HuntId,
			len(clients), slice(clients, 10))
		err := self.scheduleRequestOnClients(ctx, org_config_obj, hunt.StartRequest, clients)
		if err != nil {
			return err
		}

		query := json.Format(updateAllClientHuntId, clients, hunt_id,
			Clock.Now().UnixNano())

		// Update all the client records to the latest hunt timestamp
		// and mark them as having executed this hunt.
		err = cvelo_services.UpdateByQuery(ctx, org_config_obj.OrgId,
			"clients", query)
		if err != nil {
			return err
		}
	}

	return nil
}

// Update all affected clients' timestamp to ensure next query we do
// not select them again.
func (self *Plan) Close(ctx context.Context,
	org_config_obj *config_proto.Config) error {
	return nil
}

func NewPlan() *Plan {
	return &Plan{
		HuntsToClients:            make(map[string][]string),
		HuntsByHuntId:             make(map[string]*api_proto.Hunt),
		ClientIdToHunts:           make(map[string][]*api_proto.Hunt),
		MonitoringTables:          make(map[string]*crypto_proto.VeloMessage),
		MonitoringTablesToClients: make(map[string][]string),
	}
}
