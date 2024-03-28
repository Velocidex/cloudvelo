package launcher

import (
	"context"
	"errors"
	"fmt"
	"sort"

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
const (
	prefixQuery = `{"prefix": {"id": "%v"}}`
	regexQuery  = `{"regexp": {"id": "%v[_task|_stats|_stats_completed|_completed]*"}}`

	getCollectionsQuery = `{
  "query": {
    "bool": {
      "must": [
        {
          "match": {
            "doc_type": "collection"
          }
        }, {
          "match": {
            "client_id": %q
          }
        }
      ]
    }
  },
  "size": 10000
}
`

	getFlowsQuery = `
{
  "sort": [{
    "session_id": {"order": "desc"}
  }],
  "query": {
     "bool": {
       "must": [
           {"match": {"client_id" : %q}},
           {"match": {"type": "main"}},
           {"match": {"doc_type": "collection"}}
       ]}
  },
  "_source": true,
  "from": %q,
  "size": %q
}
`
)

func (self Launcher) GetFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	options result_sets.ResultSetOptions,
	offset int64, length int64) (*api_proto.ApiFlowResponse, error) {

	lookup := make(map[string]*flows_proto.ArtifactCollectorContext)

	query := fmt.Sprintf(getCollectionsQuery, client_id)
	records, _, err := cvelo_services.QueryElasticRaw(ctx,
		config_obj.OrgId, "transient", query)
	if err != nil {
		return nil, err
	}

	// Read all the records and merge them into unique records.
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
		Total: uint64(len(lookup)),
	}
	for _, record := range lookup {
		launcher.UpdateFlowStats(record)
		result.Items = append(result.Items, record)
	}

	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].SessionId > result.Items[j].SessionId
	})
	return result, nil
}

// Are any queries currenrly running.
func is_running(context *flows_proto.ArtifactCollectorContext) bool {
	if context.State == flows_proto.ArtifactCollectorContext_ERROR {
		return false
	}

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

	// If the final message is errored or cancelled we update this
	// message.
	if stats_context.State == flows_proto.ArtifactCollectorContext_ERROR {
		collection_context.State = stats_context.State
		collection_context.Status = stats_context.Status
	}

	return collection_context
}

func processIds(queryTemplate string, ids []string) string {
	query := ""
	for i, id := range ids {
		query += fmt.Sprintf(queryTemplate, id)
		if i < len(ids)-1 {
			query += ","
		}
	}
	return query
}
