package repository_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/cloudvelo/services/repository"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

const (
	customArtifact = `
name: Custom.Tagged
type: CLIENT
sources:
- query: SELECT * FROM info()
`
	builtInArtifact = "Generic.Client.Info"
)

type MetadataTestSuite struct {
	*testsuite.CloudTestSuite
}

func (self *MetadataTestSuite) TestArtifactMetadata() {
	config_obj := self.ConfigObj.VeloConf()

	manager, err := services.GetRepositoryManager(config_obj)
	assert.NoError(self.T(), err)

	repo, err := manager.GetGlobalRepository(config_obj)
	assert.NoError(self.T(), err)

	// No metadata yet.
	tags, err := repo.Tags(self.Ctx, config_obj)
	assert.NoError(self.T(), err)
	assert.Empty(self.T(), tags)

	_, err = manager.SetArtifactFile(self.Ctx, config_obj, "admin",
		customArtifact, "")
	assert.NoError(self.T(), err)

	// Setting metadata used to fail with "not implemented".
	err = manager.SetArtifactMetadata(self.Ctx, config_obj, "admin",
		"Custom.Tagged", &artifacts_proto.ArtifactMetadata{
			Tags: []string{"Triage", "Windows"},
		})
	assert.NoError(self.T(), err)

	// Built in artifacts are held by the parent but can still carry
	// org specific metadata.
	err = manager.SetArtifactMetadata(self.Ctx, config_obj, "admin",
		builtInArtifact, &artifacts_proto.ArtifactMetadata{
			Tags:  []string{"Basic", "Windows"},
			Basic: true,
		})
	assert.NoError(self.T(), err)

	tags, err = repo.Tags(self.Ctx, config_obj)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), []string{"Basic", "Triage", "Windows"}, tags)

	artifact, pres := repo.Get(self.Ctx, config_obj, "Custom.Tagged")
	assert.True(self.T(), pres)
	assert.Equal(self.T(), []string{"Triage", "Windows"}, artifact.Metadata.GetTags())

	artifact, pres = repo.Get(self.Ctx, config_obj, builtInArtifact)
	assert.True(self.T(), pres)
	assert.True(self.T(), artifact.Metadata.GetBasic())

	// The shared built in held by the root org must not be modified.
	org_manager, err := services.GetOrgManager()
	assert.NoError(self.T(), err)

	root_config, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
	assert.NoError(self.T(), err)

	root_manager, err := services.GetRepositoryManager(root_config)
	assert.NoError(self.T(), err)

	root_repo, err := root_manager.GetGlobalRepository(root_config)
	assert.NoError(self.T(), err)

	artifact, pres = root_repo.Get(self.Ctx, root_config, builtInArtifact)
	assert.True(self.T(), pres)
	assert.Nil(self.T(), artifact.Metadata)

	// Re-saving the definition keeps its metadata and returns it so
	// callers like artifact_set can update it.
	artifact, err = manager.SetArtifactFile(self.Ctx, config_obj, "admin",
		customArtifact, "")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), []string{"Triage", "Windows"}, artifact.Metadata.GetTags())

	// Metadata is persisted, not just cached: a fresh repository
	// sees it.
	fresh := repository.NewRepository(self.Ctx, config_obj)
	fresh.SetParent(root_repo, root_config)

	artifact, pres = fresh.Get(self.Ctx, config_obj, "Custom.Tagged")
	assert.True(self.T(), pres)
	assert.Equal(self.T(), []string{"Triage", "Windows"}, artifact.Metadata.GetTags())

	// Deleting the artifact removes its metadata.
	err = manager.DeleteArtifactFile(self.Ctx, config_obj, "admin",
		"Custom.Tagged")
	assert.NoError(self.T(), err)

	tags, err = repo.Tags(self.Ctx, config_obj)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), []string{"Basic", "Windows"}, tags)
}

func TestMetadata(t *testing.T) {
	suite.Run(t, &MetadataTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{},
	})
}
