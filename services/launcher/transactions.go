package launcher

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var resumeFlowNotSupportedError = status.Error(codes.Unimplemented,
	"ResumeFlow not supported: upload transactions are not recorded")

// ResumeFlow is refused because cloudvelo does not record upload
// transactions, so there is nothing to replay. The upstream
// implementation would still reset the flow to RUNNING, clear query
// errors and queue an empty ResumeTransactions request to the client.
func (self *Launcher) ResumeFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id string) ([]*actions_proto.UploadTransaction, error) {
	return nil, resumeFlowNotSupportedError
}
