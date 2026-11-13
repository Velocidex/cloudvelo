package indexing_test

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/indexing"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

const (
	onlineClient  = "C.1000000000000001"
	offlineClient = "C.1000000000000002"
	plainClient   = "C.1000000000000003"
	emptiedClient = "C.1000000000000004"
	unknownClient = "C.ffffffffffffffff"
)

type SearchTestSuite struct {
	*testsuite.CloudTestSuite
}

func (self *SearchTestSuite) SetupTest() {
	self.CloudTestSuite.SetupTest()

	// The ONLINE filter compares against the wall clock, not the
	// mocked suite clock.
	now := uint64(time.Now().UnixNano())

	self.seed(onlineClient, api.ClientRecord{
		ClientId: onlineClient, Hostname: "alpha-1",
		Fqdn: "alpha-1.corp.example.com",
		Type: "main", DocType: "clients"})
	self.seed(onlineClient+"_ping", api.ClientRecord{
		ClientId: onlineClient, Ping: now,
		Type: "ping", DocType: "clients"})
	self.seed(onlineClient+"_labels", api.ClientRecord{
		ClientId: onlineClient, Labels: []string{"Prod"},
		LowerLabels: []string{"prod"}, DocType: "clients"})
	self.seed(onlineClient+"_interrogate", api.ClientRecord{
		ClientId: onlineClient, LastInterrogate: "F.1234",
		Type: "interrogation", DocType: "clients"})

	self.seed(offlineClient, api.ClientRecord{
		ClientId: offlineClient, Hostname: "alpha-2",
		Type: "main", DocType: "clients"})
	self.seed(offlineClient+"_ping", api.ClientRecord{
		ClientId: offlineClient,
		Ping:     now - uint64(72*time.Hour),
		Type:     "ping", DocType: "clients"})
	self.seed(offlineClient+"_labels", api.ClientRecord{
		ClientId: offlineClient, Labels: []string{"Prod"},
		LowerLabels: []string{"prod"}, DocType: "clients"})

	self.seed(plainClient, api.ClientRecord{
		ClientId: plainClient, Hostname: "beta-1",
		Type: "main", DocType: "clients"})
}

// The ping, label and interrogation documents must not be counted as
// extra clients.
func (self *SearchTestSuite) TestDefaultViewCountsMainRecordsOnly() {
	self.assertSearch("all", api_proto.SearchClientsRequest_UNFILTERED, 3, 3)
}

func (self *SearchTestSuite) TestOnlineFilter() {
	self.assertSearch("all", api_proto.SearchClientsRequest_ONLINE, 1, 1)
}

func (self *SearchTestSuite) TestHostOperator() {
	self.assertSearch("host:alpha", api_proto.SearchClientsRequest_UNFILTERED, 2, 2)
}

func (self *SearchTestSuite) TestLabelOperator() {
	self.assertSearch("label:prod", api_proto.SearchClientsRequest_UNFILTERED, 2, 2)
}

func (self *SearchTestSuite) TestClientOperator() {
	self.assertSearch("client:"+onlineClient,
		api_proto.SearchClientsRequest_UNFILTERED, 1, 1)
	self.assertSearch("client:"+unknownClient,
		api_proto.SearchClientsRequest_UNFILTERED, 0, 0)
}

// A hostname search returning fewer rows than the limit must report
// the hostname total.
func (self *SearchTestSuite) TestFreeTextHostname() {
	self.assertSearch("alpha", api_proto.SearchClientsRequest_UNFILTERED, 2, 2)
}

func (self *SearchTestSuite) TestFreeTextLabel() {
	self.assertSearch("prod", api_proto.SearchClientsRequest_UNFILTERED, 2, 2)
}

func (self *SearchTestSuite) TestFreeTextClientId() {
	self.assertSearch(onlineClient, api_proto.SearchClientsRequest_UNFILTERED, 1, 1)
	self.assertSearch(unknownClient, api_proto.SearchClientsRequest_UNFILTERED, 0, 0)
}

func (self *SearchTestSuite) TestFreeTextNoMatch() {
	self.assertSearch("gamma", api_proto.SearchClientsRequest_UNFILTERED, 0, 0)
}

func (self *SearchTestSuite) TestApiClientFqdn() {
	config_obj := self.ConfigObj.VeloConf()

	indexer, err := services.GetIndexer(config_obj)
	assert.NoError(self.T(), err)

	client, err := indexer.FastGetApiClient(self.Ctx, config_obj, onlineClient)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), "alpha-1", client.OsInfo.Hostname)
	assert.Equal(self.T(), "alpha-1.corp.example.com", client.OsInfo.Fqdn)

	// Indexed before the fqdn field existed.
	client, err = indexer.FastGetApiClient(self.Ctx, config_obj, offlineClient)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), "alpha-2", client.OsInfo.Fqdn)
}

