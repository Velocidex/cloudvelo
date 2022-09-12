package main

import (
	"fmt"

	"www.velocidex.com/golang/cloudvelo/startup"
	"www.velocidex.com/golang/velociraptor/gui/velociraptor"
)

var (
	gui = app.Command("gui", "Start the GUI server")
)

func doGUI() error {
	config_obj, err := loadConfig(makeDefaultConfigLoader())
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	// Load the GUI assets
	velociraptor.Init()

	ctx, cancel := install_sig_handler()
	defer cancel()

	// Now start the frontend services
	sm, err := startup.StartGUIServices(ctx, config_obj)
	if err != nil {
		return fmt.Errorf("starting frontend: %w", err)
	}
	defer sm.Close()

	sm.Wg.Wait()

	return nil
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
