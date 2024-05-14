package datastoretest

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"os"
	"testing"
	"www.velocidex.com/golang/cloudvelo/datastore"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	get_datastore_doc_query = `
{"sort": {"timestamp": {"order": "desc"}},
 "size": 1,
    "query": {
        "bool": {
            "must": [
                {
                    "prefix": {
                        "id": %q
                    }
                },
                {
                    "match": {
                        "doc_type": "datastore"
                    }
                }
            ]
        }
    }
}`
)

type DatastoreTest struct {
	*testsuite.CloudTestSuite
	ctx context.Context
}

func (self *DatastoreTest) TestDownloadQueriesOnTransientIndex() {
	var vfs_path = "/downloads/hunts/H.CP12M5IRRKRUE/H.CP12M5IRRKRUE.json.db"
	var serialized = "\"{\"timestamp\":1715614088,\"components\":[\"downloads\",\"hunts\",\"H.CP12M5IRRKRUE\",\"H.CP12M5IRRKRUE.zip\"],\"type\":\"zip\"}\""
	record := datastore.DatastoreRecord{
		ID:        cvelo_services.MakeId(vfs_path),
		Type:      "Generic",
		VFSPath:   vfs_path,
		JSONData:  serialized,
		DocType:   "datastore",
		Timestamp: utils.GetTime().Now().UnixNano(),
	}

	err := cvelo_services.SetElasticIndex(self.ctx,
		"test", "transient", "", record)

	serialized = "{\"timestamp\":1715614088,\"total_uncompressed_bytes\":21375,\"total_compressed_bytes\":20054,\"total_container_files\":18,\"hash\":\"095079e35e17c37bd5ac6f602a15173697f404a2ac9ea4b1f54c653ab25706e2\",\"total_duration\":1,\"components\":[\"downloads\",\"hunts\",\"H.CP12M5IRRKRUE\",\"H.CP12M5IRRKRUE.zip\"],\"type\":\"zip\"}"
	record = datastore.DatastoreRecord{
		ID:        cvelo_services.MakeId(vfs_path),
		Type:      "Generic",
		VFSPath:   vfs_path,
		JSONData:  serialized,
		DocType:   "datastore",
		Timestamp: utils.GetTime().Now().UnixNano(),
	}
	err = cvelo_services.SetElasticIndex(self.ctx,
		"test", "transient", "", record)

	id := cvelo_services.MakeId(vfs_path)
	hit, err := cvelo_services.GetElasticRecord(
		self.ctx, "test", "transient", id)
	assert.Nil(self.T(), hit)
	if assert.Error(self.T(), err) {
		assert.Equal(self.T(), os.ErrNotExist, err)
	}
	result, _, err := cvelo_services.QueryElasticRaw(self.ctx, "test",
		"transient", json.Format(get_datastore_doc_query, cvelo_services.MakeId(vfs_path)))

	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(result))
}
func TestDataStore(t *testing.T) {
	suite.Run(t, &DatastoreTest{
		CloudTestSuite: &testsuite.CloudTestSuite{
			Indexes: []string{"transient"},
		},
	})
}
