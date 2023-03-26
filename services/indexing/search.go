package indexing

// Implement client searching

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

var (
	// Supported list of search verbs
	verbs = []string{
		"label:",
		"host:",
		"mac:",
		"client:",
		"recent:",
		"after:",
		"ip:",
	}
)

func splitIntoOperatorAndTerms(term string) (string, string) {
	if term == "all" {
		return "all", ""
	}

	// Client IDs can be searched directly.
	if strings.HasPrefix(term, "C.") || strings.HasPrefix(term, "c.") {
		return "client", term
	}

	parts := strings.SplitN(term, ":", 2)
	if len(parts) == 1 {
		// Bare search terms mean hostname or fqdn
		return "", parts[0]
	}
	return parts[0], parts[1]
}

const (
	clientsMRUQuery = `
{"sort": [{
    "timestamp": {"order": "desc", "unmapped_type": "long"}
  }],
  "query": {"term": {"username": %q}},
  "from": %q, "size": %q
}`
)

// Get the recent clients viewed by the principal sorted in most
// recently used order.
func (self *Indexer) searchRecents(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal string, term string, from, limit uint64) (
	[]*api.ClientRecord, error) {

	hits, err := cvelo_services.QueryElasticRaw(
		ctx, config_obj.OrgId, "user_mru", json.Format(
			clientsMRUQuery, principal, from, limit))
	if err != nil {
		return nil, err
	}

	client_ids := []string{}

	for _, hit := range hits {
		item := &MRUItem{}
		err = json.Unmarshal(hit, item)
		if err != nil {
			continue
		}

		client_ids = append(client_ids, item.ClientId)
	}

	return api.GetMultipleClients(ctx, config_obj, client_ids)
}

const (
	allClientsQuery    = `{"range": {"first_seen_at": {"gt": 0}}}`
	recentClientsQuery = `{
   "range": {
     "ping": {"gt": %q}
   }
}`

	getAllClientsQuery = `
{"sort": [{
    "client_id": {"order": "asc", "unmapped_type": "keyword"}
 }],
 "_source": false,
 "query": {"bool": {"must": [%s]}}
 %s
}
`
	getAllClientsNamesQuery = `
{"sort": [{
    "client_id": {"order": "asc", "unmapped_type": "keyword"}
 }],
 "query": {"bool": {"must": [%s]}}
 %s
}
`

	getSortedClientsQuery = `
{%s
 "query": {"bool": {"must": [%s]}},
 "_source": false
 %s
}
`
	sortQueryPart = `
"sort": [{
    %q: {"order": %q, "unmapped_type": "long"}
 }],
`
	limitQueryPart = `,
 "from": %q, "size": %q
`

	getAllClientsAgg = `
{
 "query": {"prefix": {%q: {"value": %q,  "case_insensitive": true}}},
 "aggs": {
    "genres": {
      "terms": { "field": %q }
    }
  },
 "fields": [ "client_id" ],
 "size": 0
}
`
)

func (self *Indexer) getAllClients(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	limit uint64) ([]*api.ClientRecord, error) {

	// If the user wants to sort we search by filename - this is
	// currently the only field we can sort on.
	if in.Sort == api_proto.SearchClientsRequest_SORT_DOWN ||
		in.Sort == api_proto.SearchClientsRequest_SORT_UP {
		return self.searchClientsByHost(ctx, config_obj,
			"host", "", in, limit)
	}

	terms := []string{allClientsQuery}
	return self.searchWithTerms(ctx, config_obj,
		in.Filter, terms, in.Offset, in.Limit)
}

const (
	fieldSearchQuery = `{"prefix": {%q: {"value": %q, "case_insensitive": true}}}`
)

func (self *Indexer) searchClientsByLabel(
	ctx context.Context,
	config_obj *config_proto.Config,
	operator, label string,
	in *api_proto.SearchClientsRequest,
	limit uint64) ([]*api.ClientRecord, error) {

	terms := []string{json.Format(fieldSearchQuery, "labels", label)}
	return self.searchWithTerms(ctx, config_obj,
		in.Filter, terms, in.Offset, in.Limit)
}

