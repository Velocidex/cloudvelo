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

const (
	getAvailableArtifactsQuery = `
{
  "sort": [{
    "artifact": {"order": "asc", "unmapped_type" : "keyword"}
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
	getAvailableServerArtifactsQuery = `
{
  "sort": [{
    "artifact": {"order": "asc", "unmapped_type" : "keyword"}
  }],
  "query": {
    "bool": {
      "must": [
        {"match": {"client_id": %q}},
        {"match": {"type": %q}}
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
)

// List all the event artifacts available for this client.
func listAvailableEventArtifacts(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	var query string
	if in.ClientId == "" || in.ClientId == "server" {
		// Get server artifacts. Although we dont have a server
		// artifacts runner, it is still possible for server artifacts
		// to be written by various services (e.g. Audit manager).
		query = json.Format(getAvailableServerArtifactsQuery,
			"server", "results")

	} else {
		// Even if client events are not generated there are always
		// some query logs sent so we can aggregate by unique log
		// messages.
		query = json.Format(getAvailableArtifactsQuery, in.ClientId, "logs")
	}

	hits, err := cvelo_services.QueryElasticAggregations(ctx,
		config_obj.OrgId, "results",
		query)
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
    "date": {"order": "asc", "unmapped_type" : "long"}
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

const getAvailableServerEventTimesQuery = `
{
  "sort": [{
    "date": {"order": "asc", "unmapped_type" : "long"}
  }],
  "query": {
    "bool": {
      "must": [
        {"match": {"client_id": "server"}},
        {"match": {"type": "results"}},
        {"match": {"artifact": %q}}
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

	var query string
	if in.ClientId == "" || in.ClientId == "server" {
		query = json.Format(getAvailableServerEventTimesQuery, in.Artifact)

	} else {
		query = json.Format(getAvailableEventTimesQuery, in.ClientId,
			"results", in.Artifact)
	}

	hits, err := cvelo_services.QueryElasticAggregations(ctx,
		config_obj.OrgId, "results", query)
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
