package main

import (
	"fmt"

	crypto_server "www.velocidex.com/golang/cloudvelo/crypto/server"

	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/server"
	"www.velocidex.com/golang/cloudvelo/startup"
)

var (
	communicator = app.Command("frontend", "Run the server frontend")
)

func makeElasticBackend(
	config_obj *config.Config,
	crypto_manager *crypto_server.ServerCryptoManager) (server.CommunicatorBackend, error) {

	return server.NewElasticBackend(config_obj, crypto_manager)
}

func doCommunicator() error {
	config_obj, err := loadConfig(makeDefaultConfigLoader())
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartCommunicatorServices(ctx, config_obj)
	defer sm.Close()
	if err != nil {
		return err
	}

	var backend server.CommunicatorBackend
	crypto_manager, err := crypto_server.NewServerCryptoManager(
		sm.Ctx, config_obj.VeloConf(), sm.Wg)
	if err != nil {
		return err
	}

	backend, err = makeElasticBackend(config_obj, crypto_manager)
	if err != nil {
		return err
	}

	server, err := server.NewCommunicator(
		config_obj, crypto_manager, backend)
	err = server.Start(ctx, config_obj.VeloConf(), sm.Wg)
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
