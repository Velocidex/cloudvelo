package orgs

import (
	"fmt"

	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/filestore"
	"www.velocidex.com/golang/cloudvelo/result_sets/simple"
	"www.velocidex.com/golang/cloudvelo/result_sets/timed"
	"www.velocidex.com/golang/cloudvelo/schema"
	"www.velocidex.com/golang/cloudvelo/services/repository"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	velo_repository "www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *OrgManager) getContext(org_id string) (*OrgContext, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if org_id == "" {
		org_id = "root"
	}

	org_context, pres := self.orgs[org_id]
	if !pres {
		return nil, fmt.Errorf("Unknown org id: %v", org_id)
	}

	return org_context, nil
}

func (self *OrgManager) makeNewOrgContext(org_id, name, nonce string) (*OrgContext, error) {
	// Create a new service container and cache it for next time
	record := &api_proto.OrgRecord{
		// For backwards compatibility
		OrgId: org_id,
		Id:    org_id,
		Name:  name,
		Nonce: nonce,
	}

	if utils.IsRootOrg(org_id) {
		record.OrgId = "root"
		record.Id = "root"
		record.Name = "<root>"
		record.Nonce = self.config_obj.Client.Nonce
	}

	org_config := self.makeNewConfigObj(record)
	service_manager := &LazyServiceContainer{
		wg:         self.wg,
		ctx:        self.ctx,
		config_obj: org_config,
	}

	org_context := &OrgContext{
		record:     record,
		config_obj: org_config,
		service:    service_manager,
	}

	if self.cloud_config != nil {
		// Set up the indexes for the new org.
		file_store_obj, err := filestore.NewS3Filestore(self.ctx,
			&config.Config{
				Config: *org_config,
				Cloud:  *self.cloud_config,
			})
		if err != nil {
			return nil, err
		}

		// Register a filestore for this org
		file_store.OverrideFilestoreImplementation(org_config, file_store_obj)

		// Register result set factories
		// Register our result set implementations
		result_sets.RegisterResultSetFactory(simple.ResultSetFactory{})
		result_sets.RegisterTimedResultSetFactory(timed.TimedFactory{})

		// TODO: This needs to be very quick.
		err = schema.Initialize(self.ctx,
			self.config_obj,
			org_id, schema.NO_FILTER, schema.DO_NOT_RESET_INDEX)
		if err != nil {
			return nil, err
		}
	}

	// Create a repository manager for the org.
	repo_manager, err := repository.NewRepositoryManager(
		self.ctx, self.wg, org_config)
	if err != nil {
		return nil, err
	}
	service_manager.repository = repo_manager

	err = repository.LoadArtifactsFromConfig(repo_manager, org_config)
	if err != nil {
		return nil, err
	}

	// The Root org will contain all the built in artifacts
	if utils.IsRootOrg(org_id) {
		self.root_repo = repo_manager

		// Assume the built in artifacts are OK so we dont need to
		// validate them at runtime.
		err = repo_manager.LoadBuiltInArtifacts(
			self.ctx, org_config, false /* validate */)
		if err != nil {
			return nil, err
		}

		err = repository.LoadArtifactsFromConfig(repo_manager, org_config)
		if err != nil {
			return nil, err
		}

		err = repository.LoadOverridenArtifacts(org_config, repo_manager)
		if err != nil {
			return nil, err
		}

	} else {
		// Set the parent of this repo as the root org's repository.
		root_repo, err := self.root_repo.GetGlobalRepository(self.config_obj)
		if err != nil {
			return nil, err
		}

		repo_manager.SetParent(self.config_obj, root_repo)
	}
	return org_context, nil
}

func (self *OrgManager) makeClientOrgContext(org_id, name, nonce string) (*OrgContext, error) {
	// Create a new service container and cache it for next time
	record := &api_proto.OrgRecord{
		OrgId: org_id,
		Id:    org_id,
		Name:  name,
		Nonce: nonce,
	}

	if utils.IsRootOrg(org_id) {
		record.OrgId = "root"
		record.Id = "root"
		record.Name = "<root>"
		record.Nonce = self.config_obj.Client.Nonce
	}

	org_config := self.makeNewConfigObj(record)
	service_manager := &LazyServiceContainer{
		wg:         self.wg,
		ctx:        self.ctx,
		config_obj: org_config,
	}

	org_context := &OrgContext{
		record:     record,
		config_obj: org_config,
		service:    service_manager,
	}

	self.mu.Lock()
	self.orgs["root"] = org_context
	self.mu.Unlock()

	// Create a repository manager for the org.
	repo_manager, err := repository.NewClientRepositoryManager(
		self.ctx, self.wg, org_config)
	if err != nil {
		return nil, err
	}
	service_manager.repository = repo_manager

	// Assume the built in artifacts are OK so we dont need to
	// validate them at runtime.
	err = velo_repository.LoadBuiltInArtifacts(self.ctx, org_config,
		repo_manager.(*velo_repository.RepositoryManager), false /* validate */)
	if err != nil {
		return nil, err
	}

	err = repository.LoadArtifactsFromConfig(repo_manager, org_config)
	if err != nil {
		return nil, err
	}

	err = repository.LoadOverridenArtifacts(org_config, repo_manager)
	if err != nil {
		return nil, err
	}

	return org_context, nil
}

func (self *OrgManager) Services(org_id string) services.ServiceContainer {
	context, err := self.getContext(org_id)
	if err != nil {
		fmt.Printf("Error %v: %v\n", org_id, err.Error())
		panic(err)
	}
	return context.service
}
