package testsuite

import (
	"context"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/schema"
	"www.velocidex.com/golang/cloudvelo/services/orgs"
	velo_config "www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type CloudTestSuite struct {
	suite.Suite

	OrgId     string
	ConfigObj *config.Config

	Indexes []string

	Sm     *services.Service
	Ctx    context.Context
	cancel func()
}

func (self *CloudTestSuite) LoadConfig() *config.Config {
	loader := config.ConfigLoader{
		VelociraptorLoader: new(velo_config.Loader).
			WithRequiredFrontend().WithVerbose(true),
		ConfigText: SERVER_CONFIG,
	}
	config_obj, err := loader.Load()
	require.NoError(self.T(), err)

	return config_obj
}

func (self *CloudTestSuite) TearDownTest() {
	self.cancel()
	self.Sm.Close()
}

func (self *CloudTestSuite) SetupTest() {
	if self.ConfigObj == nil {
		self.ConfigObj = self.LoadConfig()
	}

	self.Ctx, self.cancel = context.WithTimeout(context.Background(),
		time.Second*60)

	config_obj := self.ConfigObj.VeloConf()
	sm := services.NewServiceManager(self.Ctx, config_obj)
	org_manager, err := orgs.NewOrgManager(sm.Ctx, sm.Wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	test_org := self.OrgId
	if test_org == "" {
		test_org = "test"
	}
	_, err = org_manager.CreateNewOrg("test", test_org)
	assert.NoError(self.T(), err)

	self.Sm = sm
	self.ConfigObj.OrgId = test_org

	if len(self.Indexes) == 0 {
		err = schema.Initialize(self.Ctx, config_obj, test_org,
			schema.NO_FILTER, schema.RESET_INDEX)
		assert.NoError(self.T(), err)
	} else {
		for _, i := range self.Indexes {
			err = schema.Initialize(self.Ctx, config_obj, test_org,
				i, schema.RESET_INDEX)
			assert.NoError(self.T(), err)
		}
	}
}
