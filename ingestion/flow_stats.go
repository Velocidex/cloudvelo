package ingestion

import (
	"context"
	"strings"

	"google.golang.org/protobuf/proto"

	"www.velocidex.com/golang/cloudvelo/schema/api"
	"www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/launcher"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	velo_utils "www.velocidex.com/golang/velociraptor/utils"
)

// When we receive a status we need to modify the collection record.
func (self Ingestor) HandleFlowStats(
	ctx context.Context,
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	if message == nil ||
		message.FlowStats == nil ||
		message.Source == "" ||
		message.SessionId == "" {
		return nil
	}

	msg := message.FlowStats
	query_stats := msg.QueryStatus

	// A client that does not know the flow can not complete it, so
	// the flow is recorded as failed and the hunt stats are left
	// alone.
	unknown_flow := isUnknownFlowReply(msg)
	if unknown_flow {
		// A flow that is missing or already finished must not be
		// changed by a late reply.
		collection_context, err := launcher.LoadCollectionContextFromIndex(
			ctx, config_obj, message.Source, message.SessionId)
		if err != nil {
			if !velo_utils.IsNotFound(err) {
				logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
				logger.Error("HandleFlowStats: Unable to load flow %v for %v: %v",
					message.SessionId, message.Source, err)
			}
			return nil
		}

		if launcher.IsTerminalFlowState(collection_context.State) {
			return nil
		}

		query_stats = failRunningQueries(collection_context.QueryStats,
			msg.QueryStatus[0].GetErrorMessage())
	}

	collector_context := &flows_proto.ArtifactCollectorContext{
		ClientId:                   message.Source,
		SessionId:                  message.SessionId,
		TotalUploadedFiles:         msg.TotalUploadedFiles,
		TotalExpectedUploadedBytes: msg.TotalExpectedUploadedBytes,
		TotalUploadedBytes:         msg.TotalUploadedBytes,
		TotalCollectedRows:         msg.TotalCollectedRows,
		TotalLogs:                  msg.TotalLogs,
		ActiveTime:                 msg.Timestamp,
		QueryStats:                 query_stats,
	}

	failed, completed := calcFlowOutcome(collector_context)

	// Progress messages are written to this doc id
	doc_id := api.GetDocumentIdForCollection(
		message.Source, message.SessionId, "stats")

	// Because we can not guarantee the order of messages written we
	// will write the final stat message to a different id. This
	// ensures it can not be overwritten with an incomplete status
	if completed {
		doc_id = api.GetDocumentIdForCollection(
			message.Source, message.SessionId, "completed")
	}

	stats := api.ArtifactCollectorRecordFromProto(collector_context, doc_id)
	stats.Type = "stats"

	// The status needs to hit the DB quickly, so the GUI can show
	// progress as the collection is received. The bulk data is still
	// stored asyncronously.
	err := services.SetElasticIndex(ctx,
		config_obj.OrgId,
		"transient", services.DocIdRandom,
		stats)
	if err != nil {
		return err
	}

	if unknown_flow {
		return nil
	}

	return self.maybeHandleHuntFlowStats(
		ctx, config_obj, collector_context, failed, completed)
}

// The client replies with a single UNKNOWN_FLOW status when asked
// about a flow it does not know. Older clients send a "Flow not
// known" error instead.
func isUnknownFlowReply(msg *crypto_proto.FlowStats) bool {
	if msg == nil || !msg.FlowComplete || len(msg.QueryStatus) != 1 {
		return false
	}

	status := msg.QueryStatus[0]
	switch status.GetStatus() {
	case crypto_proto.VeloStatus_UNKNOWN_FLOW:
		return true
	case crypto_proto.VeloStatus_OK:
		return false
	}
	return strings.HasPrefix(status.GetErrorMessage(), "Flow not known")
}

// Fails the queries that were still running and keeps the stats of
// the ones that completed.
func failRunningQueries(
	query_stats []*crypto_proto.VeloStatus,
	error_message string) []*crypto_proto.VeloStatus {

	var result []*crypto_proto.VeloStatus
	for _, s := range query_stats {
		s = proto.Clone(s).(*crypto_proto.VeloStatus)
		if s.Status == crypto_proto.VeloStatus_PROGRESS {
			s.Status = crypto_proto.VeloStatus_GENERIC_ERROR
			s.ErrorMessage = error_message
		}
		result = append(result, s)
	}

	// The client never reported any queries for this flow.
	if len(result) == 0 {
		result = append(result, &crypto_proto.VeloStatus{
			Status:       crypto_proto.VeloStatus_GENERIC_ERROR,
			ErrorMessage: error_message,
		})
	}

	return result
}
