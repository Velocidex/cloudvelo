package ingestion

import (
	"time"

	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

const (
	painlessScript = `
ctx._source.query_stats.add(params.status);
ctx._source.last_active_time = params.time;
if(ctx._source.start_time == 0) {
  ctx._source.start_time = params.time;
}
`
	// Just store all the status messages - we actually parse them
	// later when we read the record out to determine the final
	// collection state in schema.api.ArtifactCollectorContextToProto.
	updateQuery = `
{
    "script" : {
        "source": %q,
        "lang": "painless",
        "params": {
          "time": %q,
          "status": %q
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

	return services.UpdateIndex(
		config_obj.OrgId, "collections", message.SessionId,
		json.Format(updateQuery,
			painlessScript,
			time.Now().UnixNano()/1000,
			json.MustMarshalString(message.Status)))
}