func (self *Indexer) searchClientsByHost(
	ctx context.Context,
	config_obj *config_proto.Config,
	operator, hostname string,
	in *api_proto.SearchClientsRequest,
	limit uint64) ([]*api.ClientRecord, error) {

	if in.NameOnly {
		return self.searchWithPrefixedNames(ctx, config_obj,
			"hostname", operator, hostname, in.Offset, in.Limit)
	}

	terms := []string{json.Format(fieldSearchQuery, "hostname", hostname)}
	if hostname == "*" {
		terms = []string{allClientsQuery}
	}

	dir := "asc"
	if in.Sort == api_proto.SearchClientsRequest_SORT_DOWN {
		dir = "desc"
	}

	sorter := json.Format(sortQueryPart, "hostname", dir)
	return self.searchWithSortTerms(ctx, config_obj,
		sorter, in.Filter, terms, in.Offset, in.Limit)
}

func (self *Indexer) searchClientsByMac(
	ctx context.Context,
	config_obj *config_proto.Config,
	operator, mac string,
	in *api_proto.SearchClientsRequest,
	limit uint64) ([]*api.ClientRecord, error) {

	if in.NameOnly {
		return self.searchWithPrefixedNames(ctx, config_obj,
			"mac_addresses", operator, mac, in.Offset, in.Limit)
	}

	terms := []string{json.Format(fieldSearchQuery, "mac_addresses", mac)}
	return self.searchWithTerms(ctx, config_obj,
		in.Filter, terms, in.Offset, in.Limit)
}

// This names query does not use aggregation because the fields it
// typically looks at should be mostly unique
func (self *Indexer) searchWithPrefixedNames(
	ctx context.Context,
	config_obj *config_proto.Config,
	field, operator, term string,
	offset, limit uint64) ([]*api.ClientRecord, error) {

	prefix, filter := splitSearchTermIntoPrefixAndFilter(term)
	query := json.Format(
		getAllClientsNamesQuery,
		json.Format(fieldSearchQuery, field, prefix),
		json.Format(limitQueryPart, offset, limit+1))

	hits, err := cvelo_services.QueryElasticRaw(
		ctx, config_obj.OrgId, "clients", query)
	if err != nil {
		return nil, err
	}

	result := []*api.ClientRecord{}
	for _, hit := range hits {
		record := &api.ClientRecord{}
		err = json.Unmarshal(hit, record)
		if err != nil || record.ClientId == "" {
			continue
		}

		if filterClientInfo(record, field, filter) {
			result = append(result, record)
		}
	}
	return result, nil
}

func filterClientInfo(client_info *api.ClientRecord,
	field string, filter *regexp.Regexp) bool {
	return true
}

func (self *Indexer) searchWithTerms(
	ctx context.Context,
	config_obj *config_proto.Config,
	filter api_proto.SearchClientsRequest_Filters,
	terms []string,
	offset, limit uint64) ([]*api.ClientRecord, error) {
	sorter := json.Format(sortQueryPart, "client_id", "asc")
	return self.searchWithSortTerms(ctx, config_obj,
		sorter, filter, terms, offset, limit)
}

func (self *Indexer) searchWithSortTerms(
	ctx context.Context,
	config_obj *config_proto.Config,
	sorter string,
	filter api_proto.SearchClientsRequest_Filters,
	terms []string,
	offset, limit uint64) ([]*api.ClientRecord, error) {

	// Show clients that pinged more recently than 10 min ago
	if filter == api_proto.SearchClientsRequest_ONLINE {
		// We can not have both conditions because the records are
		// split: A ping range query operates on ping records but
		// other queries (like hostname queries) operate on the
		// hostname which may be in a different record.
		terms = []string{json.Format(recentClientsQuery,
			time.Now().UnixNano()-600000000000)}
	}

	query := json.Format(
		getSortedClientsQuery,
		sorter,
		strings.Join(terms, ","),
		json.Format(limitQueryPart, offset, limit+1))

	hits, err := cvelo_services.QueryElasticIds(
		ctx, config_obj.OrgId, "clients", query)
	if err != nil {
		return nil, err
	}

	return searchClientsFromHits(ctx, config_obj, hits, "", nil)
}

