package simple

import (
	"context"
	"fmt"
	"time"

	"www.velocidex.com/golang/cloudvelo/filestore"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
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
	ctx := context.Background()

	md, err := openWriterMetadata(ctx, config_obj, log_path, truncate,
		newWriterMetadata(log_path, new_id))
	if err != nil {
		return nil, err
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

// The metadata record for a new version of a result set. Its byte
// position is left unknown: after an abort the new version continues
// a client's stream part way through, so it can not start at 0.
func newWriterMetadata(
	log_path api.FSPathSpec, id string) *ResultSetMetadataRecord {
	return &ResultSetMetadataRecord{
		Timestamp: utils.GetTime().Now().UnixNano(),
		VFSPath:   NewSimpleResultSetRecord(log_path, id).VFSPath,
		ID:        id,
		EndRow:    0,
		Type:      "rs_metadata",
	}
}

// Returns the metadata record a new writer continues from. When
// appending this is the existing record, otherwise new_md is stored
// and used, which starts a new version of the result set.
func openWriterMetadata(
	ctx context.Context,
	config_obj *config_proto.Config,
	log_path api.FSPathSpec,
	truncate result_sets.WriteMode,
	new_md *ResultSetMetadataRecord) (*ResultSetMetadataRecord, error) {

	if !truncate {
		existing_md, err := getResultSetMetadata(ctx, config_obj, log_path)

		if err == nil {
			// An aborted result set can never be read again, so
			// appending to it would make the new rows unreadable
			// too. Start a new version instead so the result set
			// recovers.
			if existing_md.TotalRows >= 0 {
				return existing_md, nil
			}

			// The new version has to replace the aborted record.
			if new_md.Timestamp < existing_md.Timestamp {
				new_md.Timestamp = existing_md.Timestamp
			}
		}
	}

	err := SetResultSetMetadata(ctx, config_obj, log_path, new_md)
	if err != nil {
		return nil, err
	}
	return new_md, nil
}

// Result sets live in the Elastic "transient" index, not in the
// filestore. cloudvelo's own FlowStorageManager.DeleteFlow() removes them
// with a delete-by-query, so this filestore based entry point is never
// used here.
func (self ResultSetFactory) DeleteResultSet(
	file_store_factory api.FileStore,
	path api.FSPathSpec) error {
	return utils.NotImplementedError
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
