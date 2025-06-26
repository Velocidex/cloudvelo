package foreman

import (
	"context"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	eventsCountGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "foreman_events_gauge",
			Help: "Number of active client events applied to all clients (per organization).",
		},
		[]string{"orgId"},
	)

	eventsByLabelCountGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "foreman_events_by_label_gauge",
			Help: "Number of active client events applied to a specific label (per organization).",
		},
		[]string{"orgId"},
	)
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
		Version: uint64(utils.GetTime().Now().UnixNano()),
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
