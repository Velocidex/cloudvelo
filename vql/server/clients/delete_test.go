package clients

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/client_info"
	"www.velocidex.com/golang/cloudvelo/testsuite"

	"www.velocidex.com/golang/velociraptor/logging"
	_ "www.velocidex.com/golang/velociraptor/result_sets/simple"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

// var (
// 	sample_flow = `collections/F.1234/task.db
// collections/F.1234/uploads/ntfs/"\\.\C:"/Windows/notepad.exe
// collections/F.1234/logs
// collections/F.1234/logs.json.index
// collections/F.1234/uploads.json
// collections/F.1234/uploads.json.index
// collections/F.1234/notebook/N.F.1234-C.123.json.db
// collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT16FBL4PM.json.db
// collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT1195IMMU.json.db
// collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT1195IMMU/query_1.json
// collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT1195IMMU/query_1.json.index
// monitoring_logs/Generic.Client.Stats/2021-08-14.json
// monitoring_logs/Generic.Client.Stats/2021-08-14.json.tidx
// labels.json.db
// vfs/file.db
// vfs/file/C%3A.db
// vfs/file/C%3A/Users.db
// vfs_files/file/C%3A/Users/mike/test/1.txt.db
// artifacts/Generic.Client.Info/F.C49TC44OSO62E/Users.json
// artifacts/Generic.Client.Info/F.C49TC44OSO62E/Users.json.index
// artifacts/Generic.Client.Info/F.C49TC44OSO62E/BasicInformation.json
// artifacts/Generic.Client.Info/F.C49TC44OSO62E/BasicInformation.json.index
// tasks/task1123.db
// ping.db`
// )

type DeleteTestSuite struct {
	*testsuite.CloudTestSuite
	dir       string
	client_id string
}

func (self *DeleteTestSuite) TestDeleteClient() {
	config_obj := self.ConfigObj.VeloConf()

	Clock := &utils.MockClock{
		MockNow: time.Unix(1661391000, 0),
	}

	clients := []client_info.ClientInfo{
		// This client is currently connected
		{
			ClientId:      "C.ConnectedClient",
			Ping:          uint64(Clock.Now().UnixNano()),
			AssignedHunts: []string{},
			Labels:        []string{},
			LowerLabels:   []string{},
		},

		{
			ClientId:      "C.AlreadyRanAllClients",
			Ping:          uint64(Clock.Now().UnixNano()),
			AssignedHunts: []string{"H.AllClients"},
			Labels:        []string{},
			LowerLabels:   []string{},
		},

		{
			ClientId:      "C.WithLabelFoo",
			Ping:          uint64(Clock.Now().UnixNano()),
			Labels:        []string{"Foo"},
			LowerLabels:   []string{"foo"},
			AssignedHunts: []string{},
		},

		// This client has not been seen in a while
		{
			ClientId:      "C.OfflineClient",
			Ping:          uint64(Clock.Now().Add(-72 * time.Hour).UnixNano()),
			AssignedHunts: []string{},
			Labels:        []string{},
			LowerLabels:   []string{},
		},
	}

	// Add these clients directly into the index.
	for _, c := range clients {
		err := cvelo_services.SetElasticIndex(
			self.Ctx, config_obj.OrgId, "clients", c.ClientId, c)
		assert.NoError(self.T(), err)
	}

	client := self.getClientRecord("C.WithLabelFoo")
	assert.NotNil(self.T(), client)
	self.client_id = client.ClientId

	manager, _ := services.GetRepositoryManager(config_obj)
	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: acl_managers.NullACLManager{},
		Logger: logging.NewPlainLogger(config_obj,
			&logging.FrontendComponent),
		Env: ordereddict.NewDict(),
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()

	_ = vtesting.RunPlugin(DeleteClientPlugin{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("really_do_it", true).
			Set("client_id", self.client_id)))

	result, err := cvelo_services.GetElasticRecord(context.Background(),
		self.ConfigObj.OrgId, "clients", self.client_id)
	assert.Nil(self.T(), result)
	assert.Equal(self.T(), err, os.ErrNotExist)

	// XXX - verify that files are deleted
	// XXX - verify that this client is removed from other indexes
}

func (self *DeleteTestSuite) getClientRecord(client_id string) *client_info.ClientInfo {
	serialized, err := cvelo_services.GetElasticRecord(
		self.Ctx, self.ConfigObj.OrgId, "clients", client_id)
	assert.NoError(self.T(), err)

	result := &client_info.ClientInfo{}
	err = json.Unmarshal(serialized, &result)
	assert.NoError(self.T(), err)

	return result
}

