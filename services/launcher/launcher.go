package launcher

import (
	"context"
	"errors"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

type Launcher struct {
	launcher.Launcher
	ctx        context.Context
	config_obj *config_proto.Config
}

func (self Launcher) ScheduleArtifactCollection(
	ctx context.Context,
	config_obj *config_proto.Config,
	acl_manager vql_subsystem.ACLManager,
	repository services.Repository,
	collector_request *flows_proto.ArtifactCollectorArgs,
	completion func()) (string, error) {
	args := collector_request.CompiledCollectorArgs
	if args == nil {
		// Compile and cache the compilation for next time
		// just in case this request is reused.

		// NOTE: We assume that compiling the artifact is a
		// pure function so caching is appropriate.
		compiled, err := self.CompileCollectorArgs(
			ctx, config_obj, acl_manager, repository,
			services.CompilerOptions{
				ObfuscateNames: true,
			}, collector_request)
		if err != nil {
			return "", err
		}
		args = append(args, compiled...)
	}

	return self.ScheduleArtifactCollectionFromCollectorArgs(
		ctx, config_obj, collector_request, args, completion)
}

// Calculate the Elastic record, and collection context.
func prepareCollectionContext(ctx context.Context,
	config_obj *config_proto.Config,
	collector_request *flows_proto.ArtifactCollectorArgs,
	vql_collector_args []*actions_proto.VQLCollectorArgs) (
	collection_context *flows_proto.ArtifactCollectorContext,
	record *api.ArtifactCollectorRecord, task *crypto_proto.VeloMessage) {

	client_id := collector_request.ClientId
	session_id := collector_request.FlowId

	// Compile all the requests into specific tasks to be sent to the
	// client.
	task = &crypto_proto.VeloMessage{
		SessionId:   session_id,
		RequestId:   constants.ProcessVQLResponses,
		FlowRequest: &crypto_proto.FlowRequest{},
	}

	for _, arg := range vql_collector_args {
		// If sending to the server record who actually launched this.
		if client_id == "server" {
			arg.Principal = collector_request.Creator
		}

		// Add the session ID to the arg for use by internal plugins.
		arg.Env = append(arg.Env, &actions_proto.VQLEnv{
			Key:   "_SessionId",
			Value: session_id,
		})

		// Send an urgent request to the client.
		if collector_request.Urgent {
			task.Urgent = true
		}

		task.FlowRequest.VQLClientActions = append(
			task.FlowRequest.VQLClientActions, arg)
	}

	// Generate a new collection context for this flow.
	collection_context = &flows_proto.ArtifactCollectorContext{
		SessionId:            session_id,
		CreateTime:           uint64(time.Now().UnixNano() / 1000),
		State:                flows_proto.ArtifactCollectorContext_RUNNING,
		Request:              collector_request,
		ClientId:             client_id,
		TotalUploadedFiles:   0,
		TotalUploadedBytes:   0,
		ArtifactsWithResults: []string{},
		TotalRequests:        int64(len(vql_collector_args)),
		OutstandingRequests:  int64(len(vql_collector_args)),
	}

	record = api.ArtifactCollectorRecordFromProto(collection_context)
	record.Tasks = json.MustMarshalString([]*crypto_proto.VeloMessage{task})
	record.Type = "main"

	return collection_context, record, task
}

// The Elastic version stores collections in their own index.
func (self Launcher) ScheduleVQLCollectorArgsOnMultipleClients(
	ctx context.Context,
	config_obj *config_proto.Config,
	collector_request *flows_proto.ArtifactCollectorArgs,
	client_ids []string) error {

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	// If the client info manager supports scheduling the same request
	// on multiple clients we can do this more efficiently.
	multi_scheduler, ok := client_info_manager.(cvelo_services.MultiClientMessageQueuer)
	if ok {
		_, record, task := prepareCollectionContext(
			ctx, config_obj, collector_request,
			collector_request.CompiledCollectorArgs)

		// Write all the collection_context records
		for _, client_id := range client_ids {
			record.ClientId = client_id

			// Store the collection_context first, then queue all the tasks.
			doc_id := api.GetDocumentIdForCollection(
				client_id, collector_request.FlowId, "")
			cvelo_services.SetElasticIndexAsync(
				self.config_obj.OrgId, "collections", doc_id, record)
		}

		return multi_scheduler.QueueMessageForMultipleClients(ctx, client_ids,
			task, services.NOTIFY_CLIENT)
	}

	// Otherwise just schedule all messages one at the time.
	for _, client_id := range client_ids {
		request := proto.Clone(collector_request).(*flows_proto.ArtifactCollectorArgs)

		request.ClientId = client_id
		_, err := self.ScheduleArtifactCollectionFromCollectorArgs(
			ctx, config_obj, request, request.CompiledCollectorArgs,
			func() {})
		if err != nil {
			return err
		}
	}

	return nil
}

// The Elastic version stores collections in their own index.
func (self Launcher) ScheduleArtifactCollectionFromCollectorArgs(
	ctx context.Context,
	config_obj *config_proto.Config,
	collector_request *flows_proto.ArtifactCollectorArgs,
	vql_collector_args []*actions_proto.VQLCollectorArgs,
	completion func()) (string, error) {

	client_id := collector_request.ClientId
	if client_id == "" {
		return "", errors.New("Client id not provided.")
	}

	// Make sure we generate a new flow id if needed
	if collector_request.FlowId == "" {
		collector_request.FlowId = launcher.NewFlowId(client_id)
	}

	collection_context, record, task := prepareCollectionContext(
		ctx, config_obj, collector_request, vql_collector_args)

	// Store the collection_context first, then queue all the tasks.
	doc_id := api.GetDocumentIdForCollection(
		client_id, collector_request.FlowId, "")
	cvelo_services.SetElasticIndexAsync(
		self.config_obj.OrgId, "collections", doc_id, record)

	// Run server artifacts inline.
	if client_id == "server" {
		server_artifacts_service, err := services.GetServerArtifactRunner(
			config_obj)
		if err != nil {
			return "", err
		}

		err = server_artifacts_service.LaunchServerArtifact(
			config_obj, collector_request.FlowId,
			task.FlowRequest, collection_context)
		return collection_context.SessionId, err
	}

	// Actually queue the messages to the client
	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return "", err
	}

	client_info_manager.QueueMessageForClient(
		ctx, client_id, task,
		services.NOTIFY_CLIENT, utils.BackgroundWriter)

	return collection_context.SessionId, nil
}

