package api

import (
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

// The source of truth for this record is
// flows_proto.ArtifactCollectorContext but we extract some of the
// fields into the Elastic schema so they can be searched on.

// We use the database to manipulate exposed fields.
type ArtifactCollectorContext struct {
	ClientId   string   `json:"client_id"`
	SessionId  string   `json:"session_id"`
	Raw        string   `json:"context"`
	CreateTime uint64   `json:"create_time"`
	StartTime  uint64   `json:"start_time"`
	LastActive uint64   `json:"last_active_time"`
	QueryStats []string `json:"query_stats"`
}

func ArtifactCollectorContextToProto(
	in *ArtifactCollectorContext) (*flows_proto.ArtifactCollectorContext, error) {

	result := &flows_proto.ArtifactCollectorContext{}
	err := json.Unmarshal([]byte(in.Raw), result)
	if err != nil {
		return nil, err
	}

	result.ClientId = in.ClientId
	result.SessionId = in.SessionId
	result.CreateTime = in.CreateTime
	result.StartTime = in.StartTime
	result.ActiveTime = in.LastActive
	result.QueryStats = nil

	// Now build the full context from the statuses we received.
	for _, serialized := range in.QueryStats {
		stat := &crypto_proto.VeloStatus{}
		err := json.Unmarshal([]byte(serialized), stat)
		if err != nil {
			continue
		}

		result.QueryStats = append(result.QueryStats, stat)
	}

	UpdateFlowStats(result)

	return result, nil
}

func ArtifactCollectorContextFromProto(
	in *flows_proto.ArtifactCollectorContext) *ArtifactCollectorContext {
	status := []string{}
	for _, s := range in.QueryStats {
		status = append(status, json.MustMarshalString(s))
	}

	result := &ArtifactCollectorContext{
		ClientId:   in.ClientId,
		SessionId:  in.SessionId,
		Raw:        json.MustMarshalString(in),
		CreateTime: in.CreateTime,
		StartTime:  in.StartTime,
		QueryStats: status,
	}

	return result
}

// The collection_context contains high level stats that summarise the
// colletion. We derive this information from the specific results of
// each query.
func UpdateFlowStats(collection_context *flows_proto.ArtifactCollectorContext) {
	// Support older colletions which do not have this info
	if len(collection_context.QueryStats) == 0 {
		return
	}

	// Now update the overall collection statuses based on all the
	// individual query status. The collection status is a high level
	// overview of the entire collection.
	collection_context.State = flows_proto.ArtifactCollectorContext_RUNNING
	collection_context.Status = ""
	collection_context.Backtrace = ""
	for _, s := range collection_context.QueryStats {
		// Get the first errored query.
		if collection_context.State == flows_proto.ArtifactCollectorContext_RUNNING &&
			s.Status != crypto_proto.VeloStatus_OK {
			collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
			collection_context.Status = s.ErrorMessage
			collection_context.Backtrace = s.Backtrace
			break
		}
	}

	// Total execution duration is the sum of all the query durations
	// (this can be faster than wall time if queries run in parallel)
	collection_context.ExecutionDuration = 0
	for _, s := range collection_context.QueryStats {
		collection_context.ExecutionDuration += s.Duration
	}

	collection_context.OutstandingRequests = collection_context.TotalRequests -
		int64(len(collection_context.QueryStats))
	if collection_context.OutstandingRequests <= 0 &&
		collection_context.State == flows_proto.ArtifactCollectorContext_RUNNING {
		collection_context.State = flows_proto.ArtifactCollectorContext_FINISHED
	}
}
