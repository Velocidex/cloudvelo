package launcher

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_schema_api "www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
)

type FlowStorageManager struct {
	launcher.FlowStorageManager
}

func (self *FlowStorageManager) WriteFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext,
	completion func()) error {

	defer func() {
		if completion != nil {
			completion()
		}
	}()

	// Store the collection_context first, then queue all the tasks.
	doc_id := api.GetDocumentIdForCollection(
		flow.ClientId, flow.SessionId, "")

	record := api.ArtifactCollectorRecordFromProto(flow)
	record.Type = "main"
	record.Timestamp = utils.GetTime().Now().UnixNano()

	return cvelo_services.SetElasticIndex(ctx,
		config_obj.OrgId, "collections", doc_id, record)
}

func (self *FlowStorageManager) WriteTask(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, msg *crypto_proto.VeloMessage) error {

	doc_id := api.GetDocumentIdForCollection(client_id, msg.SessionId, "tasks")
	messages := &api_proto.ApiFlowRequestDetails{
		Items: []*crypto_proto.VeloMessage{msg},
	}

	record := api.ArtifactCollectorRecord{
		Type:      "task",
		Timestamp: utils.GetTime().Now().UnixNano(),
		SessionId: msg.SessionId,
		Tasks:     json.MustMarshalString(messages),
	}
	return cvelo_services.SetElasticIndex(ctx,
		config_obj.OrgId, "collections", doc_id, record)
}

// Not used - opensearch is handled with Launcher.GetFlows() directly.
func (self *FlowStorageManager) ListFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	options result_sets.ResultSetOptions,
	offset int64, length int64) ([]*services.FlowSummary, int64, error) {
	return nil, 0, nil
}

const getFlowDetailsQuery = `
{
  "sort": [
   {"type": {"order": "asc"}},
   {"timestamp": {"order": "asc"}}
  ],
  "query": {
     "bool": {
       "must": [
         {"match": {"client_id" : %q}},
         {"match": {"session_id" : %q}}
      ]}
  }
}
`

// Load the collector context from storage.
func (self *FlowStorageManager) LoadCollectionContext(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id string) (*flows_proto.ArtifactCollectorContext, error) {
	if flow_id == "" || client_id == "" {
		return &flows_proto.ArtifactCollectorContext{}, nil
	}

	hits, _, err := cvelo_services.QueryElasticRaw(ctx,
		config_obj.OrgId, "collections",
		json.Format(getFlowDetailsQuery, client_id, flow_id))
	if err != nil {
		return nil, err
	}

	if len(hits) == 0 {
		return nil, NotFoundError
	}

	var collection_context *flows_proto.ArtifactCollectorContext

	for _, hit := range hits {
		item := &cvelo_schema_api.ArtifactCollectorRecord{}
		err = json.Unmarshal(hit, item)
		if err != nil {
			return nil, err
		}

		stats_context, err := item.ToProto()
		if err != nil {
			continue
		}

		if collection_context == nil {
			collection_context = stats_context
			continue
		}

		collection_context = mergeRecords(collection_context, stats_context)
	}

	launcher.UpdateFlowStats(collection_context)

	return collection_context, nil
}

func (self *FlowStorageManager) GetFlowRequests(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, flow_id string,
	offset uint64, count uint64) (*api_proto.ApiFlowRequestDetails, error) {

	doc_id := api.GetDocumentIdForCollection(client_id, flow_id, "tasks")
	raw, err := cvelo_services.GetElasticRecord(
		ctx, config_obj.OrgId, "collections", doc_id)
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
	err = json.Unmarshal([]byte(record.Tasks), &messages)
	return messages, err
}
