package ingestion

import (
	"context"
	"strings"

	"www.velocidex.com/golang/cloudvelo/schema/api"
	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self Ingestor) HandlePing(
	ctx context.Context,
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	err := services.SetElasticIndex(ctx,
		config_obj.OrgId,
		"persisted", message.Source+"_ping",
		&api.ClientRecord{
			ClientId:  message.Source,
			Type:      "ping",
			Ping:      uint64(utils.GetTime().Now().UnixNano()),
			DocType:   "clients",
			Timestamp: uint64(utils.GetTime().Now().Unix()),
		})
	if err == nil ||
		strings.Contains(err.Error(), "document_missing_exception") {
		return nil
	}
	return err
}
