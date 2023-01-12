package launcher

import (
	"context"
	"errors"

	cvelo_schema_api "www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self *Launcher) CancelFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id, username string) (
	res *api_proto.StartFlowResponse, err error) {

	if flow_id == "" || client_id == "" {
		return &api_proto.StartFlowResponse{}, nil
	}

	// Update the collection context to set it as stopped.
	collection_details, err := self.GetFlowDetails(
		config_obj, client_id, flow_id)
	if err != nil {
		return nil, err
	}

	collection_context := collection_details.Context
	if collection_context == nil {
		return nil, nil
	}

	if collection_context.State != flows_proto.ArtifactCollectorContext_RUNNING {
		return nil, errors.New("Flow is not in the running state. " +
			"Can only cancel running flows.")
	}

	collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
	collection_context.Status = "Cancelled by " + username
	collection_context.Backtrace = ""

	collection_context_record := cvelo_schema_api.
		ArtifactCollectorRecordFromProto(collection_context)
	err = cvelo_services.SetElasticIndex(ctx,
		config_obj.OrgId, "collections",
		collection_context.SessionId, collection_context_record)
	if err != nil {
		return nil, err
	}

	// Queue a cancel message to the client.
	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil, err
	}

	// Queue a cancellation message to the client for this flow
	// id.
	err = client_info_manager.QueueMessageForClient(ctx, client_id,
		&crypto_proto.VeloMessage{
			Cancel:    &crypto_proto.Cancel{},
			SessionId: flow_id,
		}, true /* notify */, nil)
	if err != nil {
		return nil, err
	}

	return &api_proto.StartFlowResponse{
		FlowId: flow_id,
	}, nil
}
