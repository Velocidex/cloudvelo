package foreman

import (
	"context"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	clientsNeedingMonitoringTableUpdate = `
{
  "query": {"bool": {
    "filter": [
      {"range": {"last_event_table_version": {"lt": %q}}},
      {"range": {"ping": {"gte": %q}}}
    ]}
  },
  "_source": {
     "includes": ["labels", "client_id", "labels_timestamp", "last_event_table_version", "ping"]
  }
}
`
)

// Calculate the event monitoring update message based on the set of
// labels.
func GetClientUpdateEventTableMessage(
	ctx context.Context,
	config_obj *config_proto.Config,
	state *flows_proto.ClientEventTable,
	labels []string) *crypto_proto.VeloMessage {

	result := &actions_proto.VQLEventTable{
		Version: state.Version,
	}

	if state.Artifacts == nil {
		state.Artifacts = &flows_proto.ArtifactCollectorArgs{}
	}

	for _, event := range state.Artifacts.CompiledCollectorArgs {
		result.Event = append(result.Event,
			proto.Clone(event).(*actions_proto.VQLCollectorArgs))
	}

	// Now apply any event queries that belong to this client based on
	// labels.
	for _, table := range state.LabelEvents {
		if utils.InString(labels, table.Label) {
			for _, event := range table.Artifacts.CompiledCollectorArgs {
				result.Event = append(result.Event,
					proto.Clone(event).(*actions_proto.VQLCollectorArgs))
			}
		}
	}

	for _, event := range result.Event {
		if event.MaxWait == 0 {
			event.MaxWait = config_obj.Defaults.EventMaxWait
		}

		if event.MaxWait == 0 {
			event.MaxWait = 120
		}

		// Event queries never time out
		event.Timeout = 99999999
	}

	return &crypto_proto.VeloMessage{
		UpdateEventTable: result,
		SessionId:        constants.MONITORING_WELL_KNOWN_FLOW,
	}
}

// Turn a list of client labels into a unique key. The key is
// constructed from the labels actually in use in the event table.
func labelsKey(labels []string, state *flows_proto.ClientEventTable) string {
	result := make([]string, 0, len(labels))
	for _, table := range state.LabelEvents {
		if utils.InString(labels, table.Label) {
			result = append(result, table.Label)
		}
	}

	return strings.Join(result, "|")
}

func (self Foreman) CalculateEventTable(
	ctx context.Context,
	org_config_obj *config_proto.Config, plan *Plan) error {

	client_monitoring_service, err := services.ClientEventManager(org_config_obj)
	if err != nil {
		return err
	}

	state := client_monitoring_service.GetClientMonitoringState()

	// When do we need to update the client's monitoring table:
	// 1. The client was recently seen
	// 2. Any of its labels were changed since the last update
	// 3. The event table is newer than the last update time.
	query := json.Format(clientsNeedingMonitoringTableUpdate,
		state.Version, Clock.Now().Add(-time.Hour).UnixNano())

	hits_chan, err := cvelo_services.QueryChan(ctx, org_config_obj,
		1000, org_config_obj.OrgId, "clients",
		query, "client_id")
	if err != nil {
		return err
	}

	for hit := range hits_chan {
		client_info := &actions_proto.ClientInfo{}
		err := json.Unmarshal(hit, client_info)
		if err != nil {
			continue
		}

		key := labelsKey(client_info.Labels, state)
		_, pres := plan.MonitoringTables[key]
		if !pres {
			message := GetClientUpdateEventTableMessage(ctx, org_config_obj,
				state, client_info.Labels)
			plan.MonitoringTables[key] = message
		}

		clients, _ := plan.MonitoringTablesToClients[key]
		plan.MonitoringTablesToClients[key] = append(clients,
			client_info.ClientId)
	}

	return nil
}
