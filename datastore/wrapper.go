package datastore

import (
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type ElasticWrapper struct {
	*datastore.FileBaseDataStore
	new_imp ElasticDatastore
}

func (self ElasticWrapper) SetSubject(
	config_obj *config_proto.Config,
	path api.DSPathSpec,
	message proto.Message) error {

	err := self.new_imp.SetSubject(config_obj, path, message)
	if err != nil {
		return self.FileBaseDataStore.SetSubject(config_obj, path, message)
	}
	return err
}

func (self ElasticWrapper) GetSubject(
	config_obj *config_proto.Config,
	path api.DSPathSpec,
	message proto.Message) error {

	err := self.new_imp.GetSubject(config_obj, path, message)
	if err != nil {
		return self.FileBaseDataStore.GetSubject(config_obj, path, message)
	}
	return err
}

func NewElasticDatastore(
	config_obj *config_proto.Config) datastore.DataStore {

	return &ElasticDatastore{}
}
