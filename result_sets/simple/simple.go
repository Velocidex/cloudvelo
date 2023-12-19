package simple

import (
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
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
}

// Examine the pathspec and construct a new Elastic record. Because
// Elastic can index on multiple terms we do not need to build a
// single VFS path - we just split out the path into indexable terms
// then search them directly for a more efficient match.

// Failing this, we index on vfs path.

// This code is basically the inverse of the path manager mechanism.
func NewSimpleResultSetRecord(
	log_path api.FSPathSpec) *SimpleResultSetRecord {
	components := log_path.Components()

	// Flow collection example /clients/C.929a6a92471631e5/artifacts/Windows.System.Services/F.C9A32IVCPINMG.json
	if false && components[0] == "clients" {
		client_id := components[1]
		if components[2] == "artifacts" {
			// Single artifact no source
			if len(components) == 5 {
				return &SimpleResultSetRecord{
					Timestamp: utils.GetTime().Now().Unix(),
					ClientId:  client_id,
					FlowId:    components[4],
					Artifact:  components[3],
					Type:      "results",
				}

				// Artifact with source name
			} else if len(components) == 6 {
				return &SimpleResultSetRecord{
					Timestamp: utils.GetTime().Now().Unix(),
					ClientId:  client_id,
					FlowId:    components[4],
					Artifact:  components[3] + "/" + components[5],
					Type:      "results",
				}
			}
			// Collection logs
		} else if components[2] == "collections" {
			if components[4] == "logs" {
				return &SimpleResultSetRecord{
					Timestamp: utils.GetTime().Now().Unix(),
					ClientId:  client_id,
					FlowId:    components[3],
					Type:      "logs",
				}
			}

			if components[4] == "uploads" {
				return &SimpleResultSetRecord{
					Timestamp: utils.GetTime().Now().Unix(),
					ClientId:  client_id,
					FlowId:    components[3],
					Type:      "uploads",
				}
			}
		}
	}

	return &SimpleResultSetRecord{
		VFSPath: log_path.AsClientPath(),
	}
}
