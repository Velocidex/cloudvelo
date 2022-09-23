package ingestion

import (
	"context"
	"io/fs"
	"sort"
	"strings"
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
	cvelo_utils "www.velocidex.com/golang/cloudvelo/utils"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

const getAllItemsQuery = `
{"query": {"match_all" : {}}}
`

type IngestionTestSuite struct {
	*testsuite.CloudTestSuite
}

func (self *IngestionTestSuite) ingestGoldenMessages(
	ctx context.Context, ingestor *Ingestor, prefix string) {
	files, err := testdata.FS.ReadDir(".")
	assert.NoError(self.T(), err)

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	for _, file := range files {
		if strings.HasPrefix(file.Name(), prefix) {
			data, err := fs.ReadFile(testdata.FS, file.Name())
			assert.NoError(self.T(), err)

			message := &crypto_proto.VeloMessage{}
			err = json.Unmarshal(data, message)
			assert.NoError(self.T(), err)

			message.OrgId = "test"

			err = ingestor.Process(ctx, message)
			assert.NoError(self.T(), err)
		}
	}
}

func (self *IngestionTestSuite) TestIngestor() {
	cvelo_utils.Clock = &utils.MockClock{
		MockNow: time.Unix(1661391000, 0),
	}

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config_obj := self.ConfigObj.VeloConf()

	//	cvelo_services.SetDebugLogger(config_obj)

	crypto_manager, err := crypto_server.NewServerCryptoManager(
		ctx, config_obj, wg)
	assert.NoError(self.T(), err)

	ingestor, err := NewIngestor(self.ConfigObj, crypto_manager)
	assert.NoError(self.T(), err)

	self.ingestGoldenMessages(ctx, ingestor, "Enrollment")

	golden := ordereddict.NewDict()
	record, err := cvelo_services.GetElasticRecord(ctx,
		"test", "client_keys", "C.1352adc54e292a23-test")
	assert.NoError(self.T(), err)
	golden.Set("Enrollment", record)

	// Replay the Client.Info.Updates monitoring messages these should
	// create a new client record (This is the equivalent of the old
	// interrogation flow but happens automatically now).
	self.ingestGoldenMessages(ctx, ingestor, "Client.Info.Updates")

	record, err = cvelo_services.GetElasticRecord(ctx,
		"test", "clients", "C.1352adc54e292a23")
	assert.NoError(self.T(), err)
	golden.Set("ClientRecord", record)

	// We do not record any results though.
	records, err := cvelo_services.QueryElasticRaw(ctx,
		"test", "results", getAllItemsQuery)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 0, len(records))

	// Test VFS.ListDirectory special handling.
	err = cvelo_services.SetElasticIndex(ctx, "test",
		"collections", "F.CCMS0OJQ7LI36", &api.ArtifactCollectorContext{
			ClientId:   "C.1352adc54e292a23",
			SessionId:  "F.CCMS0OJQ7LI36",
			QueryStats: []string{},
		})

	self.ingestGoldenMessages(ctx, ingestor, "System.VFS.ListDirectory")
	record, err = cvelo_services.GetElasticRecord(ctx,
		"test", "collections", "F.CCMS0OJQ7LI36")
	assert.NoError(self.T(), err)
	golden.Set("System.VFS.ListDirectory", record)

	records, err = cvelo_services.QueryElasticRaw(ctx,
		"test", "results", getAllItemsQuery)
	assert.NoError(self.T(), err)
	golden.Set("System.VFS.ListDirectory Results", records)

	// Test VFS.DownloadFile special handling.
	err = cvelo_services.SetElasticIndex(ctx, "test",
		"collections", "F.CCM9S0N2QR4H8", &api.ArtifactCollectorContext{
			ClientId:   "C.1352adc54e292a23",
			SessionId:  "F.CCM9S0N2QR4H8",
			QueryStats: []string{},
		})

	self.ingestGoldenMessages(ctx, ingestor, "System.VFS.DownloadFile")
	record, err = cvelo_services.GetElasticRecord(ctx,
		"test", "collections", "F.CCM9S0N2QR4H8")
	assert.NoError(self.T(), err)
	golden.Set("System.VFS.DownloadFile", record)

	goldie.Assert(self.T(), "TestIngestor", json.MustMarshalIndent(golden))
}

func TestIngestor(t *testing.T) {
	suite.Run(t, &IngestionTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{
			Indexes: []string{"clients", "client_keys", "results", "collections"},
		},
	})
}
