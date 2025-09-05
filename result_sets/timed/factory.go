package timed

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

type TimedFactory struct{}

func (self TimedFactory) NewTimedResultSetWriter(
	config_obj *config_proto.Config,
	path_manager api.PathManager,
	opts *json.EncOpts,
	completion func()) (result_sets.TimedResultSetWriter, error) {
	return NewTimedResultSetWriter(
		config_obj, path_manager, opts, completion)
}

func (self TimedFactory) NewTimedResultSetWriterWithClock(
	config_obj *config_proto.Config,
	path_manager api.PathManager,
	opts *json.EncOpts,
	completion func(), clock utils.Clock) (result_sets.TimedResultSetWriter, error) {
	return NewTimedResultSetWriter(
		config_obj, path_manager, opts, completion)
}

func (self TimedFactory) NewTimedResultSetReader(
	ctx context.Context,
	config_obj *config_proto.Config,
	path_manager api.PathManager) (result_sets.TimedResultSetReader, error) {

	return &TimedResultSetReader{
		path_manager: path_manager,
		config_obj:   config_obj,
	}, nil
}
