package server_artifacts

import (
	"context"
	"sync"

	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"

	"www.velocidex.com/golang/velociraptor/services/server_artifacts"
)

type ServerArtifactsService struct {
	ctx context.Context
	wg  *sync.WaitGroup
}

func (self ServerArtifactsService) LaunchServerArtifact(
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext,
	tasks []*crypto_proto.VeloMessage) error {

	if len(tasks) == 0 {
		return nil
	}

	runner := server_artifacts.NewServerArtifactRunner(config_obj)

	self.wg.Add(1)
	go func() {
		defer self.wg.Done()

		collection_context_manager, err := NewCollectionContextManager(
			self.ctx, config_obj, collection_context)
		if err != nil {
			return
		}

		logger, err := server_artifacts.NewServerLogger(
			collection_context_manager, config_obj, collection_context.SessionId)
		if err != nil {
			return
		}

		for _, task := range tasks {
			runner.ProcessTask(
				self.ctx, config_obj, task, collection_context_manager, logger)
		}

		collection_context_manager.Close()
		logger.Close()
	}()

	return nil
}

func NewServerArtifactService(
	ctx context.Context,
	wg *sync.WaitGroup) services.ServerArtifactsService {

	return &ServerArtifactsService{
		ctx: ctx,
		wg:  wg,
	}
}
