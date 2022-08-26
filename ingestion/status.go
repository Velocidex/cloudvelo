package ingestion

import (
	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

const (
	painlessScript = `
ctx._source.total_collected_rows += params.result_rows ;
ctx._source.total_logs += params.log_rows ;
ctx._source.artifacts_with_results.addAll(params.names_with_response);
if (ctx._source.state == 1) {
   ctx._source.state = params.state
}
`
	updateQuery = `
{
    "script" : {
        "source": %q,
        "lang": "painless",
        "params": {
          "result_rows": %q,
          "log_rows": %q,
          "names_with_response": %q,
          "state": %q
       }
    }
}
`
)

// When we receive a status we need to modify the collection record.
func (self ElasticIngestor) HandleStatus(
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	if message.Status == nil {
		return nil
	}
	status := message.Status

	names_with_response := []string{}
	if len(status.NamesWithResponse) > 0 {
		names_with_response = status.NamesWithResponse
	}

	// Default state - keep on running
	state := 1

	// This is an error state - collection failed - switch collection
	// to error state.
	if status.Status != 0 {
		state = 3

		// If this is the final query then we finished.
	} else if status.QueryId == status.TotalQueries {
		state = 2
	}

	return services.UpdateIndex(
		config_obj.OrgId, "collections", message.SessionId,
		json.Format(updateQuery,
			painlessScript,
			status.ResultRows, status.LogRows,
			names_with_response, state))
}
