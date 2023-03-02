package server_artifacts

import (
	"context"
	"sync"

	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"

	"www.velocidex.com/golang/velociraptor/services/server_artifacts"
)

type ServerArtifactsRunner struct {
	*server_artifacts.ServerArtifactsRunner

	ctx          context.Context
	wg           *sync.WaitGroup
	config_obj   *config_proto.Config
	cloud_config *config.ElasticConfiguration
}

func (self *ServerArtifactsRunner) CloudConfig() *config.ElasticConfiguration {
	return self.cloud_config
}

// Run the specified server collection in the background
func (self *ServerArtifactsRunner) LaunchServerArtifact(
	config_obj *config_proto.Config,
	session_id string,
	req *crypto_proto.FlowRequest,
	collection_context *flows_proto.ArtifactCollectorContext) error {

	if len(req.VQLClientActions) == 0 {
		return nil
	}

	sub_ctx, cancel := context.WithCancel(self.ctx)
	collection_context_manager, err := server_artifacts.NewCollectionContextManager(
		sub_ctx, self.wg, config_obj, &crypto_proto.VeloMessage{
			Source:      "server",
			SessionId:   session_id,
			FlowRequest: req,
		}, collection_context)
	if err != nil {
		return err
	}

	// Write the collection to storage periodically.
	collection_context_manager.StartRefresh(self.wg)

	self.wg.Add(1)
	go func() {
		defer self.wg.Done()
		defer cancel()
		defer collection_context_manager.Save()

		self.ProcessTask(sub_ctx, config_obj,
			session_id, collection_context_manager, req)
	}()

	return nil
}

func NewServerArtifactService(
	ctx context.Context,
	config_obj *config_proto.Config,
	cloud_config *config.ElasticConfiguration,
	wg *sync.WaitGroup) services.ServerArtifactsService {

	// Start a server_artifacts runner without checking the tasks
	// queues.
	return &ServerArtifactsRunner{
		ServerArtifactsRunner: server_artifacts.NewServerArtifactRunner(
			ctx, config_obj, wg),
		ctx:          ctx,
		wg:           wg,
		cloud_config: cloud_config,
	}
}
