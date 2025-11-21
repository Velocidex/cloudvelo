package simple

import (
	"context"
	"fmt"
	"time"

	"github.com/Velocidex/ordereddict"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/result_sets/simple"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/sorter"
	"www.velocidex.com/golang/vfilter"
)

func (self ResultSetFactory) NewResultSetReaderWithOptions(
	ctx context.Context,
	config_obj *config_proto.Config,
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	options result_sets.ResultSetOptions) (result_sets.ResultSetReader, error) {

	cvelo_services.Count("NewResultSetReaderWithOptions")

	base_reader, err := self.NewResultSetReader(file_store_factory, log_path)
	if err != nil {
		// The base result set does not exist. We dont care about any
		// transformations.
		return nil, err
	}

	// First do the filtering and then do the sorting.
	return self.getFilteredReader(
		ctx, config_obj, file_store_factory,
		log_path, base_reader, options)
}

func (self ResultSetFactory) getFilteredReader(
	ctx context.Context,
	config_obj *config_proto.Config,
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	base_reader result_sets.ResultSetReader,
	options result_sets.ResultSetOptions) (result_sets.ResultSetReader, error) {

	// No filter required.
	if options.FilterColumn == "" ||
		options.FilterRegex == nil {
		return self.getSortedReader(ctx, config_obj, file_store_factory,
			log_path, base_reader, options)
	}

	transformed_path := log_path
	if options.StartIdx != 0 || options.EndIdx != 0 {
		transformed_path = transformed_path.AddUnsafeChild(
			fmt.Sprintf("Range %d-%d", options.StartIdx, options.EndIdx))
	}
	transformed_path = transformed_path.AddUnsafeChild(
		"filter", options.FilterColumn, options.FilterRegex.String())

	if options.FilterExclude {
		transformed_path = transformed_path.AddChild("exclude")
	}

	// Do we have a cached transformed result set? If yes and it is
	// newer than the base result set, then just use it.
	transformed_reader, err := self.NewResultSetReader(file_store_factory, transformed_path)
	if err == nil && transformed_reader.MTime().After(base_reader.MTime()) {
		return self.getSortedReader(ctx, config_obj, file_store_factory,
			transformed_path, transformed_reader, options)
	}

	base_reader, err = simple.WrapReaderForRange(
		base_reader, options.StartIdx, options.EndIdx)
	if err != nil {
		return nil, err
	}

	// Create the new writer
	writer, err := self.NewResultSetWriter(
		file_store_factory, transformed_path, nil, utils.SyncCompleter,
		result_sets.TruncateMode)
	if err != nil {
		return nil, err
	}

	sub_ctx, sub_cancel := utils.WithTimeout(ctx, getExpiry(config_obj))
	defer sub_cancel()

	// Filter the table with the regex
	row_chan := base_reader.Rows(sub_ctx)
outer:

	for {
		select {
		case <-sub_ctx.Done():
			break outer

		case row, ok := <-row_chan:
			if !ok {
				break outer
			}
			value, pres := row.Get(options.FilterColumn)
			if pres {
				value_str := utils.ToString(value)
				matched := options.FilterRegex.FindStringIndex(value_str) != nil

				if (options.FilterExclude && !matched) ||
					(!options.FilterExclude && matched) {
					writer.Write(row)
				}
			}
		}
	}

	if utils.IsCtxDone(sub_ctx) {
		AbortResultSet(writer)
		return nil, utils.IOError
	}

	// Flush all the writes back
	writer.Close()

	// We already took care of the subrange options so clear them
	// in case the querry is also sorted.
	options.StartIdx = 0
	options.EndIdx = 0

	// Reopen the result set
	transformed_reader, err = self.NewResultSetReader(
		file_store_factory, transformed_path)
	if err != nil {
		return nil, err
	}

	return self.getSortedReader(ctx, config_obj, file_store_factory,
		transformed_path, transformed_reader, options)
}

