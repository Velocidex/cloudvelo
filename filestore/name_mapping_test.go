package filestore

import (
	"strings"
	"testing"

	"www.velocidex.com/golang/cloudvelo/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

const testNonce = "test-obfuscation-nonce"

func orgConfig(org_id string) *config.Config {
	return &config.Config{Config: config_proto.Config{
		OrgId:            org_id,
		ObfuscationNonce: testNonce,
	}}
}

// The path upstream's FormUploadPathManager builds for an upload made
// from org_id.
func formUploadPath(org_id, filename string) []string {
	return []string{"public", "temp",
		paths.ObfuscateName(&orgConfig(org_id).Config, filename), filename}
}

func key(config_obj *config.Config, components []string) string {
	return PathspecToKey(config_obj, paths.PUBLIC_ROOT.AddChild(components[1:]...))
}

// UploadFormFile writes with the root org's filestore and
// LoadArtifactPack reads with the caller's, so both must map to the
// same key.
func TestFormUploadReadsBackFromTheRootOrg(t *testing.T) {
	upload := formUploadPath("O123", "pack.zip")

	written := key(orgConfig(services.ROOT_ORG_ID), upload)
	read := key(orgConfig("O123"), upload)

	assert.Equal(t, written, read)
	assert.True(t, strings.HasPrefix(read, "orgs/root/public/temp/"), read)
}

func TestFormUploadFromAnotherOrgIsNotMapped(t *testing.T) {
	upload := formUploadPath("O456", "pack.zip")

	written := key(orgConfig(services.ROOT_ORG_ID), upload)
	read := key(orgConfig("O123"), upload)

	assert.NotEqual(t, written, read)
	assert.True(t, strings.HasPrefix(read, "orgs/O123/public/temp/"), read)
}

func TestOtherPathsKeepTheCallersOrg(t *testing.T) {
	assert.Equal(t, "orgs/O123/public/tool.exe",
		key(orgConfig("O123"), []string{"public", "tool.exe"}))

	assert.Equal(t, "orgs/O123/temp/abc",
		PathspecToKey(orgConfig("O123"), paths.TEMP_ROOT.AddChild("abc")))

	// A form upload path whose hash doesn't match the filename.
	assert.True(t, strings.HasPrefix(
		key(orgConfig("O123"), []string{"public", "temp", "0000", "pack.zip"}),
		"orgs/O123/"))
}
