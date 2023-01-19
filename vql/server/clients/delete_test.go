package clients

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/testsuite"

	"www.velocidex.com/golang/velociraptor/logging"
	_ "www.velocidex.com/golang/velociraptor/result_sets/simple"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type DeleteTestSuite struct {
	*testsuite.CloudTestSuite
	client_id string
}

func (self *DeleteTestSuite) TestDeleteClient() {
	config_obj := self.ConfigObj.VeloConf()

	Clock := &utils.MockClock{
		MockNow: time.Unix(1661391000, 0),
	}

	clients := []api.ClientRecord{
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

	// TODO - verify that files are deleted
	// TODO - verify that this client is removed from other indexes
}

func (self *DeleteTestSuite) getClientRecord(client_id string) *api.ClientRecord {
	config_obj := self.ConfigObj.VeloConf()
	result, err := api.GetMultipleClients(self.Ctx, config_obj, []string{client_id})
	if err != nil || len(result) == 0 {
		return nil
	}

	return result[0]
}

func TestDeletePlugin(t *testing.T) {
	suite.Run(t, &DeleteTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{
			Indexes: []string{"clients"},
		},
	})
}
