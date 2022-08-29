package main

import (
	"fmt"

	errors "github.com/pkg/errors"
	crypto_server "www.velocidex.com/golang/cloudvelo/crypto/server"
	"www.velocidex.com/golang/cloudvelo/elastic_datastore"
	"www.velocidex.com/golang/cloudvelo/server"
	"www.velocidex.com/golang/cloudvelo/startup"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	communicator   = app.Command("frontend", "Run the server frontend")
	elastic_config = app.Flag("elastic_config", "If specified we build an elastic backend with this config").String()
)

func makeElasticBackend(
	config_obj *config_proto.Config,
	crypto_manager *crypto_server.ServerCryptoManager,
	elastic_config_path string) (server.CommunicatorBackend, error) {

	elastic_config, err := elastic_datastore.LoadConfig(elastic_config_path)
	if err != nil {
		return nil, err
	}

	return server.NewElasticBackend(
		config_obj, crypto_manager, elastic_config)
}

func doCommunicator() error {
	config_obj, err := makeDefaultConfigLoader().
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartCommunicatorServices(
		ctx, config_obj, *elastic_config)
	defer sm.Close()
	if err != nil {
		return err
	}

	var backend server.CommunicatorBackend
	if *elastic_config == "" {
		return errors.New("--elastic_config is required")
	}

	crypto_manager, err := crypto_server.NewServerCryptoManager(
		sm.Ctx, config_obj, sm.Wg)
	if err != nil {
		return err
	}

	backend, err = makeElasticBackend(
		config_obj, crypto_manager, *elastic_config)
	if err != nil {
		return err
	}

	server, err := server.NewCommunicator(
		config_obj, *elastic_config,
		crypto_manager, backend)
	err = server.Start(ctx, config_obj, sm.Wg)
	if err != nil {
		return err
	}

	<-ctx.Done()

	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == communicator.FullCommand() {
			FatalIfError(communicator, doCommunicator)
			return true
		}
		return false
	})
}
