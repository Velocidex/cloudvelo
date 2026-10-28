package indexing_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

const (
	onlineClient  = "C.1000000000000001"
	offlineClient = "C.1000000000000002"
	plainClient   = "C.1000000000000003"
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

func (self *SearchTestSuite) seed(id string, record api.ClientRecord) {
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
