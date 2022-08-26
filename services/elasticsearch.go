package services

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/aws/aws-sdk-go/aws/session"
	opensearch "github.com/opensearch-project/opensearch-go"
	opensearchapi "github.com/opensearch-project/opensearch-go/opensearchapi"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
	requestsigner "github.com/opensearch-project/opensearch-go/signer/aws"

	"www.velocidex.com/golang/cloudvelo/elastic_datastore"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	AsyncDelete = false
	SyncDelete  = true
)

var (
	mu             sync.Mutex
	gElasticClient *opensearch.Client
	TRUE           = true
	True           = "true"

	logger *logging.LogContext

	bulk_indexer opensearchutil.BulkIndexer
)

func Debug(format string, args ...interface{}) {
	if logger != nil {
		logger.Debug(format, args...)
	}
}

func GetIndex(org_id, index string) string {
	if org_id == "root" {
		org_id = ""
	}

	if org_id == "" {
		return index
	}
	return fmt.Sprintf(
		"%s_%s", strings.ToLower(org_id), index)
}

func DeleteDocument(org_id, index string, id string, sync bool) error {
	Debug("DeleteDocument %v\n", id)
	ctx := context.Background()
	client, err := GetElasticClient()
	if err != nil {
		return err
	}

	res, err := opensearchapi.DeleteRequest{
		Index:      GetIndex(org_id, index),
		DocumentID: id,
	}.Do(ctx, client)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if sync {
		res, err = opensearchapi.IndicesRefreshRequest{
			Index: []string{GetIndex(org_id, index)},
		}.Do(ctx, client)
		defer res.Body.Close()
	}

	return err
}

// Should be called to force the index to synchronize.
func FlushIndex(org_id, index string) error {
	ctx := context.Background()
	client, err := GetElasticClient()
	if err != nil {
		return err
	}

	res, err := opensearchapi.IndicesRefreshRequest{
		Index: []string{GetIndex(org_id, index)},
	}.Do(ctx, client)

	defer res.Body.Close()

	return err
}

func UpdateIndex(org_id, index, id string, query string) error {
	Debug("UpdateIndex %v %v\n", index, id)
	return retry(func() error {
		return _UpdateIndex(org_id, index, id, query)
	})
}

func _UpdateIndex(org_id, index, id string, query string) error {
	client, err := GetElasticClient()
	if err != nil {
		return err
	}

	es_req := opensearchapi.UpdateRequest{
		Index:      GetIndex(org_id, index),
		DocumentID: id,
		Body:       strings.NewReader(query),
		Refresh:    "true",
	}

	res, err := es_req.Do(context.Background(), client)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	// All is well we dont need to parse the results
	if !res.IsError() {
		return nil
	}

	response := ordereddict.NewDict()
	err = response.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	return makeElasticError(response)
}

func SetElasticIndexAsync(org_id, index, id string, record interface{}) error {
	Debug("SetElasticIndexAsync %v %v\n", index, id)
	mu.Lock()
	l_bulk_indexer := bulk_indexer
	mu.Unlock()

	serialized := json.MustMarshalString(record)

	return l_bulk_indexer.Add(context.Background(),
		opensearchutil.BulkIndexerItem{
			Index:      GetIndex(org_id, index),
			Action:     "index",
			DocumentID: id,
			Body:       strings.NewReader(serialized),
		})
}

func SetElasticIndex(org_id, index, id string, record interface{}) error {
	Debug("SetElasticIndex %v %v\n", index, id)
	return retry(func() error {
		return _SetElasticIndex(org_id, index, id, record)
	})
}

func _SetElasticIndex(org_id, index, id string, record interface{}) error {
	serialized := json.MustMarshalIndent(record)
	client, err := GetElasticClient()
	if err != nil {
		return err
	}

	es_req := opensearchapi.IndexRequest{
		Index:      GetIndex(org_id, index),
		DocumentID: id,
		Body:       bytes.NewReader(serialized),
		Refresh:    "true",
	}

	res, err := es_req.Do(context.Background(), client)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	// All is well we dont need to parse the results
	if !res.IsError() {
		return nil
	}

	response := ordereddict.NewDict()
	err = response.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	return makeElasticError(response)
}

