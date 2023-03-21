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

	opensearch "github.com/opensearch-project/opensearch-go/v2"
	opensearchapi "github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"github.com/opensearch-project/opensearch-go/v2/opensearchutil"
	requestsigner "github.com/opensearch-project/opensearch-go/v2/signer/aws"

	"www.velocidex.com/golang/cloudvelo/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	AsyncDelete = false
	SyncDelete  = true

	NoSortField = ""
)

var (
	mu             sync.Mutex
	gElasticClient *opensearch.Client
	TRUE           = true
	True           = "true"

	logger *logging.LogContext

	bulk_indexer *BulkIndexer
)

// The logger is normally installed in the start up sequence with
// SetDebugLogger() below.
func Debug(format string, args ...interface{}) func() {
	start := time.Now()
	return func() {
		if logger != nil {
			args = append(args, time.Now().Sub(start))
			logger.Debug(format+" in %v", args...)
		}
	}
}

type IndexInfo struct {
	Index string `json:"index"`
}

func ListIndexes(ctx context.Context) ([]string, error) {
	client, err := GetElasticClient()
	if err != nil {
		return nil, err
	}

	res, err := opensearchapi.CatIndicesRequest{
		Format: "json",
	}.Do(ctx, client)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	indexes := []*IndexInfo{}
	err = json.Unmarshal(data, &indexes)
	if err != nil {
		return nil, err
	}

	results := make([]string, len(indexes))
	for _, i := range indexes {
		results = append(results, i.Index)
	}

	return results, nil

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

func DeleteDocument(
	ctx context.Context, org_id, index string, id string, sync bool) error {

	defer Instrument("DeleteDocument")()

	defer Debug("DeleteDocument %v", id)()
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
func FlushIndex(
	ctx context.Context, org_id, index string) error {
	client, err := GetElasticClient()
	if err != nil {
		return err
	}

	res, err := opensearchapi.IndicesRefreshRequest{
		Index: []string{GetIndex(org_id, index)},
	}.Do(ctx, client)

	if err != nil {
		return err
	}

	defer res.Body.Close()

	return err
}

func UpdateIndex(
	ctx context.Context, org_id, index, id string, query string) error {
	defer Instrument("UpdateIndex")()
	defer Debug("UpdateIndex %v %v", index, id)()
	return retry(func() error {
		return _UpdateIndex(ctx, org_id, index, id, query)
	})
}

func _UpdateIndex(
	ctx context.Context, org_id, index, id string, query string) error {
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

	res, err := es_req.Do(ctx, client)
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

	return makeElasticError(data)
}

func UpdateByQuery(
	ctx context.Context, org_id, index string, query string) error {

	defer Instrument("UpdateByQuery")()

	client, err := GetElasticClient()
	if err != nil {
		return err
	}

	es_req := opensearchapi.UpdateByQueryRequest{
		Index:   []string{GetIndex(org_id, index)},
		Body:    strings.NewReader(query),
		Refresh: &TRUE,
	}

	res, err := es_req.Do(ctx, client)
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

	return makeElasticError(data)
}

func SetElasticIndexAsync(org_id, index, id string, record interface{}) error {
	defer Debug("SetElasticIndexAsync %v %v", index, id)()
	mu.Lock()
	l_bulk_indexer := bulk_indexer
	mu.Unlock()

	serialized := json.MustMarshalString(record)

	// Add with background context which might outlive our caller.
	return l_bulk_indexer.Add(context.Background(),
		opensearchutil.BulkIndexerItem{
			Index:      GetIndex(org_id, index),
			Action:     "index",
			DocumentID: id,
			Body:       strings.NewReader(serialized),
		})
}

func SetElasticIndex(ctx context.Context,
	org_id, index, id string, record interface{}) error {
	defer Instrument("SetElasticIndex")()
	defer Debug("SetElasticIndex %v %v", index, id)()

	return retry(func() error {
		return _SetElasticIndex(ctx, org_id, index, id, record)
	})
}

func _SetElasticIndex(
	ctx context.Context, org_id, index, id string, record interface{}) error {
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

	res, err := es_req.Do(ctx, client)
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

	return makeElasticError(data)
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
	Key   interface{} `json:"key"`
	Count int         `json:"doc_count"`
}

type _AggResults struct {
	Buckets []_AggBucket `json:"buckets"`
	Value   interface{}  `json:"value"`
}

type _ElasticAgg struct {
	Results _AggResults `json:"genres"`
}

type _ElasticResponse struct {
	Took         int          `json:"took"`
	Hits         _ElasticHits `json:"hits"`
	Aggregations _ElasticAgg  `json:"aggregations"`
}

// Gets a single elastic record by id.
func GetElasticRecord(
	ctx context.Context, org_id, index, id string) (json.RawMessage, error) {
	defer Debug("GetElasticRecord %v %v", index, id)()
	defer Instrument("GetElasticRecord")()

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
		return nil, makeReadElasticError(data)
	}

	found_any, pres := response.Get("found")
	if pres {
		found, ok := found_any.(bool)
		if ok && !found {
			return nil, os.ErrNotExist
		}
	}

	return nil, makeReadElasticError(data)
}

type doc_id struct {
	Id     string          `json:"_id"`
	Source json.RawMessage `json:"_source"`
}

type docs struct {
	Docs []doc_id `json:"docs"`
}

// Gets a single elastic record by id.
func GetMultipleElasticRecords(
	ctx context.Context,
	org_id, index string, ids []string) ([]json.RawMessage, error) {

	defer Instrument("GetMultipleElasticRecords")()

	if len(ids) == 0 {
		return nil, nil
	}

	if len(ids) > 4 {
		defer Debug("GetMultipleElasticRecords %v %v ...", index, ids[:4])()
	} else {
		defer Debug("GetMultipleElasticRecords %v %v", index, ids)()
	}

	client, err := GetElasticClient()
	if err != nil {
		return nil, err
	}

	d := &docs{}
	for _, id := range ids {
		d.Docs = append(d.Docs, doc_id{Id: id})
	}

	res, err := opensearchapi.MgetRequest{
		Index: GetIndex(org_id, index),
		Body:  strings.NewReader(json.MustMarshalString(d)),
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
		hit := &docs{}
		err := json.Unmarshal(data, hit)
		if err != nil {
			return nil, err
		}

		result := make([]json.RawMessage, 0, len(hit.Docs))
		for _, h := range hit.Docs {
			result = append(result, h.Source)
		}

		return result, nil
	}

	response := ordereddict.NewDict()
	err = response.UnmarshalJSON(data)
	if err != nil {
		return nil, makeReadElasticError(data)
	}

	found_any, pres := response.Get("found")
	if pres {
		found, ok := found_any.(bool)
		if ok && !found {
			return nil, os.ErrNotExist
		}
	}

	return nil, makeReadElasticError(data)
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

	defer Debug("QueryChan %v", index)()

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

	defer Instrument("DeleteByQuery")()

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

	return makeReadElasticError(data)
}

func QueryElasticAggregations(
	ctx context.Context, org_id, index, query string) ([]string, error) {

	defer Instrument("QueryElasticAggregations")()
	defer Debug("QueryElasticAggregations %v", index)()

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
		return nil, makeReadElasticError(data)
	}

	parsed := &_ElasticResponse{}
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		return nil, makeReadElasticError(data)
	}

	var results []string
	// Handle value aggregates
	if !utils.IsNil(parsed.Aggregations.Results.Value) {
		results = append(results, to_string(parsed.Aggregations.Results.Value))
		return results, nil
	}

	for _, hit := range parsed.Aggregations.Results.Buckets {
		results = append(results, to_string(hit.Key))
	}

	return results, nil
}

