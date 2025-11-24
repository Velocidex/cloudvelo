package simple

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/services"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	// Get the latest metadata record with the most recent timestamp.
	result_md_query = `
{
 "query": {"bool": {"must": [
   {"match": {"type": "rs_metadata"}},
   {"match": {"vfs_path": %q}}
 ]}},
 "sort": {"timestamp": "desc"},
 "size": 1
}
`
)

type ResultSetMetadataRecord struct {
	Timestamp int64  `json:"timestamp"`
	VFSPath   string `json:"vfs_path"`
	ID        string `json:"id"`
	EndRow    int64  `json:"end_row"`
	TotalRows int64  `json:"total_rows"`
	Type      string `json:"type"`
}

// Because we can not delete result sets in the transient index we
// need to attach a version to each result set. When we need to
// truncate the result set we just increase the version. This makes
// opening and closing result sets a bit slower as it adds one round
// trip.
func GetResultSetMetadata(
	ctx context.Context,
	config_obj *config_proto.Config,
	log_path api.FSPathSpec) (*ResultSetMetadataRecord, error) {

	cvelo_services.Count("GetResultSetMetadata")
	cvelo_services.Debug(
		cvelo_services.DEBUG_RESULT_SET,
		"GetResultSetMetadata: %v", log_path.AsClientPath())()

	base_record := NewSimpleResultSetRecord(log_path, "")

	query := json.Format(result_md_query, base_record.VFSPath)

	hits, _, err := cvelo_services.QueryElasticRaw(ctx, utils.GetOrgId(config_obj),
		"transient", query)
	if err != nil {
		return nil, err
	}

	// If there is no metadata record we just create one with an empty
	// ID. This should be used to support legacy result sets without a
	// metadata record.
	if len(hits) == 0 {
		return &ResultSetMetadataRecord{
			Timestamp: 0,
			VFSPath:   base_record.VFSPath,
			ID:        "",
			Type:      "rs_metadata",
		}, nil
	}

	record := &ResultSetMetadataRecord{}
	err = json.Unmarshal(hits[0], &record)
	if err != nil {
		return nil, err
	}

	return record, nil
}

func SetResultSetMetadata(
	ctx context.Context,
	config_obj *config_proto.Config,
	log_path api.FSPathSpec, md *ResultSetMetadataRecord) error {

	md.Timestamp = utils.GetTime().Now().UnixNano()
	return cvelo_services.SetElasticIndex(ctx, utils.GetOrgId(config_obj),
		"transient", services.DocIdRandom, md)
}
