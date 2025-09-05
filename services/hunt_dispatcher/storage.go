package hunt_dispatcher

import (
	"context"
	"errors"
	"io"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	TAGS_ID = "index_tags"
)

func (self *HuntStorageManagerImpl) ListHunts(
	ctx context.Context,
	options result_sets.ResultSetOptions,
	offset int64, length int64) ([]*api_proto.Hunt, int64, error) {

	err := self.FlushIndex(ctx)
	if err != nil {
		return nil, 0, err
	}

	hunt_path_manager := paths.NewHuntPathManager("")
	file_store_factory := file_store.GetFileStore(self.config_obj)
	rs_reader, err := result_sets.NewResultSetReaderWithOptions(
		ctx, self.config_obj, file_store_factory,
		hunt_path_manager.HuntIndex(), options)
	if err != nil {
		return nil, 0, err
	}
	defer rs_reader.Close()

	err = rs_reader.SeekToRow(offset)
	if errors.Is(err, io.EOF) {
		return nil, 0, nil
	}

	if err != nil {
		return nil, 0, err
	}

	// Highly optimized reader for speed.
	json_chan, err := rs_reader.JSON(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := []*api_proto.Hunt{}
	for serialized := range json_chan {
		summary := &HuntIndexEntry{}
		err = json.Unmarshal(serialized, summary)
		if err != nil {
			continue
		}

		// Get the full record from memory cache
		hunt_obj := &api_proto.Hunt{}
		err = json.Unmarshal([]byte(summary.Hunt), hunt_obj)
		if err != nil {
			continue
		}

		result = append(result, hunt_obj)
		if int64(len(result)) >= length {
			break
		}
	}

	return result, rs_reader.TotalRows(), nil
}

func (self *HuntStorageManagerImpl) GetTags(
	ctx context.Context) (res []string) {

	hit, err := cvelo_services.GetElasticRecord(ctx,
		self.config_obj.OrgId, "persisted", TAGS_ID)
	if err != nil {
		return nil
	}

	record := &HuntEntry{}
	err = json.Unmarshal(hit, record)
	if err != nil {
		return nil
	}

	return record.Labels
}

func (self *HuntStorageManagerImpl) Refresh(ctx context.Context,
	config_obj *config_proto.Config) error {
	return utils.NotImplementedError
}

func (self HuntStorageManagerImpl) GetHunt(
	ctx context.Context, hunt_id string) (*api_proto.Hunt, error) {
	serialized, err := cvelo_services.GetElasticRecord(ctx,
		self.config_obj.OrgId, "persisted", hunt_id)
	if err != nil {
		return nil, utils.NotFoundError
	}

	hunt_entry := &HuntEntry{}
	err = json.Unmarshal(serialized, hunt_entry)
	if err != nil {
		return nil, utils.NotFoundError
	}

	hunt_info, err := hunt_entry.GetHunt()
	if err != nil {
		return nil, utils.NotFoundError
	}

	hunt_info.Stats.AvailableDownloads, _ = availableHuntDownloadFiles(
		self.config_obj, hunt_id)

	return hunt_info, nil
}

func (self HuntStorageManagerImpl) Close(ctx context.Context) {}

func (self *HuntStorageManagerImpl) ModifyHuntObject(ctx context.Context, hunt_id string,
	cb func(hunt *hunt_dispatcher.HuntRecord) services.HuntModificationAction,
) services.HuntModificationAction {

	// A small race but this is only used by the GUI
	hunt, err := self.GetHunt(ctx, hunt_id)
	if err != nil {
		return services.HuntUnmodified
	}

	record := &hunt_dispatcher.HuntRecord{
		Hunt: hunt,
	}
	modification := cb(record)
	if modification != services.HuntUnmodified {
		err := self.SetHunt(ctx, hunt)
		if err != nil {
			return services.HuntUnmodified
		}
	}
	return modification
}

func NewHuntStorageManagerImpl(
	ctx context.Context,
	config_obj *config_proto.Config) hunt_dispatcher.HuntStorageManager {
	result := &HuntStorageManagerImpl{
		config_obj: config_obj,
		ctx:        ctx,
	}

	return result
}
