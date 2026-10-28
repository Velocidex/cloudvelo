package ingestion

import (
	"testing"

	"github.com/stretchr/testify/require"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

func TestCalcFlowOutcome(t *testing.T) {
	tests := []struct {
		name      string
		statuses  []crypto_proto.VeloStatus_ReturnedStatus
		failed    bool
		completed bool
	}{
		{"OK", []crypto_proto.VeloStatus_ReturnedStatus{
			crypto_proto.VeloStatus_OK}, false, true},
		{"PROGRESS", []crypto_proto.VeloStatus_ReturnedStatus{
			crypto_proto.VeloStatus_PROGRESS}, false, false},
		{"GENERIC_ERROR", []crypto_proto.VeloStatus_ReturnedStatus{
			crypto_proto.VeloStatus_GENERIC_ERROR}, true, true},
		{"OK+GENERIC_ERROR", []crypto_proto.VeloStatus_ReturnedStatus{
			crypto_proto.VeloStatus_OK,
			crypto_proto.VeloStatus_GENERIC_ERROR}, true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			collection_context := &flows_proto.ArtifactCollectorContext{}
			for _, s := range tc.statuses {
				collection_context.QueryStats = append(
					collection_context.QueryStats,
					&crypto_proto.VeloStatus{Status: s})
			}

			failed, completed := calcFlowOutcome(collection_context)
			require.Equal(t, tc.failed, failed)
			require.Equal(t, tc.completed, completed)
		})
	}
}
