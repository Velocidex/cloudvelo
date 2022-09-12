package simple

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/filestore"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/result_sets"
)

type ResultSetFactory struct{}

func (self ResultSetFactory) NewResultSetWriter(
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	opts *json.EncOpts,
	completion func(),
	truncate result_sets.WriteMode) (result_sets.ResultSetWriter, error) {

	org_id := filestore.GetOrgId(file_store_factory)

	if truncate {
		base_record := NewSimpleResultSetRecord(log_path)
		if base_record.VFSPath != "" {
			err := cvelo_services.DeleteByQuery(context.Background(), org_id,
				"results", json.Format(`
{"query": {"bool": {"must": [
  {"match": {"vfs_path": %q}}
]}}}`, base_record.VFSPath))
			if err != nil {
				return nil, err
			}
		}
	}

	return &ElasticSimpleResultSetWriter{
		org_id:   org_id,
		log_path: log_path,
		opts:     opts,
		ctx:      context.Background(),
	}, nil
}

func (self ResultSetFactory) NewResultSetReader(
	file_store_factory api.FileStore,
	log_path api.FSPathSpec) (result_sets.ResultSetReader, error) {

	return &SimpleResultSetReader{
		file_store_factory: file_store_factory,
		log_path:           log_path,
		base_record:        NewSimpleResultSetRecord(log_path),
	}, nil
}
