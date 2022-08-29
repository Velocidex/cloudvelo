package startup

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/foreman"
	"www.velocidex.com/golang/cloudvelo/services/orgs"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func StartForeman(
	ctx context.Context,
	config_obj *config_proto.Config,
	elastic_config_path string) (*services.Service, error) {

	// Come up with a suitable services plan depending on the frontend
	// role.
	if config_obj.Frontend == nil {
		config_obj.Frontend = &config_proto.FrontendConfig{}
	}
	if config_obj.Frontend.ServerServices == nil {
		config_obj.Frontend.ServerServices = &config_proto.ServerServicesConfig{
			ClientInfo:        true,
			RepositoryManager: true,
			Launcher:          true,
		}
	}

	sm := services.NewServiceManager(ctx, config_obj)
	_, err := orgs.NewOrgManager(sm.Ctx, sm.Wg, elastic_config_path, config_obj)
	if err != nil {
		return sm, err
	}

	return sm, sm.Start(foreman.StartForemanService)
}
