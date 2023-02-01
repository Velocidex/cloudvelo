package startup

import (
	"context"
	"fmt"
	"sync"
	"time"

	"www.velocidex.com/golang/cloudvelo/services/orgs"
	"www.velocidex.com/golang/cloudvelo/vql/uploads"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/executor"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

// StartClientServices starts the various services needed by the
// client.
func StartClientServices(
	ctx context.Context,
	config_obj *config_proto.Config,
	on_error func(ctx context.Context,
		config_obj *config_proto.Config)) (*services.Service, error) {

	// Create a suitable service plan.
	if config_obj.Frontend == nil {
		config_obj.Frontend = &config_proto.FrontendConfig{}
	}

	if config_obj.Services == nil {
		config_obj.Services = services.ClientServicesSpec()
		config_obj.Services.Launcher = true
		config_obj.Services.RepositoryManager = true
	}

	// Make sure the config crypto is ok.
	err := crypto_utils.VerifyConfig(config_obj)
	if err != nil {
		return nil, fmt.Errorf("Invalid config: %w", err)
	}

	executor.SetTempfile(config_obj)

	writeback, err := config.GetWriteback(config_obj.Client)
	if err != nil {
		return nil, err
	}

	exe, err := executor.NewClientExecutor(ctx, writeback.ClientId, config_obj)
	if err != nil {
		return nil, fmt.Errorf("Can not create executor: %w", err)
	}

	// Wait for all services to properly start
	// before we begin the comms.
	sm := services.NewServiceManager(ctx, config_obj)

	// Start the nanny first so we are covered from here on.
	err = sm.Start(executor.StartNannyService)
	if err != nil {
		return sm, err
	}

	_, err = orgs.NewClientOrgManager(sm.Ctx, sm.Wg, sm.Config)
	if err != nil {
		return sm, err
	}

	comm, err := http_comms.StartHttpCommunicatorService(
		ctx, sm.Wg, config_obj, exe, on_error)
	if err != nil {
		return sm, err
	}

	err = uploads.SetUploaderService(
		config_obj, writeback.ClientId, comm.Manager, exe)
	if err != nil {
		return nil, err
	}

	err = executor.StartEventTableService(
		ctx, sm.Wg, config_obj, exe.Outbound)
	if err != nil {
		return sm, err
	}

	return sm, initializeEventTable(sm.Ctx, sm.Wg, config_obj, exe)
}

func initializeEventTable(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	exe executor.Executor) error {

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return err
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	requests, err := launcher.CompileCollectorArgs(ctx, config_obj,
		acl_managers.NullACLManager{},
		repository, services.CompilerOptions{},
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"Client.Info.Updates"},
		})
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		logger := logging.GetLogger(config_obj, &logging.ClientComponent)

		// Feed this directly into the executor on startup.
		for {
			select {
			case <-ctx.Done():
				logger.Info("Exiting Client Info Updating Loop")
				return

			case <-time.After(30 * time.Second):
			}

			logger.Info("Sending an Generic.Client.Updates message")
			for _, req := range requests {
				exe.ProcessRequest(ctx, &crypto_proto.VeloMessage{
					SessionId:       "F.Monitoring",
					AuthState:       crypto_proto.VeloMessage_AUTHENTICATED,
					VQLClientAction: req,
				})
			}
		}
	}()

	return nil
}