func (self *Launcher) WriteFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext) error {

	// Store the collection_context first, then queue all the tasks.
	doc_id := api.GetDocumentIdForCollection(
		flow.ClientId, flow.SessionId, "stats")

	stats := api.ArtifactCollectorRecordFromProto(flow)
	stats.Type = "stats"
	stats.Timestamp = utils.GetTime().Now().UnixNano()
	return cvelo_services.SetElasticIndex(ctx,
		config_obj.OrgId, "collections", doc_id, stats)
}

func (self *Launcher) GetFlowRequests(
	config_obj *config_proto.Config,
	client_id string, flow_id string,
	offset uint64, count uint64) (*api_proto.ApiFlowRequestDetails, error) {

	doc_id := api.GetDocumentIdForCollection(client_id, flow_id, "")
	raw, err := cvelo_services.GetElasticRecord(self.ctx,
		config_obj.OrgId, "collections", doc_id)
	if err != nil {
		return nil, err
	}

	record := &api.ArtifactCollectorRecord{}
	err = json.Unmarshal(raw, record)
	if err != nil {
		return nil, err
	}

	messages := &api_proto.ApiFlowRequestDetails{
		Items: []*crypto_proto.VeloMessage{},
	}
	err = json.Unmarshal([]byte(record.Tasks), &messages.Items)
	return messages, err
}

func NewLauncherService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.Launcher, error) {

	return &Launcher{
		ctx:        ctx,
		config_obj: config_obj,
	}, nil
}
