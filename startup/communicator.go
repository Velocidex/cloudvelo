package startup

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/config"
	ingestor_services "www.velocidex.com/golang/cloudvelo/ingestion/services"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/orgs"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// StartFrontendServices starts the binary as a frontend:
func StartCommunicatorServices(
	ctx context.Context,
	config_obj *config.Config) (*services.Service, error) {

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

	sm := services.NewServiceManager(ctx, config_obj.VeloConf())
	err := cvelo_services.StartElasticSearchService(config_obj)
	if err != nil {
		return sm, err
	}

	_, err = orgs.NewOrgManager(sm.Ctx, sm.Wg, config_obj)
	if err != nil {
		return sm, err
	}

	// Start the ingestion services
	err = sm.Start(ingestor_services.StartHuntStatsUpdater)
	if err != nil {
		return sm, err
	}

	return sm, nil
}
