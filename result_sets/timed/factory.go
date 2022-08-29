package timed

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/filestore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

type TimedFactory struct{}

func (self TimedFactory) NewTimedResultSetWriter(
	file_store_factory api.FileStore,
	path_manager api.PathManager,
	opts *json.EncOpts,
	completion func()) (result_sets.TimedResultSetWriter, error) {
	return NewTimedResultSetWriter(
		file_store_factory, path_manager, opts, completion)
}

func (self TimedFactory) NewTimedResultSetWriterWithClock(
	file_store_factory api.FileStore,
	path_manager api.PathManager,
	opts *json.EncOpts,
	completion func(), clock utils.Clock) (result_sets.TimedResultSetWriter, error) {
	return NewTimedResultSetWriter(
		file_store_factory, path_manager, opts, completion)
}

func (self TimedFactory) NewTimedResultSetReader(
	ctx context.Context,
	file_store_factory api.FileStore,
	path_manager api.PathManager) (result_sets.TimedResultSetReader, error) {

	return &TimedResultSetReader{
		path_manager: path_manager,
		config_obj:   filestore.GetConfigObj(file_store_factory),
	}, nil
}
