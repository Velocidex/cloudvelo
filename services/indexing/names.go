// Search functions for name only searches (These are used for the
// quick suggestion box).

package indexing

import (
	"context"
	"strings"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

func (self *Indexer) searchClientNameOnly(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest) (*api_proto.SearchClientsResponse, error) {

	operator, term := splitIntoOperatorAndTerms(in.Query)
	switch operator {
	case "label":
		return self.searchWithNames(ctx, config_obj,
			"labels", operator, term, in.Offset, in.Limit)

	case "host":
		return self.searchWithNames(ctx, config_obj,
			"hostname", operator, term, in.Offset, in.Limit)

	default:
		return self.searchVerbsWithNames(ctx, config_obj,
			"hostname", operator, term, in.Offset, in.Limit)
	}
}

// Autocomplete the allowed verbs
func (self *Indexer) searchVerbsWithNames(
	ctx context.Context,
	config_obj *config_proto.Config,
	field, operator, term string,
	offset, limit uint64) (*api_proto.SearchClientsResponse, error) {

	res := &api_proto.SearchClientsResponse{}

	// Complete verbs first
	term = strings.ToLower(term)
	for _, verb := range verbs {
		if term == "?" ||
			strings.HasPrefix(verb, term) {
			res.Names = append(res.Names, verb)
		}
	}

	return res, nil
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

		term := hit
		if operator != "" {
			term = operator + ":" + hit
		}
		result.Names = append(result.Names, term)

	}
	return result, nil
}
