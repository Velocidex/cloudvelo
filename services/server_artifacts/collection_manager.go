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
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/server_artifacts"
)

type contextManager struct {
	context      *flows_proto.ArtifactCollectorContext
	mu           sync.Mutex
	config_obj   *config_proto.Config
	path_manager *paths.FlowPathManager
	cancel       func()

	ctx context.Context
}

func NewCollectionContextManager(
	ctx context.Context,
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext) (
	server_artifacts.CollectionContextManager, error) {

	sub_ctx, cancel := context.WithCancel(ctx)
	self := &contextManager{
		config_obj: config_obj,
		cancel:     cancel,
		context:    collection_context,
		ctx:        sub_ctx,
	}

	// Write collection context periodically to disk so the
	// GUI can track progress.
	self.StartRefresh(sub_ctx)

	return self, self.Save()
}

func (self *contextManager) Close() {
	self.Save()
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
				self.Save()
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
	self.mu.Lock()
	defer self.mu.Unlock()

	launcher, err := services.GetLauncher(self.config_obj)
	if err != nil {
		return err
	}

	details, err := launcher.GetFlowDetails(
		self.config_obj, context.ClientId, context.SessionId)
	if err != nil {
		return err
	}

	self.context = details.Context
	return nil
}

// Flush the context to disk.
func (self *contextManager) Save() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Ignore collections which are not running.
	launcher, err := services.GetLauncher(self.config_obj)
	if err != nil {
		return err
	}

	details, err := launcher.GetFlowDetails(
		self.config_obj, self.context.ClientId,
		self.context.SessionId)
	if err == nil && details.Context.Request != nil &&
		details.Context.State != flows_proto.ArtifactCollectorContext_RUNNING {
		return nil
	}

	// Store the collection_context first, then queue all the tasks.
	return cvelo_services.SetElasticIndex(self.ctx,
		self.config_obj.OrgId, "collections", self.context.SessionId,
		api.ArtifactCollectorContextFromProto(self.context))
}
