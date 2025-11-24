package services

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// A more efficient launcher
type MultiLauncher interface {
	ScheduleVQLCollectorArgsOnMultipleClients(
		ctx context.Context,
		config_obj *config_proto.Config,
		request *flows_proto.ArtifactCollectorArgs,
		client_ids []string) error
}

type Flusher interface {
	Flush()
}

func Flush(launcher services.Launcher) {
	l, ok := launcher.Storage().(Flusher)
	if ok {
		l.Flush()
	}
}
