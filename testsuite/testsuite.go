package testsuite

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/schema"
	"www.velocidex.com/golang/cloudvelo/services/orgs"
	velo_config "www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type CloudTestSuite struct {
	suite.Suite

	OrgId     string
	ConfigObj *config.Config

	Sm     *services.Service
	Ctx    context.Context
	cancel func()

	writeback_file string

	time_closer func()
}

// Allow an external file to override the config. This allows us to
// manually test with AWS credentials.
func (self *CloudTestSuite) LoadConfig() *config.Config {
	patch := ""
	override_filename := os.Getenv("VELOCIRAPTOR_TEST_CONFIG_OVERRIDE")
	if override_filename != "" {
		data, err := ioutil.ReadFile(override_filename)
		require.NoError(self.T(), err)
		fmt.Printf("Will override config with %v\n", override_filename)
		patch = string(data)
	}

	loader := config.ConfigLoader{
		VelociraptorLoader: new(velo_config.Loader).
			WithRequiredFrontend().
			WithVerbose(true).
			WithEnvLiteralLoader("VELOCONFIG"),
		ConfigText: SERVER_CONFIG,
		JSONPatch:  patch,
	}
	config_obj, err := loader.Load()
	require.NoError(self.T(), err)

	return config_obj
}

func (self *CloudTestSuite) SetupSuite() {
	if self.ConfigObj == nil {
		self.ConfigObj = self.LoadConfig()
	}

	tempfile, err := ioutil.TempFile("", "test")
	assert.NoError(self.T(), err)

	self.writeback_file = tempfile.Name()

	tempfile.Write([]byte(writeback_file))
	tempfile.Close()

	self.ConfigObj.Client.WritebackLinux = tempfile.Name()
	self.ConfigObj.Client.WritebackWindows = tempfile.Name()
	self.ConfigObj.Client.WritebackDarwin = tempfile.Name()
}

func (self *CloudTestSuite) TearDownSuite() {
	os.Remove(self.writeback_file)
	if self.time_closer != nil {
		self.time_closer()
	}
}

func (self *CloudTestSuite) TearDownTest() {
	self.cancel()
	self.Sm.Close()

	// SetupTest installs a new clock per test, so it has to be restored
	// per test too - otherwise every test but the last leaks one.
	if self.time_closer != nil {
		self.time_closer()
		self.time_closer = nil
	}
}

func (self *CloudTestSuite) SetupTest() {
	self.time_closer = utils.MockTime(&utils.IncClock{NowTime: 1744634710})

	self.Ctx, self.cancel = context.WithTimeout(context.Background(),
		time.Second*60)

	config_obj := self.ConfigObj
	sm := services.NewServiceManager(self.Ctx, config_obj.VeloConf())
	org_manager, err := orgs.NewOrgManager(sm.Ctx, sm.Wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	test_org := self.OrgId
	if test_org == "" {
		test_org = defaultTestOrg()
	}

	// Delete the previous indexes for the org.
	err = schema.Delete(self.Ctx, config_obj.VeloConf(), test_org, schema.NO_FILTER)
	assert.NoError(self.T(), err)

	_, err = org_manager.CreateNewOrg("test", test_org, services.RandomNonce)
	assert.NoError(self.T(), err)

	self.Sm = sm
	self.OrgId = test_org
	self.ConfigObj.OrgId = test_org

	// Make sure the index templates are initialized if needed.
	err = schema.InstallIndexTemplates(self.Ctx, config_obj)
	assert.NoError(self.T(), err)
}

var invalidOrgChars = regexp.MustCompile("[^a-z0-9_]+")

// Returns an org named after the test package, e.g.
// test_services_launcher. SetupTest deletes the org's indexes and go
// test runs packages in parallel, so packages sharing an org delete
// each other's data mid test.
func defaultTestOrg() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "test"
	}

	// go test runs in the package directory. Name the org after its
	// path in the module so packages with the same name differ.
	name := filepath.Base(cwd)
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			rel, err := filepath.Rel(dir, cwd)
			if err == nil && rel != "." {
				name = rel
			}
			break
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}

	return "test_" + invalidOrgChars.ReplaceAllString(
		strings.ToLower(filepath.ToSlash(name)), "_")
}
