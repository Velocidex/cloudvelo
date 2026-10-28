package ingestion

import (
	"testing"

	"github.com/stretchr/testify/require"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

func TestIsUnknownFlowReply(t *testing.T) {
	tests := []struct {
		name     string
		msg      *crypto_proto.FlowStats
		expected bool
	}{
		{"UNKNOWN_FLOW with upstream message", &crypto_proto.FlowStats{
			FlowComplete: true,
			QueryStatus: []*crypto_proto.VeloStatus{{
				Status:       crypto_proto.VeloStatus_UNKNOWN_FLOW,
				ErrorMessage: "Flow not known - maybe the client crashed?",
			}},
		}, true},
		{"UNKNOWN_FLOW with other message", &crypto_proto.FlowStats{
			FlowComplete: true,
			QueryStatus: []*crypto_proto.VeloStatus{{
				Status:       crypto_proto.VeloStatus_UNKNOWN_FLOW,
				ErrorMessage: "Something else",
			}},
		}, true},
		{"prefix with GENERIC_ERROR", &crypto_proto.FlowStats{
			FlowComplete: true,
			QueryStatus: []*crypto_proto.VeloStatus{{
				Status:       crypto_proto.VeloStatus_GENERIC_ERROR,
				ErrorMessage: "Flow not known - maybe the client crashed?",
			}},
		}, true},
		{"prefix with OK is not matched", &crypto_proto.FlowStats{
			FlowComplete: true,
			QueryStatus: []*crypto_proto.VeloStatus{{
				Status:       crypto_proto.VeloStatus_OK,
				ErrorMessage: "Flow not known",
			}},
		}, false},
		{"flow not complete", &crypto_proto.FlowStats{
			QueryStatus: []*crypto_proto.VeloStatus{{
				Status:       crypto_proto.VeloStatus_UNKNOWN_FLOW,
				ErrorMessage: "Flow not known - maybe the client crashed?",
			}},
		}, false},
		{"two query statuses", &crypto_proto.FlowStats{
			FlowComplete: true,
			QueryStatus: []*crypto_proto.VeloStatus{{
				Status: crypto_proto.VeloStatus_UNKNOWN_FLOW,
			}, {
				Status: crypto_proto.VeloStatus_OK,
			}},
		}, false},
		{"nil query status", &crypto_proto.FlowStats{
			FlowComplete: true,
		}, false},
		{"empty query status", &crypto_proto.FlowStats{
			FlowComplete: true,
			QueryStatus:  []*crypto_proto.VeloStatus{},
		}, false},
		{"nil message", nil, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, isUnknownFlowReply(tc.msg))
		})
	}
}