type _ElasticHit struct {
	Index  string          `json:"_index"`
	Source json.RawMessage `json:"_source"`
	Id     string          `json:"_id"`
}

type _ElasticHits struct {
	Hits []_ElasticHit `json:"hits"`
}

type _AggBucket struct {
	Key   string `json:"key"`
	Count int    `json:"doc_count"`
}

type _AggResults struct {
	Buckets []_AggBucket `json:"buckets"`
}

type _ElasticAgg struct {
	Results _AggResults `json:"genres"`
}

type _ElasticResponse struct {
	Took         int          `json:"took"`
	Hits         _ElasticHits `json:"hits"`
	Aggregations _ElasticAgg  `json:"aggregations"`
}

func GetElasticRecord(
	ctx context.Context, org_id, index, id string) (json.RawMessage, error) {
	Debug("GetElasticRecord %v %v\n", index, id)
	client, err := GetElasticClient()
	if err != nil {
		return nil, err
	}

	res, err := opensearchapi.GetRequest{
		Index:      GetIndex(org_id, index),
		DocumentID: id,
	}.Do(ctx, client)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// All is well we dont need to parse the results
	if !res.IsError() {
		hit := &_ElasticHit{}
		err := json.Unmarshal(data, hit)
		return hit.Source, err
	}

	response := ordereddict.NewDict()
	err = response.UnmarshalJSON(data)
	if err != nil {
		return nil, err
	}

	found_any, pres := response.Get("found")
	if pres {
		found, ok := found_any.(bool)
		if ok && !found {
			return nil, os.ErrNotExist
		}
	}

	return nil, makeElasticError(response)
}

// Automatically take care of paging by returning a channel.  Query
// should be a JSON query **without** a sorting clause, or "size"
// clause.
// This function will modify the query to add a sorting column and
// automatically apply the search_after to page through the
// results. Currently we do not take a point in time snapshot so
// results are approximate.
func QueryChan(
	ctx context.Context,
	config_obj *config_proto.Config,
	page_size int,
	org_id, index, query, sort_field string) (
	chan json.RawMessage, error) {

	Debug("QueryChan %v\n", index)

	output_chan := make(chan json.RawMessage)

	query = strings.TrimSpace(query)
	var part_query string
	if sort_field != "" {
		part_query = json.Format(`{"sort":[{%q: "asc"}], "size":%q,`,
			sort_field, page_size)
	} else {
		part_query = json.Format(`{"size":%q,`, page_size)
	}
	part_query += query[1:]

	part, err := QueryElasticRaw(ctx, org_id, index, part_query)
	if err != nil {
		return nil, err
	}

	var search_after interface{}
	var pres bool

	go func() {
		defer close(output_chan)

		for {
			if len(part) == 0 {
				return
			}
			for idx, p := range part {
				select {
				case <-ctx.Done():
					return
				case output_chan <- p:
				}

				// On the last row we look at the result so we can get
				// the next part.
				if idx == len(part)-1 {
					row := ordereddict.NewDict()
					err := row.UnmarshalJSON(p)
					if err != nil {
						logger := logging.GetLogger(config_obj,
							&logging.FrontendComponent)
						logger.Error("QueryChan: %v", err)
						return
					}

					search_after, pres = row.Get(sort_field)
					if !pres {
						return
					}
				}
			}

			// Form the next query using the search_after value.
			part_query := json.Format(`
{"sort":[{%q: "asc"}], "size":%q,"search_after": [%q],`,
				sort_field, page_size, search_after) + query[1:]

			part, err = QueryElasticRaw(ctx, org_id, index, part_query)
			if err != nil {
				logger := logging.GetLogger(config_obj,
					&logging.FrontendComponent)
				logger.Error("QueryChan: %v", err)
				return
			}
		}
	}()

	return output_chan, nil
}

