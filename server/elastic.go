package server

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/crypto/server"
	"www.velocidex.com/golang/cloudvelo/elastic_datastore"
	"www.velocidex.com/golang/cloudvelo/ingestion"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// Test with an elastic backend
type ElasticBackend struct {
	ingestor *ingestion.ElasticIngestor
}

func NewElasticBackend(
	config_obj *config_proto.Config,
	crypto_manager *server.ServerCryptoManager,
	config *elastic_datastore.ElasticConfiguration) (
	*ElasticBackend, error) {
	ingestor, err := ingestion.NewElasticIngestor(
		config_obj, config, crypto_manager)
	if err != nil {
		return nil, err
	}

	return &ElasticBackend{ingestor: ingestor}, nil
}

// For accepting messages FROM client to SERVER
func (self ElasticBackend) Send(
	ctx context.Context, messages []*crypto_proto.VeloMessage) error {
	for _, msg := range messages {
		err := self.ingestor.Process(ctx, msg)
		if err != nil {
			return err
		}
	}
	return nil
}

// For accepting messages FROM server to CLIENT
func (self ElasticBackend) Receive(
	ctx context.Context, client_id string, org_id string) (
	message []*crypto_proto.VeloMessage, err error) {

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	org_config_obj, err := org_manager.GetOrgConfig(org_id)
	if err != nil {
		return nil, err
	}

	client_info_manager, err := services.GetClientInfoManager(org_config_obj)
	if err != nil {
		return nil, err
	}
	return client_info_manager.GetClientTasks(ctx, client_id)
}
