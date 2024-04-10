package timed

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/filestore"
	"www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

// This is the record we store in the elastic datastore. Timed Results
// are usually written from event artifacts.
type TimedResultSetRecord struct {
	ClientId  string `json:"client_id"`
	FlowId    string `json:"flow_id"`
	Artifact  string `json:"artifact"`
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	Date      int64  `json:"date"` // Timestamp rounded down to the UTC day
	VFSPath   string `json:"vfs_path"`
	JSONData  string `json:"data"`
}

// Examine the pathspec and construct a new Elastic record.
func NewTimedResultSetRecord(
	path_manager api.PathManager) *TimedResultSetRecord {
	filename, _ := path_manager.GetPathForWriting()
	vfs_path := ""
	if filename != nil {
		vfs_path = filename.AsClientPath()
	}

	now := utils.GetTime().Now()
	day := now.Truncate(24 * time.Hour).Unix()

	switch t := path_manager.(type) {
	case *artifacts.ArtifactPathManager:
		return &TimedResultSetRecord{
			ClientId:  t.ClientId,
			FlowId:    t.FlowId,
			Artifact:  t.FullArtifactName,
			Type:      "results",
			VFSPath:   vfs_path,
			Timestamp: now.UnixNano(),
			Date:      day,
		}

	case *artifacts.ArtifactLogPathManager:
		return &TimedResultSetRecord{
			ClientId:  t.ClientId,
			FlowId:    t.FlowId,
			Artifact:  t.FullArtifactName,
			Type:      "logs",
			VFSPath:   vfs_path,
			Timestamp: utils.GetTime().Now().UnixNano(),
			Date:      day,
		}

	default:
		return &TimedResultSetRecord{
			Timestamp: utils.GetTime().Now().UnixNano(),
			Date:      day,
			VFSPath:   vfs_path,
		}
	}
}

type ElasticTimedResultSetWriter struct {
	file_store_factory api.FileStore
	path_manager       api.PathManager
	opts               *json.EncOpts
	ctx                context.Context
}

func (self ElasticTimedResultSetWriter) WriteJSONL(
	serialized []byte, total_rows int) {

	record := NewTimedResultSetRecord(self.path_manager)
	record.JSONData = string(serialized)

	services.SetElasticIndex(self.ctx,
		filestore.GetOrgId(self.file_store_factory),
		"transient", "", record)
}

func (self ElasticTimedResultSetWriter) Write(row *ordereddict.Dict) {
	serialized, err := json.MarshalWithOptions(row, self.opts)
	if err != nil {
		return
	}

	self.WriteJSONL(serialized, 1)
}

func (self ElasticTimedResultSetWriter) Flush() {

}

func (self ElasticTimedResultSetWriter) Close() {

}

func NewTimedResultSetWriter(
	file_store_factory api.FileStore,
	path_manager api.PathManager,
	opts *json.EncOpts,
	completion func()) (result_sets.TimedResultSetWriter, error) {

	return &ElasticTimedResultSetWriter{
		file_store_factory: file_store_factory,
		path_manager:       path_manager,
		opts:               opts,
		ctx:                context.Background(),
	}, nil
}