func DeleteByQuery(
	ctx context.Context, org_id, index, query string) error {
	client, err := GetElasticClient()
	if err != nil {
		return err
	}

	res, err := opensearchapi.DeleteByQueryRequest{
		Index:   []string{GetIndex(org_id, index)},
		Body:    strings.NewReader(query),
		Refresh: &TRUE,
	}.Do(ctx, client)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	// All is well we dont need to parse the results
	if !res.IsError() {
		return nil
	}

	response := ordereddict.NewDict()
	err = response.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	return makeElasticError(response)
}

func QueryElasticAggregations(
	ctx context.Context, org_id, index, query string) ([]string, error) {

	Debug("QueryElasticAggregations %v\n", index)

	es, err := GetElasticClient()
	if err != nil {
		return nil, err
	}
	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex(GetIndex(org_id, index)),
		es.Search.WithBody(strings.NewReader(query)),
		es.Search.WithPretty(),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	fmt.Println(string(data))

	// There was an error so we need to relay it
	if res.IsError() {
		response := ordereddict.NewDict()
		err = response.UnmarshalJSON(data)
		if err != nil {
			return nil, err
		}

		return nil, makeElasticError(response)
	}

	parsed := &_ElasticResponse{}
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		return nil, err
	}

	var results []string
	for _, hit := range parsed.Aggregations.Results.Buckets {
		results = append(results, hit.Key)
	}

	return results, nil
}

func QueryElasticRaw(
	ctx context.Context,
	org_id, index, query string) ([]json.RawMessage, error) {

	es, err := GetElasticClient()
	if err != nil {
		return nil, err
	}
	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex(GetIndex(org_id, index)),
		es.Search.WithBody(strings.NewReader(query)),
		es.Search.WithPretty(),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// There was an error so we need to relay it
	if res.IsError() {
		response := ordereddict.NewDict()
		err = response.UnmarshalJSON(data)
		if err != nil {
			return nil, err
		}

		return nil, makeElasticError(response)
	}

	parsed := &_ElasticResponse{}
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		return nil, err
	}

	var results []json.RawMessage
	for _, hit := range parsed.Hits.Hits {
		results = append(results, hit.Source)
	}

	return results, nil
}

// Return only Ids of matching documents.
func QueryElasticIds(
	ctx context.Context,
	org_id, index, query string) ([]string, error) {

	es, err := GetElasticClient()
	if err != nil {
		return nil, err
	}
	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex(GetIndex(org_id, index)),
		es.Search.WithBody(strings.NewReader(query)),
		es.Search.WithPretty(),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// There was an error so we need to relay it
	if res.IsError() {
		response := ordereddict.NewDict()
		err = response.UnmarshalJSON(data)
		if err != nil {
			return nil, err
		}

		return nil, makeElasticError(response)
	}

	parsed := &_ElasticResponse{}
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		return nil, err
	}

	var results []string
	for _, hit := range parsed.Hits.Hits {
		results = append(results, hit.Id)
	}

	return results, nil
}

type Result struct {
	JSON json.RawMessage
	Id   string
}

func QueryElastic(
	ctx context.Context,
	org_id, index, query string) ([]Result, error) {

	es, err := GetElasticClient()
	if err != nil {
		return nil, err
	}
	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex(GetIndex(org_id, index)),
		es.Search.WithBody(strings.NewReader(query)),
		es.Search.WithPretty(),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// There was an error so we need to relay it
	if res.IsError() {
		response := ordereddict.NewDict()
		err = response.UnmarshalJSON(data)
		if err != nil {
			return nil, err
		}

		return nil, makeElasticError(response)
	}

	parsed := &_ElasticResponse{}
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		return nil, err
	}

	var results []Result
	for _, hit := range parsed.Hits.Hits {
		results = append(results, Result{
			JSON: hit.Source,
			Id:   hit.Id,
		})
	}

	return results, nil
}

