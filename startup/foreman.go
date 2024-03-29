package startup

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/foreman"
	"www.velocidex.com/golang/cloudvelo/services/orgs"
	"www.velocidex.com/golang/velociraptor/api"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func StartForeman(
	ctx context.Context,
	config_obj *config.Config) (*services.Service, error) {

	// Come up with a suitable services plan depending on the frontend
	// role.
	if config_obj.Frontend == nil {
		config_obj.Frontend = &config_proto.FrontendConfig{}
	}
	if config_obj.Services == nil {
		config_obj.Services = &config_proto.ServerServicesConfig{
			ClientInfo:        true,
			RepositoryManager: true,
			Launcher:          true,
		}
	}

	sm := services.NewServiceManager(ctx, config_obj.VeloConf())
	_, err := orgs.NewOrgManager(sm.Ctx, sm.Wg, config_obj)
	if err != nil {
		return sm, err
	}

	err = api.StartMonitoringService(sm.Ctx, sm.Wg, config_obj.VeloConf())
	if err != nil {
		return sm, err
	}

	err = foreman.StartForemanService(sm.Ctx, sm.Wg, config_obj)
	return sm, err
}
