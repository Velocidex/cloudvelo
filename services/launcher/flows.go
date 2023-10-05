package launcher

import (
	"context"
	"errors"
	"strings"

	cvelo_schema_api "www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services/launcher"
)

var (
	NotFoundError = errors.New("Not found")
)

// Get all the flow IDs for this client.
const getFlowsQuery = `
{
  "sort": [{
    "session_id": {"order": "desc"}
  }],
  "query": {
     "bool": {
       "must": [
           {"match": {"client_id" : %q}},
           {"match": {"type": "main"}}
       ]}
  },
  "_source": false,
  "from": %q,
  "size": %q
}
`

func (self Launcher) GetFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	options result_sets.ResultSetOptions,
	offset int64, length int64) (*api_proto.ApiFlowResponse, error) {

	ids, total, err := cvelo_services.QueryElasticIds(ctx,
		self.config_obj.OrgId, "collections",
		json.Format(getFlowsQuery, client_id, offset, length))
	if err != nil {
		return nil, err
	}

	var flow_ids []string
	var lookup_ids []string

	// The ArtifactCollectorContext object is made up of several parts:
	// 1. The first part is the created context the GUI has created.
	// 2. The second part is the statistics written by the client's
	//    flow tracker.
	//
	// We need to merge them together and return a combined protobuf.
	lookup := make(map[string]*flows_proto.ArtifactCollectorContext)

	for _, id := range ids {
		flow_id := strings.Split(id, "_")[0]
		flow_ids = append(flow_ids, flow_id)
		lookup_ids = append(lookup_ids, id)
		lookup_ids = append(lookup_ids, id+"_tasks")
		lookup_ids = append(lookup_ids, id+"_completed")
		lookup_ids = append(lookup_ids, id+"_stats")
	}

	records, err := cvelo_services.GetMultipleElasticRecords(ctx,
		config_obj.OrgId, "collections", lookup_ids)
	if err != nil {
		return nil, err
	}

	for _, record := range records {
		item := &cvelo_schema_api.ArtifactCollectorRecord{}
		err = json.Unmarshal(record, &item)
		if err != nil {
			continue
		}

		stats_context, err := item.ToProto()
		if err != nil {
			continue
		}

		collection_context, pres := lookup[item.SessionId]
		if !pres {
			lookup[item.SessionId] = stats_context
			continue
		}
		lookup[item.SessionId] = mergeRecords(collection_context, stats_context)
	}

	// Return the results in the required order
	result := &api_proto.ApiFlowResponse{
		Total: uint64(total),
	}
	for _, flow_id := range flow_ids {
		record, pres := lookup[flow_id]
		if !pres {
			continue
		}
		launcher.UpdateFlowStats(record)
		result.Items = append(result.Items, record)
	}
	return result, nil
}

// Are any queries currenrly running.
func is_running(context *flows_proto.ArtifactCollectorContext) bool {
	for _, s := range context.QueryStats {
		if s.Status == crypto_proto.VeloStatus_PROGRESS {
			return true
		}
	}
	return false
}

func mergeRecords(
	collection_context *flows_proto.ArtifactCollectorContext,
	stats_context *flows_proto.ArtifactCollectorContext) *flows_proto.ArtifactCollectorContext {

	if stats_context.Request != nil {
		collection_context.Request = stats_context.Request
	}

	// Copy relevant fields into the main context
	if stats_context.TotalUploadedFiles > 0 {
		collection_context.TotalUploadedFiles = stats_context.TotalUploadedFiles
	}

	if stats_context.TotalExpectedUploadedBytes > 0 {
		collection_context.TotalExpectedUploadedBytes = stats_context.TotalExpectedUploadedBytes
	}

	if stats_context.TotalUploadedBytes > 0 {
		collection_context.TotalUploadedBytes = stats_context.TotalUploadedBytes
	}

	if stats_context.TotalCollectedRows > 0 {
		collection_context.TotalCollectedRows = stats_context.TotalCollectedRows
	}

	if stats_context.TotalLogs > 0 {
		collection_context.TotalLogs = stats_context.TotalLogs
	}

	if stats_context.ActiveTime > 0 {
		collection_context.ActiveTime = stats_context.ActiveTime
	}

	if stats_context.CreateTime > 0 {
		collection_context.CreateTime = stats_context.CreateTime
	}

	// We will encounter two records with QueryStats: the progress
	// messages and the completed messages. Make sure that if we see a
	// completion message it always replaces the progress message
	// regardless which order it appears.
	if len(stats_context.QueryStats) > 0 {
		if len(collection_context.QueryStats) == 0 ||
			is_running(collection_context) && !is_running(stats_context) {
			collection_context.QueryStats = stats_context.QueryStats
		}
	}

	return collection_context
}
