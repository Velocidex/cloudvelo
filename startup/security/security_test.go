package security_test

import (
	"strings"
	"testing"

	"github.com/alecthomas/assert"
	"www.velocidex.com/golang/cloudvelo/startup/security"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/accessors/file_store"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/vql/common"
)

func TestFsAllowOnly(t *testing.T) {
	apply(t, &config_proto.Security{
		AllowedFsAccessorPrefix: []string{"/clients"},
	})

	assertFsAllowed(t, "/clients/C.1/x")
	assertFsDenied(t, "/temp/x")
}

func TestFsDenyOnly(t *testing.T) {
	apply(t, &config_proto.Security{
		DeniedFsAccessorPrefix: []string{"/temp"},
	})

	assertFsDenied(t, "/temp/x")
	assertFsAllowed(t, "/clients/x")
	assertFsAllowed(t, "/backups/x")
}

func TestFsAllowDenyOverlap(t *testing.T) {
	apply(t, &config_proto.Security{
		AllowedFsAccessorPrefix: []string{"/clients"},
		DeniedFsAccessorPrefix:  []string{"/clients/C.1/secret"},
	})

	assertFsDenied(t, "/clients/C.1/secret/x")
	assertFsAllowed(t, "/clients/C.1/other")
	assertFsDenied(t, "/temp/x")
}

func TestNoSecurityBlock(t *testing.T) {
	apply(t, nil)

	assertFsAllowed(t, "/clients/x")
	assertFsAllowed(t, "/temp/x")
	for _, denied := range []string{
		"/acl/x", "/backups/x", "/config/x", "/orgs/x", "/secrets/x", "/users/x",
	} {
		assertFsDenied(t, denied)
	}
	assert.NoError(t, file.CheckPath("/var/tmp/x"))
}

func TestFileAccessorPrefixes(t *testing.T) {
	apply(t, &config_proto.Security{
		DeniedFileAccessorPrefix: []string{"/var/tmp/x"},
	})

	assert.Error(t, file.CheckPath("/var/tmp/x/y"))
	assert.NoError(t, file.CheckPath("/var/tmp/z"))
}

func TestShadowedEnvVarsAddedOnce(t *testing.T) {
	config_obj := &config_proto.Config{Security: &config_proto.Security{
		ShadowedEnvVars: []string{"SECRET_ONE", "SECRET_TWO"},
	}}
	apply(t, config_obj.Security)
	assert.NoError(t, security.Apply(config_obj))

	for _, name := range []string{"SECRET_ONE", "SECRET_TWO"} {
		count := 0
		for _, s := range common.ShadowedEnv {
			if s == name {
				count++
			}
		}
		assert.Equal(t, 1, count, name)
	}
}

func TestHostedFsAllowList(t *testing.T) {
	apply(t, &config_proto.Security{
		AllowedFsAccessorPrefix: []string{"/clients", "/downloads", "/notebooks"},
	})

	assertFsAllowed(t, "/clients/C.1/collections/F.1/uploads/x")
	assertFsAllowed(t, "/downloads/C.1/F.1/x.zip")
	assertFsAllowed(t, "/notebooks/N.1/attach/x")
	assertFsDenied(t, "/hunts/H.1/x")
	assertFsDenied(t, "/server_artifacts/x")
}

func apply(t *testing.T, security_block *config_proto.Security) {
	shadowed := append([]string{}, common.ShadowedEnv...)
	t.Cleanup(func() {
		file.SetPrefixes(nil, nil)
		file_store.SetPrefixes(nil, nil)
		common.ShadowedEnv = shadowed
	})

	assert.NoError(t, security.Apply(
		&config_proto.Config{Security: security_block}))
}

func fsPath(path string) []string {
	return strings.Split(strings.TrimPrefix(path, "/"), "/")
}

func assertFsAllowed(t *testing.T, path string) {
	assert.NoError(t, file_store.IsFileAccessible(
		path_specs.NewUnsafeFilestorePath(fsPath(path)...)), path)
}

func assertFsDenied(t *testing.T, path string) {
	assert.Error(t, file_store.IsFileAccessible(
		path_specs.NewUnsafeFilestorePath(fsPath(path)...)), path)
}
