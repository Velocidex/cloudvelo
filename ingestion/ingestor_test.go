package ingestion

import (
	"context"
	"io/fs"
	"path"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	crypto_server "www.velocidex.com/golang/cloudvelo/crypto/server"
	ingestor_services "www.velocidex.com/golang/cloudvelo/ingestion/services"
	"www.velocidex.com/golang/cloudvelo/ingestion/testdata"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/hunt_dispatcher"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

const (
	getAllItemsQuery = `
{"query": {"match_all" : {}}, "size": 1000}`

	getAllItemsQueryForType = `{
    "query": {
        "bool": {
            "must": [
                {
                    "match": {
                        "doc_type": "vfs"
                    }
                }
            ]
        }
    },
    "size": 10000
}
`
	getCollectionQuery = `
{
  "size": 10000,
  "sort": [
  {
    "type": {"order": "asc"}
  }],
  "query": {
     "bool": {
       "must": [
         {"match": {"client_id" : %q}},
         {"match": {"session_id" : %q}},
         {"match": {"doc_type" : %q}}
      ]}
  }
}
`
)

type IngestionTestSuite struct {
	*testsuite.CloudTestSuite

	golden *ordereddict.Dict

	wg     *sync.WaitGroup
	ctx    context.Context
	cancel func()

	ingestor *Ingestor
}

func (self *IngestionTestSuite) ingestGoldenMessages(
	ctx context.Context, ingestor *Ingestor, prefix string) {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(1661385600, 0)))
	defer closer()

	files, err := testdata.FS.ReadDir(prefix)
	assert.NoError(self.T(), err)

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	for _, file := range files {
		data, err := fs.ReadFile(testdata.FS, path.Join(prefix, file.Name()))
		assert.NoError(self.T(), err)

		message := &crypto_proto.VeloMessage{}
		err = json.Unmarshal(data, message)
		assert.NoError(self.T(), err)

		message.OrgId = self.ConfigObj.OrgId

		err = ingestor.Process(ctx, message)
		assert.NoError(self.T(), err)
	}

	err = cvelo_services.FlushBulkIndexer()
	assert.NoError(self.T(), err)
}

func (self *IngestionTestSuite) TestEnrollment() {

	self.ingestGoldenMessages(self.ctx, self.ingestor, "Enrollment")

	client_id := "C.1352adc54e292a23"

	record, err := cvelo_services.GetElasticRecord(self.ctx,
		self.ConfigObj.OrgId, "persisted",
		utils.ClientIdFromConfigObj(client_id, self.ConfigObj.VeloConf())+"_key")
	assert.NoError(self.T(), err)
	self.golden.Set("Enrollment", record)

	// Replay the Client.Info.Updates monitoring messages these should
	// create a new client record (This is the equivalent of the old
	// interrogation flow but happens automatically now).
	self.ingestGoldenMessages(self.ctx, self.ingestor, "Server.Internal.ClientInfo")

	config_obj := self.ConfigObj.VeloConf()
	result, err := api.GetMultipleClients(self.ctx, config_obj, []string{client_id})
	assert.NoError(self.T(), err)
	self.golden.Set("ClientRecord", result[0])

	// Record results in monitoring data.
	records, _, err := cvelo_services.QueryElasticRaw(self.ctx,
		self.ConfigObj.OrgId, "transient", getAllItemsQuery)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(records))

	goldie.Assert(self.T(), "TestEnrollment",
		json.MustMarshalIndent(self.golden))
}

