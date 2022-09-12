package startup

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/config"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/orgs"
	"www.velocidex.com/golang/cloudvelo/services/sanity"
	"www.velocidex.com/golang/cloudvelo/services/server_artifacts"
	"www.velocidex.com/golang/velociraptor/api"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// StartFrontendServices starts the binary as a frontend:
func StartGUIServices(
	ctx context.Context,
	config_obj *config.Config) (*services.Service, error) {

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

	sm := services.NewServiceManager(ctx, config_obj.VeloConf())

	_, err := orgs.NewOrgManager(sm.Ctx, sm.Wg, config_obj)
	if err != nil {
		return sm, err
	}

	cvelo_services.RegisterServerArtifactsService(
		server_artifacts.NewServerArtifactService(sm.Ctx, sm.Wg))

	// Start the listening server
	server_builder, err := api.NewServerBuilder(
		sm.Ctx, config_obj.VeloConf(), sm.Wg)
	if err != nil {
		return sm, err
	}

	// Start the gRPC API server on the master only.
	err = server_builder.WithAPIServer(sm.Ctx, sm.Wg)
	if err != nil {
		return sm, err
	}

	// Start the sanity service to initialize if needed.
	err = sanity.NewSanityCheckService(sm.Ctx, sm.Wg, config_obj)
	if err != nil {
		return sm, err
	}

	return sm, server_builder.StartServer(sm.Ctx, sm.Wg)
}
