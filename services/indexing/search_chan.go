package indexing

// Implement client searching with channel based API

import (
	"context"
	"errors"
	"regexp"
	"strings"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/vfilter"
)

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

		query := json.Format(strings.TrimSpace(getAllClientsQuery),
			strings.Join(terms, ","), "")

		// Page the query in parts. First part specifies the size.
		part_query := `{"size":1000,` + query[1:]
		hits, err := cvelo_services.QueryElasticIds(
			ctx, config_obj.OrgId, "clients", part_query)
		if err != nil || len(hits) == 0 {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("searchWithTermsChan: %v", err)
			return
		}

		for {
			clients, err := searchClientsFromHits(ctx, config_obj, hits, "", nil)
			if err != nil {
				logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
				logger.Error("searchWithTermsChan: %v", err)
				return
			}

			last_client_id := ""

			for _, c := range clients.Items {
				last_client_id = c.ClientId
				select {
				case <-ctx.Done():
					return
				case output_chan <- c:
				}
			}

			// Get the next batch
			hits, err = cvelo_services.QueryElasticIds(
				ctx, config_obj.OrgId, "clients",
				json.Format(`{"search_after":[%q],`, last_client_id)+part_query[1:])
			if err != nil || len(hits) == 0 {
				logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
				logger.Error("searchWithTermsChan: %v", err)
				return
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
