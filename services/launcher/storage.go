package launcher

import (
	"context"
	"errors"
	"fmt"
	"io"

	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_schema_api "www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	cvelo_utils "www.velocidex.com/golang/cloudvelo/utils"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Written in the flow index
type FlowSummary struct {
	FlowId    string                                `json:"FlowId"`
	Artifacts []string                              `json:"Artifacts"`
	Created   uint64                                `json:"Created"`
	Creator   string                                `json:"Creator"`
	Flow      *flows_proto.ArtifactCollectorContext `json:"_Flow"`
}

func (self *FlowSummary) Summary() *services.FlowSummary {
	return &services.FlowSummary{
		FlowId:    self.FlowId,
		Artifacts: self.Artifacts,
		Created:   self.Created,
		Creator:   self.Creator,
	}
}

type FlowStorageManager struct {
	launcher.FlowStorageManager

	cache *cvelo_utils.Cache
}

func (self *FlowStorageManager) WriteFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext,
	completion func()) error {

	defer func() {
		if completion != nil &&
			!utils.CompareFuncs(completion, utils.SyncCompleter) {
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
		config_obj.OrgId, "transient",
		cvelo_services.DocIdRandom, record)
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
		config_obj.OrgId, "transient",
		cvelo_services.DocIdRandom, record)
}

func (self *FlowStorageManager) ListFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	options result_sets.ResultSetOptions,
	offset int64, length int64) ([]*services.FlowSummary, int64, error) {

	cvelo_services.Count("ListFlows")

	result := []*services.FlowSummary{}
	client_path_manager := paths.NewClientPathManager(client_id)
	file_store_factory := file_store.GetFileStore(config_obj)

	// Try to open the result set.
	rs_reader, err := result_sets.NewResultSetReaderWithOptions(
		ctx, config_obj, file_store_factory,
		client_path_manager.FlowIndex(), options)

	// Index does not exist yet.
	if err != nil || self.shouldRebuildIndex(
		ctx, config_obj, client_id, rs_reader) {

		// Force a rebuild of the index.
		err1 := self.buildIndex(ctx, config_obj, client_id)
		if err1 != nil {
			return nil, 0, fmt.Errorf("buildIndex: %w", err)
		}

		// Try to open it again. Hopefully it works now.
		rs_reader, err = result_sets.NewResultSetReaderWithOptions(
			ctx, config_obj, file_store_factory,
			client_path_manager.FlowIndex(), options)
	}

	if err != nil || rs_reader == nil || rs_reader.TotalRows() <= 0 {
		return result, 0, nil
	}

	err = rs_reader.SeekToRow(offset)
	if errors.Is(err, io.EOF) {
		return result, 0, nil
	}

	if err != nil {
		return nil, 0, fmt.Errorf("SeekToRow %v %w", offset, err)
	}

	// Highly optimized reader for speed.
	json_chan, err := rs_reader.JSON(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("JSON %w", err)
	}

	for serialized := range json_chan {
		summary := &FlowSummary{}
		err = json.Unmarshal(serialized, summary)
		if err == nil {
			result = append(result, summary.Summary())

			key := client_id + "/" + summary.FlowId
			self.cache.Set(key, summary.Flow)
		}
		if int64(len(result)) >= length {
			break
		}
	}

	return result, rs_reader.TotalRows(), nil
}

const (
	getFlowDetailsQuery = `
{
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

	// Hit the cache for the collection context
	key := client_id + "/" + flow_id
	flow_any, pres, age := self.cache.Get(key)
	// Is the value too old?
	if age.IsZero() {
		// Try to re-populate the cache by reading the index in.
		_, _, err := self.ListFlows(ctx, config_obj,
			client_id, result_sets.ResultSetOptions{},
			0, 10000)
		if err != nil {
			return nil, err
		}

		// Try to hit the cache again.
		flow_any, pres, age = self.cache.Get(key)

		// Should not really happen.
		if age.IsZero() {
			return nil, utils.NotFoundError
		}
	}

	if !pres {
		return nil, utils.NotFoundError
	}

	flow, ok := flow_any.(*flows_proto.ArtifactCollectorContext)
	if ok {
		return flow, nil
	}

	return nil, utils.NotFoundError
}

// The old slow version of LoadCollectionContext.
// TODO: Remove when the new code is working well.
func (self *FlowStorageManager) LoadCollectionContextSlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id string) (*flows_proto.ArtifactCollectorContext, error) {

	cvelo_services.Count("LoadCollectionContext")

	hit_chan, err := cvelo_services.QueryChan(ctx,
		config_obj, 1000, config_obj.OrgId, "transient",
		json.Format(getFlowDetailsQuery, client_id, flow_id), "timestamp")
	if err != nil {
		return nil, err
	}

	var collection_context *flows_proto.ArtifactCollectorContext

	for hit := range hit_chan {
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
	if collection_context == nil {
		return nil, errors.New("Not found")
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
