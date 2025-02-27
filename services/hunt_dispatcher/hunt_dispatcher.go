package hunt_dispatcher

import (
	"context"
	"errors"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/utils"
)

type HuntEntry struct {
	HuntId    string `json:"hunt_id"`
	Timestamp int64  `json:"timestamp"`
	Expires   uint64 `json:"expires"`
	Scheduled uint64 `json:"scheduled"`
	Completed uint64 `json:"completed"`
	Errors    uint64 `json:"errors"`
	Hunt      string `json:"hunt"`
	State     string `json:"state"`
	DocType   string `json:"doc_type"`
}

func (self *HuntEntry) GetHunt() (*api_proto.Hunt, error) {

	// Protobufs must only be marshalled/unmarshalled using protojson
	// because they are not compatible with the standard json package.
	hunt_info := &api_proto.Hunt{}
	err := protojson.Unmarshal([]byte(self.Hunt), hunt_info)
	if err != nil {
		return nil, err
	}

	hunt_info.Stats = &api_proto.HuntStats{
		TotalClientsScheduled:   self.Scheduled,
		TotalClientsWithResults: self.Completed,
		TotalClientsWithErrors:  self.Errors,
	}
	switch self.State {
	case "PAUSED":
		hunt_info.State = api_proto.Hunt_PAUSED

	case "RUNNING":
		hunt_info.State = api_proto.Hunt_RUNNING

	case "STOPPED":
		hunt_info.State = api_proto.Hunt_STOPPED

	case "ARCHIVED":
		hunt_info.State = api_proto.Hunt_ARCHIVED
	}

	return hunt_info, nil
}

type HuntStorageManagerImpl struct {
	ctx        context.Context
	config_obj *config_proto.Config
}

func (self HuntStorageManagerImpl) ApplyFuncOnHunts(
	ctx context.Context,
	options services.HuntSearchOptions,
	cb func(hunt *api_proto.Hunt) error) error {

	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var query string
	switch options {
	case services.AllHunts:
		query = getAllHunts
	case services.OnlyRunningHunts:
		query = getAllActiveHunts
	default:
		return errors.New("HuntSearchOptions not supported")
	}

	out, err := cvelo_services.QueryChan(
		sub_ctx, self.config_obj, 1000, self.config_obj.OrgId,
		"persisted", query, "hunt_id")
	if err != nil {
		return err
	}

	for hit := range out {
		entry := &HuntEntry{}
		err := json.Unmarshal(hit, entry)
		if err != nil {
			return err
		}

		hunt_obj, err := entry.GetHunt()
		if err != nil {
			continue
		}

		// Allow the cb to cancel the query.
		err = cb(hunt_obj)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self HuntStorageManagerImpl) GetLastTimestamp() uint64 {
	return 0
}

func (self HuntStorageManagerImpl) SetHunt(
	ctx context.Context, hunt *api_proto.Hunt) error {
	hunt_id := hunt.HuntId
	if hunt_id == "" {
		return errors.New("Invalid hunt")
	}

	serialized, err := protojson.Marshal(hunt)
	if err != nil {
		return err
	}

	record := &HuntEntry{
		HuntId:    hunt_id,
		Timestamp: int64(hunt.CreateTime / 1000000),
		Expires:   hunt.Expires,
		Hunt:      string(serialized),
		State:     hunt.State.String(),
		DocType:   "hunts",
	}

	if hunt.Stats != nil {
		record.Scheduled = hunt.Stats.TotalClientsScheduled
		record.Completed = hunt.Stats.TotalClientsWithResults
		record.Errors = hunt.Stats.TotalClientsWithErrors
	}

	return cvelo_services.SetElasticIndex(self.ctx,
		self.config_obj.OrgId,
		"persisted", hunt.HuntId,
		record)
}

func (self HuntStorageManagerImpl) GetHunt(
	ctx context.Context, hunt_id string) (*api_proto.Hunt, error) {
	serialized, err := cvelo_services.GetElasticRecord(context.Background(),
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

// TODO add sort and from/size clause
const (
	getAllHuntsQuery = `
{
    "query": {
      "bool": {
        "must": [{"match": {
                    "doc_type": "hunts"
                 }}]
      }
    },"sort": [{
    "hunt_id": {"order": "desc", "unmapped_type": "keyword"}
}],
 "from": %q, "size": %q
}
`
	getAllActiveHunts = `
{
    "query": {
        "bool": {
            "must": [
                {
                    "match": {
                        "doc_type": "hunts"
                    }
                },
                {
                    "match": {
                        "state": "RUNNING"
                    }
                }
            ]
        }
    }
}
`
	getAllHunts = `
{
    "query": {
        "bool": {
            "must": [
                {
                    "match": {
                        "doc_type": "hunts"
                    }
                }
            ]
        }
    }
}
`
)

// TODO
func (self *HuntStorageManagerImpl) ListHunts(
	ctx context.Context,
	options result_sets.ResultSetOptions,
	start_row, length int64) ([]*api_proto.Hunt, int64, error) {

	hits, _, err := cvelo_services.QueryElasticRaw(
		ctx, self.config_obj.OrgId,
		"persisted", json.Format(getAllHuntsQuery, start_row, length))
	if err != nil {
		return nil, 0, err
	}

	var result []*api_proto.Hunt
	for _, hit := range hits {
		entry := &HuntEntry{}
		err = json.Unmarshal(hit, entry)
		if err != nil {
			continue
		}

		hunt_info, err := entry.GetHunt()
		if err != nil {
			continue
		}

		result = append(result, hunt_info)
	}

	return result, int64(len(result)), nil
}

type HuntDispatcher struct {
	*hunt_dispatcher.HuntDispatcher
	config_obj *config_proto.Config
}

func NewHuntDispatcher(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.IHuntDispatcher, error) {

	base_dispatcher := hunt_dispatcher.MakeHuntDispatcher(config_obj)
	service := &HuntDispatcher{
		config_obj:     config_obj,
		HuntDispatcher: base_dispatcher,
	}
	service.HuntDispatcher.Store = NewHuntStorageManagerImpl(ctx, config_obj)

	return service, nil
}
