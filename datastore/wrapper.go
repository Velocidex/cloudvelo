package datastore

import (
	"context"

	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/cloudvelo/config"
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
	ctx context.Context,
	config_obj *config.Config) datastore.DataStore {

	return &ElasticDatastore{
		ctx: ctx,
	}
}
