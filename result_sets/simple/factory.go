package simple

import (
	"context"
	"fmt"
	"time"

	"www.velocidex.com/golang/cloudvelo/filestore"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

/*
  Since we write result sets on the transient index we can not
  actually delete anything.

  Therefore we end up writing a new ID to indicate a new result set
  and we write a metadata record to point the result set at the latest
  ID.
*/

type ResultSetFactory struct{}

func (self ResultSetFactory) NewResultSetWriter(
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	opts *json.EncOpts,
	completion func(),
	truncate result_sets.WriteMode) (result_sets.ResultSetWriter, error) {

	cvelo_services.Count("NewResultSetWriter")

	cloud_config_obj := filestore.GetConfigObj(file_store_factory)
	rows_per_result_set := cloud_config_obj.Cloud.RowsPerResultSet
	if rows_per_result_set == 0 {
		rows_per_result_set = 1000
	}

	max_size_per_packet := cloud_config_obj.Cloud.MaxSizePerPacket
	if max_size_per_packet == 0 {
		max_size_per_packet = 1024 * 1024
	}

	config_obj := cloud_config_obj.VeloConf()

	new_id := fmt.Sprintf("%v", utils.GetGUID())
	base_record := NewSimpleResultSetRecord(log_path, new_id)
	ctx := context.Background()

	var existing bool
	md := &ResultSetMetadataRecord{
		Timestamp: utils.GetTime().Now().UnixNano(),
		VFSPath:   base_record.VFSPath,
		ID:        new_id,
		EndRow:    0,
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
		org_id:              utils.GetOrgId(config_obj),
		config_obj:          config_obj,
		log_path:            log_path,
		opts:                opts,
		ctx:                 context.Background(),
		sync:                utils.CompareFuncs(completion, utils.SyncCompleter),
		version:             md.ID,
		md:                  md,
		start_row:           md.EndRow,
		rows_per_result_set: rows_per_result_set,
		max_size_per_packet: max_size_per_packet,
	}, nil
}

func (self ResultSetFactory) NewResultSetReader(
	file_store_factory api.FileStore,
	log_path api.FSPathSpec) (result_sets.ResultSetReader, error) {

	cvelo_services.Count("NewResultSetReader")

	ctx := context.Background()

	cloud_config_obj := filestore.GetConfigObj(file_store_factory)
	config_obj := cloud_config_obj.VeloConf()

	// Get the existing metadata record
	existing_md, err := GetResultSetMetadata(ctx, config_obj, log_path)
	if err != nil {
		return nil, utils.NotFoundError
	}

	// This signifies that the result set is incomplete - we can not
	// open it. It happens when we abort the writing of the result set
	// prematurely.
	if existing_md.TotalRows < 0 {
		return nil, utils.NotFoundError
	}

	base_record := NewSimpleResultSetRecord(log_path, existing_md.ID)

	// Backwards compatibility - should not be needed with newer results
	if existing_md.EndRow == 0 {
		org_id := filestore.GetOrgId(file_store_factory)
		last_rec, err := getLastRecord(ctx, org_id, base_record)
		if err == nil {
			existing_md.EndRow = last_rec.EndRow
		}
	}

	return &SimpleResultSetReader{
		file_store_factory: file_store_factory,
		log_path:           log_path,
		mtime:              time.Unix(0, existing_md.Timestamp),
		base_record:        base_record,
		md:                 existing_md,
	}, nil
}
