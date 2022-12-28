package ingestion

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/schema/api"
	"www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/utils"
	cvelo_utils "www.velocidex.com/golang/cloudvelo/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

// When we receive a status we need to modify the collection record.
func (self Ingestor) HandleStatus(
	ctx context.Context,
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	if message.Status == nil {
		return nil
	}

	// Just store all the status messages - we actually parse them
	// later when we read the record out to determine the final
	// collection state in schema.api.ArtifactCollectorContextToProto.
	return services.SetElasticIndex(ctx,
		config_obj.OrgId, "collections",
		message.SessionId+"_status_"+cvelo_utils.GetId(),
		api.ArtifactCollectorContext{
			ClientId:  message.Source,
			SessionId: message.SessionId,
			Timestamp: utils.Clock.Now().UnixNano(),
			QueryStats: []string{
				json.MustMarshalString(message.Status),
			}})
}
