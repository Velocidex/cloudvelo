package datastore

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/velociraptor/datastore"
)

func NewElasticDatastore(
	ctx context.Context,
	config_obj *config.Config) datastore.DataStore {

	return &ElasticDatastore{
		ctx: ctx,
	}
}
