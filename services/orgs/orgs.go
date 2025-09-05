package orgs

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/cloudvelo/config"
	cvelo_datastore "www.velocidex.com/golang/cloudvelo/datastore"
	"www.velocidex.com/golang/cloudvelo/filestore"
	"www.velocidex.com/golang/cloudvelo/result_sets/simple"
	"www.velocidex.com/golang/cloudvelo/schema"
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

type OrgRecord struct {
	*api_proto.OrgRecord
	DocType string `json:"doc_type"`
}

type OrgContext struct {
	record     *OrgRecord
	config_obj *config_proto.Config
	service    services.ServiceContainer
}

type OrgManager struct {
	mu sync.Mutex

	ctx context.Context
	wg  *sync.WaitGroup

	// The base global config object
	config_obj   *config_proto.Config
	cloud_config *config.ElasticConfiguration

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
		return nil, utils.NotFoundError
	}
	return result.record.OrgRecord, nil
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
		return "", utils.NotFoundError
	}
	return result, nil
}

func (self *OrgManager) CreateNewOrg(name, id, nonce string) (
	*api_proto.OrgRecord, error) {

	if id == "" {
		id = NewOrgId()
	} else {
		// Org already exists, just provide the old one
		record, err := self.GetOrg(id)
		if err == nil {
			return record, nil
		}
	}

	if nonce == services.RandomNonce {
		nonce = NewNonce()
	}

	org_context, err := self.makeNewOrgContext(id, name, nonce)
	if err != nil {
		return nil, err
	}

	self.mu.Lock()
	self.orgs[org_context.record.Id] = org_context
	self.org_id_by_nonce[org_context.record.Nonce] = org_context.record.Id
	self.mu.Unlock()

	// Write the org into the index.
	return org_context.record.OrgRecord,
		cvelo_services.SetElasticIndex(self.ctx,
			services.ROOT_ORG_ID,
			"persisted", org_context.record.Id,
			org_context.record)
}

func (self *OrgManager) makeNewConfigObj(
	record *api_proto.OrgRecord) *config_proto.Config {

	result := proto.Clone(self.config_obj).(*config_proto.Config)
	result.OrgId = record.Id
	result.OrgName = record.Name

	if result.Client != nil {
		// Client config does not leak org id! We use the nonce to tie
		// org id back to the org.
		result.Client.Nonce = record.Nonce
	}

	return result
}

func (self *OrgManager) Scan() error {
	hits, _, err := cvelo_services.QueryElasticRaw(
		self.ctx, services.ROOT_ORG_ID,
		"persisted", `
{
 "query": {
   "bool":{
     "must":[{
       "match":{
         "doc_type":"orgs"
       }
     }]
   }},
   "size": 10000
 }`)
	if err != nil {
		return err
	}

	for _, hit := range hits {
		record := &OrgRecord{}
		err = json.Unmarshal(hit, record)
		if err == nil {
			// Read existing records for backwards compatibility
			if record.OrgId != "" && record.Id == "" {
				record.Id = record.OrgId
			}

			self.mu.Lock()
			if utils.IsRootOrg(record.Id) {
				record.Id = "root"
				record.Name = "<root>"
				record.Nonce = self.config_obj.Client.Nonce
			}

			_, pres := self.orgs[record.Id]
			self.mu.Unlock()

			if !pres {
				org_context, err := self.makeNewOrgContext(
					record.Id, record.Name, record.Nonce)
				if err != nil {
					continue
				}

				self.mu.Lock()
				self.orgs[record.Id] = org_context
				self.org_id_by_nonce[record.Nonce] = record.Id
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

	org_context, err := self.makeClientOrgContext(
		"root", "<root>", config_obj.Client.Nonce)
	if err != nil {
		return err
	}

	self.mu.Lock()
	self.orgs[org_context.record.Id] = org_context
	self.org_id_by_nonce[org_context.record.Nonce] = org_context.record.Id
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
	config_obj *config.Config,
	wg *sync.WaitGroup) error {
	logger := logging.GetLogger(&config_obj.Config, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Org Manager service.")

	if config_obj.Client == nil {
		return errors.New("Config object is missing a Client section")
	}

	err := users.StartUserManager(ctx, wg, config_obj)
	if err != nil {
		return err
	}

	err = cvelo_services.StartElasticSearchService(ctx, config_obj)
	if err != nil {
		return err
	}

	err = cvelo_services.StartBulkIndexService(self.ctx, self.wg,
		cvelo_services.PrimaryOpenSearch, config_obj)
	if err != nil {
		return err
	}

	if config_obj.Cloud.SecondaryAddresses != nil {
		err = cvelo_services.StartBulkIndexService(self.ctx, self.wg,
			cvelo_services.SecondaryOpenSearch, config_obj)
		if err != nil {
			return err
		}
	}

	// Ensure database is properly initialized
	// Make sure the database is properly configured
	err = schema.InstallIndexTemplates(ctx, config_obj)
	if err != nil {
		return err
	}

	// Install the ElasticDatastore
	datastore.OverrideDatastoreImplementation(
		cvelo_datastore.NewElasticDatastore(ctx, config_obj))

	file_store_obj, err := filestore.NewS3Filestore(ctx, config_obj)
	if err != nil {
		return err
	}

	// Filestore implementation uses s3 as a backend.
	file_store.OverrideFilestoreImplementation(
		config_obj.VeloConf(), file_store_obj)

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
	self.orgs[org_context.record.Id] = org_context
	self.org_id_by_nonce[org_context.record.Nonce] = org_context.record.Id
	self.mu.Unlock()

	// Get the root org's repository manager
	manager, err := services.GetRepositoryManager(config_obj.VeloConf())
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
	config_obj *config.Config) (services.OrgManager, error) {

	service := &OrgManager{
		config_obj:      &config_obj.Config,
		cloud_config:    &config_obj.Cloud,
		ctx:             ctx,
		wg:              wg,
		orgs:            make(map[string]*OrgContext),
		org_id_by_nonce: make(map[string]string),
	}

	_, err := services.GetOrgManager()
	if err != nil {
		services.RegisterOrgManager(service)
	}

	// Cleanup when the context is removed.
	wg.Add(1)
	go func() {
		defer wg.Done()

		<-ctx.Done()
		services.RegisterOrgManager(nil)
	}()

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

	// Cleanup when the context is removed.
	wg.Add(1)
	go func() {
		defer wg.Done()

		<-ctx.Done()
		services.RegisterOrgManager(nil)
	}()

	return service, service.StartClientOrgManager(ctx, config_obj, wg)
}
