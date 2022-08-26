/*

The ingestor receives VeloMessages from the client and inserts them
into the elastic backend using the correct schema so they may easily
be viewed by the GUI.

*/

package ingestion

import (
	"context"

	"github.com/opensearch-project/opensearch-go"
	"www.velocidex.com/golang/cloudvelo/crypto/server"
	"www.velocidex.com/golang/cloudvelo/elastic_datastore"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
)

// Responsible for inserting VeloMessage objects into elastic.
type ElasticIngestor struct {
	client *opensearch.Client

	crypto_manager *server.ServerCryptoManager

	index string
}

func (self ElasticIngestor) Process(
	ctx context.Context, message *crypto_proto.VeloMessage) error {
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	config_obj, err := org_manager.GetOrgConfig(message.OrgId)
	if err != nil {
		return err
	}

	// Only accept unauthenticated enrolment requests. Everything
	// below is authenticated.
	if message.AuthState == crypto_proto.VeloMessage_UNAUTHENTICATED {
		return self.HandleEnrolment(config_obj, message)
	}

	// Handle the monitoring data - write to timed result set.
	if message.SessionId == constants.MONITORING_WELL_KNOWN_FLOW {
		if message.LogMessage != nil {
			return self.HandleMonitoringLogs(config_obj, message)
		}

		if message.VQLResponse != nil {
			return self.HandleMonitoringResponses(ctx, config_obj, message)
		}

		return nil
	}

	err = self.maybeHandleHuntResponse(config_obj, message)
	if err != nil {
		return err
	}

	// Handle regular collections - use simple result sets to store
	// them.
	if message.LogMessage != nil {
		return self.HandleLogs(config_obj, message)
	}

	if message.VQLResponse != nil {
		return self.HandleResponses(config_obj, message)
	}

	if message.Status != nil {
		return self.HandleStatus(config_obj, message)
	}

	if message.ForemanCheckin != nil {
		return self.HandlePing(config_obj, message)
	}

	if message.FileBuffer != nil {
		return self.HandleUploads(config_obj, message)
	}

	json.Dump(message)

	return nil
}

func NewElasticIngestor(
	config_obj *config_proto.Config,
	config *elastic_datastore.ElasticConfiguration,
	crypto_manager *server.ServerCryptoManager) (*ElasticIngestor, error) {

	client, err := cvelo_services.GetElasticClient()
	if err != nil {
		return nil, err
	}

	// TODO: Not used any more
	index := config.Index
	if index == "" {
		index = "velociraptor"
	}

	return &ElasticIngestor{
		client:         client,
		crypto_manager: crypto_manager,
		index:          index,
	}, nil
}
