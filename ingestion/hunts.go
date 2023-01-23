package ingestion

import (
	"context"
	"strings"

	ingestor_services "www.velocidex.com/golang/cloudvelo/ingestion/services"
	"www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/hunt_dispatcher"
	"www.velocidex.com/golang/cloudvelo/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services/launcher"
)

func (self Ingestor) maybeHandleHuntResponse(
	ctx context.Context,
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	// Hunt responses have special SessionId like "F.1234.H.1234"
	parts := strings.Split(message.SessionId, ".H.")
	if len(parts) == 1 {
		return nil
	}

	hunt_id := "H." + parts[1]

	// All hunt requests start with an initial log message - we use
	// this log message to increment the hunt scheduled parts and
	// assign the collection to the hunt.
	if message.VQLResponse != nil && message.VQLResponse.Query != nil &&
		strings.Contains(message.VQLResponse.Query.VQL, "Starting Hunt") {

		// Increment the hunt's scheduled count.
		ingestor_services.HuntStatsManager.Update(hunt_id).IncScheduled()
		hunt_flow_entry := &hunt_dispatcher.HuntFlowEntry{
			HuntId:    hunt_id,
			ClientId:  message.Source,
			FlowId:    message.SessionId,
			Timestamp: utils.Clock.Now().Unix(),
			Status:    "started",
		}

		return services.SetElasticIndex(ctx,
			config_obj.OrgId, "hunt_flows",
			message.SessionId, hunt_flow_entry)
	}

	return nil
}

// When a collection is completed and the collection is part of the
// hunt we need to update the hunt's collection list and stats.
func (self Ingestor) maybeHandleHuntFlowStats(
	ctx context.Context,
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext) error {

	// Hunt responses have special SessionId like "F.1234.H.1234"
	parts := strings.Split(collection_context.SessionId, ".H.")
	if len(parts) == 1 {
		return nil
	}

	hunt_id := "H." + parts[1]

	// Figure out if an error occurs
	launcher.UpdateFlowStats(collection_context)

	// An error occured - change the hunt status to error.
	if collection_context.State == flows_proto.ArtifactCollectorContext_RUNNING {
		return nil
	}

	if collection_context.State == flows_proto.ArtifactCollectorContext_ERROR {
		ingestor_services.HuntStatsManager.Update(hunt_id).IncError()
		hunt_flow_entry := &hunt_dispatcher.HuntFlowEntry{
			HuntId:    hunt_id,
			ClientId:  collection_context.ClientId,
			FlowId:    collection_context.SessionId,
			Timestamp: utils.Clock.Now().Unix(),
			Status:    "error",
		}

		return services.SetElasticIndex(ctx,
			config_obj.OrgId, "hunt_flows",
			collection_context.SessionId, hunt_flow_entry)
	}

	// This collection is done, update the hunt status.
	ingestor_services.HuntStatsManager.Update(hunt_id).IncCompleted()
	hunt_flow_entry := &hunt_dispatcher.HuntFlowEntry{
		HuntId:    hunt_id,
		ClientId:  collection_context.ClientId,
		FlowId:    collection_context.SessionId,
		Timestamp: utils.Clock.Now().Unix(),
		Status:    "finished",
	}

	return services.SetElasticIndex(ctx,
		config_obj.OrgId, "hunt_flows",
		collection_context.SessionId, hunt_flow_entry)
}
