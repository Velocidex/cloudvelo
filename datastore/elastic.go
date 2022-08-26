/*
  Velociraptor has an abstract data store interface. The datastore is
  used to store simple records atomically.

  The file based Datastore uses the file path to combine a number of
  different entities into a path. Accessing the data means an exact
  match on each member of the path.

  In the elastic based datastore we match multiple indexes exactly to
  access the record. Therefore we need to map from the DSPathSpec to
  an elastic base record.
*/

package datastore

import (
	"errors"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	InvalidPath = errors.New("InvalidPath")
)

type DatastoreRecord struct {
	ClientId string `json:"client_id"`
	VFSPath  string `json:"vfs_path"`
	FlowId   string `json:"flow_id"`
	Artifact string `json:"artifact"`
	Type     string `json:"type"`
	JSONData string `json:"data"`
}

func DSPathSpecToRecord(path api.DSPathSpec) (*DatastoreRecord, error) {
	// Use the tag to figure out what type of path this is. This
	// allows us to build the correct record.
	switch path.Tag() {

	// A VFS path represents a VFSListDirectory response.
	case "VFS":
		components := path.Components()
		if len(components) < 4 {
			return nil, InvalidPath
		}

		return &DatastoreRecord{
			ClientId: components[1],
			Type:     "VFS",
			VFSPath:  utils.JoinComponents(components[3:], "/"),
		}, nil

	default:
		return nil, InvalidPath
	}
}
