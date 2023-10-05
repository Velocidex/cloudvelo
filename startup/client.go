package startup

import (
	"context"
	"fmt"

	"www.velocidex.com/golang/cloudvelo/services/orgs"
	"www.velocidex.com/golang/cloudvelo/vql/uploads"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/writeback"
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

	writeback_service := writeback.GetWritebackService()
	writeback, err := writeback_service.GetWriteback(config_obj)
	if err != nil {
		return nil, err
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

	exe, err := executor.NewClientExecutor(ctx, writeback.ClientId, config_obj)
	if err != nil {
		return nil, fmt.Errorf("Can not create executor: %w", err)
	}

	comm, err := http_comms.StartHttpCommunicatorService(
		ctx, sm.Wg, config_obj, exe, on_error)
	if err != nil {
		return sm, err
	}

	err = uploads.InstallVeloCloudUploader(
		ctx, config_obj, nil, writeback.ClientId, comm.Manager)
	if err != nil {
		return nil, err
	}

	return sm, nil
}
