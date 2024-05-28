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

	// Mapping between unique update key and list of clients.
	MonitoringTablesToClients map[string][]string

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

	// If the launcher support bulk scheduling do this.
	multi_launcher, ok := laucher_manager.(cvelo_services.MultiLauncher)
	if ok {
		request_copy := proto.Clone(request).(*flows_proto.ArtifactCollectorArgs)
		if request_copy == nil {
			return errors.New("Invalid hunt: no StartRequest")
		}

		return multi_launcher.ScheduleVQLCollectorArgsOnMultipleClients(
			ctx, org_config_obj, request_copy, clients)
	}

	// Otherwise just schedule clients one at the time.
	for _, client_id := range clients {
		request_copy := proto.Clone(request).(*flows_proto.ArtifactCollectorArgs)
		request_copy.ClientId = client_id
		_, err := laucher_manager.WriteArtifactCollectionRecord(
			ctx, org_config_obj, request_copy,
			request_copy.CompiledCollectorArgs,
			func(task *crypto_proto.VeloMessage) {
				client_manager, err := services.GetClientInfoManager(org_config_obj)
				if err != nil {
					return
				}

				client_manager.QueueMessageForClient(
					ctx, client_id, task,
					services.NOTIFY_CLIENT, utils.BackgroundWriter)
			})
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
		for _, client_id := range clients {
			cvelo_services.SetElasticIndexAsync(
				org_config_obj.OrgId,
				"persisted", client_id+"_last_event_version",
				cvelo_services.BulkUpdateIndex, &api.ClientRecord{
					ClientId: client_id,
					LastEventTableVersion: self.
						current_monitoring_state.Version,
					DocType: "clients",
				})
		}

		err := self.sendMessageToClients(
			ctx, org_config_obj, message, clients)
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

		err := self.scheduleRequestOnClients(
			ctx, org_config_obj, hunt.StartRequest, clients)
		if err != nil {
			return err
		}

		// Write a new AssignedHunts record to indicate this hunt ran
		// on this client.
		for _, client_id := range clients {

			// This leaks data as we dont have a way to delete them.
			cvelo_services.SetElasticIndexAsync(
				org_config_obj.OrgId,
				"persisted", cvelo_services.DocIdRandom,
				cvelo_services.BulkUpdateIndex, &api.ClientRecord{
					ClientId:      client_id,
					AssignedHunts: []string{hunt.HuntId},
					DocType:       "clients",
				})
		}
	}

	return nil
}

// Update all affected clients' timestamp to ensure next query we do
// not select them again.
func (self *Plan) closePlan(ctx context.Context,
	org_config_obj *config_proto.Config) error {
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

		current_monitoring_state: client_monitoring_service.GetClientMonitoringState(),
	}, nil
}