func (self ResultSetFactory) getSortedReader(
	ctx context.Context,
	config_obj *config_proto.Config,
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	base_reader result_sets.ResultSetReader,
	options result_sets.ResultSetOptions) (result_sets.ResultSetReader, error) {

	// No sorting required.
	if options.SortColumn == "" {
		return simple.WrapReaderForRange(base_reader, options.StartIdx, options.EndIdx)
	}

	transformed_path := log_path
	if options.StartIdx != 0 || options.EndIdx != 0 {
		transformed_path = transformed_path.AddUnsafeChild(
			fmt.Sprintf("Range %d-%d", options.StartIdx, options.EndIdx))
	}

	if options.SortAsc {
		transformed_path = transformed_path.AddUnsafeChild(
			"sorted", options.SortColumn, "asc")
	} else {
		transformed_path = transformed_path.AddUnsafeChild(
			"sorted", options.SortColumn, "desc")
	}

	// Do we have a cached transformed result set? If yes and it is
	// newer than the base result set, then just use it.
	transformed_reader, err := self.NewResultSetReader(file_store_factory, transformed_path)
	if err == nil && transformed_reader.MTime().After(base_reader.MTime()) {
		return simple.WrapReaderForRange(transformed_reader, options.StartIdx, options.EndIdx)
	}

	stacker_path := transformed_path.AddChild("stack")

	// Nope - we have to build the new cache from the original table.
	scope := vql_subsystem.MakeScope()

	reader, err := simple.WrapReaderForRange(base_reader, options.StartIdx, options.EndIdx)
	if err != nil {
		return nil, err
	}

	// Create the new writer
	writer, err := self.NewResultSetWriter(
		file_store_factory, transformed_path, nil, utils.SyncCompleter,
		result_sets.TruncateMode)
	if err != nil {
		return nil, err
	}

	sub_ctx, sub_cancel := utils.WithTimeout(ctx, getExpiry(config_obj))
	defer sub_cancel()

	sorter_input_chan := make(chan vfilter.Row)

	sorted_chan, closer, err := simple.NewStacker(sub_ctx, scope,
		stacker_path,
		file_store_factory, self,
		sorter.MergeSorter{10000}.Sort(
			ctx, scope, sorter_input_chan,
			options.SortColumn, options.SortAsc),
		options.SortColumn)
	if err != nil {
		return nil, err
	}

	defer closer()

	// Now write into the sorter and read the sorted results.
	go func() {
		defer close(sorter_input_chan)

		row_chan := reader.Rows(sub_ctx)
		for {
			select {
			case <-sub_ctx.Done():
				return

			case row, ok := <-row_chan:
				if !ok {
					return
				}
				sorter_input_chan <- row
			}
		}
	}()

	for row := range sorted_chan {
		row_dict, ok := row.(*ordereddict.Dict)
		if ok {
			writer.Write(row_dict)
		}
	}

	if utils.IsCtxDone(sub_ctx) {
		AbortResultSet(writer)
		return nil, utils.IOError
	}

	// Close synchronously to flush the data
	writer.Close()

	// Reopen the result set and return it.
	result, err := self.NewResultSetReader(file_store_factory, transformed_path)
	if err != nil {
		return nil, err
	}

	result.SetStacker(stacker_path)
	return result, nil
}

func getExpiry(config_obj *config_proto.Config) time.Duration {
	// Default is 10 min to filter the file.
	if config_obj.Defaults != nil &&
		config_obj.Defaults.NotebookCellTimeoutMin > 0 {
		return time.Duration(
			config_obj.Defaults.NotebookCellTimeoutMin) * time.Minute
	}

	return 10 * time.Minute
}

// Abort writing the result set by setting the TotalRows to -1 to
// signal this result set is incomplete.
func AbortResultSet(writer result_sets.ResultSetWriter) {
	rs_writer, ok := writer.(*ElasticSimpleResultSetWriter)
	if ok {
		rs_writer.Abort()
	}
}