func (self *IngestionTestSuite) TestListDirectory() {
	// To keep things stable we mock the clock to be constant.
	closer := utils.MockTime(&utils.IncClock{NowTime: 1661391000})
	defer closer()

	client_id := "C.77ad4285690698d9"
	flow_id := "F.CEV6I8LHAT83O"

	// Test VFS.ListDirectory special handling.
	err := cvelo_services.SetElasticIndex(self.ctx,
		self.ConfigObj.OrgId, "transient", flow_id, api.ArtifactCollectorRecordFromProto(
			&flows_proto.ArtifactCollectorContext{
				ClientId:   client_id,
				SessionId:  flow_id,
				CreateTime: uint64(utils.GetTime().Now().UnixNano()),
			}, flow_id))

	self.ingestGoldenMessages(self.ctx, self.ingestor, "System.VFS.ListDirectory")
	records, _, err := cvelo_services.QueryElasticRaw(self.ctx,
		self.ConfigObj.OrgId, "transient",
		json.Format(getCollectionQuery, client_id, flow_id, "collection"))
	assert.NoError(self.T(), err)
	self.golden.Set("System.VFS.ListDirectory", records)

	records, _, err = cvelo_services.QueryElasticRaw(self.ctx,
		self.ConfigObj.OrgId, "transient", getAllItemsQuery)
	assert.NoError(self.T(), err)
	sort_records(records)
	self.golden.Set("System.VFS.ListDirectory Results", records)

	// Check the VFS entry for the top directory now - There should be
	// no downloads yet but a full directory listing.
	query := getAllItemsQueryForType
	records, _, err = cvelo_services.QueryElasticRaw(self.ctx,
		self.ConfigObj.OrgId, "transient", query)
	assert.NoError(self.T(), err)
	sort_records(records)
	self.golden.Set("System.VFS.ListDirectory vfs", records)

	// Now make sure the launcher service can reassemble the split
	// collections object from multiple records.
	config_obj := self.ConfigObj.VeloConf()

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	flow_details, err := launcher.GetFlowDetails(
		self.Ctx, config_obj, services.GetFlowOptions{},
		client_id, flow_id)
	assert.NoError(self.T(), err)

	self.golden.Set("System.VFS.ListDirectory FlowDetail",
		flow_details)

	goldie.Assert(self.T(), "TestListDirectory",
		json.MustMarshalIndent(self.golden))
}

func (self *IngestionTestSuite) TestVFSDownload() {

	// Replay the ListDirectory artifact messages to the ingestor.
	client_id := "C.77ad4285690698d9"
	list_flow_id := "F.CEV6I8LHAT83O"

	// Add a VFS.DownloadFile collection and replay messages.
	err := cvelo_services.SetElasticIndex(self.ctx, self.ConfigObj.OrgId,
		"transient", "", api.ArtifactCollectorRecordFromProto(
			&flows_proto.ArtifactCollectorContext{
				ClientId:   client_id,
				SessionId:  list_flow_id,
				CreateTime: uint64(utils.GetTime().Now().UnixNano()),
			}, list_flow_id))

	self.ingestGoldenMessages(self.Ctx, self.ingestor, "System.VFS.ListDirectory")

	// Now replay a download flow to fetch the file data. This should
	// add download records to the VFS index so the directory listing
	// below will include the data.
	download_flow_id := "F.CEV7IE8TURDBS"

	// Test VFS.ListDirectory special handling.
	err = cvelo_services.SetElasticIndex(self.ctx,
		self.ConfigObj.OrgId, "transient", "",
		api.ArtifactCollectorRecordFromProto(
			&flows_proto.ArtifactCollectorContext{
				ClientId:   client_id,
				SessionId:  download_flow_id,
				CreateTime: uint64(utils.GetTime().Now().UnixNano()),
			}, download_flow_id))

	self.ingestGoldenMessages(self.Ctx, self.ingestor, "System.VFS.DownloadFile")

	err = cvelo_services.FlushBulkIndexer()
	assert.NoError(self.T(), err)

	config_obj := self.ConfigObj.VeloConf()

	vfs_service, err := services.GetVFSService(config_obj)
	assert.NoError(self.T(), err)

	table, err := vfs_service.ListDirectoryFiles(self.Ctx,
		config_obj,
		&api_proto.GetTableRequest{
			ClientId:      client_id,
			FlowId:        list_flow_id,
			VfsComponents: []string{"auto", "test"},
			Rows:          1000,
			StartRow:      0,
		})
	assert.NoError(self.T(), err)

	goldie.Assert(self.T(), "TestVFSDownload", json.MustMarshalIndent(table))
}

