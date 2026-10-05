package launcher

import (
	"testing"

	"github.com/stretchr/testify/require"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services/launcher"
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

func statsSnapshot(status crypto_proto.VeloStatus_ReturnedStatus,
	rows, bytes int64) *flows_proto.ArtifactCollectorContext {
	return &flows_proto.ArtifactCollectorContext{
		ClientId:  "C.1234",
		SessionId: "F.1234",
		QueryStats: []*crypto_proto.VeloStatus{
			{Status: status, ResultRows: rows, UploadedBytes: bytes},
		},
	}
}

// foldFlowIndex mirrors how buildIndex (index.go) folds a client's
// collection records for one session, so these tests exercise the real
// merge path rather than an arbitrary loop.
func foldFlowIndex(
	records ...*flows_proto.ArtifactCollectorContext) *flows_proto.ArtifactCollectorContext {
	var acc *flows_proto.ArtifactCollectorContext
	for i, r := range records {
		if i == 0 {
			acc = &flows_proto.ArtifactCollectorContext{
				ClientId:  r.ClientId,
				SessionId: r.SessionId,
				State:     r.State,
			}
		}
		acc = mergeRecords(acc, r)
	}
	launcher.UpdateFlowStats(acc)
	return acc
}

func TestMergeRecordsProgressNotFrozen(t *testing.T) {
	acc := foldFlowIndex(
		statsSnapshot(crypto_proto.VeloStatus_PROGRESS, 5, 100),
		statsSnapshot(crypto_proto.VeloStatus_PROGRESS, 50, 5000),
	)

	require.Equal(t, uint64(50), acc.TotalCollectedRows)
	require.Equal(t, uint64(5000), acc.TotalUploadedBytes)
	require.Equal(t, flows_proto.ArtifactCollectorContext_RUNNING, acc.State)
}

func TestMergeRecordsCompletionRecorded(t *testing.T) {
	acc := foldFlowIndex(
		statsSnapshot(crypto_proto.VeloStatus_PROGRESS, 5, 100),
		statsSnapshot(crypto_proto.VeloStatus_OK, 42, 12345),
		statsSnapshot(crypto_proto.VeloStatus_PROGRESS, 999, 999999),
	)

	require.Equal(t, uint64(42), acc.TotalCollectedRows)
	require.Equal(t, uint64(12345), acc.TotalUploadedBytes)
	require.Equal(t, flows_proto.ArtifactCollectorContext_FINISHED, acc.State)
}
