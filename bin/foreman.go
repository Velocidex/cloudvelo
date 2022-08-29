package main

import (
	"fmt"

	"www.velocidex.com/golang/cloudvelo/startup"
)

var (
	foreman = app.Command("foreman", "Run the foreman batch program")
)

func doForeman() error {
	config_obj, err := makeDefaultConfigLoader().
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartForeman(ctx, config_obj, *elastic_config)
	defer sm.Close()
	if err != nil {
		return err
	}

	<-ctx.Done()

	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == foreman.FullCommand() {
			FatalIfError(foreman, doForeman)
			return true
		}
		return false
	})
}
