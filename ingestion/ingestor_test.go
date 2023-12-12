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
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	crypto_server "www.velocidex.com/golang/cloudvelo/crypto/server"
	"www.velocidex.com/golang/cloudvelo/ingestion/testdata"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
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

		message.OrgId = "test"

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
		"test", "client_keys", client_id+"-test")
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
		"test", "results", getAllItemsQuery)
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
		"test", "results", flow_id, api.ArtifactCollectorRecordFromProto(
			&flows_proto.ArtifactCollectorContext{
				ClientId:   client_id,
				SessionId:  flow_id,
				CreateTime: uint64(utils.GetTime().Now().UnixNano()),
			}, flow_id))

	self.ingestGoldenMessages(self.ctx, self.ingestor, "System.VFS.ListDirectory")
	records, _, err := cvelo_services.QueryElasticRaw(self.ctx,
		"test", "results",
		json.Format(getCollectionQuery, client_id, flow_id, "collection"))
	assert.NoError(self.T(), err)
	self.golden.Set("System.VFS.ListDirectory", records)

	records, _, err = cvelo_services.QueryElasticRaw(self.ctx,
		"test", "results", getAllItemsQuery)
	assert.NoError(self.T(), err)
	sort_records(records)
	self.golden.Set("System.VFS.ListDirectory Results", records)

	// Check the VFS entry for the top directory now - There should be
	// no downloads yet but a full directory listing.
	query := getAllItemsQueryForType
	records, _, err = cvelo_services.QueryElasticRaw(self.ctx,
		"test", "results", query)
	assert.NoError(self.T(), err)
	sort_records(records)
	self.golden.Set("System.VFS.ListDirectory vfs", records)

	// Now make sure the launcher service can reassemble the split
	// collections object from multiple records.
	config_obj := self.ConfigObj.VeloConf()

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	flow_details, err := launcher.GetFlowDetails(
		self.Ctx, config_obj, client_id, flow_id)
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
	err := cvelo_services.SetElasticIndex(self.ctx, "test",
		"results", "", api.ArtifactCollectorRecordFromProto(
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
		"test", "results", "",
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
		self.ctx, "test", "results", getAllItemsQuery)
	assert.NoError(self.T(), err)

	self.ingestGoldenMessages(self.ctx, self.ingestor, "Generic.Client.Stats")
	records, _, err := cvelo_services.QueryElasticRaw(self.ctx,
		"test", "results", getAllItemsQuery)
	assert.NoError(self.T(), err)
	sort_records(records)
	self.golden.Set("Generic.Client.Stats Results", records)

	goldie.Assert(self.T(), "TestClientEventMonitoring",
		json.MustMarshalIndent(self.golden))
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
		CloudTestSuite: &testsuite.CloudTestSuite{
			Indexes: []string{"clients", "results", "hunts"},
		},
	})
}

func sort_records(records []json.RawMessage) {
	sort.Slice(records, func(i, j int) bool {
		lhs := string(records[i])
		rhs := string(records[j])
		return lhs < rhs
	})
}
