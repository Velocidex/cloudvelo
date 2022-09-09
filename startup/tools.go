package startup

import (
	"context"

	cvelo_datastore "www.velocidex.com/golang/cloudvelo/datastore"
	"www.velocidex.com/golang/cloudvelo/filestore"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/orgs"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/services"
)

func StartToolServices(
	ctx context.Context,
	elastic_config_path string,
	config_obj *config_proto.Config) (*services.Service, error) {
	sm := services.NewServiceManager(ctx, config_obj)
	_, err := orgs.NewOrgManager(sm.Ctx, sm.Wg, elastic_config_path, config_obj)
	if err != nil {
		return sm, err
	}

	// Install the ElasticDatastore
	datastore.OverrideDatastoreImplementation(
		cvelo_datastore.NewElasticDatastore(config_obj))

	file_store_obj, err := filestore.NewS3Filestore(config_obj, elastic_config_path)
	if err != nil {
		return nil, err
	}
	file_store.OverrideFilestoreImplementation(config_obj, file_store_obj)

	err = cvelo_services.StartElasticSearchService(
		config_obj, elastic_config_path)
	if err != nil {
		return nil, err
	}

	return sm, nil
}
