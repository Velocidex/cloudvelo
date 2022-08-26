package client_monitoring

import (
	"context"
	"sync"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

type ConfigEntry struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type ClientMonitoringManager struct {
	config_obj *config_proto.Config
}

func (self ClientMonitoringManager) CheckClientEventsVersion(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, client_version uint64) bool {
	return false
}

// Get the message to send to the client in order to force it
// to update.
func (self ClientMonitoringManager) GetClientUpdateEventTableMessage(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) *crypto_proto.VeloMessage {

	return &crypto_proto.VeloMessage{}
}

func (self ClientMonitoringManager) makeDefaultClientMonitoringLabel() *flows_proto.ClientEventTable {
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("Creating default Client Monitoring Service")

	return &flows_proto.ClientEventTable{
		Artifacts: &flows_proto.ArtifactCollectorArgs{
			Artifacts: self.config_obj.Frontend.DefaultClientMonitoringArtifacts,
		},
		LabelEvents: []*flows_proto.LabelEvents{
			{
				Label: "Quarantine",
				Artifacts: &flows_proto.ArtifactCollectorArgs{
					Artifacts: []string{
						"Windows.Remediation.QuarantineMonitor",
					},
				},
			},
		},
	}
}

// Get the full client monitoring table.
func (self ClientMonitoringManager) GetClientMonitoringState() *flows_proto.ClientEventTable {
	ctx := context.Background()
	serialized, err := cvelo_services.GetElasticRecord(
		ctx, self.config_obj.OrgId, "config", "client_monitoring")
	if err != nil {
		return self.makeDefaultClientMonitoringLabel()
	}

	entry := &ConfigEntry{}
	err = json.Unmarshal(serialized, entry)
	if err != nil {
		return self.makeDefaultClientMonitoringLabel()
	}

	result := &flows_proto.ClientEventTable{}
	err = json.Unmarshal([]byte(entry.Data), result)
	if err != nil {
		return self.makeDefaultClientMonitoringLabel()
	}

	return result
}

// Set the client monitoring table.
func (self ClientMonitoringManager) SetClientMonitoringState(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal string,
	state *flows_proto.ClientEventTable) error {

	return cvelo_services.SetElasticIndex(self.config_obj.OrgId,
		"config", "client_monitoring",
		&ConfigEntry{
			Type: "client_monitoring",
			Data: json.MustMarshalString(state),
		})
}

func NewClientMonitoringService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.ClientEventTable, error) {

	event_table := &ClientMonitoringManager{
		config_obj: config_obj,
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Client Monitoring Service")
	return event_table, nil
}
