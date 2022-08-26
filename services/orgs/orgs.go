package orgs

import (
	"context"
	"sync"

	"google.golang.org/protobuf/proto"
	cvelo_datastore "www.velocidex.com/golang/cloudvelo/datastore"
	"www.velocidex.com/golang/cloudvelo/filestore"
	"www.velocidex.com/golang/cloudvelo/result_sets/simple"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
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
	return context.config_obj, err
}

func (self *OrgManager) GetOrg(org_id string) (*api_proto.OrgRecord, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if org_id == "root" {
		org_id = ""
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

	org_record := &api_proto.OrgRecord{
		Name:  name,
		OrgId: id,
		Nonce: NewNonce(),
	}

	return org_record, nil
}

func (self *OrgManager) makeNewConfigObj(
	record *api_proto.OrgRecord) *config_proto.Config {

	result := proto.Clone(self.config_obj).(*config_proto.Config)

	// The Root org is untouched.
	if record.OrgId == "" {
		return result
	}

	if result.Client != nil {
		result.OrgId = record.OrgId
		result.OrgName = record.Name

		// Client config does not leak org id! We use the nonce to tie
		// org id back to the org.
		result.Client.Nonce = record.Nonce
	}

	return result
}

func (self *OrgManager) Scan() error {
	return nil
}

func (self *OrgManager) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Org Manager service.")

	err := cvelo_services.StartElasticSearchService(
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
		cvelo_datastore.NewElasticDatastore(config_obj))

	file_store_obj, err := filestore.NewS3Filestore(
		config_obj, self.elastic_config_path)
	if err != nil {
		return err
	}
	file_store.OverrideFilestoreImplementation(config_obj, file_store_obj)

	// Register our result set implementations
	result_sets.RegisterResultSetFactory(simple.ResultSetFactory{})

	// Get the root org's repository manager
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	self.root_repo = manager
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
