package main

import (
	"fmt"

	"www.velocidex.com/golang/cloudvelo/startup"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

var (
	// Run the client.
	client_cmd = app.Command("client", "Run the velociraptor client")
)

func doRunClient() error {
	ctx, cancel := install_sig_handler()
	defer cancel()

	// Include the writeback in the client's configuration.
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredClient().
		WithRequiredLogging().
		WithFileLoader(*config_path).
		WithWriteback().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	// Report any errors from this function.
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	defer func() {
		if err != nil {
			logger.Error("<red>doRunClient Error:</> %v", err)
		}
	}()

	sm, err := startup.StartClientServices(ctx, config_obj, on_error)
	sm.Close()
	if err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == client_cmd.FullCommand() {
			FatalIfError(client_cmd, doRunClient)
			return true
		}
		return false
	})
}
