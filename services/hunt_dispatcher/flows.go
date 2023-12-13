package hunt_dispatcher

import (
	"context"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/vfilter"
)

const (
	getHuntsFlowsQuery = `{ "from": %q,
  "query": {
    "bool": {
      "must": [
         {"match": {"hunt_id" : %q}},
         {"match": {"doc_type" : "hunt_flow"}}
      ]}
  },
  "size": 10000
}
`
)

type HuntFlowEntry struct {
	HuntId    string `json:"hunt_id"`
	Timestamp int64  `json:"timestamp"`
	ClientId  string `json:"client_id"`
	FlowId    string `json:"flow_id"`
	Status    string `json:"status"`
	Type      string `json:"type"`
	DocType   string `json:"doc_type"`
}

func (self HuntDispatcher) GetFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	hunt_id string, start int) chan *api_proto.FlowDetails {
	output_chan := make(chan *api_proto.FlowDetails)

	go func() {
		defer close(output_chan)

		laucher_manager, err := services.GetLauncher(config_obj)
		if err != nil {
			return
		}

		query := json.Format(getHuntsFlowsQuery, start, hunt_id)
		hits, err := cvelo_services.QueryChan(
			ctx, config_obj, 1000, self.config_obj.OrgId, "results", query, "timestamp")
		if err != nil {
			scope.Log("GetFlows for hunt %v: %v", hunt_id, err)
			return
		}

		for hit := range hits {
			entry := &HuntFlowEntry{}
			err = json.Unmarshal(hit, entry)
			if err != nil {
				continue
			}
			flow_details, err := laucher_manager.GetFlowDetails(
				ctx, config_obj, entry.ClientId, entry.FlowId)
			if err != nil {
				continue
			}
			output_chan <- flow_details
		}

	}()
	return output_chan
}
