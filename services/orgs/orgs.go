package orgs

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	cvelo_datastore "www.velocidex.com/golang/cloudvelo/datastore"
	"www.velocidex.com/golang/cloudvelo/filestore"
	"www.velocidex.com/golang/cloudvelo/result_sets/simple"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/users"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type OrgContext struct {
	record     *api_proto.OrgRecord
	config_obj *config_proto.Config
	service    services.ServiceContainer
}

type OrgManager struct {
	mu sync.Mutex

	ctx context.Context
	wg  *sync.WaitGroup

	// The base global config object
	config_obj          *config_proto.Config
	elastic_config_path string

	// Each org has a separate config object.
	orgs            map[string]*OrgContext
	org_id_by_nonce map[string]string

	root_repo services.RepositoryManager
}

func (self *OrgManager) ListOrgs() []*api_proto.OrgRecord {
	result := []*api_proto.OrgRecord{}
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, item := range self.orgs {
		result = append(result, proto.Clone(item.record).(*api_proto.OrgRecord))
	}

	return result
}

func (self *OrgManager) GetOrgConfig(org_id string) (*config_proto.Config, error) {
	context, err := self.getContext(org_id)
	if err != nil {
		return nil, err
	}
	return context.config_obj, err
}

func (self *OrgManager) GetOrg(org_id string) (*api_proto.OrgRecord, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if utils.IsRootOrg(org_id) {
		org_id = services.ROOT_ORG_ID
	}

	result, pres := self.orgs[org_id]
	if !pres {
		return nil, services.NotFoundError
	}
	return result.record, nil
}

func (self *OrgManager) OrgIdByNonce(nonce string) (string, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Nonce corresponds to the root config
	if self.config_obj.Client != nil &&
		self.config_obj.Client.Nonce == nonce {
		return "", nil
	}

	result, pres := self.org_id_by_nonce[nonce]
	if !pres {
		return "", services.NotFoundError
	}
	return result, nil
}

func (self *OrgManager) CreateNewOrg(name, id string) (
	*api_proto.OrgRecord, error) {

	if id == "" {
		id = NewOrgId()
	}

	org_context, err := self.makeNewOrgContext(
		id, name, NewNonce())
	if err != nil {
		return nil, err
	}

	self.mu.Lock()
	self.orgs[org_context.record.OrgId] = org_context
	self.org_id_by_nonce[org_context.record.Nonce] = org_context.record.OrgId
	self.mu.Unlock()

	org_record := org_context.record

	// Write the org into the index.
	return org_record, cvelo_services.SetElasticIndex(self.ctx,
		services.ROOT_ORG_ID,
		"orgs", org_record.OrgId, org_record)
}

func (self *OrgManager) makeNewConfigObj(
	record *api_proto.OrgRecord) *config_proto.Config {

	result := proto.Clone(self.config_obj).(*config_proto.Config)

	result.OrgId = record.OrgId
	result.OrgName = record.Name

	if result.Client != nil {
		// Client config does not leak org id! We use the nonce to tie
		// org id back to the org.
		result.Client.Nonce = record.Nonce
	}

	return result
}

func (self *OrgManager) Scan() error {
	hits, err := cvelo_services.QueryElasticRaw(
		self.ctx, services.ROOT_ORG_ID,
		"orgs", `{"query": {"match_all" : {}}}`)
	if err != nil {
		return err
	}

	for _, hit := range hits {
		record := &api_proto.OrgRecord{}
		err = json.Unmarshal(hit, record)
		if err == nil {
			self.mu.Lock()
			if utils.IsRootOrg(record.OrgId) {
				record.OrgId = "root"
				record.Name = "<root>"
				record.Nonce = self.config_obj.Client.Nonce
			}

			_, pres := self.orgs[record.OrgId]
			self.mu.Unlock()

			if !pres {
				org_context, err := self.makeNewOrgContext(
					record.OrgId, record.Name, record.Nonce)
				if err != nil {
					continue
				}

				self.mu.Lock()
				self.orgs[record.OrgId] = org_context
				self.org_id_by_nonce[record.Nonce] = record.OrgId
				self.mu.Unlock()
			}
		}
	}

	return nil
}

func (self *OrgManager) StartClientOrgManager(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Client Org Manager service.")

	org_context, err := self.makeNewOrgContext(
		"root", "<root>", config_obj.Client.Nonce)
	if err != nil {
		return err
	}

	self.mu.Lock()
	self.orgs[org_context.record.OrgId] = org_context
	self.org_id_by_nonce[org_context.record.Nonce] = org_context.record.OrgId
	self.mu.Unlock()

	// Get the root org's repository manager
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}
	self.root_repo = manager

	return nil
}

func (self *OrgManager) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Org Manager service.")

	if config_obj.Client == nil {
		return errors.New("Config object is missing a Client section")
	}

	err := users.StartUserManager(ctx, wg, config_obj)
	if err != nil {
		return err
	}

	err = cvelo_services.StartElasticSearchService(
		config_obj, self.elastic_config_path)
	if err != nil {
		return err
	}

	err = cvelo_services.StartBulkIndexService(self.ctx, self.wg, config_obj)
	if err != nil {
		return err
	}

	// Install the ElasticDatastore
	datastore.OverrideDatastoreImplementation(
		cvelo_datastore.NewElasticDatastore(ctx, config_obj))

	file_store_obj, err := filestore.NewS3Filestore(
		config_obj, self.elastic_config_path)
	if err != nil {
		return err
	}

	// Filestore implementation uses s3 as a backend.
	file_store.OverrideFilestoreImplementation(config_obj, file_store_obj)

	// Register our result set implementations
	result_sets.RegisterResultSetFactory(simple.ResultSetFactory{})

	org_context, err := self.makeNewOrgContext(
		"root", "<root>", config_obj.Client.Nonce)
	if err != nil {
		return err
	}

	err = self.Scan()
	if err != nil {
		return err
	}

	self.mu.Lock()
	self.orgs[org_context.record.OrgId] = org_context
	self.org_id_by_nonce[org_context.record.Nonce] = org_context.record.OrgId
	self.mu.Unlock()

	// Get the root org's repository manager
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}
	self.root_repo = manager

	// Do first scan inline so we have valid data on exit.
	err = self.Scan()
	if err != nil {
		return err
	}

	// Start syncing the mutation_manager
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(10 * time.Second):
				self.Scan()
			}
		}

	}()

	return nil
}

func NewOrgManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	elastic_config_path string,
	config_obj *config_proto.Config) (services.OrgManager, error) {

	service := &OrgManager{
		config_obj:          config_obj,
		ctx:                 ctx,
		wg:                  wg,
		elastic_config_path: elastic_config_path,

		orgs:            make(map[string]*OrgContext),
		org_id_by_nonce: make(map[string]string),
	}

	_, err := services.GetOrgManager()
	if err != nil {
		services.RegisterOrgManager(service)
	}

	return service, service.Start(ctx, config_obj, wg)
}

func NewClientOrgManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.OrgManager, error) {

	service := &OrgManager{
		config_obj: config_obj,
		ctx:        ctx,
		wg:         wg,

		orgs:            make(map[string]*OrgContext),
		org_id_by_nonce: make(map[string]string),
	}

	_, err := services.GetOrgManager()
	if err != nil {
		services.RegisterOrgManager(service)
	}

	return service, service.StartClientOrgManager(ctx, config_obj, wg)
}
