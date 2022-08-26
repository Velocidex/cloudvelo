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

type ServerArtifactsService struct{}

func (self ServerArtifactsService) LaunchServerArtifact(
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext,
	tasks []*crypto_proto.VeloMessage) error {

	if len(tasks) == 0 {
		return nil
	}

	runner := server_artifacts.NewServerArtifactRunner(config_obj)
	ctx := context.Background()
	go func() {
		collection_context_manager, err := NewCollectionContextManager(
			ctx, config_obj, collection_context)
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
				ctx, config_obj, task, collection_context_manager, logger)
		}

		collection_context_manager.Close()
		logger.Close()
	}()

	return nil
}

func NewServerArtifactService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.ServerArtifactsService, error) {

	return &ServerArtifactsService{}, nil
}
