package datastoretest

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
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

func (self *DatastoreTest) TestDownloadQuery() {

	var serialized = "\"{\"timestamp\":1715614088,\"components\":[\"downloads\",\"hunts\",\"H.CP12M5IRRKRUE\",\"H.CP12M5IRRKRUE.zip\"],\"type\":\"zip\"}\""
	record := datastore.DatastoreRecord{
		ID:        cvelo_services.MakeId("/downloads/hunts/H.CP12M5IRRKRUE/H.CP12M5IRRKRUE.json.db"),
		Type:      "Generic",
		VFSPath:   "/downloads/hunts/H.CP12M5IRRKRUE/H.CP12M5IRRKRUE.json.db",
		JSONData:  serialized,
		DocType:   "datastore",
		Timestamp: utils.GetTime().Now().UnixNano(),
	}

	cvelo_services.SetElasticIndex(self.ctx,
		"test", "transient", "", record)
	var vfs_path = "/downloads/hunts/H.CP12M5IRRKRUE/H.CP12M5IRRKRUE.json.db"
	serialized = "{\"timestamp\":1715614088,\"total_uncompressed_bytes\":21375,\"total_compressed_bytes\":20054,\"total_container_files\":18,\"hash\":\"095079e35e17c37bd5ac6f602a15173697f404a2ac9ea4b1f54c653ab25706e2\",\"total_duration\":1,\"components\":[\"downloads\",\"hunts\",\"H.CP12M5IRRKRUE\",\"H.CP12M5IRRKRUE.zip\"],\"type\":\"zip\"}"
	record = datastore.DatastoreRecord{
		ID:        cvelo_services.MakeId(vfs_path),
		Type:      "Generic",
		VFSPath:   vfs_path,
		JSONData:  serialized,
		DocType:   "datastore",
		Timestamp: utils.GetTime().Now().UnixNano(),
	}
	cvelo_services.SetElasticIndex(self.ctx,
		"test", "transient", "", record)

	id := cvelo_services.MakeId(vfs_path)
	_, err := cvelo_services.GetElasticRecord(
		self.ctx, self.OrgId, "transient", id)

	assert.NoError(self.T(), err)

	_, _, err = cvelo_services.QueryElasticRaw(self.ctx, self.OrgId,
		"transient", json.Format(get_datastore_doc_query, cvelo_services.MakeId("/downloads/hunts/H.CP12M5IRRKRUE/H.CP12M5IRRKRUE.json.db")))
	assert.NoError(self.T(), err)
}

func TestDataStore(t *testing.T) {
	suite.Run(t, &DatastoreTest{
		CloudTestSuite: &testsuite.CloudTestSuite{
			Indexes: []string{"transient"},
		},
	})
}
