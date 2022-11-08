package orgs

import (
	"context"
	"errors"
	"sync"

	"www.velocidex.com/golang/cloudvelo/services/acl_manager"
	"www.velocidex.com/golang/cloudvelo/services/client_info"
	"www.velocidex.com/golang/cloudvelo/services/client_monitoring"
	"www.velocidex.com/golang/cloudvelo/services/hunt_dispatcher"
	"www.velocidex.com/golang/cloudvelo/services/indexing"
	"www.velocidex.com/golang/cloudvelo/services/inventory"
	"www.velocidex.com/golang/cloudvelo/services/labeler"
	"www.velocidex.com/golang/cloudvelo/services/launcher"
	"www.velocidex.com/golang/cloudvelo/services/notebook"
	"www.velocidex.com/golang/cloudvelo/services/notifier"
	"www.velocidex.com/golang/cloudvelo/services/vfs_service"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/broadcast"
	"www.velocidex.com/golang/velociraptor/services/journal"
)

// A Service container that creates different org services on demand.
type LazyServiceContainer struct {
	ctx        context.Context
	wg         *sync.WaitGroup
	config_obj *config_proto.Config

	// Elastic is too slow to serve the repository manager directly so
	// we cache it here.
	repository services.RepositoryManager
}

func (self *LazyServiceContainer) FrontendManager() (services.FrontendManager, error) {
	return nil, errors.New("LazyServiceContainer.FrontendManager is Not implemented")
}

func (self *LazyServiceContainer) Notifier() (services.Notifier, error) {
	return notifier.NewNotificationService(self.ctx, self.wg, self.config_obj)
}

func (self *LazyServiceContainer) ServerEventManager() (services.ServerEventManager, error) {
	return nil, errors.New("LazyServiceContainer.ServerEventManager is Not implemented")
}

func (self *LazyServiceContainer) ClientEventManager() (services.ClientEventTable, error) {
	return client_monitoring.NewClientMonitoringService(self.ctx, self.wg, self.config_obj)
}

func (self *LazyServiceContainer) NotebookManager() (services.NotebookManager, error) {
	return notebook.NewNotebookManagerService(self.ctx, self.wg, self.config_obj), nil
}

func (self *LazyServiceContainer) Launcher() (services.Launcher, error) {
	return launcher.NewLauncherService(self.ctx, self.wg, self.config_obj)
}

func (self *LazyServiceContainer) HuntDispatcher() (services.IHuntDispatcher, error) {
	return hunt_dispatcher.NewHuntDispatcher(self.ctx, self.wg, self.config_obj)
}

func (self *LazyServiceContainer) Indexer() (services.Indexer, error) {
	return indexing.NewIndexingService(self.ctx, self.wg, self.config_obj)
}

func (self *LazyServiceContainer) RepositoryManager() (services.RepositoryManager, error) {
	if self.repository == nil {
		return nil, errors.New("Repository Manager not initialized!")
	}
	return self.repository, nil
}

func (self *LazyServiceContainer) VFSService() (services.VFSService, error) {
	return vfs_service.NewVFSService(self.ctx, self.wg, self.config_obj)
}

func (self *LazyServiceContainer) Labeler() (services.Labeler, error) {
	return labeler.NewLabelerService(self.ctx, self.wg, self.config_obj)
}

func (self *LazyServiceContainer) Journal() (services.JournalService, error) {
	return journal.NewJournalService(self.ctx, self.wg, self.config_obj)
}

func (self *LazyServiceContainer) ClientInfoManager() (services.ClientInfoManager, error) {
	return client_info.NewClientInfoManager(self.config_obj)
}

func (self *LazyServiceContainer) Inventory() (services.Inventory, error) {
	return inventory.NewInventoryDummyService(self.ctx, self.wg, self.config_obj)
}

func (self *LazyServiceContainer) BroadcastService() (services.BroadcastService, error) {
	return broadcast.NewBroadcastService(self.config_obj), nil
}

func (self *LazyServiceContainer) ACLManager() (services.ACLManager, error) {
	return acl_manager.NewACLManager(self.ctx, self.wg, self.config_obj), nil
}