// func (self *DeleteTestSuite) SetupTest() {
// 	self.ConfigObj = self.LoadConfig()
//
// 	var err error
// 	self.dir, err = ioutil.TempDir("", "delete_test")
// 	assert.NoError(self.T(), err)
//
// 	self.ConfigObj.Datastore.Implementation = "FileBaseDataStore"
// 	self.ConfigObj.Datastore.FilestoreDirectory = self.dir
// 	self.ConfigObj.Datastore.Location = self.dir
//
// 	self.client_id = "C.12312"
// 	// self.TestSuite.SetupTest()
// }
//
// func (self *DeleteTestSuite) TearDownTest() {
// 	err := os.RemoveAll(self.dir)
// 	assert.NoError(self.T(), err)
// }
//
// func (self *DeleteTestSuite) TestDeleteClient() {
// 	ConfigObj := self.ConfigObj
// 	db, err := datastore.GetDB(ConfigObj)
// 	assert.NoError(self.T(), err)
//
// 	golden := ordereddict.NewDict()
//
// 	file_store_factory := file_store.GetFileStore(ConfigObj)
//
// 	for _, line := range strings.Split(sample_flow, "\n") {
// 		line = "/clients/" + self.client_id + "/" + line
// 		if strings.HasSuffix(line, ".db") {
// 			db.SetSubject(self.ConfigObj,
// 				paths.DSPathSpecFromClientPath(line),
// 				&emptypb.Empty{})
// 		} else {
// 			path_spec := paths.FSPathSpecFromClientPath(line)
// 			fd, err := file_store_factory.WriteFile(path_spec)
// 			assert.NoError(self.T(), err)
// 			fd.Close()
// 		}
// 	}
//
// 	// Populate the client's space with some data.
// 	client_info := &actions_proto.ClientInfo{
// 		ClientId: self.client_id,
// 	}
//
// 	client_path_manager := paths.NewClientPathManager(self.client_id)
// 	db.SetSubject(self.ConfigObj,
// 		client_path_manager.Ping(), client_info)
// 	db.SetSubject(self.ConfigObj,
// 		client_path_manager.Path(), client_info)
//
// 	// Get a list of all filestore items before deletion
// 	before := []string{}
// 	err = filepath.WalkDir(self.dir,
// 		func(path string, d fs.DirEntry, err error) error {
// 			path = strings.TrimPrefix(path, self.dir)
// 			path = strings.ReplaceAll(path, "\\", "/")
//
// 			before = append(before, path)
// 			return nil
// 		})
// 	assert.NoError(self.T(), err)
// 	golden.Set("Before filestore", before)
//
// 	manager, _ := services.GetRepositoryManager(self.ConfigObj)
// 	builder := services.ScopeBuilder{
// 		Config:     self.ConfigObj,
// 		ACLManager: acl_managers.NullACLManager{},
// 		Logger: logging.NewPlainLogger(self.ConfigObj,
// 			&logging.FrontendComponent),
// 		Env: ordereddict.NewDict(),
// 	}
//
// 	scope := manager.BuildScope(builder)
// 	defer scope.Close()
//
// 	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
// 	defer cancel()
//
// 	result := vtesting.RunPlugin(DeleteClientPlugin{}.Call(ctx, scope,
// 		ordereddict.NewDict().
// 			Set("really_do_it", true).
// 			Set("client_id", self.client_id)))
//
// 	after := []string{}
// 	err = filepath.WalkDir(self.dir,
// 		func(path string, d fs.DirEntry, err error) error {
// 			path = strings.TrimPrefix(path, self.dir)
// 			path = strings.ReplaceAll(path, "\\", "/")
//
// 			after = append(after, path)
// 			return nil
// 		})
// 	assert.NoError(self.T(), err)
// 	golden.Set("After filestore", after)
//
// 	sort.Slice(result, func(i, j int) bool {
// 		l, _ := result[i].(*ordereddict.Dict).GetString("vfs_path")
// 		r, _ := result[j].(*ordereddict.Dict).GetString("vfs_path")
// 		return l < r
// 	})
//
// 	golden.Set("Files deleted", result)
// 	goldie.Assert(self.T(), "TestDeleteClient", json.MustMarshalIndent(golden))
// }

func TestDeletePlugin(t *testing.T) {
	suite.Run(t, &DeleteTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{
			Indexes: []string{"clients"},
		},
	})
}
