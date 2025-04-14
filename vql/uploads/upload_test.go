package uploads_test

import (
	"context"
	"io/ioutil"
	"strings"
	"sync"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/stretchr/testify/suite"
	crypto_server "www.velocidex.com/golang/cloudvelo/crypto/server"
	"www.velocidex.com/golang/cloudvelo/filestore"
	"www.velocidex.com/golang/cloudvelo/server"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	"www.velocidex.com/golang/cloudvelo/vql/uploads"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto/client"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	velo_uploads "www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

	_ "www.velocidex.com/golang/cloudvelo/vql_plugins"
	_ "www.velocidex.com/golang/velociraptor/accessors/data"
	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	_ "www.velocidex.com/golang/velociraptor/accessors/sparse"
)

type UploaderTestSuite struct {
	*testsuite.CloudTestSuite

	golden *ordereddict.Dict
}

func (self *UploaderTestSuite) SetupTest() {
	self.CloudTestSuite.SetupTest()

	writeback_service := writeback.GetWritebackService()
	writeback_service.LoadWriteback(self.ConfigObj.VeloConf())
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
	writeback_service := writeback.GetWritebackService()
	writeback, err := writeback_service.GetWriteback(org_config_obj)
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

	err = uploads.InstallVeloCloudUploader(ctx,
		org_config_obj, nil, writeback.ClientId, comm.Manager)
	assert.NoError(self.T(), err)
}

func (self *UploaderTestSuite) TestUploader() {
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

	resp := responder.TestResponderWithFlowId(
		self.ConfigObj.VeloConf(), "F.1234")

	// Build a scope to run a query.
	builder := services.ScopeBuilder{
		Config:       org_config_obj,
		ClientConfig: org_config_obj.Client,
		ACLManager:   acl_managers.NullACLManager{},
		Logger: logging.NewPlainLogger(
			org_config_obj, &logging.FrontendComponent),
	}

	manager, err := services.GetRepositoryManager(org_config_obj)
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	scope.SetContext(constants.SCOPE_RESPONDER_CONTEXT, resp)

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

	// Check the file was written on the server.
	file_store_factory := file_store.GetFileStore(org_config_obj)
	assert.NotNil(self.T(), file_store_factory)

	reader, err := file_store_factory.ReadFile(test_file)
	assert.NoError(self.T(), err)

	data, err := ioutil.ReadAll(reader)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), "Hello world", string(data))
}

func (self *UploaderTestSuite) TestSparseUploader() {
	ctx := self.Sm.Ctx
	wg := self.Sm.Wg

	org_manager, err := services.GetOrgManager()
	assert.NoError(self.T(), err)

	org_config_obj, err := org_manager.GetOrgConfig(self.OrgId)
	assert.NoError(self.T(), err)

	// This is the uploaded key within the test org.
	test_file := path_specs.NewSafeFilestorePath("clients",
		"C.1352adc54e292a23", "collections", "F.1231",
		"uploads", "sparse",
		// sha256 of "/sparse.txt"
		"0be2d1424fd102c85547fbcb46838ebe8617d98bb088c0bce5ab70234da97e90").
		SetType(api.PATH_TYPE_FILESTORE_ANY)
	self.clearFilestorePath(org_config_obj, test_file)

	self.startServerCommunicator(ctx, wg, org_config_obj)
	self.startClientCommunicator(ctx, wg, org_config_obj)

	resp := responder.TestResponderWithFlowId(
		self.ConfigObj.VeloConf(), "F.1231")

	// Build a scope to run a query.
	builder := services.ScopeBuilder{
		Config:       org_config_obj,
		ClientConfig: org_config_obj.Client,
		ACLManager:   acl_managers.NullACLManager{},
		Logger: logging.NewPlainLogger(
			org_config_obj, &logging.FrontendComponent),
	}

	manager, err := services.GetRepositoryManager(org_config_obj)
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	scope.SetContext(constants.SCOPE_RESPONDER_CONTEXT, resp)

	// Run the query which should upload the file
	res := (&uploads.UploadFunction{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("accessor", "sparse").
			Set("file", `{
   "DelegatePath": "the lazy fox jumped over the dog",
   "DelegateAccessor": "data",
   "Path": "[{\"Offset\":0,\"Length\":5},{\"Offset\":10,\"Length\":5}]"
}`).
			Set("name", `/sparse.txt`))

	upload_res, ok := res.(*velo_uploads.UploadResponse)
	assert.True(self.T(), ok)

	assert.Equal(self.T(), "", upload_res.Error)
	assert.Equal(self.T(), uint64(15), upload_res.Size)
	assert.Equal(self.T(), uint64(10), upload_res.StoredSize)

	golden := ordereddict.NewDict().
		Set("UploadResponse", upload_res)

	// Check the file was written on the server.
	file_store_factory := file_store.GetFileStore(org_config_obj)
	assert.NotNil(self.T(), file_store_factory)

	reader, err := file_store_factory.ReadFile(test_file)
	assert.NoError(self.T(), err)

	data, err := ioutil.ReadAll(reader)
	assert.NoError(self.T(), err)

	// 5 bytes at the start, skip 5 bytes then 5 bytes from offset 10
	assert.Equal(self.T(), "the lox ju", string(data))

	golden.Set("Sparse File Data", string(data))

	objects := self.checkForKey("F.1231")
	golden.Set("S3 files", objects)

	idx_path := test_file.SetType(api.PATH_TYPE_FILESTORE_SPARSE_IDX)
	reader, err = file_store_factory.ReadFile(idx_path)
	assert.NoError(self.T(), err)

	data, err = ioutil.ReadAll(reader)
	assert.NoError(self.T(), err)

	golden.Set("IDX file", string(data))

	goldie.Assert(self.T(), "TestSparseUploader", json.MustMarshalIndent(golden))

}

func (self *UploaderTestSuite) checkForKey(filter string) []string {
	// Check the actual path in the bucket we are in.
	session, err := filestore.GetS3Session(self.ConfigObj)
	assert.NoError(self.T(), err)

	svc := s3.New(session)
	res, err := svc.ListObjects(&s3.ListObjectsInput{
		Bucket: &self.ConfigObj.Cloud.Bucket,
	})
	assert.NoError(self.T(), err)

	result := []string{}
	for _, c := range res.Contents {
		if c.Key != nil && strings.Contains(*c.Key, filter) {
			result = append(result, *c.Key)
		}
	}

	return result
}

func TestUploader(t *testing.T) {
	suite.Run(t, &UploaderTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{
			Indexes: []string{"persisted"},
			OrgId:   "test",
		},
		golden: ordereddict.NewDict(),
	})
}
