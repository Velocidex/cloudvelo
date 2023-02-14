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

func calcFlowOutcome(collection_context *flows_proto.ArtifactCollectorContext) (
	failed, completed bool) {

	for _, s := range collection_context.QueryStats {
		switch s.Status {

		// Flow is not completed as one of the queries is still
		// running.
		case crypto_proto.VeloStatus_PROGRESS:
			return false, false

			// Flow failed by it may still be running.
		case crypto_proto.VeloStatus_GENERIC_ERROR:
			failed = true

			// This query is ok
		case crypto_proto.VeloStatus_OK:
		}
	}

	return failed, true
}

// When a collection is completed and the collection is part of the
// hunt we need to update the hunt's collection list and stats.
func (self Ingestor) maybeHandleHuntFlowStats(
	ctx context.Context,
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext,
	failed, completed bool) error {

	// Ignore messages for incompleted flows
	if !completed {
		return nil
	}

	// Hunt responses have special SessionId like "F.1234.H.1234"
	parts := strings.Split(collection_context.SessionId, ".H.")
	if len(parts) == 1 {
		return nil
	}

	hunt_id := "H." + parts[1]

	// Increment the failed flow counter
	if failed {
		ingestor_services.HuntStatsManager.Update(hunt_id).IncError()
	} else {

		// This collection is done, update the hunt status.
		ingestor_services.HuntStatsManager.Update(hunt_id).IncCompleted()
	}

	return nil
}
