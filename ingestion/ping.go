package ingestion

import (
	"context"
	"strings"

	"www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

const (
	setPingRecord = `
{
    "client_id": %q,
    "ping": %q
}
`
)

func (self Ingestor) HandlePing(
	ctx context.Context,
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {
	err := services.SetElasticIndexAsync(
		config_obj.OrgId, "clients", message.Source+"_ping",
		json.Format(setPingRecord, message.Source,
			utils.Clock.Now().UnixNano()))
	if err == nil ||
		strings.Contains(err.Error(), "document_missing_exception") {
		return nil
	}
	return err
}
