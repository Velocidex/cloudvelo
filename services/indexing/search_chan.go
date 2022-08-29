package indexing

// Implement client searching with channel based API

import (
	"context"
	"errors"
	"regexp"
	"strings"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"
)

const getAllClientsQueryChan = `
{"query": {"match_all" : {}}}
`

func (self *Indexer) getAllClientsChan(
	ctx context.Context, scope vfilter.Scope,
	config_obj *config_proto.Config) (chan *api_proto.ApiClient, error) {
	terms := []string{allClientsQuery}
	return self.searchWithTermsChan(ctx, config_obj, terms)
}

func (self *Indexer) searchClientsByMacChan(
	ctx context.Context,
	config_obj *config_proto.Config,
	mac string) (chan *api_proto.ApiClient, error) {

	terms := []string{json.Format(fieldSearchQuery, "mac_addresses", mac)}
	return self.searchWithTermsChan(ctx, config_obj, terms)
}

func (self *Indexer) searchClientsByHostChan(
	ctx context.Context,
	config_obj *config_proto.Config,
	hostname string) (chan *api_proto.ApiClient, error) {

	terms := []string{json.Format(fieldSearchQuery, "hostname", hostname)}
	return self.searchWithTermsChan(ctx, config_obj, terms)
}

func (self *Indexer) searchClientsByLabelChan(
	ctx context.Context,
	config_obj *config_proto.Config,
	label string) (chan *api_proto.ApiClient, error) {

	terms := []string{json.Format(fieldSearchQuery, "lower_labels", label)}
	return self.searchWithTermsChan(ctx, config_obj, terms)
}

func (self *Indexer) searchWithTermsChan(
	ctx context.Context,
	config_obj *config_proto.Config,
	terms []string) (chan *api_proto.ApiClient, error) {

	output_chan := make(chan *api_proto.ApiClient)

	go func() {
		defer close(output_chan)

		query := json.Format(
			getAllClientsQuery, strings.Join(terms, ","), "")

		hits, err := cvelo_services.QueryChan(ctx, config_obj, 1000,
			config_obj.OrgId, "clients", query, "")
		if err != nil {
			return
		}

		for hit := range hits {
			client_info := &actions_proto.ClientInfo{}
			err = json.Unmarshal(hit, client_info)
			if err == nil {
				client_info.Ping *= 1000000
				output_chan <- _makeApiClient(client_info)
			}
		}
	}()

	return output_chan, nil
}

func (self *Indexer) SearchClientsChan(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	search_term string, principal string) (chan *api_proto.ApiClient, error) {

	operator, term := splitIntoOperatorAndTerms(search_term)
	switch operator {
	case "all":
		return self.getAllClientsChan(ctx, scope, config_obj)

	case "label":
		return self.searchClientsByLabelChan(ctx, config_obj, term)

	case "", "host":
		return self.searchClientsByHostChan(ctx, config_obj, term)

	case "mac":
		return self.searchClientsByMacChan(ctx, config_obj, term)

	default:
		return nil, errors.New("Invalid search operator " + operator)
	}
}

// When searching the index, the user may provide wild cards.
func splitSearchTermIntoPrefixAndFilter(search_term string) (string, *regexp.Regexp) {

	parts := strings.Split(search_term, "*")
	// No wild cards present
	if len(parts) == 1 {
		return search_term, nil
	}

	// Last component is a wildcard, just ignore it (e.g. win* )
	if len(parts) == 2 && parts[1] == "" {
		return parts[0], nil
	}

	// Try to interpret the filter as a glob
	filter_regex := "(?i)" + glob.FNmatchTranslate(search_term)
	filter, err := regexp.Compile(filter_regex)
	if err != nil {
		return parts[0], nil
	}

	return parts[0], filter
}
