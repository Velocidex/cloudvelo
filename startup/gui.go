package startup

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/config"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/orgs"
	"www.velocidex.com/golang/cloudvelo/services/sanity"
	"www.velocidex.com/golang/cloudvelo/services/server_artifacts"
	"www.velocidex.com/golang/velociraptor/accessors"
	file_store_accessor "www.velocidex.com/golang/velociraptor/accessors/file_store"
	"www.velocidex.com/golang/velociraptor/api"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
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
	if config_obj.Services == nil {
		config_obj.Services = &config_proto.ServerServicesConfig{
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
		server_artifacts.NewServerArtifactService(
			sm.Ctx, config_obj.VeloConf(), sm.Wg))

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

	// Register the "fs" accessor for accessing the filestore in VQL
	fs_factory := file_store_accessor.NewFileStoreFileSystemAccessor(
		config_obj.VeloConf())
	accessors.Register("fs", fs_factory,
		`Provide access to the server's filestore and datastore.

Many VQL plugins produce references to files stored on the server. This accessor can be used to open those files and read them. Typically references to filestore or datastore files have the "fs:" or "ds:" prefix.
`)

	// Apply allow listing to restrict server functionality.  It is
	// possible to extend the default allow list in the config file.
	// By default we enabled a restrictive hard coded allow list that
	// provides no access to the running container.
	if config_obj.Defaults != nil {
		if len(config_obj.Defaults.AllowedPlugins) > 0 {
			allowed_plugins = append(
				allowed_plugins, config_obj.Defaults.AllowedPlugins...)
		}

		if len(config_obj.Defaults.AllowedAccessors) > 0 {
			allowed_accessors = append(allowed_accessors,
				config_obj.Defaults.AllowedAccessors...)
		}

		if len(config_obj.Defaults.AllowedFunctions) > 0 {
			allowed_functions = append(allowed_functions,
				config_obj.Defaults.AllowedFunctions...)
		}
	}

	logger := logging.GetLogger(
		config_obj.VeloConf(), &logging.FrontendComponent)
	logger.Info("Restricting VQL plugins to set %v and functions to set %v\n",
		allowed_plugins, allowed_functions)
	logger.Info("Restricting VQL accessors to %v\n", allowed_accessors)

	err = vql_subsystem.EnforceVQLAllowList(allowed_plugins, allowed_functions)
	if err != nil {
		return sm, err
	}
	err = accessors.EnforceAccessorAllowList(allowed_accessors)
	if err != nil {
		return sm, err
	}

	return sm, server_builder.StartServer(sm.Ctx, sm.Wg)
}
