package main

import (
	"fmt"

	"www.velocidex.com/golang/cloudvelo/services/users"
	"www.velocidex.com/golang/cloudvelo/startup"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	// Command line interface for VQL commands.
	orgs_command = app.Command("orgs", "Manage orgs")

	orgs_user_add     = orgs_command.Command("user_add", "Add a user to an org")
	orgs_user_add_org = orgs_user_add.Arg("org_id", "Org ID to add user to").
				Required().String()
	orgs_user_add_org_name = orgs_user_add.Arg("org_name", "Org ID to add user to").
				Required().String()
	orgs_user_add_user = orgs_user_add.Arg("username", "Username to add").
				Required().String()
)

func doOrgUserAdd() error {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().
		WithRequiredUser().
		WithRequiredLogging().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, *elastic_config, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	err = sm.Start(users.StartUserManager)
	if err != nil {
		return err
	}

	user_manager := services.GetUserManager()
	record, err := user_manager.GetUser(*orgs_user_add_user)
	if err != nil {
		return err
	}

	record.Orgs = append(record.Orgs, &api_proto.Org{
		Name: *orgs_user_add_org_name,
		Id:   *orgs_user_add_org,
	})

	return user_manager.SetUser(record)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case orgs_user_add.FullCommand():
			FatalIfError(orgs_user_add, doOrgUserAdd)

		default:
			return false
		}
		return true
	})
}
