package indexing

// Implement client searching

import (
	"context"
	"regexp"
	"strings"
	"time"

	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	// Supported list of search verbs
	verbs = []string{
		"label:",
		"host:",
		"mac:",
		"client:",
		"recent:",
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
    "timestamp": {"order": "desc"}
  }],
  "query": {"match": {"username": %q}},
  "from": %q, "size": %q
}`
)

// Get the recent clients viewed by the principal sorted in most
// recently used order.
func (self *Indexer) searchRecents(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal string, term string, from, limit uint64) (
	*api_proto.SearchClientsResponse, error) {

	hits, err := cvelo_services.QueryElasticRaw(
		ctx, config_obj.OrgId, "user_mru", json.Format(
			clientsMRUQuery, principal, from, limit))
	if err != nil {
		return nil, err
	}

	result := &api_proto.SearchClientsResponse{}
	for _, hit := range hits {
		item := &MRUItem{}
		err = json.Unmarshal(hit, item)
		if err != nil {
			continue
		}
		api_client, err := self.FastGetApiClient(
			ctx, config_obj, item.ClientId)
		if err != nil {
			return nil, err
		}

		result.Items = append(result.Items, api_client)
	}

	return result, nil
}

const (
	allClientsQuery    = `{"match_all" : {}}`
	recentClientsQuery = `{
   "range": {
     "ping": {"gt": %q}
   }
}`

	getAllClientsQuery = `
{"sort": [{
    "client_id": {"order": "asc"}
 }],
 "_source": false,
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
    %q: {"order": %q}
 }],
`
	limitQueryPart = `,
 "from": %q, "size": %q
`

	getAllClientsAgg = `
{
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
	in *api_proto.SearchClientsRequest) (
	*api_proto.SearchClientsResponse, error) {

	terms := []string{allClientsQuery}
	return self.searchWithTerms(ctx, config_obj,
		in.Filter, terms, in.Offset, in.Limit)
}

const (
	fieldSearchQuery = `{"prefix": {%q: %q}}`
)

func (self *Indexer) searchClientsByLabel(
	ctx context.Context,
	config_obj *config_proto.Config,
	operator, label string,
	in *api_proto.SearchClientsRequest,
	limit uint64) (*api_proto.SearchClientsResponse, error) {

	if in.NameOnly {
		return self.searchWithNames(ctx, config_obj,
			"lower_labels", operator, label, in.Offset, in.Limit)
	}

	terms := []string{json.Format(fieldSearchQuery, "lower_labels", label)}
	return self.searchWithTerms(ctx, config_obj,
		in.Filter, terms, in.Offset, in.Limit)
}

func (self *Indexer) searchClientsByHost(
	ctx context.Context,
	config_obj *config_proto.Config,
	operator, hostname string,
	in *api_proto.SearchClientsRequest,
	limit uint64) (*api_proto.SearchClientsResponse, error) {

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
	limit uint64) (*api_proto.SearchClientsResponse, error) {

	if in.NameOnly {
		return self.searchWithPrefixedNames(ctx, config_obj,
			"mac_addresses", operator, mac, in.Offset, in.Limit)
	}

	terms := []string{json.Format(fieldSearchQuery, "mac_addresses", mac)}
	return self.searchWithTerms(ctx, config_obj,
		in.Filter, terms, in.Offset, in.Limit)
}

func (self *Indexer) searchWithNames(
	ctx context.Context,
	config_obj *config_proto.Config,
	field, operator, label string,
	offset, limit uint64) (*api_proto.SearchClientsResponse, error) {

	query := json.Format(getAllClientsAgg, field, offset, limit+1)

	hits, err := cvelo_services.QueryElasticAggregations(
		ctx, config_obj.OrgId, "clients", query)
	if err != nil {
		return nil, err
	}

	prefix, filter := splitSearchTermIntoPrefixAndFilter(label)

	result := &api_proto.SearchClientsResponse{}
	for _, hit := range hits {
		if !strings.HasPrefix(hit, prefix) {
			continue
		}

		if filter != nil && !filter.MatchString(hit) {
			continue
		}

		result.Names = append(result.Names, operator+":"+hit)

	}
	return result, nil
}

