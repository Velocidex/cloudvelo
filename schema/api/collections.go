package api

import (
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

// The source of truth for this record is
// flows_proto.ArtifactCollectorContext but we extract some of the
// fields into the Elastic schema so they can be searched on.

// We use the database to manipulate exposed fields.
type ArtifactCollectorContext struct {
	ClientId                   string   `json:"client_id"`
	SessionId                  string   `json:"session_id"`
	Raw                        string   `json:"context"`
	CreateTime                 uint64   `json:"create_time"`
	StartTime                  uint64   `json:"start_time"`
	TotalUploadedFiles         uint64   `json:"total_uploaded_files"`
	TotalExpectedUploadedBytes uint64   `json:"total_expected_uploaded_bytes"`
	TotalUploadedBytes         uint64   `json:"total_uploaded_bytes"`
	TotalCollectedRows         uint64   `json:"total_collected_rows"`
	TotalLogs                  uint64   `json:"total_logs"`
	OutstandingRequests        int64    `json:"outstanding_requests"`
	ExecutionDuration          int64    `json:"execution_duration"`
	State                      int64    `json:"state"`
	ArtifactsWithResults       []string `json:"artifacts_with_results"`
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
	result.TotalUploadedFiles = in.TotalUploadedFiles
	result.TotalExpectedUploadedBytes = in.TotalExpectedUploadedBytes
	result.TotalUploadedBytes = in.TotalUploadedBytes
	result.TotalCollectedRows = in.TotalCollectedRows
	result.TotalLogs = in.TotalLogs
	result.OutstandingRequests = in.OutstandingRequests
	result.ExecutionDuration = in.ExecutionDuration
	switch in.State {
	case 1:
		result.State = flows_proto.ArtifactCollectorContext_RUNNING
	case 2:
		result.State = flows_proto.ArtifactCollectorContext_FINISHED
	case 3:
		result.State = flows_proto.ArtifactCollectorContext_ERROR
	}
	result.ArtifactsWithResults = in.ArtifactsWithResults

	return result, nil
}

func ArtifactCollectorContextFromProto(
	in *flows_proto.ArtifactCollectorContext) *ArtifactCollectorContext {
	result := &ArtifactCollectorContext{
		ClientId:                   in.ClientId,
		SessionId:                  in.SessionId,
		Raw:                        json.MustMarshalString(in),
		CreateTime:                 in.CreateTime,
		StartTime:                  in.StartTime,
		TotalUploadedFiles:         in.TotalUploadedFiles,
		TotalExpectedUploadedBytes: in.TotalExpectedUploadedBytes,
		TotalUploadedBytes:         in.TotalUploadedBytes,
		TotalCollectedRows:         in.TotalCollectedRows,
		TotalLogs:                  in.TotalLogs,
		OutstandingRequests:        in.OutstandingRequests,
		ExecutionDuration:          in.ExecutionDuration,
		ArtifactsWithResults:       in.ArtifactsWithResults,
	}

	switch in.State {
	case flows_proto.ArtifactCollectorContext_RUNNING:
		result.State = 1
	case flows_proto.ArtifactCollectorContext_FINISHED:
		result.State = 2
	case flows_proto.ArtifactCollectorContext_ERROR:
		result.State = 3
	}

	return result
}
