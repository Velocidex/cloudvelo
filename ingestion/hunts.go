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
)

const (
	updateHuntScheduledQuery = `
{
  "script" : {
    "source": "ctx._source.scheduled++ ;",
    "lang": "painless"
  }
}
`

	updateHuntCompletedQuery = `
{
  "script" : {
    "source": "ctx._source.completed++ ;",
    "lang": "painless"
  }
}
`
	updateHuntErrorQuery = `
{
  "script" : {
    "source": "ctx._source.errors++ ;",
    "lang": "painless"
  }
}
`
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

	if message.Status != nil {
		// An error occured - change the hunt status to error.
		if message.Status.Status != 0 {
			ingestor_services.HuntStatsManager.Update(hunt_id).IncError()
			hunt_flow_entry := &hunt_dispatcher.HuntFlowEntry{
				HuntId:    hunt_id,
				ClientId:  message.Source,
				FlowId:    message.SessionId,
				Timestamp: utils.Clock.Now().Unix(),
				Status:    "error",
			}

			return services.SetElasticIndex(ctx,
				config_obj.OrgId, "hunt_flows",
				message.SessionId, hunt_flow_entry)
		}

		// This collection is done, update the hunt status.
		if message.Status.QueryId == message.Status.TotalQueries {
			ingestor_services.HuntStatsManager.Update(hunt_id).IncCompleted()
			hunt_flow_entry := &hunt_dispatcher.HuntFlowEntry{
				HuntId:    hunt_id,
				ClientId:  message.Source,
				FlowId:    message.SessionId,
				Timestamp: utils.Clock.Now().Unix(),
				Status:    "finished",
			}

			return services.SetElasticIndex(ctx,
				config_obj.OrgId, "hunt_flows",
				message.SessionId, hunt_flow_entry)
		}
	}

	return nil
}