func (self *Indexer) SearchClients(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	principal string) (*api_proto.SearchClientsResponse, error) {

	// Handle name only searches.
	if in.NameOnly {
		return self.searchClientNameOnly(ctx, config_obj, in)
	}

	result := &api_proto.SearchClientsResponse{}
	records, err := self.SearchClientRecords(ctx, config_obj, in, principal)
	if err != nil {
		return nil, err
	}

	for _, r := range records {
		result.Items = append(result.Items, _makeApiClient(r))
	}

	return result, nil
}

func (self *Indexer) SearchClientRecords(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	principal string) ([]*api.ClientRecord, error) {

	limit := uint64(50)
	if in.Limit > 0 {
		limit = in.Limit
	}

	operator, term := splitIntoOperatorAndTerms(in.Query)
	switch operator {
	case "all":
		return self.getAllClients(ctx, config_obj, in, limit)

	case "label":
		return self.searchClientsByLabel(ctx, config_obj,
			operator, term, in, limit)

	case "host":
		return self.searchClientsByHost(ctx, config_obj,
			operator, term, in, limit)

	case "mac":
		return self.searchClientsByMac(ctx, config_obj,
			operator, term, in, limit)

	case "client":
		return self.searchClientsByClientId(
			ctx, config_obj, operator, term, in)

	case "recent":
		return self.searchRecents(ctx, config_obj,
			principal, term, in.Offset, limit)

	default:
		return self.searchVerbs(ctx, config_obj, in, limit)
	}
}

func (self *Indexer) searchClientsByClientId(
	ctx context.Context,
	config_obj *config_proto.Config,
	operator, client_id string,
	in *api_proto.SearchClientsRequest) ([]*api.ClientRecord, error) {

	return api.GetMultipleClients(ctx, config_obj, []string{client_id})
}

// Free form search term, try to fill in as many suggestions as
// possible.
func (self *Indexer) searchVerbs(ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	limit uint64) ([]*api.ClientRecord, error) {

	items := []*api.ClientRecord{}

	// Not a verb maybe a hostname
	if uint64(len(items)) < in.Limit {
		res, err := self.searchClientsByHost(
			ctx, config_obj, "host", in.Query, in, in.Limit)
		if err == nil {
			items = append(items, res...)
		}
	}

	// Maybe a label
	if uint64(len(items)) < in.Limit {
		res, err := self.searchClientsByLabel(
			ctx, config_obj, "label", in.Query, in, in.Limit)
		if err == nil {
			items = append(items, res...)
		}
	}

	return items, nil
}

func searchClientsFromHits(
	ctx context.Context,
	config_obj *config_proto.Config,
	hits []string,
	field string, filter *regexp.Regexp) ([]*api.ClientRecord, error) {

	client_ids := ordereddict.NewDict()
	for _, id := range hits {
		client_id := api.GetClientIdFromDocId(id)
		_, pres := client_ids.Get(client_id)
		if !pres {
			client_ids.Set(client_id, true)
		}
	}

	result := []*api.ClientRecord{}
	if client_ids.Len() == 0 {
		return result, nil
	}

	client_infos, err := api.GetMultipleClients(
		ctx, config_obj, client_ids.Keys())
	if err != nil {
		return nil, err
	}

	for _, client_record := range client_infos {
		if filterClientInfo(client_record, field, filter) {
			result = append(result, client_record)
		}
	}

	return result, nil
}