func to_string(a interface{}) string {
	switch t := a.(type) {
	case string:
		return t

	default:
		return string(json.MustMarshalIndent(a))
	}
}

func QueryElasticRaw(
	ctx context.Context,
	org_id, index, query string) ([]json.RawMessage, error) {

	defer Instrument("QueryElasticRaw")()
	defer Debug("QueryElasticRaw %v", index)()

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
		return nil, makeReadElasticError(data)
	}

	parsed := &_ElasticResponse{}
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		return nil, makeReadElasticError(data)
	}

	var results []json.RawMessage
	for _, hit := range parsed.Hits.Hits {
		results = append(results, hit.Source)
	}

	return results, nil
}

// Return only Ids of matching documents.
// You probably want to add the following to the query:
// "_source": false
func QueryElasticIds(
	ctx context.Context,
	org_id, index, query string) ([]string, error) {

	defer Instrument("QueryElasticIds")()
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
		return nil, makeReadElasticError(data)
	}

	parsed := &_ElasticResponse{}
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		return nil, makeReadElasticError(data)
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

	defer Instrument("QueryElastic")()

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
		return nil, makeReadElasticError(data)
	}

	parsed := &_ElasticResponse{}
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		return nil, makeReadElasticError(data)
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

func StartElasticSearchService(config_obj *config.Config) error {
	cfg := opensearch.Config{
		Addresses: config_obj.Cloud.Addresses,
	}

	CA_Pool := x509.NewCertPool()
	crypto.AddPublicRoots(CA_Pool)

	if config_obj.Cloud.RootCerts != "" &&
		!CA_Pool.AppendCertsFromPEM([]byte(config_obj.Cloud.RootCerts)) {
		return errors.New("cloud ingestion: Unable to add root certs")
	}

	cfg.Transport = &http.Transport{
		MaxIdleConnsPerHost:   10,
		ResponseHeaderTimeout: 100 * time.Second,
		TLSClientConfig: &tls.Config{
			ClientSessionCache: tls.NewLRUClientSessionCache(100),
			RootCAs:            CA_Pool,
			InsecureSkipVerify: config_obj.Cloud.DisableSSLSecurity,
		},
		//DisableCompression: true,
	}

	if config_obj.Cloud.Username != "" && config_obj.Cloud.Password != "" {
		cfg.Username = config_obj.Cloud.Username
		cfg.Password = config_obj.Cloud.Password
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

func makeElasticError(data []byte) error {
	response := ordereddict.NewDict()
	err := response.UnmarshalJSON(data)
	if err != nil {
		return fmt.Errorf("Elastic Error: %v", string(data))
	}

	err_type := utils.GetString(response, "error.type")
	err_reason := utils.GetString(response, "error.reason")
	if false && err_type != "" && err_reason != "" {
		return fmt.Errorf("Elastic Error: %v: %v", err_type, err_reason)
	}

	return fmt.Errorf("Elastic Error: %v", response)
}

// For read operations, an index not found error is not considered an
// error - we just return no results.
func makeReadElasticError(data []byte) error {
	response := ordereddict.NewDict()
	err := response.UnmarshalJSON(data)
	if err != nil {
		return fmt.Errorf("Elastic Error: %v", string(data))
	}

	err_type := utils.GetString(response, "error.type")
	if err_type == "index_not_found_exception" {
		return nil
	}

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

type BulkIndexer struct {
	opensearchutil.BulkIndexer
	ctx        context.Context
	config_obj *config_proto.Config
	mu         sync.Mutex

	indexes map[string]bool
}

func (self *BulkIndexer) Add(ctx context.Context, item opensearchutil.BulkIndexerItem) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.indexes[item.Index] = true
	return self.BulkIndexer.Add(ctx, item)
}

func (self *BulkIndexer) Close() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	elastic_client, err := GetElasticClient()
	if err != nil {
		return err
	}

	new_bulk_indexer, err := opensearchutil.NewBulkIndexer(
		opensearchutil.BulkIndexerConfig{
			Client:        elastic_client,
			FlushInterval: time.Second * 10,
			OnFlushStart: func(ctx context.Context) context.Context {
				logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
				logger.Debug("Flushing bulk indexer.")
				return ctx
			},
			OnError: func(ctx context.Context, err error) {
				if err != nil {
					logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
					logger.Error("BulkIndexerConfig: %v", err)
				}
			},
		})
	if err != nil {
		return err
	}

	ctx := context.Background()
	err = self.BulkIndexer.Close(ctx)
	if err != nil {
		return err
	}

	indexes := []string{}
	for i := range self.indexes {
		indexes = append(indexes, i)
	}
	res, err := opensearchapi.IndicesRefreshRequest{
		Index: indexes,
	}.Do(ctx, elastic_client)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	self.BulkIndexer = new_bulk_indexer
	return nil
}

func FlushBulkIndexer() error {
	mu.Lock()
	b := bulk_indexer
	mu.Unlock()

	if b != nil {
		return b.Close()
	}
	return nil
}

func StartBulkIndexService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config.Config) error {
	elastic_client, err := GetElasticClient()
	if err != nil {
		return err
	}

	new_bulk_indexer, err := opensearchutil.NewBulkIndexer(
		opensearchutil.BulkIndexerConfig{
			Client:        elastic_client,
			FlushInterval: time.Second * 2,
			OnFlushStart: func(ctx context.Context) context.Context {
				//logger := logging.GetLogger(
				// config_obj.VeloConf(), &logging.FrontendComponent)
				//logger.Debug("Flushing bulk indexer.")
				return ctx
			},
			OnError: func(ctx context.Context, err error) {
				if err != nil {
					logger := logging.GetLogger(
						config_obj.VeloConf(), &logging.FrontendComponent)
					logger.Error("BulkIndexerConfig: %v", err)
				}
			},
		})
	if err != nil {
		return err
	}

	mu.Lock()
	bulk_indexer = &BulkIndexer{
		BulkIndexer: new_bulk_indexer,
		config_obj:  config_obj.VeloConf(),
		ctx:         ctx,
		indexes:     make(map[string]bool),
	}
	mu.Unlock()

	// Ensure we flush the indexer before we exit.
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		FlushBulkIndexer()
	}()

	return err
}
