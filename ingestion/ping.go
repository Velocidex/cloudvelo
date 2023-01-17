package ingestion

import (
	"context"
	"strings"

	"www.velocidex.com/golang/cloudvelo/schema/api"
	"www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

func (self Ingestor) HandlePing(
	ctx context.Context,
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	err := services.SetElasticIndex(ctx,
		config_obj.OrgId, "clients", message.Source+"_ping",
		&api.ClientInfo{
			ClientId: message.Source,
			Type:     "ping",
			Ping:     uint64(utils.Clock.Now().UnixNano()),
		})
	if err == nil ||
		strings.Contains(err.Error(), "document_missing_exception") {
		return nil
	}
	return err
}
