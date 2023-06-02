package repository_test

import (
	"fmt"
	"github.com/Velocidex/yaml/v2"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/cloudvelo/services/repository"
	"www.velocidex.com/golang/cloudvelo/testsuite"
)

type RepositoryManagerTest struct {
	*testsuite.CloudTestSuite
	wg  *sync.WaitGroup
	mgr *repository.RepositoryManager
}

const ValidArtifact string = `
name: Server.Monitor.Health
description: |
  Some Description
type: server_event
reports:
  - type: server_event
    template: |
      # STATIC TEXT
`

const UnknownArtifact string = `
name: I.DONT.EXIST
description: |
  Some Description
type: server_event
reports:
  - type: server_event
    template: |
      # STATIC TEXT
`

const InvalidArtifact string = `
Foo: Server.Monitor.Health
Bar: |
  Some Description
`

// run before each test
func (self *RepositoryManagerTest) BeforeTest(suiteName, testName string) {
	mgr, err := repository.NewRepositoryManager(self.Ctx, self.wg, self.ConfigObj.VeloConf())
	assert.NoError(self.T(), err, "Failed to create Repo Manager")
	err = mgr.LoadBuiltInArtifacts(self.Ctx, self.ConfigObj.VeloConf())
	assert.NoError(self.T(), err, "Failed to Add Built In Artifacts")

	self.mgr = mgr
}

func (self *RepositoryManagerTest) TestOverrideBuiltInArtifactSuccess() {
	err := self.mgr.OverrideBuiltInArtifact(self.ConfigObj.VeloConf(), "Server.Monitor.Health", []byte(ValidArtifact))
	assert.NoError(self.T(), err)

	// the artifact should be overridden
	match, err := self.checkArtifact("Server.Monitor.Health", ValidArtifact)
	assert.NoError(self.T(), err)
	assert.True(self.T(), match)
}

func (self *RepositoryManagerTest) TestOverrideBuiltInArtifactUnknownArtifact() {
	err := self.mgr.OverrideBuiltInArtifact(self.ConfigObj.VeloConf(), "I.DONT.EXIST", []byte(UnknownArtifact))
	assert.NoError(self.T(), err)

	// the artifact should be available
	match, err := self.checkArtifact("I.DONT.EXIST", UnknownArtifact)
	assert.NoError(self.T(), err)
	assert.True(self.T(), match)
}

func (self *RepositoryManagerTest) TestOverrideBuiltInArtifactInvalidArtifact() {
	err := self.mgr.OverrideBuiltInArtifact(self.ConfigObj.VeloConf(), "Server.Monitor.Health", []byte(InvalidArtifact))
	assert.Error(self.T(), err)

	// the artifact should not be overridden
	match, err := self.checkArtifact("Server.Monitor.Health", InvalidArtifact)
	assert.NoError(self.T(), err)
	assert.False(self.T(), match)
}

func (self *RepositoryManagerTest) checkArtifact(name string, data string) (bool, error) {

	global_repo, err := self.mgr.GetGlobalRepository(self.ConfigObj.VeloConf())

	actual_artifact, found := global_repo.Get(self.Ctx, self.ConfigObj.VeloConf(), name)
	if !found {
		// we expect that regardless the built-in artifact should exist, if it doesn't, blow up
		return false, fmt.Errorf("Failed to find artifact")
	}

	// verify the artifact contains the new data
	expected_artifact := &artifacts_proto.Artifact{}
	err = yaml.UnmarshalStrict([]byte(data), expected_artifact)
	if err != nil {
		// if the artifact content is bad then it can't exist
		return false, nil
	}
	// the expected artifact to have the following properties
	expected_artifact.Compiled = true
	expected_artifact.BuiltIn = true

	// Remove the RAW value as it's not useful for this comparison
	actual_artifact.Raw = ""

	if expected_artifact.String() != actual_artifact.String() {
		return false, nil
	}

	return true, nil
}

func TestRepositoryManager(t *testing.T) {
	suite.Run(t, &RepositoryManagerTest{
		CloudTestSuite: &testsuite.CloudTestSuite{
			OrgId: "root",
		},
		wg: &sync.WaitGroup{},
	})
}
