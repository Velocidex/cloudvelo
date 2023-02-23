package main

import (
	"context"
	"fmt"
	"path"
	"sync"

	config "www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
)

var (
	pool_client_command = app.Command(
		"pool_client", "Run a pool client for load testing.")

	pool_client_number = pool_client_command.Flag(
		"number", "Total number of clients to run.").Int()

	pool_client_writeback_dir = pool_client_command.Flag(
		"writeback_dir", "The directory to store all writebacks.").Default(".").
		ExistingDir()

	pool_client_concurrency = pool_client_command.Flag(
		"concurrency", "How many real queries to run.").Default("10").Int()
)

type counter struct {
	i  int
	mu sync.Mutex
}

func (self *counter) Inc() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.i++

	if self.i%100 == 0 {
		fmt.Printf("Starting %v clients\n", self.i)
	}
}

func doPoolClient() error {
	number_of_clients := *pool_client_number
	if number_of_clients <= 0 {
		number_of_clients = 2
	}

	client_config, err := loadConfig(makeDefaultConfigLoader().
		WithRequiredClient().
		WithVerbose(*verbose_flag))
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	// Start all the services
	sm := services.NewServiceManager(ctx, client_config.VeloConf())
	defer sm.Close()

	server.IncreaseLimits(client_config.VeloConf())

	err = startup.StartPoolClientServices(sm, client_config.VeloConf())
	if err != nil {
		return err
	}

	// Make a copy of all the configs for each client.
	serialized, _ := json.Marshal(client_config)

	c := counter{}

	for i := 0; i < number_of_clients; i++ {
		go func(i int) error {
			client_config := &config_proto.Config{}
			err := json.Unmarshal(serialized, &client_config)
			if err != nil {
				return fmt.Errorf("Copying configs: %w", err)
			}

			filename := fmt.Sprintf("pool_client.yaml.%d", i)
			client_config.Client.WritebackLinux = path.Join(
				*pool_client_writeback_dir, filename)

			// Create an in memory ring buffer because the file ring
			// buffer assumes there is only one communicator!
			client_config.Client.WritebackWindows = client_config.Client.WritebackLinux
			if client_config.Client.LocalBuffer != nil {
				client_config.Client.LocalBuffer.DiskSize = 0
			}
			client_config.Client.Concurrency = uint64(*pool_client_concurrency)
			// Disable client info updates in pool clients
			client_config.Client.ClientInfoUpdateTime = -1

			// Make sure the config is ok.
			err = crypto_utils.VerifyConfig(client_config)
			if err != nil {
				return fmt.Errorf("Invalid config: %w", err)
			}

			writeback, err := config.GetWriteback(client_config.Client)
			if err != nil {
				return err
			}

			exe, err := executor.NewPoolClientExecutor(
				ctx, writeback.ClientId, client_config, i)
			if err != nil {
				return fmt.Errorf("Can not create executor: %w", err)
			}

			_, err = http_comms.StartHttpCommunicatorService(
				sm.Ctx, sm.Wg, client_config, exe,
				func(ctx context.Context, config_obj *config_proto.Config) {})
			if err != nil {
				return err
			}

			c.Inc()
			return nil
		}(i)
	}

	// Block forever.
	<-ctx.Done()
	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case pool_client_command.FullCommand():
			FatalIfError(pool_client_command, doPoolClient)
		default:
			return false
		}
		return true
	})
}
