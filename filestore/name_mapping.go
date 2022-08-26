package filestore

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func PathspecToKey(
	config_obj *config_proto.Config, path_spec api.FSPathSpec) string {

	// Figure out where to store the file on disk. Sanitize the path
	// name properly.
	return path_spec.AsFilestoreFilename(
		&config_proto.Config{
			Datastore: &config_proto.DatastoreConfig{},
		})
}