func (self *IngestionTestSuite) TestClientEventMonitoring() {

	// Get Client Event Monitoring Clear the results so we get a clean
	// golden image.
	err := cvelo_services.DeleteByQuery(
		self.ctx, self.ConfigObj.OrgId, "transient", getAllItemsQuery)
	assert.NoError(self.T(), err)

	self.ingestGoldenMessages(self.ctx, self.ingestor, "Generic.Client.Stats")
	records, _, err := cvelo_services.QueryElasticRaw(self.ctx,
		self.ConfigObj.OrgId, "transient", getAllItemsQuery)
	assert.NoError(self.T(), err)
	sort_records(records)
	self.golden.Set("Generic.Client.Stats Results", records)

	goldie.Assert(self.T(), "TestClientEventMonitoring",
		json.MustMarshalIndent(self.golden))
}

func (self *IngestionTestSuite) TestUnknownFlow() {
	// To keep things stable we mock the clock to be constant.
	closer := utils.MockTime(&utils.IncClock{NowTime: 1661391000})
	defer closer()

	client_id := "C.77ad4285690698d9"
	flow_id := "F.CUNKNOWNFLOW1"

	self.seedCollection(client_id, flow_id)
	self.ingestGoldenMessages(self.Ctx, self.ingestor, "UnknownFlow")

	flow_details := self.getFlowDetails(client_id, flow_id)
	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR,
		flow_details.Context.State)
	assert.Equal(self.T(), "Flow not known - maybe the client crashed?",
		flow_details.Context.Status)

	// Only the query that was still running fails.
	assert.Equal(self.T(), int64(3), flow_details.Context.TotalRequests)
	assert.Equal(self.T(), uint64(12), flow_details.Context.TotalCollectedRows)
	assert.Equal(self.T(), uint64(2048), flow_details.Context.TotalUploadedBytes)
	assert.Equal(self.T(), crypto_proto.VeloStatus_OK,
		flow_details.Context.QueryStats[0].Status)
	assert.Equal(self.T(), crypto_proto.VeloStatus_OK,
		flow_details.Context.QueryStats[1].Status)
	assert.Equal(self.T(), crypto_proto.VeloStatus_GENERIC_ERROR,
		flow_details.Context.QueryStats[2].Status)

	self.golden.Set("UnknownFlow FlowDetail", flow_details)
	goldie.Assert(self.T(), "TestUnknownFlow",
		json.MustMarshalIndent(self.golden))
}

func (self *IngestionTestSuite) TestUnknownFlowHunt() {
	client_id := "C.77ad4285690698d9"
	flow_id := "F.CUNKNOWNFLOW2.H"
	hunt_id := "H.CUNKNOWNFLOW2"

	// Scope the hunt stats updater to this test so later tests see
	// the previous manager.
	previous := ingestor_services.HuntStatsManager
	ctx, cancel := context.WithCancel(self.Ctx)
	wg := &sync.WaitGroup{}
	defer func() {
		cancel()
		wg.Wait()
		ingestor_services.HuntStatsManager = previous
	}()

	err := ingestor_services.StartHuntStatsUpdater(
		ctx, wg, self.ConfigObj.VeloConf())
	assert.NoError(self.T(), err)

	err = cvelo_services.SetElasticIndex(self.Ctx,
		self.ConfigObj.OrgId, "persisted", hunt_id,
		&hunt_dispatcher.HuntEntry{
			HuntId:  hunt_id,
			DocType: "hunts",
		})
	assert.NoError(self.T(), err)

	self.seedCollection(client_id, flow_id)
	self.ingestGoldenMessages(self.Ctx, self.ingestor, "UnknownFlowHunt")

	flow_details := self.getFlowDetails(client_id, flow_id)
	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR,
		flow_details.Context.State)

	err = ingestor_services.HuntStatsManager.Update(hunt_id).Flush(self.Ctx)
	assert.NoError(self.T(), err)

	record, err := cvelo_services.GetElasticRecord(self.Ctx,
		self.ConfigObj.OrgId, "persisted", hunt_id)
	assert.NoError(self.T(), err)

	hunt_entry := &hunt_dispatcher.HuntEntry{}
	err = json.Unmarshal(record, hunt_entry)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), uint64(0), hunt_entry.Completed)
	assert.Equal(self.T(), uint64(0), hunt_entry.Errors)
}

