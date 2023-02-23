package foreman

import (
	"context"
	"errors"

	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
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
	// Mapping between hunt id and the list of clients that will be
	// scheduled in this run.
	HuntsToClients map[string][]string

	// Lookup of hunt objects for each hunt id
	HuntsByHuntId map[string]*api_proto.Hunt

	// A mapping between each client id and the list of all hunts to
	// be scheduled on this client.
	ClientIdToHunts map[string][]*api_proto.Hunt

	ClientIdToClientRecords map[string]*api.ClientRecord

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

	clientAssignedHunts map[string]*api.ClientRecord

	current_monitoring_state *flows_proto.ClientEventTable
}

func (self *Plan) assignClientToHunt(
	client_info *api.ClientRecord, hunt *api_proto.Hunt) {
	client_id := client_info.ClientId
	planned_hunts, _ := self.ClientIdToHunts[client_id]
	if !huntsContain(planned_hunts, hunt.HuntId) {
		self.ClientIdToHunts[client_id] = append(planned_hunts, hunt)
	}
	self.ClientIdToClientRecords[client_id] = client_info
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

		// Update all the client records to the latest event table
		// version
		self.updateLastEventTimestamp(org_config_obj, clients)

		err := self.sendMessageToClients(ctx, org_config_obj, message, clients)
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
	self.HuntsToClients = make(map[string][]string)

	for client_id, hunts := range self.ClientIdToHunts {
		for _, h := range hunts {
			clients, _ := self.HuntsToClients[h.HuntId]

			// Make a copy
			if !utils.InString(clients, client_id) {
				self.HuntsToClients[h.HuntId] = append([]string{client_id}, clients...)
			}
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

		logger.Info("Scheduling hunt %v on %v clients: %v", hunt.HuntId,
			len(clients), slice(clients, 10))

		self.updateAssignedHunts(org_config_obj, clients, hunt.HuntId)

		err := self.scheduleRequestOnClients(
			ctx, org_config_obj, hunt.StartRequest, clients)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *Plan) updateAssignedHunts(
	config_obj *config_proto.Config,
	client_ids []string, hunt_id string) {

	for _, client_id := range client_ids {
		client_record, pres := self.ClientIdToClientRecords[client_id]
		if !pres {
			continue
		}

		if !utils.InString(client_record.AssignedHunts, hunt_id) {
			client_record.AssignedHunts = append(
				client_record.AssignedHunts, hunt_id)
		}

		self.clientAssignedHunts[client_id] = &api.ClientRecord{
			ClientId:              client_id,
			AssignedHunts:         client_record.AssignedHunts,
			LastEventTableVersion: client_record.LastEventTableVersion,
			LastHuntTimestamp:     client_record.LastHuntTimestamp,
		}
	}
}

func (self *Plan) updateLastEventTimestamp(
	config_obj *config_proto.Config, client_ids []string) {

	for _, client_id := range client_ids {
		client_record, pres := self.ClientIdToClientRecords[client_id]
		if !pres {
			continue
		}

		self.clientAssignedHunts[client_id] = &api.ClientRecord{
			ClientId:              client_id,
			AssignedHunts:         client_record.AssignedHunts,
			LastEventTableVersion: client_record.LastEventTableVersion,
			LastHuntTimestamp:     client_record.LastHuntTimestamp,
		}
	}
}

// Update all affected clients' timestamp to ensure next query we do
// not select them again.
func (self *Plan) closePlan(ctx context.Context,
	org_config_obj *config_proto.Config) error {

	for client_id, record := range self.clientAssignedHunts {
		cvelo_services.SetElasticIndexAsync(org_config_obj.OrgId, "clients",
			client_id+"_hunts", record)
	}
	return nil
}

func NewPlan(config_obj *config_proto.Config) (*Plan, error) {
	client_monitoring_service, err := services.ClientEventManager(config_obj)
	if err != nil {
		return nil, err
	}

	return &Plan{
		HuntsToClients:            make(map[string][]string),
		HuntsByHuntId:             make(map[string]*api_proto.Hunt),
		ClientIdToHunts:           make(map[string][]*api_proto.Hunt),
		ClientIdToClientRecords:   make(map[string]*api.ClientRecord),
		MonitoringTables:          make(map[string]*crypto_proto.VeloMessage),
		MonitoringTablesToClients: make(map[string][]string),
		clientAssignedHunts:       make(map[string]*api.ClientRecord),
		current_monitoring_state:  client_monitoring_service.GetClientMonitoringState(),
	}, nil
}
