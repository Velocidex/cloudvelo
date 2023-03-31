package launcher

import (
	"context"
	"errors"
	"sort"

	cvelo_schema_api "www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services/launcher"
)

var (
	NotFoundError = errors.New("Not found")
)

// Get all the flows for this client.
const getFlowsQuery = `
{
  "sort": [{
    "session_id": {"order": "desc"}
  }],
  "query": {
     "match": {"client_id" : %q}
  },
  "from": %q,
  "size": %q
}
`

func (self Launcher) GetFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, include_archived bool,
	flow_filter func(flow *flows_proto.ArtifactCollectorContext) bool,
	offset uint64, length uint64) (*api_proto.ApiFlowResponse, error) {

	if length == 0 {
		length = 1000
	} else {
		length = length * 4
	}

	// Elastic limits the size of queries. This should be large enough
	// though.
	if length > 2000 {
		length = 2000
	}

	// Each flow is divided into at least 4 records so we need to
	// overfetch records to have all the data.
	hits, err := cvelo_services.QueryElasticRaw(ctx,
		self.config_obj.OrgId, "collections",
		json.Format(getFlowsQuery, client_id, offset, length))
	if err != nil {
		return nil, err
	}

	// The ArtifactCollectorContext object is made up of two parts:
	// 1. The first part is the created context the GUI has created.
	// 2. The second part is the statistics written by the client's
	//    flow tracker.
	//
	// We need to merge them together and return a combined protobuf.
	lookup := make(map[string]*flows_proto.ArtifactCollectorContext)

	for _, hit := range hits {
		item := &cvelo_schema_api.ArtifactCollectorRecord{}
		err = json.Unmarshal(hit, &item)
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

	result := &api_proto.ApiFlowResponse{}
	for _, record := range lookup {
		launcher.UpdateFlowStats(record)
		result.Items = append(result.Items, record)
	}

	// Show newer sessions before older sessions.
	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].SessionId > result.Items[j].SessionId
	})

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
