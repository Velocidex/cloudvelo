package startup

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/config"
	cvelo_datastore "www.velocidex.com/golang/cloudvelo/datastore"
	"www.velocidex.com/golang/cloudvelo/filestore"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/orgs"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/services"
)

func StartToolServices(
	ctx context.Context, config_obj *config.Config) (*services.Service, error) {
	sm := services.NewServiceManager(ctx, config_obj.VeloConf())
	_, err := orgs.NewOrgManager(sm.Ctx, sm.Wg, config_obj)
	if err != nil {
		return sm, err
	}

	// Install the ElasticDatastore
	datastore.OverrideDatastoreImplementation(
		cvelo_datastore.NewElasticDatastore(ctx, config_obj))

	file_store_obj, err := filestore.NewS3Filestore(ctx, config_obj)
	if err != nil {
		return nil, err
	}
	file_store.OverrideFilestoreImplementation(
		config_obj.VeloConf(), file_store_obj)

	err = cvelo_services.StartElasticSearchService(config_obj)
	if err != nil {
		return nil, err
	}

	return sm, nil
}
