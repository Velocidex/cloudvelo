package server_artifacts

import (
	"context"
	"sync"

	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"

	"www.velocidex.com/golang/velociraptor/services/server_artifacts"
)

type ServerArtifactsRunner struct {
	*server_artifacts.ServerArtifactsRunner

	ctx        context.Context
	wg         *sync.WaitGroup
	config_obj *config_proto.Config
}

// Run the specified server collection in the background
func (self *ServerArtifactsRunner) LaunchServerArtifact(
	config_obj *config_proto.Config,
	session_id string,
	req *crypto_proto.FlowRequest) error {

	if len(req.VQLClientActions) == 0 {
		return nil
	}

	self.wg.Add(1)
	go func() {
		defer self.wg.Done()

		self.ProcessTask(self.ctx, config_obj, session_id, req)
	}()

	return nil
}

func NewServerArtifactService(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) services.ServerArtifactsService {

	// Start a server_artifacts runner without checking the tasks
	// queues.
	return &ServerArtifactsRunner{
		ServerArtifactsRunner: server_artifacts.NewServerArtifactRunner(config_obj),
		config_obj:            config_obj,
		ctx:                   ctx,
		wg:                    wg,
	}
}
