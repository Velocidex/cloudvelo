package main

import (
	"fmt"

	"www.velocidex.com/golang/cloudvelo/schema"
	"www.velocidex.com/golang/cloudvelo/services"
)

var (
	elastic_command = app.Command(
		"elastic", "Manipulate the elastic datastore")

	elastic_command_reset = elastic_command.Command(
		"reset", "Drop all the indexes and recreate them")

	elastic_command_reset_org_id = elastic_command.Flag(
		"org_id", "An OrgID to initialize").String()

	elastic_command_recreate = elastic_command.Flag(
		"recreate", "Also recreate the inde").Bool()

	elastic_command_reset_filter = elastic_command_reset.Arg(
		"index_filter", "If specified only re-create these indexes").String()
)

func doResetElastic() error {
	config_obj, err := loadConfig(makeDefaultConfigLoader())
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	err = services.StartElasticSearchService(config_obj)
	if err != nil {
		return err
	}

	// Make sure the database is properly configured
	err = schema.InstallIndexTemplates(ctx, config_obj.VeloConf())
	if err != nil {
		return err
	}

	if *elastic_command_reset_org_id == "" {
		return schema.DeleteAllOrgs(ctx,
			config_obj.VeloConf(), *elastic_command_reset_filter)

	}

	return schema.Delete(ctx,
		config_obj.VeloConf(),
		*elastic_command_reset_org_id,
		*elastic_command_reset_filter)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == elastic_command_reset.FullCommand() {
			FatalIfError(elastic_command_reset, doResetElastic)
			return true
		}
		return false
	})
}
