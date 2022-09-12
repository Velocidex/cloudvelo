package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/cloudvelo/startup"
	"www.velocidex.com/golang/cloudvelo/vql/uploads"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/uploader"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/cloudvelo/vql_plugins"
)

var (
	// Command line interface for VQL commands.
	query   = app.Command("query", "Run a VQL query")
	queries = query.Arg("queries", "The VQL Query to run.").
		Required().Strings()
	max_wait = app.Flag("max_wait", "Maximum time to queue results.").
			Default("10").Int()
	env_map = query.Flag("env", "Environment for the query.").
		StringMap()
	file_store_uploader = query.Flag("with_fs_uploader",
		"Install a filestore uploader").Bool()
)

func doQuery() error {
	cloud_config_obj, err := loadConfig(
		makeDefaultConfigLoader().WithRequiredLogging())
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	config_obj := cloud_config_obj.VeloConf()

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, cloud_config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	env := ordereddict.NewDict()
	for k, v := range *env_map {
		env.Set(k, v)
	}

	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     log.New(&LogWriter{config_obj}, "", 0),
		Env:        env,
	}

	if *file_store_uploader {
		output_path_spec := path_specs.NewSafeFilestorePath("/")
		builder.Uploader = uploader.NewFileStoreUploader(
			config_obj,
			file_store.GetFileStore(config_obj),
			output_path_spec)

		// Install the uploader globally
		uploads.SetUploaderService(config_obj, "C.1234", nil)
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	out_fd := os.Stdout
	start_time := time.Now()
	defer func() {
		scope.Log("Completed query in %v", time.Now().Sub(start_time))
	}()

	for _, query := range *queries {
		statements, err := vfilter.MultiParse(query)
		kingpin.FatalIfError(err, "Unable to parse VQL Query")

		for _, vql := range statements {
			for result := range vfilter.GetResponseChannel(
				vql, ctx, scope,
				vql_subsystem.MarshalJsonl(scope),
				10, *max_wait) {
				_, err := out_fd.Write(result.Payload)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case query.FullCommand():
			FatalIfError(query, doQuery)

		default:
			return false
		}
		return true
	})
}
