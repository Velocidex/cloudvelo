package launcher

import (
	"context"
	"errors"

	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

var (
	NotFoundError = errors.New("Not found")
)

// Get all the flows for this client. TODO: implement paging
const getFlowsQuery = `
{
  "sort": [{
    "create_time": {"order": "desc"}
  }],
  "query": {
     "match": {"client_id" : %q}
  },
  "from": %q,
  "size": %q
}
`

func (self Launcher) GetFlows(
	config_obj *config_proto.Config,
	client_id string, include_archived bool,
	flow_filter func(flow *flows_proto.ArtifactCollectorContext) bool,
	offset uint64, length uint64) (*api_proto.ApiFlowResponse, error) {

	if length == 0 {
		length = 1000
	}

	hits, err := cvelo_services.QueryElasticRaw(context.Background(),
		self.config_obj.OrgId, "collections",
		json.Format(getFlowsQuery, client_id, offset, length))
	if err != nil {
		return nil, err
	}

	result := &api_proto.ApiFlowResponse{}
	for _, hit := range hits {
		item := &api.ArtifactCollectorContext{}
		err = json.Unmarshal(hit, &item)
		if err == nil {
			flow, err := api.ArtifactCollectorContextToProto(item)
			if err == nil {
				cleanUpContext(flow)
				result.Items = append(result.Items, flow)
			}
		}
	}
	return result, nil
}

const getFlowDetailsQuery = `
{
  "query": {
     "bool": {
       "must": [
         {"match": {"client_id" : %q}},
         {"match": {"session_id" : %q}}
      ]}
  }
}
`

func (self *Launcher) GetFlowDetails(
	config_obj *config_proto.Config,
	client_id string, flow_id string) (*api_proto.FlowDetails, error) {
	if flow_id == "" || client_id == "" {
		return &api_proto.FlowDetails{}, nil
	}

	hits, err := cvelo_services.QueryElasticRaw(context.Background(),
		self.config_obj.OrgId, "collections",
		json.Format(getFlowDetailsQuery, client_id, flow_id))
	if err != nil {
		return nil, err
	}

	if len(hits) == 0 {
		return nil, NotFoundError
	}

	item := &api.ArtifactCollectorContext{}
	err = json.Unmarshal(hits[0], item)
	if err != nil {
		return nil, err
	}

	flow, err := api.ArtifactCollectorContextToProto(item)
	if err != nil {
		return nil, err
	}
	cleanUpContext(flow)

	return &api_proto.FlowDetails{
		Context: flow,
		// AvailableDownloads: availableDownloads,
	}, nil
}

func cleanUpContext(item *flows_proto.ArtifactCollectorContext) {
	// Remove empty string from the ArtifactsWithResults
	results := []string{}
	for _, i := range item.ArtifactsWithResults {
		if i != "" {
			results = append(results, i)
		}
	}

	item.ArtifactsWithResults = results
}
