package filestore

import (
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func PathspecToKey(path_spec api.FSPathSpec) string {
	return strings.TrimPrefix(path_spec.AsFilestoreFilename(
		&config_proto.Config{
			Datastore: &config_proto.DatastoreConfig{},
		}), "/")
}
