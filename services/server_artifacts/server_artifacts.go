package server_artifacts

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"

	"www.velocidex.com/golang/velociraptor/services/server_artifacts"
)

type ServerArtifactRunner struct {
	*server_artifacts.ServerArtifactRunner

	ctx          context.Context
	wg           *sync.WaitGroup
	config_obj   *config_proto.Config
	cloud_config *config.ElasticConfiguration
}

func (self *ServerArtifactRunner) CloudConfig() *config.ElasticConfiguration {
	return self.cloud_config
}

// Just leave a message for the main runner that the collection should
// be cancelled.
func (self *ServerArtifactRunner) Cancel(
	ctx context.Context, flow_id, principal string) {
	cvelo_services.SetElasticIndex(
		self.ctx, self.config_obj.OrgId, "collections",
		flow_id+"_cancel", &api.ArtifactCollectorRecord{
			ClientId:  principal,
			SessionId: flow_id,
		})
}

// Run the specified server collection in the background
func (self *ServerArtifactRunner) LaunchServerArtifact(
	config_obj *config_proto.Config,
	session_id string,
	req *crypto_proto.FlowRequest,
	collection_context *flows_proto.ArtifactCollectorContext) error {

	if len(req.VQLClientActions) == 0 {
		return nil
	}

	collection_context_manager, err := server_artifacts.NewCollectionContextManager(
		self.ctx, self.wg, config_obj, req, collection_context)
	if err != nil {
		return err
	}

	sub_ctx, cancel := context.WithCancel(self.ctx)

	// Write the collection to storage periodically.
	collection_context_manager.StartRefresh(self.wg)

	// Check for cancellation messages.
	self.wg.Add(1)
	go func() {
		defer self.wg.Done()

		for {
			select {
			case <-sub_ctx.Done():
				return

			case <-utils.GetTime().After(10 * time.Second):
				// If this record appears, we immediately cancel.
				serialized, err := cvelo_services.GetElasticRecord(sub_ctx,
					self.config_obj.OrgId, "collections", session_id+"_cancel")
				if err == nil {
					record := &api.ArtifactCollectorRecord{}
					err = json.Unmarshal(serialized, record)
					if err == nil {
						// Call our base class to cancel the
						// collection.
						self.ServerArtifactRunner.Cancel(sub_ctx,
							session_id, record.ClientId)
					}
					return
				}
			}
		}
	}()

	self.wg.Add(1)
	go func() {
		defer self.wg.Done()
		defer cancel()
		defer collection_context_manager.Close(self.ctx)

		self.ProcessTask(sub_ctx, config_obj,
			session_id, collection_context_manager, req)
	}()

	return nil
}

func NewServerArtifactService(
	ctx context.Context,
	config_obj *config_proto.Config,
	cloud_config *config.ElasticConfiguration,
	wg *sync.WaitGroup) (services.ServerArtifactRunner, error) {

	return &ServerArtifactRunner{
		ServerArtifactRunner: server_artifacts.NewServerArtifactRunner(
			ctx, config_obj, wg),
		ctx:          ctx,
		wg:           wg,
		config_obj:   config_obj,
		cloud_config: cloud_config,
	}, nil
}
