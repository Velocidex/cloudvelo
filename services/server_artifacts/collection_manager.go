package server_artifacts

import (
	"context"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services/server_artifacts"
)

type contextManager struct {
	mu           sync.Mutex
	config_obj   *config_proto.Config
	path_manager *paths.FlowPathManager
	session_id   string
	context      *flows_proto.ArtifactCollectorContext
	cancel       func()

	ctx context.Context
}

func NewCollectionContextManager(
	ctx context.Context,
	config_obj *config_proto.Config,
	session_id string) (
	server_artifacts.CollectionContextManager, error) {

	sub_ctx, cancel := context.WithCancel(ctx)
	self := &contextManager{
		config_obj: config_obj,
		cancel:     cancel,
		context: &flows_proto.ArtifactCollectorContext{
			ClientId:  "server",
			SessionId: session_id,
		},
		ctx: sub_ctx,
	}

	// Write collection context periodically to disk so the
	// GUI can track progress.
	self.StartRefresh(sub_ctx)

	return self, self.Save()
}

func (self *contextManager) Close() {
	err := self.Save()
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("ServerArtifactsRunner: %v", err)
	}

	self.cancel()
}

func (self *contextManager) GetContext() *flows_proto.ArtifactCollectorContext {
	self.mu.Lock()
	defer self.mu.Unlock()

	return proto.Clone(self.context).(*flows_proto.ArtifactCollectorContext)
}

// Starts a go routine which saves the context state so the GUI can monitor progress.
func (self *contextManager) StartRefresh(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				self.Save()
				return

			case <-time.After(time.Duration(10) * time.Second):
				// Context is finalized no more modifications are allowed.
				if self.context.State != 1 {
					return
				}
				err := self.Save()
				if err != nil {
					logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
					logger.Error("ServerArtifactsRunner: %v", err)
				}
			}
		}
	}()
}

// Allow modification of the context under lock.
func (self *contextManager) Modify(cb func(context *flows_proto.ArtifactCollectorContext)) {
	self.mu.Lock()
	defer self.mu.Unlock()

	cb(self.context)
}

func (self *contextManager) Load(context *flows_proto.ArtifactCollectorContext) error {
	return nil
}

// Flush the context to disk.
func (self *contextManager) Save() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Store the collection_context first, then queue all the tasks.
	return cvelo_services.SetElasticIndex(self.ctx,
		self.config_obj.OrgId, "collections",
		self.context.SessionId+"_stats",
		api.ArtifactCollectorRecordFromProto(self.context))
}