func (self *SearchTestSuite) TestLabelWildcards() {
	for _, query := range []string{
		"label:*", "label:", "label:pro", "label:p*d", "label:*ROD",
		"label:p*o", "label:*r*o", "label:*RO"} {
		self.assertSearchIds(query, onlineClient, offlineClient)
	}
	self.assertSearchIds("label:x*")
	self.assertSearchIds("label:p*x")
	self.assertSearchIds("label:ro*")
	self.assertSearchIds("label:pro?")
}

func (self *SearchTestSuite) TestHostWildcards() {
	self.assertSearchIds("host:A*", onlineClient, offlineClient)
	self.assertSearchIds("host:*-1", onlineClient, plainClient)
	self.assertSearchIds("host:a*2", offlineClient)
	self.assertSearchIds("host:al*a", onlineClient, offlineClient)
	self.assertSearchIds("host:*et*", plainClient)
}

func (self *SearchTestSuite) TestLabelNone() {
	self.seedEmptiedLabels()
	self.assertSearchIds("label:none", plainClient, emptiedClient)
	assert.Equal(self.T(), sorted(plainClient, emptiedClient),
		sorted(self.searchChanIds("label:none")...))
}

func (self *SearchTestSuite) TestLabelNonePagesLabelledClients() {
	defer indexing.SetLabelledClientsLimits(1, 10)()
	self.assertSearchIds("label:none", plainClient)
}

func (self *SearchTestSuite) TestLabelNoneFailsAboveTermsLimit() {
	defer indexing.SetLabelledClientsLimits(1, 1)()

	_, err := self.search("label:none", false)
	assert.ErrorContains(self.T(), err, "at most 1 labelled clients")

	config_obj := self.ConfigObj.VeloConf()
	indexer, err := services.GetIndexer(config_obj)
	assert.NoError(self.T(), err)
	_, err = indexer.SearchClientsChan(self.Ctx, nil, config_obj, "label:none", "")
	assert.ErrorContains(self.T(), err, "at most 1 labelled clients")
}

func (self *SearchTestSuite) TestLabelNameSuggestionWildcards() {
	result, err := self.search("label:P*d", true)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), []string{"label:Prod"}, result.Names)
}

func (self *SearchTestSuite) seedEmptiedLabels() {
	self.seed(emptiedClient, api.ClientRecord{
		ClientId: emptiedClient, Hostname: "beta-2",
		Type: "main", DocType: "clients"})
	self.seed(emptiedClient+"_labels", map[string]interface{}{
		"client_id": emptiedClient, "labels": []string{},
		"lower_labels": []string{}, "doc_type": "clients"})
}

func (self *SearchTestSuite) search(query string, name_only bool) (
	*api_proto.SearchClientsResponse, error) {
	config_obj := self.ConfigObj.VeloConf()

	indexer, err := services.GetIndexer(config_obj)
	assert.NoError(self.T(), err)
	return indexer.SearchClients(self.Ctx, config_obj,
		&api_proto.SearchClientsRequest{
			Query:    query,
			Limit:    10,
			NameOnly: name_only,
		}, "admin")
}

func (self *SearchTestSuite) assertSearchIds(query string, expected ...string) {
	result, err := self.search(query, false)
	assert.NoError(self.T(), err)

	ids := []string{}
	for _, item := range result.Items {
		ids = append(ids, item.ClientId)
	}
	assert.Equal(self.T(), sorted(expected...), sorted(ids...), "Items for %q", query)
	assert.Equal(self.T(), uint64(len(expected)), result.Total, "Total for %q", query)
}

func (self *SearchTestSuite) searchChanIds(query string) []string {
	config_obj := self.ConfigObj.VeloConf()

	indexer, err := services.GetIndexer(config_obj)
	assert.NoError(self.T(), err)

	search_chan, err := indexer.SearchClientsChan(
		self.Ctx, nil, config_obj, query, "")
	assert.NoError(self.T(), err)

	ids := []string{}
	for client := range search_chan {
		ids = append(ids, client.ClientId)
	}
	return ids
}

func sorted(ids ...string) []string {
	result := append([]string{}, ids...)
	sort.Strings(result)
	return result
}

func (self *SearchTestSuite) seed(id string, record interface{}) {
	err := cvelo_services.SetElasticIndex(
		self.Ctx, self.OrgId, "persisted", id, record)
	assert.NoError(self.T(), err)
}

func (self *SearchTestSuite) assertSearch(query string,
	filter api_proto.SearchClientsRequest_Filters,
	expected_total uint64, expected_items int) {

	config_obj := self.ConfigObj.VeloConf()

	indexer, err := services.GetIndexer(config_obj)
	assert.NoError(self.T(), err)

	result, err := indexer.SearchClients(self.Ctx, config_obj,
		&api_proto.SearchClientsRequest{
			Query:  query,
			Filter: filter,
			Limit:  10,
		}, "admin")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), expected_total, result.Total, "Total for %q", query)
	assert.Equal(self.T(), expected_items, len(result.Items), "Items for %q", query)
}

func TestSearch(t *testing.T) {
	suite.Run(t, &SearchTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{},
	})
}
