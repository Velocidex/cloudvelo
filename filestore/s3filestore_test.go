package filestore_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type S3FilestoreTest struct {
	*testsuite.CloudTestSuite
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
			Indexes: []string{"clients"},
		},
	})
}
