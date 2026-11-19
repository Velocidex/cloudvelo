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

	onlineUnlabelledClient = "C.1000000000000005"
	pingedRecentlyClient   = "C.1000000000000006"
	pingedLongAgoClient    = "C.1000000000000007"
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
		Fqdn: "alpha-1.corp.example.com", System: "windows",
		MacAddresses: []string{"00:11:22:33:44:55"},
		Type:         "main", DocType: "clients"})
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
		ClientId: offlineClient, Hostname: "alpha-2", System: "windows",
		MacAddresses: []string{"00:11:22:33:44:66"},
		Type:         "main", DocType: "clients"})
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

func (self *SearchTestSuite) TestOnlineFilterKeepsQuery() {
	self.seedOnlineUnlabelled()
	for _, query := range []string{
		"host:alpha", "alpha", "label:prod", "label:*", "prod",
		"os:win", "mac:00:11"} {
		self.assertOnlineIds(query, onlineClient)
	}
	self.assertOnlineIds("host:beta", onlineUnlabelledClient)
	self.assertOnlineIds("host:gamma")
	self.assertOnlineIds("all", onlineClient, onlineUnlabelledClient)
}

func (self *SearchTestSuite) TestOnlineFilterKeepsSortedHostSearch() {
	self.seedOnlineUnlabelled()
	result, err := self.searchWith(&api_proto.SearchClientsRequest{
		Query: "all", Filter: api_proto.SearchClientsRequest_ONLINE,
		Sort: api_proto.SearchClientsRequest_SORT_DOWN, Limit: 10})
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), []string{onlineUnlabelledClient, onlineClient},
		itemIds(result))
	assert.Equal(self.T(), uint64(2), result.Total)
}

func (self *SearchTestSuite) TestOnlineFilterLabelNone() {
	self.seedOnlineUnlabelled()
	self.assertOnlineIds("label:none", onlineUnlabelledClient)
}

func (self *SearchTestSuite) TestOnlineFilterClientOperator() {
	self.assertOnlineIds("client:"+onlineClient, onlineClient)
	self.assertOnlineIds("client:" + offlineClient)
	self.assertOnlineIds(offlineClient)
}

func (self *SearchTestSuite) TestOnlineFilterRecent() {
	for _, client_id := range []string{onlineClient, offlineClient} {
		self.seed("admin:"+client_id, indexing.MRUItem{
			Username: "admin", ClientId: client_id,
			Timestamp: time.Now().Unix(), DocType: "user_mru"})
	}
	self.assertSearchIds("recent:", onlineClient, offlineClient)
	self.assertOnlineIds("recent:", onlineClient)
}

func (self *SearchTestSuite) TestOnlineFilterWindowIsFifteenMinutes() {
	now := time.Now()
	for client_id, ping := range map[string]time.Time{
		pingedRecentlyClient: now.Add(-12 * time.Minute),
		pingedLongAgoClient:  now.Add(-20 * time.Minute),
	} {
		self.seed(client_id, api.ClientRecord{
			ClientId: client_id, Hostname: "gamma-" + client_id,
			Type: "main", DocType: "clients"})
		self.seed(client_id+"_ping", api.ClientRecord{
			ClientId: client_id, Ping: uint64(ping.UnixNano()),
			Type: "ping", DocType: "clients"})
	}
	self.assertOnlineIds("host:gamma", pingedRecentlyClient)
	self.assertOnlineIds("client:"+pingedRecentlyClient, pingedRecentlyClient)
	self.assertOnlineIds("client:" + pingedLongAgoClient)
}

func (self *SearchTestSuite) TestOnlineFilterWithNoOnlineClients() {
	self.seed(onlineClient+"_ping", api.ClientRecord{
		ClientId: onlineClient,
		Ping:     uint64(time.Now().Add(-72 * time.Hour).UnixNano()),
		Type:     "ping", DocType: "clients"})
	self.assertOnlineIds("all")
	self.assertOnlineIds("host:alpha")
}

func (self *SearchTestSuite) TestOnlineFilterPagesOnlineClients() {
	self.seedOnlineUnlabelled()
	defer indexing.SetLabelledClientsLimits(1, 10)()
	self.assertOnlineIds("all", onlineClient, onlineUnlabelledClient)
}

func (self *SearchTestSuite) TestOnlineFilterFailsAboveTermsLimit() {
	self.seedOnlineUnlabelled()
	defer indexing.SetLabelledClientsLimits(1, 1)()

	for _, query := range []string{"host:alpha", "label:prod", "recent:"} {
		_, err := self.searchWith(&api_proto.SearchClientsRequest{
			Query: query, Filter: api_proto.SearchClientsRequest_ONLINE,
			Limit: 10})
		assert.ErrorContains(self.T(), err, "at most 1 online clients", query)
	}
}

func (self *SearchTestSuite) seedOnlineUnlabelled() {
	self.seed(onlineUnlabelledClient, api.ClientRecord{
		ClientId: onlineUnlabelledClient, Hostname: "beta-3",
		System: "linux", MacAddresses: []string{"aa:bb:cc:dd:ee:ff"},
		Type: "main", DocType: "clients"})
	self.seed(onlineUnlabelledClient+"_ping", api.ClientRecord{
		ClientId: onlineUnlabelledClient, Ping: uint64(time.Now().UnixNano()),
		Type: "ping", DocType: "clients"})
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

func (self *SearchTestSuite) searchWith(in *api_proto.SearchClientsRequest) (
	*api_proto.SearchClientsResponse, error) {
	config_obj := self.ConfigObj.VeloConf()

	indexer, err := services.GetIndexer(config_obj)
	assert.NoError(self.T(), err)
	return indexer.SearchClients(self.Ctx, config_obj, in, "admin")
}

func (self *SearchTestSuite) assertSearchIds(query string, expected ...string) {
	self.assertFilteredIds(query, api_proto.SearchClientsRequest_UNFILTERED, expected...)
}

func (self *SearchTestSuite) assertOnlineIds(query string, expected ...string) {
	self.assertFilteredIds(query, api_proto.SearchClientsRequest_ONLINE, expected...)
}

func (self *SearchTestSuite) assertFilteredIds(query string,
	filter api_proto.SearchClientsRequest_Filters, expected ...string) {
	result, err := self.searchWith(&api_proto.SearchClientsRequest{
		Query: query, Filter: filter, Limit: 10})
	assert.NoError(self.T(), err, query)

	assert.Equal(self.T(), sorted(expected...), sorted(itemIds(result)...),
		"Items for %q filter %v", query, filter)
	assert.Equal(self.T(), uint64(len(expected)), result.Total,
		"Total for %q filter %v", query, filter)
}

func itemIds(result *api_proto.SearchClientsResponse) []string {
	ids := []string{}
	for _, item := range result.Items {
		ids = append(ids, item.ClientId)
	}
	return ids
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
