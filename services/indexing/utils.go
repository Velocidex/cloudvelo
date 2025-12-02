package indexing

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Force all the client info to be loaded into the memory cache.
func PopulateClientInfoCache(
	ctx context.Context,
	config_obj *config_proto.Config) error {

	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return err
	}

	output_chan, err := indexer.SearchClientsChan(ctx,
		nil, config_obj, "all",
		utils.GetSuperuserName(config_obj))
	if err != nil {
		return err
	}

	for _ = range output_chan {
	}
	return nil
}
