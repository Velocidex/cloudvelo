package launcher

import (
	"context"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
)

type Launcher struct {
	services.Launcher
	config_obj *config_proto.Config
}

func NewLauncherService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.Launcher, error) {

	launcher_service, err := launcher.NewLauncherService(ctx, wg, config_obj)
	if err != nil {
		return nil, err
	}
	launcher_service.(*launcher.Launcher).Storage_ = &FlowStorageManager{}

	return &Launcher{
		Launcher:   launcher_service,
		config_obj: config_obj,
	}, nil
}
