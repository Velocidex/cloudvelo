package main

import (
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

	config_obj, err := loadConfig(makeDefaultConfigLoader().
		WithRequiredClient().
		WithRequiredLogging().
		WithWriteback())
	if err != nil {
		return err
	}

	// Report any errors from this function.
	logger := logging.GetLogger(&config_obj.Config, &logging.ClientComponent)
	defer func() {
		if err != nil {
			logger.Error("<red>doRunClient Error:</> %v", err)
		}
	}()

	sm, err := startup.StartClientServices(ctx, &config_obj.Config, on_error)
	defer sm.Close()

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