func GetElasticClient() (*opensearch.Client, error) {
	mu.Lock()
	defer mu.Unlock()

	if gElasticClient == nil {
		return nil, errors.New("Elastic configuration not initialized")
	}

	return gElasticClient, nil
}

func SetElasticClient(c *opensearch.Client) {
	mu.Lock()
	defer mu.Unlock()

	gElasticClient = c
}

func SetDebugLogger(config_obj *config_proto.Config) {
	mu.Lock()
	defer mu.Unlock()

	logger = logging.GetLogger(config_obj, &logging.FrontendComponent)
}

func StartElasticSearchService(
	config_obj *config_proto.Config, elastic_config_path string) error {

	if elastic_config_path == "" {
		return nil
	}

	elastic_config, err := elastic_datastore.LoadConfig(elastic_config_path)
	if err != nil {
		return err
	}

	cfg := opensearch.Config{
		Addresses: elastic_config.Addresses,
	}

	CA_Pool := x509.NewCertPool()
	crypto.AddPublicRoots(CA_Pool)

	if elastic_config.RootCerts != "" &&
		!CA_Pool.AppendCertsFromPEM([]byte(elastic_config.RootCerts)) {
		return errors.New("elastic ingestion: Unable to add root certs")
	}

	cfg.Transport = &http.Transport{
		MaxIdleConnsPerHost:   10,
		ResponseHeaderTimeout: 100 * time.Second,
		TLSClientConfig: &tls.Config{
			ClientSessionCache: tls.NewLRUClientSessionCache(100),
			RootCAs:            CA_Pool,
			InsecureSkipVerify: elastic_config.DisableSSLSecurity,
		},
	}

	if elastic_config.Username != "" && elastic_config.Password != "" {
		cfg.Username = elastic_config.Username
		cfg.Password = elastic_config.Password
	} else {
		signer, err := requestsigner.NewSigner(session.Options{SharedConfigState: session.SharedConfigEnable})
		if err != nil {
			return err
		}
		cfg.Signer = signer
	}

	client, err := opensearch.NewClient(cfg)
	if err != nil {
		return err
	}

	// Fetch info immediately to verify that we can actually connect
	// to the server.
	res, err := client.Info()
	if err != nil {
		return err
	}

	defer res.Body.Close()

	// Set the global elastic client
	SetElasticClient(client)

	return nil
}

func makeElasticError(response *ordereddict.Dict) error {
	err_type := utils.GetString(response, "error.type")
	err_reason := utils.GetString(response, "error.reason")
	if false && err_type != "" && err_reason != "" {
		return fmt.Errorf("Elastic Error: %v: %v", err_type, err_reason)
	}

	return fmt.Errorf("Elastic Error: %v", response)
}

// Convert the item into a unique document ID - This is needed when
// the item can be longer than the maximum 512 bytes.
func MakeId(item string) string {
	hash := sha1.Sum([]byte(item))
	return hex.EncodeToString(hash[:])
}

func StartBulkIndexService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	elastic_client, err := GetElasticClient()
	if err != nil {
		return err
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	new_bulk_indexer, err := opensearchutil.NewBulkIndexer(
		opensearchutil.BulkIndexerConfig{
			Client: elastic_client,
			OnError: func(ctx context.Context, err error) {
				if err != nil {
					logger.Error("BulkIndexerConfig: %v", err)
				}
			},
		})
	if err != nil {
		return err
	}

	mu.Lock()
	bulk_indexer = new_bulk_indexer
	mu.Unlock()

	// Ensure we flush the indexer before we exit.
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		subctx, cancel := context.WithTimeout(context.Background(),
			30*time.Second)
		defer cancel()

		bulk_indexer.Close(subctx)
	}()

	return err
}
