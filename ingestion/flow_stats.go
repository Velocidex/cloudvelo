package ingestion

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/schema/api"
	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
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

	collector_context := &flows_proto.ArtifactCollectorContext{
		ClientId:                   message.Source,
		SessionId:                  message.SessionId,
		TotalUploadedFiles:         msg.TotalUploadedFiles,
		TotalExpectedUploadedBytes: msg.TotalExpectedUploadedBytes,
		TotalUploadedBytes:         msg.TotalUploadedBytes,
		TotalCollectedRows:         msg.TotalCollectedRows,
		TotalLogs:                  msg.TotalLogs,
		ActiveTime:                 msg.Timestamp,
		QueryStats:                 msg.QueryStatus,
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
		config_obj.OrgId, "transient", "", stats)
	if err != nil {
		return err
	}

	return self.maybeHandleHuntFlowStats(
		ctx, config_obj, collector_context, failed, completed)
}
