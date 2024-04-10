package filestore_test

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/cloudvelo/filestore"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type S3FilestoreTest struct {
	*testsuite.CloudTestSuite
}

func (self *S3FilestoreTest) checkForKey() []string {
	// Check the actual path in the bucket we are in.
	session, err := filestore.GetS3Session(self.ConfigObj)
	assert.NoError(self.T(), err)

	svc := s3.New(session)
	res, err := svc.ListObjects(&s3.ListObjectsInput{
		Bucket: &self.ConfigObj.Cloud.Bucket,
	})
	assert.NoError(self.T(), err)

	result := []string{}
	for _, c := range res.Contents {
		if c.Key != nil && strings.Contains(*c.Key, "file") {
			result = append(result, *c.Key)
		}
	}

	return result
}

func (self *S3FilestoreTest) TestS3FileWriting() {
	config_obj := self.ConfigObj.VeloConf()
	file_store_factory := file_store.GetFileStore(config_obj)
	assert.NotNil(self.T(), file_store_factory)

	test_file := path_specs.NewUnsafeFilestorePath("Test", "file")
	writer, err := file_store_factory.WriteFile(test_file)
	assert.NoError(self.T(), err)

	err = writer.Truncate()
	assert.NoError(self.T(), err)

	_, err = writer.Write([]byte("hello"))
	assert.NoError(self.T(), err)

	writer.Close()

	// Read the file back
	reader, err := file_store_factory.ReadFile(test_file)
	assert.NoError(self.T(), err)

	data, err := ioutil.ReadAll(reader)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), "hello", string(data))

	// Read small ranges
	reader.Seek(1, os.SEEK_SET)
	buff := make([]byte, 2)
	_, err = reader.Read(buff)
	assert.NoError(self.T(), err)

	// A partial range read
	assert.Equal(self.T(), "el", string(buff))

	// Make sure the underlying key name reflects the org name in it
	keys := self.checkForKey()
	assert.Equal(self.T(), 1, len(keys))
	assert.Equal(self.T(), "orgs/test/Test/file.json", keys[0])

	// Now delete the file.
	err = file_store_factory.Delete(test_file)
	assert.NoError(self.T(), err)

	reader, err = file_store_factory.ReadFile(test_file)
	assert.NoError(self.T(), err)

	// No data is available
	data, err = ioutil.ReadAll(reader)
	assert.Error(self.T(), err)

}

func TestS3Filestore(t *testing.T) {
	suite.Run(t, &S3FilestoreTest{
		CloudTestSuite: &testsuite.CloudTestSuite{
			Indexes: []string{"persisted"},
		},
	})
}
