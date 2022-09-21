package hunt_dispatcher

import (
	"context"
	"errors"
	"sync"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
)

type HuntEntry struct {
	HuntId    string `json:"hunt_id"`
	Timestamp int64  `json:"timestamp"`
	Scheduled uint64 `json:"scheduled"`
	Completed uint64 `json:"completed"`
	Errors    uint64 `json:"errors"`
	Hunt      string `json:"hunt"`
	State     string `json:"state"`
}

func (self *HuntEntry) GetHunt() (*api_proto.Hunt, error) {
	hunt_info := &api_proto.Hunt{}
	err := json.Unmarshal([]byte(self.Hunt), hunt_info)
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
	}

	return hunt_info, nil
}

type HuntDispatcher struct {
	ctx        context.Context
	config_obj *config_proto.Config
}

func (self HuntDispatcher) ApplyFuncOnHunts(cb func(hunt *api_proto.Hunt) error) error {
	return errors.New("HuntDispatcher.ApplyFuncOnHunts Not implemented")
}

func (self HuntDispatcher) GetLastTimestamp() uint64 {
	return 0
}

func (self HuntDispatcher) SetHunt(hunt *api_proto.Hunt) error {
	hunt_id := hunt.HuntId
	if hunt_id == "" {
		return errors.New("Invalid hunt")
	}

	serialized, err := json.Marshal(hunt)
	if err != nil {
		return err
	}

	record := &HuntEntry{
		HuntId: hunt_id,
		Hunt:   string(serialized),
		State:  hunt.State.String(),
	}

	if hunt.Stats != nil {
		record.Scheduled = hunt.Stats.TotalClientsScheduled
		record.Completed = hunt.Stats.TotalClientsWithResults
		record.Errors = hunt.Stats.TotalClientsWithErrors
	}

	return cvelo_services.SetElasticIndex(self.ctx,
		self.config_obj.OrgId,
		"hunts", hunt.HuntId, record)
}

func (self HuntDispatcher) GetHunt(hunt_id string) (*api_proto.Hunt, bool) {
	serialized, err := cvelo_services.GetElasticRecord(context.Background(),
		self.config_obj.OrgId, "hunts", hunt_id)
	if err != nil {
		return nil, false
	}

	hunt_entry := &HuntEntry{}
	err = json.Unmarshal(serialized, hunt_entry)
	if err != nil {
		return nil, false
	}

	hunt_info, err := hunt_entry.GetHunt()
	if err != nil {
		return nil, false
	}

	hunt_info.Stats.AvailableDownloads, _ = availableHuntDownloadFiles(
		self.config_obj, hunt_id)

	return hunt_info, true
}

func (self HuntDispatcher) MutateHunt(config_obj *config_proto.Config,
	mutation *api_proto.HuntMutation) error {
	return errors.New("HuntDispatcher.HuntMutation Not implemented")
}

func (self HuntDispatcher) Refresh(config_obj *config_proto.Config) error {
	return nil
}

func (self HuntDispatcher) Close(config_obj *config_proto.Config) {}

const getAllHuntsQuery = `
{"query": {"match_all" : {}},
 "sort": [{"hunt_id": "desc"}],
 "from": %q, "size": %q}
`

func (self HuntDispatcher) ListHunts(
	ctx context.Context, config_obj *config_proto.Config,
	in *api_proto.ListHuntsRequest) (
	*api_proto.ListHuntsResponse, error) {

	hits, err := cvelo_services.QueryElasticRaw(
		ctx, self.config_obj.OrgId,
		"hunts", json.Format(getAllHuntsQuery, in.Offset, in.Count))
	if err != nil {
		return nil, err
	}

	result := &api_proto.ListHuntsResponse{}
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
		result.Items = append(result.Items, hunt_info)
	}

	return result, nil
}

func NewHuntDispatcher(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.IHuntDispatcher, error) {
	service := &HuntDispatcher{
		ctx:        ctx,
		config_obj: config_obj,
	}

	return service, nil
}
