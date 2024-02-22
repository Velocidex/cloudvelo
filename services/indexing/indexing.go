package indexing

import (
	"context"
	"errors"
	"strings"
	"sync"

	cvelo_api "www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

type ElasticIndexRecord struct {
	Term     string `json:"term"`
	ClientId string `json:"client_id"`
	DocType  string `json:"doc_type"`
}

type Indexer struct {
	config_obj *config_proto.Config
	ctx        context.Context
}

func (self Indexer) SetIndex(client_id, term string) error {
	return cvelo_services.SetElasticIndex(self.ctx,
		self.config_obj.OrgId,
		"persisted", "", &ElasticIndexRecord{
			Term:     term,
			ClientId: client_id,
			DocType:  "index",
		})
}

// Clear a search term on a client
func (self Indexer) UnsetIndex(client_id, term string) error {
	return errors.New("Indexer.UnsetIndex Not implemented")
}

func (self Indexer) getIndexRecords(
	ctx context.Context,
	config_obj *config_proto.Config,
	query string, output_chan chan *api_proto.IndexRecord) {
	hits, err := cvelo_services.QueryChan(ctx, config_obj, 1000,
		config_obj.OrgId, "persisted", query, cvelo_services.NoSortField)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("getIndexRecords: %v", err)
		return
	}

	for hit := range hits {
		record := &api_proto.ClientMetadata{}
		err = json.Unmarshal(hit, record)
		if err == nil {
			select {
			case <-ctx.Done():
				return
			case output_chan <- &api_proto.IndexRecord{Entity: record.ClientId}:
			}
		}
	}
}

// Search the index for clients matching the term
func (self Indexer) SearchIndexWithPrefix(
	ctx context.Context,
	config_obj *config_proto.Config,
	prefix string) <-chan *api_proto.IndexRecord {
	output_chan := make(chan *api_proto.IndexRecord)

	go func() {
		defer close(output_chan)

		operator, term := splitIntoOperatorAndTerms(prefix)
		switch operator {
		case "all":
			query := `
{
	"query": {"bool": {
		"must": [
				%s,
					{
                    "match": {
                        "doc_type": "index"
                    }
                	}
				]
			}},
	"_source": {"includes": ["client_id"]}
}`
			self.getIndexRecords(ctx, config_obj, query, output_chan)
			return

		case "label":
			terms := []string{json.Format(fieldSearchQuery, "labels", term)}
			query := json.Format(
				getAllClientsQuery, strings.Join(terms, ","),
				`,{"_source":{"includes":["client_id"]}}`)
			self.getIndexRecords(ctx, config_obj, query, output_chan)
			return

		default:
		}
	}()
	return output_chan
}

func (self Indexer) SetSimpleIndex(
	config_obj *config_proto.Config,
	index_urn api.DSPathSpec,
	entity string,
	keywords []string) error {
	return errors.New("Indexer.SetSimpleIndex Not implemented")
}

func (self Indexer) UnsetSimpleIndex(
	config_obj *config_proto.Config,
	index_urn api.DSPathSpec,
	entity string,
	keywords []string) error {
	return errors.New("Indexer.UnsetSimpleIndex Not implemented")
}

func (self Indexer) CheckSimpleIndex(
	config_obj *config_proto.Config,
	index_urn api.DSPathSpec,
	entity string,
	keywords []string) error {
	return errors.New("Indexer.CheckSimpleIndex Not implemented")
}

func (self Indexer) FastGetApiClient(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) (*api_proto.ApiClient, error) {

	records, err := cvelo_api.GetMultipleClients(
		ctx, config_obj, []string{client_id})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, services.NotFoundError
	}

	return _makeApiClient(records[0]), nil
}

func _makeApiClient(client_info *cvelo_api.ClientRecord) *api_proto.ApiClient {
	fqdn := client_info.Hostname
	return &api_proto.ApiClient{
		ClientId: client_info.ClientId,
		Labels:   client_info.Labels,
		AgentInformation: &api_proto.AgentInformation{
			BuildTime: client_info.BuildTime,
			Version:   client_info.ClientVersion,
			Name:      "Velociraptor",
		},
		OsInfo: &api_proto.Uname{
			System:       client_info.System,
			Hostname:     client_info.Hostname,
			Release:      client_info.Release,
			Machine:      client_info.Architecture,
			Fqdn:         fqdn,
			MacAddresses: client_info.MacAddresses,
		},
		FirstSeenAt:           client_info.FirstSeenAt / 1000,
		LastSeenAt:            client_info.Ping / 1000,
		LastHuntTimestamp:     client_info.LastHuntTimestamp,
		LastEventTableVersion: client_info.LastEventTableVersion,
		LastInterrogateFlowId: client_info.LastInterrogate,
	}
}

func NewIndexingService(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) (*Indexer, error) {
	indexer := &Indexer{
		config_obj: config_obj,
		ctx:        ctx,
	}
	return indexer, nil
}
