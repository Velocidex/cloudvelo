package main

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/cloudvelo/startup"
	"www.velocidex.com/golang/velociraptor/api"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/gui/velociraptor"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	gui = app.Command("gui", "Start the GUI server")
)

func doGUI() error {
	config_obj, err := makeDefaultConfigLoader().
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	velociraptor.Init()

	ctx, cancel := install_sig_handler()
	defer cancel()

	// Now start the frontend services
	sm, err := startup.StartFrontendServices(ctx, config_obj, *elastic_config)
	if err != nil {
		return fmt.Errorf("starting frontend: %w", err)
	}
	defer sm.Close()

	sm.Wg.Wait()

	return nil
}

// Start the frontend service.
func startFrontend(sm *services.Service) (*api.Builder, error) {
	config_obj := sm.Config

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.WithFields(logrus.Fields{
		"version":    config_obj.Version.Version,
		"build_time": config_obj.Version.BuildTime,
		"commit":     config_obj.Version.Commit,
	}).Info("<green>Starting</> Frontend.")

	logger.Info("Disabling artifact compression.")
	config_obj.Frontend.DoNotCompressArtifacts = true

	config_obj.Frontend.ServerServices = &config_proto.ServerServicesConfig{
		ApiServer: true,
		GuiServer: true,
	}

	server_builder, err := api.NewServerBuilder(sm.Ctx, config_obj, sm.Wg)
	if err != nil {
		return nil, err
	}

	// Start the gRPC API server.
	err = server_builder.WithAPIServer(sm.Ctx, sm.Wg)
	if err != nil {
		return nil, err
	}

	return server_builder, server_builder.StartServer(sm.Ctx, sm.Wg)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == gui.FullCommand() {
			FatalIfError(gui, doGUI)
			return true
		}
		return false
	})
}
