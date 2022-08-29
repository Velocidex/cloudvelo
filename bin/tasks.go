package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/cloudvelo/startup"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	tasks           = app.Command("tasks", "Queue Message For Client")
	tasks_client_id = tasks.Flag("client_id", "The client id to queue the message for").
			Required().String()

	tasks_message = tasks.Flag("json", "A json file with the VeloMessage to send").
			Required().File()
)

func doTasks() error {
	config_obj, err := makeDefaultConfigLoader().
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	if *elastic_config == "" {
		return errors.New("Elastic config is required")
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, *elastic_config, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	// Read the tasks
	data, err := ioutil.ReadAll(*tasks_message)
	if err != nil {
		return fmt.Errorf("While parsing %v: %v", *tasks_message, err)
	}

	tasks := []*crypto_proto.VeloMessage{}
	err = json.Unmarshal(data, &tasks)
	if err != nil {
		return fmt.Errorf("While parsing %v: %v", *tasks_message, err)
	}

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	for _, msg := range tasks {
		err := client_info_manager.QueueMessageForClient(
			ctx, *tasks_client_id, msg, false, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == tasks.FullCommand() {
			FatalIfError(tasks, doTasks)
			return true
		}
		return false
	})
}
