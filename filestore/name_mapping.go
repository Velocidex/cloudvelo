package filestore

import (
	"strings"

	"www.velocidex.com/golang/cloudvelo/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
)

var (
	EmptyConfig = &config_proto.Config{
		Datastore: &config_proto.DatastoreConfig{},
	}
)

func PathspecToKey(config_obj *config.Config,
	path_spec api.FSPathSpec) string {
	org_path_spec := path_specs.NewUnsafeFilestorePath(
		"orgs", config_obj.OrgId).AddChild(path_spec.Components()...)

	return strings.TrimPrefix(org_path_spec.AsFilestoreFilename(
		EmptyConfig), "/")
}
