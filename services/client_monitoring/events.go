package client_monitoring

import (
	"context"
	"strconv"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

func (self *ClientMonitoringManager) ListAvailableEventResults(
	ctx context.Context,
	in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	if in.Artifact == "" {
		return listAvailableEventArtifacts(ctx, self.config_obj, in)
	}
	return listAvailableEventTimestamps(ctx, self.config_obj, in)
}

const getAvailableArtifactsQuery = `
{
  "sort": [{
    "artifact": {"order": "asc"}
  }],
  "query": {
    "bool": {
      "must": [
        {"match": {"client_id": %q}},
        {"match": {"type": %q}},
        {"match": {"flow_id": "F.Monitoring"}}
      ]}
  },
  "size": 0,
  "aggs": {
    "genres": {
      "terms": {
        "field": "artifact"
      }
    }
  }
}
`

// List all the event artifacts available for this client.
func listAvailableEventArtifacts(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	if in.ClientId == "" {
		in.ClientId = "server"
	}

	hits, err := cvelo_services.QueryElasticAggregations(ctx,
		config_obj.OrgId, "results",
		json.Format(getAvailableArtifactsQuery, in.ClientId, "results"))
	if err != nil {
		return nil, err
	}

	result := &api_proto.ListAvailableEventResultsResponse{}
	for _, h := range hits {
		result.Logs = append(result.Logs, &api_proto.AvailableEvent{
			Artifact: h,
		})
	}

	return result, nil
}

const getAvailableEventTimesQuery = `
{
  "sort": [{
    "date": {"order": "asc"}
  }],
  "query": {
    "bool": {
      "must": [
        {"match": {"client_id": %q}},
        {"match": {"type": %q}},
        {"match": {"artifact": %q}},
        {"match": {"flow_id": "F.Monitoring"}}
      ]}
  },
  "size": 0,
  "aggs": {
    "genres": {
      "terms": {
        "field": "date"
      }
    }
  }
}
`

func listAvailableEventTimestamps(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	if in.ClientId == "" {
		in.ClientId = "server"
	}

	hits, err := cvelo_services.QueryElasticAggregations(ctx,
		config_obj.OrgId, "results",
		json.Format(getAvailableEventTimesQuery, in.ClientId,
			"results", in.Artifact))
	if err != nil {
		return nil, err
	}

	result := &api_proto.ListAvailableEventResultsResponse{
		Logs: []*api_proto.AvailableEvent{{
			Artifact: in.Artifact,
		}},
	}
	for _, h := range hits {
		ts, err := strconv.ParseInt(h, 10, 64)
		if err == nil {
			result.Logs[0].RowTimestamps = append(
				result.Logs[0].RowTimestamps, int32(ts))
		}
	}

	return result, nil
}
