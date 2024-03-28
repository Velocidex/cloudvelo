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

	record := api.ArtifactCollectorRecordFromProto(flow, doc_id)
	record.Type = "main"
	record.Timestamp = utils.GetTime().Now().UnixNano()

	return cvelo_services.SetElasticIndex(ctx,
		config_obj.OrgId, "transient", "", record)
}

func (self *FlowStorageManager) WriteTask(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, msg *crypto_proto.VeloMessage) error {

	doc_id := api.GetDocumentIdForCollection(client_id, msg.SessionId, "task")
	messages := &api_proto.ApiFlowRequestDetails{
		Items: []*crypto_proto.VeloMessage{msg},
	}

	record := api.ArtifactCollectorRecord{
		Type:      "task",
		Timestamp: utils.GetTime().Now().UnixNano(),
		SessionId: msg.SessionId,
		ClientId:  client_id,
		Tasks:     json.MustMarshalString(messages),
		Doc_Type:  "task",
		ID:        doc_id,
	}
	return cvelo_services.SetElasticIndex(ctx,
		config_obj.OrgId, "transient", "", record)
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

const (
	getFlowDetailsQuery = `
{
  "sort": [
   {"type": {"order": "asc"}},
   {"timestamp": {"order": "asc"}}
  ],
  "query": {
     "bool": {
       "must": [
         {"match": {"client_id" : %q}},
         {"match": {"session_id" : %q}},
         {"match": {"doc_type" : "collection"}}
      ]}
  }
}
`
	getFlowTasksQuery = `
{
  "sort": [
   {"type": {"order": "asc"}},
   {"timestamp": {"order": "asc"}}
  ],
  "query": {
     "bool": {
       "must": [
         {"match": {"client_id" : %q}},
         {"match": {"session_id" : %q}},
         {"match": {"doc_type" : "task"}}
      ]}
  }
}
`
)

// Load the collector context from storage.
func (self *FlowStorageManager) LoadCollectionContext(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id string) (*flows_proto.ArtifactCollectorContext, error) {
	if flow_id == "" || client_id == "" {
		return &flows_proto.ArtifactCollectorContext{}, nil
	}

	hits, _, err := cvelo_services.QueryElasticRaw(ctx,
		config_obj.OrgId, "transient",
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

	if flow_id == "" || client_id == "" {
		return &api_proto.ApiFlowRequestDetails{}, nil
	}

	hits, _, err := cvelo_services.QueryElasticRaw(ctx,
		config_obj.OrgId, "transient",
		json.Format(getFlowTasksQuery, client_id, flow_id))
	if err != nil {
		return nil, err
	}

	if len(hits) == 0 {
		return nil, NotFoundError
	}

	item := &api.ArtifactCollectorRecord{}
	messages := &api_proto.ApiFlowRequestDetails{}
	for _, hit := range hits {
		err = json.Unmarshal(hit, item)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal([]byte(item.Tasks), messages)
		if err != nil {
			return nil, err
		}
	}

	return messages, nil
}
