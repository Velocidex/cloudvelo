package repository

import (
	"context"
	"os"
	"testing"

	"github.com/alecthomas/assert"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
)

func TestSetArtifactFileImpersonation(t *testing.T) {
	config_obj := &config_proto.Config{}
	impersonating := readFixture(t, "fixtures/impersonating_artifact.yaml")
	plain := readFixture(t, "fixtures/plain_artifact.yaml")

	for _, tc := range []struct {
		name       string
		principal  string
		definition string
		allowed    bool
	}{
		{"impersonates the calling user", "admin", impersonating, true},
		{"impersonates another user", "mallory", impersonating, false},
		{"impersonation set by the superuser",
			utils.GetSuperuserName(config_obj), impersonating, true},
		{"username differs only in case", "Admin", impersonating, false},
		{"no impersonation", "mallory", plain, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := newInMemoryManager(config_obj)

			artifact, err := manager.SetArtifactFile(context.Background(),
				config_obj, tc.principal, tc.definition, "Custom.")

			if tc.allowed {
				assert.NoError(t, err)
				assert.NotNil(t, artifact)
				return
			}
			assert.EqualError(t, err,
				"Artifact can only impersonate calling user '"+tc.principal+"'")
			assert.Nil(t, artifact)
		})
	}
}

func newInMemoryManager(config_obj *config_proto.Config) *RepositoryManager {
	return &RepositoryManager{
		global_repository: &repository.Repository{
			Data: make(map[string]*artifacts_proto.Artifact),
		},
		config_obj: config_obj,
		ctx:        context.Background(),
	}
}

func readFixture(t *testing.T, path string) string {
	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	return string(data)
}
