package simple

import (
	"context"
	"fmt"

	"www.velocidex.com/golang/cloudvelo/filestore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ResultSetFactory struct{}

func (self ResultSetFactory) NewResultSetWriter(
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	opts *json.EncOpts,
	completion func(),
	truncate result_sets.WriteMode) (result_sets.ResultSetWriter, error) {

	cloud_config_obj := filestore.GetConfigObj(file_store_factory)
	config_obj := cloud_config_obj.VeloConf()

	new_id := fmt.Sprintf("%v", utils.GetGUID())
	base_record := NewSimpleResultSetRecord(log_path, new_id)
	ctx := context.Background()

	var existing bool
	md := &ResultSetMetadataRecord{
		Timestamp: utils.GetTime().Now().UnixNano(),
		VFSPath:   base_record.VFSPath,
		ID:        new_id,
		Type:      "rs_metadata",
	}

	if !truncate {
		// Get the existing metadata record
		existing_md, err := GetResultSetMetadata(ctx, config_obj, log_path)
		if err == nil {
			md = existing_md
			existing = true
		}
	}

	if !existing {
		err := SetResultSetMetadata(ctx, config_obj, log_path, md)
		if err != nil {
			return nil, err
		}
	}

	return &ElasticSimpleResultSetWriter{
		org_id:   utils.GetOrgId(config_obj),
		log_path: log_path,
		opts:     opts,
		ctx:      context.Background(),
		sync:     utils.CompareFuncs(completion, utils.SyncCompleter),
		version:  md.ID,
	}, nil
}

func (self ResultSetFactory) NewResultSetReader(
	file_store_factory api.FileStore,
	log_path api.FSPathSpec) (result_sets.ResultSetReader, error) {

	ctx := context.Background()

	cloud_config_obj := filestore.GetConfigObj(file_store_factory)
	config_obj := cloud_config_obj.VeloConf()

	// Get the existing metadata record
	existing_md, err := GetResultSetMetadata(ctx, config_obj, log_path)
	if err != nil {
		return nil, utils.NotFoundError
	}

	return &SimpleResultSetReader{
		file_store_factory: file_store_factory,
		log_path:           log_path,
		base_record:        NewSimpleResultSetRecord(log_path, existing_md.ID),
	}, nil
}
