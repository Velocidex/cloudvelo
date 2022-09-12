package ingestion

import (
	"context"
	"strings"
	"time"

	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

const (
	updatePingQueryScript = `
ctx._source.ping = params.now;
if (ctx._source.first_seen_at == 0 ) {
   ctx._source.first_seen_at = params.now;
}
`
	updatePingQuery = `
{
    "script" : {
        "source": %q,
        "lang": "painless",
        "params": {
          "now": %q
       }
    }
}
`
)

func (self ElasticIngestor) HandlePing(
	ctx context.Context,
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {
	err := services.UpdateIndex(ctx,
		config_obj.OrgId, "clients", message.Source,
		json.Format(updatePingQuery,
			updatePingQueryScript, time.Now().Unix()))
	if err == nil ||
		strings.Contains(err.Error(), "document_missing_exception") {
		return nil
	}
	return err
}
