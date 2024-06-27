package main

import (
	"fmt"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/schema"
	"www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	elastic_command = app.Command(
		"elastic", "Manipulate the elastic datastore")

	elastic_command_reset = elastic_command.Command(
		"reset", "Drop all the indexes and recreate them")

	elastic_command_reset_org_id = elastic_command_reset.Flag(
		"org_id", "An OrgID to initialize").String()

	elastic_command_recreate = elastic_command.Flag(
		"recreate", "Also recreate the inde").Bool()

	elastic_command_reset_filter = elastic_command_reset.Arg(
		"index_filter", "If specified only re-create these indexes").String()

	elastic_command_dump = elastic_command.Command(
		"dump", "Dump the entire index - probably only useful for debugging")

	elastic_command_dump_index = elastic_command_dump.Flag(
		"index", "Name of index to dump").Default("transient").String()

	elastic_command_dump_org_id = elastic_command_dump.Flag(
		"org_id", "An OrgID to initialize").String()

	elastic_command_dump_filter = elastic_command_dump.Flag(
		"filter", "A VQL Lambda to filter rows by e.g. x=> x.type =~ 'results'").
		Default("x=>TRUE").String()
)

func doResetElastic() error {
	config_obj, err := loadConfig(makeDefaultConfigLoader())
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	err = services.StartElasticSearchService(ctx, config_obj)
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

func doDumpElastic() error {
	config_obj, err := loadConfig(makeDefaultConfigLoader())
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	err = services.StartElasticSearchService(ctx, config_obj)
	if err != nil {
		return err
	}

	lambda, err := vfilter.ParseLambda(*elastic_command_dump_filter)
	if err != nil {
		return err
	}
	scope := vql_subsystem.MakeScope()

	rows, err := services.QueryChan(ctx, config_obj.VeloConf(), 1000,
		*elastic_command_dump_org_id,
		*elastic_command_dump_index,
		`{"query": {"match_all" : {}}}`, "_id")
	if err != nil {
		return err
	}

	count := 0
	for raw_message := range rows {
		item := ordereddict.NewDict()
		err := json.Unmarshal(raw_message, &item)
		if err != nil {
			continue
		}

		filter := lambda.Reduce(ctx, scope, []vfilter.Any{item})
		if !scope.Bool(filter) {
			continue
		}

		item.Set("Count", count)
		count++

		fmt.Println(string(json.MustMarshalIndent(item)))
	}

	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == elastic_command_reset.FullCommand() {
			FatalIfError(elastic_command_reset, doResetElastic)
			return true
		}

		if command == elastic_command_dump.FullCommand() {
			FatalIfError(elastic_command_dump, doDumpElastic)
			return true
		}

		return false
	})
}
