package indexing

import (
	"context"
	"errors"
	"sync"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

type ElasticIndexRecord struct {
	Term     string `json:"term"`
	ClientId string `json:"client_id"`
}

type Indexer struct {
	config_obj *config_proto.Config
}

func (self Indexer) SetIndex(client_id, term string) error {
	return cvelo_services.SetElasticIndex(self.config_obj.OrgId,
		"index", "", &ElasticIndexRecord{
			Term:     term,
			ClientId: client_id,
		})
}

// Clear a search term on a client
func (self Indexer) UnsetIndex(client_id, term string) error {
	return errors.New("Not implemented")
}

// Search the index for clients matching the term
func (self Indexer) SearchIndexWithPrefix(
	ctx context.Context,
	config_obj *config_proto.Config,
	prefix string) <-chan *api_proto.IndexRecord {
	output_chan := make(chan *api_proto.IndexRecord)
	go func() {
		defer close(output_chan)

	}()

	return output_chan
}

func (self Indexer) SetSimpleIndex(
	config_obj *config_proto.Config,
	index_urn api.DSPathSpec,
	entity string,
	keywords []string) error {
	return errors.New("Not implemented")
}

func (self Indexer) UnsetSimpleIndex(
	config_obj *config_proto.Config,
	index_urn api.DSPathSpec,
	entity string,
	keywords []string) error {
	return errors.New("Not implemented")
}

func (self Indexer) CheckSimpleIndex(
	config_obj *config_proto.Config,
	index_urn api.DSPathSpec,
	entity string,
	keywords []string) error {
	return errors.New("Not implemented")
}

func (self Indexer) FastGetApiClient(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) (*api_proto.ApiClient, error) {

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil, err
	}

	client_info, err := client_info_manager.Get(ctx, client_id)
	if err != nil {
		return nil, err
	}

	if client_info == nil {
		return nil, errors.New("Invalid client_info")
	}

	return _makeApiClient(&client_info.ClientInfo), nil
}

func _makeApiClient(client_info *actions_proto.ClientInfo) *api_proto.ApiClient {
	fqdn := client_info.Fqdn
	if fqdn == "" {
		fqdn = client_info.Hostname
	}

	return &api_proto.ApiClient{
		ClientId: client_info.ClientId,
		Labels:   client_info.Labels,
		AgentInformation: &api_proto.AgentInformation{
			Version: client_info.ClientVersion,
			Name:    client_info.ClientName,
		},
		OsInfo: &api_proto.Uname{
			System:       client_info.System,
			Hostname:     client_info.Hostname,
			Release:      client_info.Release,
			Machine:      client_info.Architecture,
			Fqdn:         fqdn,
			MacAddresses: client_info.MacAddresses,
		},
		FirstSeenAt:                 client_info.FirstSeenAt,
		LastSeenAt:                  client_info.Ping,
		LastIp:                      client_info.IpAddress,
		LastInterrogateFlowId:       client_info.LastInterrogateFlowId,
		LastInterrogateArtifactName: client_info.LastInterrogateArtifactName,
	}
}

func NewIndexingService(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) (*Indexer, error) {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Indexing Service.")

	indexer := &Indexer{
		config_obj: config_obj,
	}
	return indexer, nil
}
