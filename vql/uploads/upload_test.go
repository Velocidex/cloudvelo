package uploads_test

import (
	"context"
	"io/ioutil"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	crypto_server "www.velocidex.com/golang/cloudvelo/crypto/server"
	"www.velocidex.com/golang/cloudvelo/server"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	cvelo_utils "www.velocidex.com/golang/cloudvelo/utils"
	"www.velocidex.com/golang/cloudvelo/vql/uploads"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto/client"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	velo_uploads "www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/cloudvelo/vql_plugins"
	_ "www.velocidex.com/golang/velociraptor/accessors/data"
)

type UploaderTestSuite struct {
	*testsuite.CloudTestSuite

	golden *ordereddict.Dict
}

// Remove the key from the filestore and make sure it is cleared.
func (self *UploaderTestSuite) clearFilestorePath(
	org_config_obj *config_proto.Config, test_file api.FSPathSpec) {
	// Clear the file just in case.
	file_store_factory := file_store.GetFileStore(org_config_obj)
	assert.NotNil(self.T(), file_store_factory)
	file_store_factory.Delete(test_file)

	// Check the file is gone.
	reader, err := file_store_factory.ReadFile(test_file)
	assert.NoError(self.T(), err)

	_, err = ioutil.ReadAll(reader)
	assert.Error(self.T(), err)
}

// Starts the server communicator and backend
func (self *UploaderTestSuite) startServerCommunicator(
	ctx context.Context, wg *sync.WaitGroup,
	org_config_obj *config_proto.Config) {
	crypto_manager, err := crypto_server.NewServerCryptoManager(
		ctx, org_config_obj, wg)
	assert.NoError(self.T(), err)

	backend, err := server.NewElasticBackend(self.ConfigObj, crypto_manager)
	assert.NoError(self.T(), err)

	server, err := server.NewCommunicator(
		self.ConfigObj, crypto_manager, backend)
	err = server.Start(ctx, org_config_obj, wg)
	assert.NoError(self.T(), err)
}

func (self *UploaderTestSuite) startClientCommunicator(
	ctx context.Context, wg *sync.WaitGroup,
	org_config_obj *config_proto.Config) {
	// Create the components for the client
	writeback, err := config.GetWriteback(org_config_obj.Client)
	assert.NoError(self.T(), err)

	exe, err := executor.NewClientExecutor(ctx, writeback.ClientId, org_config_obj)
	assert.NoError(self.T(), err)

	comm, err := http_comms.StartHttpCommunicatorService(
		ctx, wg, org_config_obj, exe,
		func(ctx context.Context, config_obj *config_proto.Config) {})
	assert.NoError(self.T(), err)

	_, err = comm.Manager.(*client.ClientCryptoManager).AddCertificate(
		org_config_obj, []byte(org_config_obj.Frontend.Certificate))
	assert.NoError(self.T(), err)

	err = uploads.SetUploaderService(
		org_config_obj, writeback.ClientId, comm.Manager, exe)
	assert.NoError(self.T(), err)
}

func (self *UploaderTestSuite) TestUploader() {
	cvelo_utils.Clock = &utils.MockClock{
		MockNow: time.Unix(1661391000, 0),
	}

	ctx := self.Sm.Ctx
	wg := self.Sm.Wg

	org_manager, err := services.GetOrgManager()
	assert.NoError(self.T(), err)

	org_config_obj, err := org_manager.GetOrgConfig(self.OrgId)
	assert.NoError(self.T(), err)

	// This is the uploaded key within the test org.
	test_file := path_specs.NewSafeFilestorePath("clients",
		"C.1352adc54e292a23", "collections", "F.1234",
		"uploads", "data",
		// sha256 of "hi.txt"
		"d34252834beade56edfebacfa365a815c0f4a80e836160e34ab22383bd5eb37d").
		SetType(api.PATH_TYPE_FILESTORE_ANY)
	self.clearFilestorePath(org_config_obj, test_file)

	self.startServerCommunicator(ctx, wg, org_config_obj)
	self.startClientCommunicator(ctx, wg, org_config_obj)

	// Build a scope to run a query.
	builder := services.ScopeBuilder{
		Config:       org_config_obj,
		ClientConfig: org_config_obj.Client,
		ACLManager:   acl_managers.NullACLManager{},
		Logger: logging.NewPlainLogger(
			org_config_obj, &logging.FrontendComponent),

		// SessionId is expected to be provided to the client by the
		// VQL request.
		Env: ordereddict.NewDict().
			Set("_SessionId", "F.1234"),
	}

	manager, err := services.GetRepositoryManager(org_config_obj)
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	// Run the query which should upload the file
	res := (&uploads.UploadFunction{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("accessor", "data").
			Set("file", "Hello world").
			Set("name", "hi.txt"))

	upload_res, ok := res.(*velo_uploads.UploadResponse)
	assert.True(self.T(), ok)

	// No errors
	assert.Equal(self.T(), "", upload_res.Error)
	assert.Equal(self.T(), uint64(11), upload_res.Size)
	assert.Equal(self.T(), uint64(11), upload_res.StoredSize)

	utils.Debug(res)

	// Check the file was written on the server.
	file_store_factory := file_store.GetFileStore(org_config_obj)
	assert.NotNil(self.T(), file_store_factory)

	reader, err := file_store_factory.ReadFile(test_file)
	assert.NoError(self.T(), err)

	data, err := ioutil.ReadAll(reader)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), "Hello world", string(data))
}

func TestUploader(t *testing.T) {
	suite.Run(t, &UploaderTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{
			Indexes: []string{"client_keys"},
		},
		golden: ordereddict.NewDict(),
	})
}
