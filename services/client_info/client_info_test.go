package client_info_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type ClientInfoTestSuite struct {
	*testsuite.CloudTestSuite
}

// Set must not write a client record for an invalid client id. An
// empty id is the worst case: it would index a main record under a
// random document id.
func (self *ClientInfoTestSuite) TestSetRejectsInvalidClientIds() {
	config_obj := self.ConfigObj.VeloConf()

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	assert.NoError(self.T(), err)

	for _, client_id := range []string{"", "notaclient"} {
		err := client_info_manager.Set(self.Ctx, &services.ClientInfo{
			ClientInfo: &actions_proto.ClientInfo{
				ClientId: client_id,
				Hostname: "rejected",
			}})
		assert.Error(self.T(), err, "client id %q", client_id)
	}

	count, err := cvelo_services.QueryCountAPI(self.Ctx, self.OrgId, "persisted",
		`{"query": {"term": {"hostname": "rejected"}}}`)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 0, count)

	err = client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{
			ClientId: "C.1234",
			Hostname: "accepted",
		}})
	assert.NoError(self.T(), err)

	client_info, err := client_info_manager.Get(self.Ctx, "C.1234")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), "accepted", client_info.Hostname)
}

func TestClientInfo(t *testing.T) {
	suite.Run(t, &ClientInfoTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{},
	})
}
