package client_monitoring

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
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

	state := self.GetClientMonitoringState()

	result := &actions_proto.VQLEventTable{
		Version: uint64(time.Now().UnixNano()),
	}

	if state.Artifacts == nil {
		state.Artifacts = &flows_proto.ArtifactCollectorArgs{}
	}

	for _, event := range state.Artifacts.CompiledCollectorArgs {
		result.Event = append(result.Event, proto.Clone(event).(*actions_proto.VQLCollectorArgs))
	}

	// Now apply any event queries that belong to this client based on labels.
	labeler := services.GetLabeler(config_obj)
	for _, table := range state.LabelEvents {
		if labeler.IsLabelSet(ctx, config_obj, client_id, table.Label) {
			for _, event := range table.Artifacts.CompiledCollectorArgs {
				result.Event = append(result.Event,
					proto.Clone(event).(*actions_proto.VQLCollectorArgs))
			}
		}
	}

	// Add a bit of randomness to the max wait to spread out
	// client's updates so they do not syncronize load on the
	// server.
	for _, event := range result.Event {
		// Ensure responses do not come back too quickly
		// because this increases the load on the server. We
		// need the client to queue at least 60 seconds worth
		// of data before reconnecting.
		if event.MaxWait == 0 {
			event.MaxWait = config_obj.Defaults.EventMaxWait
		}

		if event.MaxWait == 0 {
			event.MaxWait = 120
		}

		jitter := config_obj.Defaults.EventMaxWaitJitter
		if jitter == 0 {
			jitter = 20
		}
		event.MaxWait += uint64(rand.Intn(int(jitter)))

		// Event queries never time out
		event.Timeout = 99999999
	}

	return &crypto_proto.VeloMessage{
		UpdateEventTable: result,
		SessionId:        constants.MONITORING_WELL_KNOWN_FLOW,
	}
}

func (self ClientMonitoringManager) makeDefaultClientMonitoringLabel() *flows_proto.ClientEventTable {
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("Creating default Client Monitoring Service")

	return &flows_proto.ClientEventTable{
		Version: uint64(time.Now().UnixNano()),
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
		table := self.makeDefaultClientMonitoringLabel()
		self.SetClientMonitoringState(ctx, self.config_obj, "", table)
		return table
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

	// Compile the monitoring state at the last minute to pick up any
	// changed in the artifacts.
	self.compileState(ctx, self.config_obj, result)

	return result
}

func (self ClientMonitoringManager) compileArtifactCollectorArgs(
	ctx context.Context,
	config_obj *config_proto.Config,
	artifact *flows_proto.ArtifactCollectorArgs) (
	[]*actions_proto.VQLCollectorArgs, error) {

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return nil, err
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	return launcher.CompileCollectorArgs(
		ctx, config_obj, acl_managers.NullACLManager{},
		repository, services.CompilerOptions{
			ObfuscateNames:         true,
			IgnoreMissingArtifacts: true,
		}, artifact)
}

func (self ClientMonitoringManager) compileState(
	ctx context.Context,
	config_obj *config_proto.Config,
	state *flows_proto.ClientEventTable) (err error) {
	if state.Artifacts == nil {
		state.Artifacts = &flows_proto.ArtifactCollectorArgs{}
	}

	// Compile all the artifacts now for faster dispensing.
	compiled, err := self.compileArtifactCollectorArgs(
		ctx, config_obj, state.Artifacts)
	if err != nil {
		return err
	}
	state.Artifacts.CompiledCollectorArgs = compiled

	// Now compile the label specific events
	for _, table := range state.LabelEvents {
		compiled, err := self.compileArtifactCollectorArgs(
			ctx, config_obj, table.Artifacts)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("Unable to start client monitoring service: Error "+
				"compiling artifacts %v: %v", table.Artifacts, err)
			logger.Error("Please correct client_monitoring config file at " +
				"<datastore>/config/client_monitoring.json.db")
			return err
		}
		table.Artifacts.CompiledCollectorArgs = compiled
	}

	return nil
}

// Set the client monitoring table.
func (self ClientMonitoringManager) SetClientMonitoringState(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal string,
	state *flows_proto.ClientEventTable) error {

	state.Version = uint64(time.Now().UnixNano())

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
	return event_table, nil
}
