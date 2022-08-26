package startup

import (
	"context"

	ingestor_services "www.velocidex.com/golang/cloudvelo/ingestion/services"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/orgs"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// StartFrontendServices starts the binary as a frontend:
func StartCommunicatorServices(
	ctx context.Context,
	config_obj *config_proto.Config,
	elastic_config_path string) (*services.Service, error) {

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

	err := cvelo_services.StartElasticSearchService(
		config_obj, elastic_config_path)
	if err != nil {
		return nil, err
	}

	sm := services.NewServiceManager(ctx, config_obj)
	_, err = orgs.NewOrgManager(sm.Ctx, sm.Wg, elastic_config_path, config_obj)
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
