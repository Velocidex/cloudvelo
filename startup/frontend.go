package startup

import (
	"context"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/orgs"
	"www.velocidex.com/golang/cloudvelo/services/server_artifacts"
	"www.velocidex.com/golang/cloudvelo/services/users"
	"www.velocidex.com/golang/velociraptor/api"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// StartFrontendServices starts the binary as a frontend:
// 1.
func StartFrontendServices(
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
			GuiServer:           true,
			ClientInfo:          true,
			JournalService:      true,
			NotificationService: true,
			NotebookService:     true,
			RepositoryManager:   true,
			HuntDispatcher:      true,
			IndexServer:         true,
			VfsService:          true,
			Label:               true,
			Launcher:            true,
			ServerArtifacts:     true,
			ClientMonitoring:    true,
			MonitoringService:   true,
		}
	}

	sm := services.NewServiceManager(ctx, config_obj)
	_, err := orgs.NewOrgManager(sm.Ctx, sm.Wg, elastic_config_path, config_obj)
	if err != nil {
		return sm, err
	}

	err = sm.Start(users.StartUserManager)
	if err != nil {
		return sm, err
	}

	cvelo_services.RegisterServerArtifactsService(
		server_artifacts.NewServerArtifactService(sm.Ctx, sm.Wg, config_obj))

	// Start the listening server
	server_builder, err := api.NewServerBuilder(sm.Ctx, config_obj, sm.Wg)
	if err != nil {
		return sm, err
	}

	// Start the gRPC API server on the master only.
	err = server_builder.WithAPIServer(sm.Ctx, sm.Wg)
	if err != nil {
		return sm, err
	}

	return sm, server_builder.StartServer(sm.Ctx, sm.Wg)
}
