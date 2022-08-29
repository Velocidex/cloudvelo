package main

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"runtime/pprof"
	"runtime/trace"

	"github.com/Velocidex/survey"
	errors "github.com/pkg/errors"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"

	// Import all vql plugins.
	_ "www.velocidex.com/golang/velociraptor/vql_plugins"
)

type CommandHandler func(command string) bool

var (
	app = kingpin.New("velociraptor",
		"An advanced incident response and monitoring agent.")

	config_path = app.Flag("config", "The configuration file.").
			Short('c').String()
	override_flag = app.Flag("config_override", "A json object to override the config.").
			Short('o').String()
	verbose_flag = app.Flag(
		"verbose", "Enabled verbose logging.").Short('v').
		Default("false").Bool()

	profile_flag = app.Flag(
		"profile", "Write profiling information to this file.").String()

	trace_flag = app.Flag(
		"trace", "Write trace information to this file.").String()

	trace_vql_flag = app.Flag("trace_vql", "Enable VQL tracing.").Bool()

	logging_flag = app.Flag(
		"logfile", "Write to this file as well").String()

	tempdir_flag = app.Flag(
		"tempdir", "Write all temp files to this directory").String()

	command_handlers []CommandHandler
)

// Try to unlock encrypted API keys
func maybe_unlock_api_config(config_obj *config_proto.Config) error {
	if config_obj.ApiConfig == nil || config_obj.ApiConfig.ClientPrivateKey == "" {
		return nil
	}

	// If the key is locked ask for a password.
	private_key := []byte(config_obj.ApiConfig.ClientPrivateKey)
	block, _ := pem.Decode(private_key)
	if block == nil {
		return errors.New("Unable to decode private key.")
	}

	if x509.IsEncryptedPEMBlock(block) {
		password := ""
		err := survey.AskOne(
			&survey.Password{Message: "Password:"},
			&password,
			survey.WithValidator(survey.Required))
		if err != nil {
			return err
		}

		decrypted_block, err := x509.DecryptPEMBlock(
			block, []byte(password))
		if err != nil {
			return err
		}
		config_obj.ApiConfig.ClientPrivateKey = string(
			pem.EncodeToMemory(&pem.Block{
				Bytes: decrypted_block,
				Type:  block.Type,
			}))
	}
	return nil
}

var (
	default_config *config_proto.Config
)

func main() {
	app.HelpFlag.Short('h')
	app.UsageTemplate(kingpin.CompactUsageTemplate)
	args := os.Args[1:]

	// If no args are given check if there is an embedded config
	// with autoexec.
	if len(args) == 0 {
		config_obj, err := new(config.Loader).WithVerbose(*verbose_flag).
			WithEmbedded().LoadAndValidate()
		if err == nil && config_obj.Autoexec != nil && config_obj.Autoexec.Argv != nil {
			for _, arg := range config_obj.Autoexec.Argv {
				args = append(args, os.ExpandEnv(arg))
			}
			logging.Prelog("Autoexec with parameters: %v", args)
		} else {
			args = []string{"--help"}
		}
	}

	command := kingpin.MustParse(app.Parse(args))

	if *trace_flag != "" {
		f, err := os.Create(*trace_flag)
		kingpin.FatalIfError(err, "trace file.")
		err = trace.Start(f)
		kingpin.FatalIfError(err, "trace file.")
		defer trace.Stop()
	}

	if *profile_flag != "" {
		f2, err := os.Create(*profile_flag)
		kingpin.FatalIfError(err, "Profile file.")

		err = pprof.StartCPUProfile(f2)
		kingpin.FatalIfError(err, "Profile file.")
		defer pprof.StopCPUProfile()

	}

	for _, command_handler := range command_handlers {
		if command_handler(command) {
			break
		}
	}
}

func makeDefaultConfigLoader() *config.Loader {
	return new(config.Loader).
		WithVerbose(*verbose_flag).
		WithTempdir(*tempdir_flag).
		WithFileLoader(*config_path).
		WithEmbedded().
		WithEnvLoader("VELOCIRAPTOR_CONFIG").
		WithCustomValidator(initDebugServer).
		WithLogFile(*logging_flag).
		WithOverride(*override_flag)
}
