package simple

import (
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

// This is the record we store in the elastic datastore. Simple
// Results sets are written from collections and contain a table rows.
type SimpleResultSetRecord struct {
	ClientId  string `json:"client_id"`
	FlowId    string `json:"flow_id"`
	Artifact  string `json:"artifact"`
	Type      string `json:"type"`
	StartRow  int64  `json:"start_row"`
	EndRow    int64  `json:"end_row"`
	VFSPath   string `json:"vfs_path"`
	JSONData  string `json:"data"`
	TotalRows uint64 `json:"total_rows"`
	Timestamp int64  `json:"timestamp"`
	ID        string `json:"id"`
}

// Examine the pathspec and construct a new Elastic record. Because
// Elastic can index on multiple terms we do not need to build a
// single VFS path - we just split out the path into indexable terms
// then search them directly for a more efficient match.

// Failing this, we index on vfs path.

// This code is basically the inverse of the path manager mechanism.
func NewSimpleResultSetRecord(
	log_path api.FSPathSpec,
	id string) *SimpleResultSetRecord {
	return &SimpleResultSetRecord{
		VFSPath: log_path.AsClientPath(),
		ID:      id,
	}
}
