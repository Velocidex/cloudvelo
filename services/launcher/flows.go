package launcher

import (
	"errors"
	"fmt"

	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

var (
	NotFoundError = errors.New("Not found")
)

// Get all the flow IDs for this client.
const (
	prefixQuery = `{"prefix": {"id": "%v"}}`
	regexQuery  = `{"regexp": {"id": "%v[_task|_stats|_stats_completed|_completed]*"}}`

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

	// Each FlowStats message the client sends contains the complete,
	// cumulative status of every query in the flow so far (see
	// responder.FlowContext.getStats() upstream) - never a partial
	// update. Therefore, as long as our accumulator has not yet reached
	// a terminal state, always adopt the latest snapshot wholesale: it
	// is always at least as current as what we have. Once the
	// accumulator reaches a terminal state (is_running == false), stop
	// updating - this correctly freezes on the final message even if a
	// stray/out-of-order message arrives later.
	if len(stats_context.QueryStats) > 0 {
		if len(collection_context.QueryStats) == 0 ||
			is_running(collection_context) {
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
