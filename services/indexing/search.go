package indexing

// Implement client searching

import (
	"context"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
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
		ctx, self.config_obj.OrgId, "user_mru", json.Format(
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
 "query": {"bool": {"must": [%s]}}
 %s
}
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
 "size": 0
}
`
)

func (self *Indexer) getAllClients(
	ctx context.Context, in *api_proto.SearchClientsRequest) (
	*api_proto.SearchClientsResponse, error) {

	terms := []string{allClientsQuery}
	return self.searchWithTerms(ctx, in.Filter, terms, in.Offset, in.Limit)
}

const (
	fieldSearchQuery = `{"prefix": {%q: %q}}`
)

func (self *Indexer) searchClientsByLabel(
	ctx context.Context, operator, label string,
	in *api_proto.SearchClientsRequest,
	limit uint64) (*api_proto.SearchClientsResponse, error) {

	if in.NameOnly {
		return self.searchWithNames(ctx, "lower_labels", operator, label, in.Offset, in.Limit)
	}

	terms := []string{json.Format(fieldSearchQuery, "lower_labels", label)}
	return self.searchWithTerms(ctx, in.Filter, terms, in.Offset, in.Limit)
}

func (self *Indexer) searchClientsByHost(
	ctx context.Context, operator, hostname string,
	in *api_proto.SearchClientsRequest,
	limit uint64) (*api_proto.SearchClientsResponse, error) {

	if in.NameOnly {
		return self.searchWithPrefixedNames(ctx, "hostname",
			operator, hostname, in.Offset, in.Limit)
	}

	terms := []string{json.Format(fieldSearchQuery, "hostname", hostname)}
	return self.searchWithTerms(ctx, in.Filter, terms, in.Offset, in.Limit)
}

func (self *Indexer) searchClientsByMac(
	ctx context.Context, operator, mac string,
	in *api_proto.SearchClientsRequest,
	limit uint64) (*api_proto.SearchClientsResponse, error) {

	if in.NameOnly {
		return self.searchWithPrefixedNames(
			ctx, "mac_addresses", operator, mac, in.Offset, in.Limit)
	}

	terms := []string{json.Format(fieldSearchQuery, "mac_addresses", mac)}
	return self.searchWithTerms(ctx, in.Filter, terms, in.Offset, in.Limit)
}

func (self *Indexer) searchWithNames(
	ctx context.Context, field, operator, label string,
	offset, limit uint64) (*api_proto.SearchClientsResponse, error) {

	query := json.Format(getAllClientsAgg, field, offset, limit+1)

	hits, err := cvelo_services.QueryElasticAggregations(
		ctx, self.config_obj.OrgId, "clients", query)
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
	ctx context.Context, field, operator, term string,
	offset, limit uint64) (*api_proto.SearchClientsResponse, error) {

	prefix, filter := splitSearchTermIntoPrefixAndFilter(term)
	query := json.Format(
		getAllClientsQuery,
		json.Format(fieldSearchQuery, field, prefix),
		json.Format(limitQueryPart, offset, limit+1))

	hits, err := cvelo_services.QueryElasticRaw(
		ctx, self.config_obj.OrgId, "clients", query)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	result := &api_proto.SearchClientsResponse{}
	for _, hit := range hits {
		client_info := ordereddict.NewDict()
		err = json.Unmarshal(hit, client_info)
		if err != nil {
			continue
		}

		field_data, pres := client_info.GetString(field)
		if !pres {
			continue
		}

		if !strings.HasPrefix(field_data, prefix) {
			continue
		}

		if filter != nil && !filter.MatchString(field_data) {
			continue
		}

		_, pres = seen[field_data]
		if !pres {
			result.Names = append(
				result.Names, operator+":"+field_data)
		}
		seen[field_data] = true
	}
	return result, nil
}

func (self *Indexer) searchWithTerms(
	ctx context.Context,
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
		getAllClientsQuery, strings.Join(terms, ","),
		json.Format(limitQueryPart, offset, limit+1))

	hits, err := cvelo_services.QueryElasticRaw(
		ctx, self.config_obj.OrgId, "clients", query)
	if err != nil {
		return nil, err
	}

	result := &api_proto.SearchClientsResponse{}
	for _, hit := range hits {
		client_info := &actions_proto.ClientInfo{}
		err = json.Unmarshal(hit, client_info)
		if err == nil {
			client_info.Ping /= 1000
			result.Items = append(result.Items, _makeApiClient(client_info))
		}
	}

	return result, nil
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
		return self.getAllClients(ctx, in)

	case "label":
		return self.searchClientsByLabel(ctx, operator, term, in, limit)

	case "host":
		return self.searchClientsByHost(ctx, operator, term, in, limit)

	case "mac":
		return self.searchClientsByMac(ctx, operator, term, in, limit)

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
		return self.searchWithPrefixedNames(ctx, "client_id",
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
			ctx, "host", in.Query, in, in.Limit)
		if err == nil {
			terms = append(terms, res.Names...)
			items = append(items, res.Items...)
		}
	}

	// Maybe a label
	if uint64(len(terms)) < in.Limit {
		res, err := self.searchClientsByLabel(
			ctx, "label", in.Query, in, in.Limit)
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
