package launcher

import (
	"testing"

	"github.com/stretchr/testify/require"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

func TestMergeRecordsNilAccumulator(t *testing.T) {
	stats := &flows_proto.ArtifactCollectorContext{
		ClientId:   "C.1234",
		SessionId:  "F.1234",
		ActiveTime: 1234,
	}

	merged := mergeRecords(nil, stats)
	require.NotNil(t, merged)
	require.Equal(t, "F.1234", merged.SessionId)
	require.Equal(t, uint64(1234), merged.ActiveTime)
}

func TestMergeRecordsNilRecord(t *testing.T) {
	base := &flows_proto.ArtifactCollectorContext{
		ClientId:  "C.1234",
		SessionId: "F.1234",
	}

	merged := mergeRecords(base, nil)
	require.Same(t, base, merged)
}

func TestMergeRecordsBothNil(t *testing.T) {
	require.Nil(t, mergeRecords(nil, nil))
}