// This names query does not use aggregation because the fields it
// typically looks at should be mostly unique
func (self *Indexer) searchWithPrefixedNames(
	ctx context.Context,
	config_obj *config_proto.Config,
	field, operator, term string,
	offset, limit uint64) (*api_proto.SearchClientsResponse, error) {

	prefix, filter := splitSearchTermIntoPrefixAndFilter(term)
	query := json.Format(
		getAllClientsQuery,
		json.Format(fieldSearchQuery, field, prefix),
		json.Format(limitQueryPart, offset, limit+1))

	hits, err := cvelo_services.QueryElasticIds(
		ctx, config_obj.OrgId, "clients", query)
	if err != nil {
		return nil, err
	}

	return searchClientsFromHits(ctx, config_obj, hits, field, filter)
}

func filterClientInfo(client_info *services.ClientInfo,
	field string, filter *regexp.Regexp) bool {
	return true
}

func (self *Indexer) searchWithTerms(
	ctx context.Context,
	config_obj *config_proto.Config,
	filter api_proto.SearchClientsRequest_Filters,
	terms []string,
	offset, limit uint64) (
	*api_proto.SearchClientsResponse, error) {
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
	offset, limit uint64) (
	*api_proto.SearchClientsResponse, error) {

	// Show clients that pinged more recently than 10 min ago
	if filter == api_proto.SearchClientsRequest_ONLINE {
		terms = append(terms, json.Format(recentClientsQuery,
			time.Now().UnixNano()-600000000000))
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

	limit := uint64(50)
	if in.Limit > 0 {
		limit = in.Limit
	}

	operator, term := splitIntoOperatorAndTerms(in.Query)
	switch operator {
	case "all":
		return self.getAllClients(ctx, config_obj, in)

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
	in *api_proto.SearchClientsRequest) (*api_proto.SearchClientsResponse, error) {

	if in.NameOnly {
		return self.searchWithPrefixedNames(ctx, config_obj, "client_id",
			operator, client_id, in.Offset, in.Limit)
	}

	result := &api_proto.SearchClientsResponse{}
	api_client, err := self.FastGetApiClient(
		ctx, config_obj, client_id)
	if err != nil {
		// Client is not found - not an actual error.
		return result, nil
	}

	result.Items = append(result.Items, api_client)
	return result, nil
}

// Free form search term, try to fill in as many suggestions as
// possible.
func (self *Indexer) searchVerbs(ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	limit uint64) (*api_proto.SearchClientsResponse, error) {

	terms := []string{}
	items := []*api_proto.ApiClient{}

	term := strings.ToLower(in.Query)
	for _, verb := range verbs {
		if strings.HasPrefix(verb, term) {
			terms = append(terms, verb)
		}
	}

	// Not a verb maybe a hostname
	if uint64(len(terms)) < in.Limit {
		res, err := self.searchClientsByHost(
			ctx, config_obj, "host", in.Query, in, in.Limit)
		if err == nil {
			terms = append(terms, res.Names...)
			items = append(items, res.Items...)
		}
	}

	// Maybe a label
	if uint64(len(terms)) < in.Limit {
		res, err := self.searchClientsByLabel(
			ctx, config_obj, "label", in.Query, in, in.Limit)
		if err == nil {
			terms = append(terms, res.Names...)
			items = append(items, res.Items...)
		}
	}

	return &api_proto.SearchClientsResponse{
		Names: terms,
		Items: items,
	}, nil
}

func searchClientsFromHits(
	ctx context.Context,
	config_obj *config_proto.Config,
	hits []string,
	field string, filter *regexp.Regexp) (*api_proto.SearchClientsResponse, error) {

	client_ids := make([]string, 0, len(hits))
	for _, id := range hits {
		if strings.HasSuffix(id, "ping") ||
			strings.HasSuffix(id, "labels") {
			continue
		}
		client_ids = append(client_ids, id)
	}

	result := &api_proto.SearchClientsResponse{}
	client_infos, err := api.GetMultipleClients(ctx, config_obj, client_ids)
	if err != nil {
		return nil, err
	}

	for _, client_info := range client_infos {
		if filterClientInfo(client_info, field, filter) {
			result.Items = append(result.Items,
				_makeApiClient(&client_info.ClientInfo))
		}
	}

	return result, nil
}
