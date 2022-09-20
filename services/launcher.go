package services

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

// A more efficient launcher
type MultiLauncher interface {
	ScheduleVQLCollectorArgsOnMultipleClients(
		ctx context.Context,
		config_obj *config_proto.Config,
		request *flows_proto.ArtifactCollectorArgs,
		client_ids []string) error
}
