package filestore

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/vql/uploads"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	EmptyConfig = &config_proto.Config{
		Datastore: &config_proto.DatastoreConfig{},
	}
)

func PathspecToKey(config_obj *config.Config,
	path_spec api.FSPathSpec) string {

	base_path := path_spec.
		SetType(api.PATH_TYPE_FILESTORE_ANY)

	key := strings.TrimPrefix(
		base_path.AsFilestoreFilename(&config_proto.Config{
			Datastore: &config_proto.DatastoreConfig{
				FilestoreDirectory: "orgs/" +
					utils.NormalizedOrgId(config_obj.OrgId),
			},
		}), "/")

	if path_spec.Type() == api.PATH_TYPE_FILESTORE_SPARSE_IDX {
		key += ".idx"
	}

	return key
}

// Build an S3 key from a client upload request.
func S3KeyForClientUpload(
	org_id string, request *uploads.UploadRequest) string {

	components := append([]string{"orgs",
		utils.NormalizedOrgId(org_id)},
		S3ComponentsForClientUpload(request)...)

	// Support index files.
	return strings.Join(components, "/")
}

func S3ComponentsForClientUpload(request *uploads.UploadRequest) []string {
	base := []string{"clients", request.ClientId, "collections",
		request.SessionId, "uploads", request.Accessor}

	// Encode the client path in a safe way for s3 paths:
	// 1. S3 path are limited to 1024 bytes
	// 2. We do not need to go back from an S3 path to a client path
	//    so we can safely use a one way hash function.

	client_path := strings.Join(request.Components, "\x00")
	h := sha256.New()
	h.Write([]byte(client_path))

	file_name := fmt.Sprintf("%02x", h.Sum(nil))

	// Support index files.
	if request.Type == "idx" {
		file_name += ".idx"
	}

	return append(base, file_name)
}