func (self *IngestionTestSuite) TestUnknownFlowAfterFinished() {
	client_id := "C.77ad4285690698d9"
	flow_id := "F.CUNKNOWNFLOW3"

	self.seedCollection(client_id, flow_id)
	self.ingestGoldenMessages(self.Ctx, self.ingestor, "UnknownFlowFinished")

	flow_details := self.getFlowDetails(client_id, flow_id)
	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_FINISHED,
		flow_details.Context.State)

	records, _, err := cvelo_services.QueryElasticRaw(self.Ctx,
		self.ConfigObj.OrgId, "transient",
		json.Format(getCollectionQuery, client_id, flow_id, "collection"))
	assert.NoError(self.T(), err)

	// Only the seeded record and the completed stats.
	assert.Equal(self.T(), 2, len(records))
	for _, record := range records {
		assert.NotContains(self.T(), string(record), "Flow not known")
	}
}

func (self *IngestionTestSuite) TestUnknownFlowLookupFailure() {
	data, err := testdata.FS.ReadFile("UnknownFlow/Msg_02.json")
	assert.NoError(self.T(), err)

	message := &crypto_proto.VeloMessage{}
	err = json.Unmarshal(data, message)
	assert.NoError(self.T(), err)

	// The flow lookup fails, which must not fail the rest of the
	// client's batch.
	ctx, cancel := context.WithCancel(self.Ctx)
	cancel()

	err = self.ingestor.HandleFlowStats(ctx, self.ConfigObj.VeloConf(), message)
	assert.NoError(self.T(), err)
}

func (self *IngestionTestSuite) seedCollection(client_id, flow_id string) {
	err := cvelo_services.SetElasticIndex(self.Ctx,
		self.ConfigObj.OrgId, "transient", cvelo_services.DocIdRandom,
		api.ArtifactCollectorRecordFromProto(
			&flows_proto.ArtifactCollectorContext{
				ClientId:   client_id,
				SessionId:  flow_id,
				CreateTime: uint64(utils.GetTime().Now().UnixNano()),
			}, flow_id))
	assert.NoError(self.T(), err)
}

func (self *IngestionTestSuite) getFlowDetails(
	client_id, flow_id string) *api_proto.FlowDetails {
	config_obj := self.ConfigObj.VeloConf()

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	flow_details, err := launcher.GetFlowDetails(
		self.Ctx, config_obj, services.GetFlowOptions{},
		client_id, flow_id)
	assert.NoError(self.T(), err)

	return flow_details
}

func (self *IngestionTestSuite) SetupTest() {
	self.CloudTestSuite.SetupTest()

	closer := utils.MockTime(&utils.IncClock{NowTime: 1661391000})
	defer closer()

	self.golden = ordereddict.NewDict()

	self.wg = &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	self.cancel = cancel

	config_obj := self.ConfigObj.VeloConf()

	//	cvelo_services.SetDebugLogger(config_obj)

	crypto_manager, err := crypto_server.NewServerCryptoManager(
		ctx, config_obj, self.wg)
	assert.NoError(self.T(), err)

	self.ingestor, err = NewIngestor(self.ConfigObj, crypto_manager)
	assert.NoError(self.T(), err)
}

func (self *IngestionTestSuite) TearDownTest() {
	self.CloudTestSuite.TearDownTest()

	self.cancel()
	self.wg.Wait()
}

func TestIngestor(t *testing.T) {
	suite.Run(t, &IngestionTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{},
	})
}

func sort_records(records []json.RawMessage) {
	sort.Slice(records, func(i, j int) bool {
		lhs := string(records[i])
		rhs := string(records[j])
		return lhs < rhs
	})
}
