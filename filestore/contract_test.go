package filestore_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/cloudvelo/filestore"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type S3ContractTest struct {
	*testsuite.CloudTestSuite
	store *filestore.S3Filestore
}

func (self *S3ContractTest) SetupTest() {
	self.CloudTestSuite.SetupTest()

	store, err := filestore.NewS3Filestore(self.Ctx, self.ConfigObj)
	assert.NoError(self.T(), err)
	self.store = store
}

func (self *S3ContractTest) TearDownTest() {
	client := self.inspector()
	listed, err := client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket: aws.String(self.ConfigObj.Cloud.Bucket),
		Prefix: aws.String("orgs/" + self.OrgId + "/"),
	})
	assert.NoError(self.T(), err)
	for _, object := range listed.Contents {
		_, err := client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
			Bucket: aws.String(self.ConfigObj.Cloud.Bucket),
			Key:    object.Key,
		})
		assert.NoError(self.T(), err)
	}
	self.CloudTestSuite.TearDownTest()
}

func (self *S3ContractTest) inspector() *s3.Client {
	cloud := self.ConfigObj.Cloud
	return s3.New(s3.Options{
		Region:       cloud.AWSRegion,
		BaseEndpoint: aws.String(cloud.Endpoint),
		UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider(
			cloud.CredentialsKey, cloud.CredentialsSecret, ""),
	})
}

func (self *S3ContractTest) key(path api.FSPathSpec) string {
	return "orgs/" + self.OrgId + "/" +
		strings.Join(path.Components(), "/") + ".json"
}

func (self *S3ContractTest) head(path api.FSPathSpec) (*s3.HeadObjectOutput, error) {
	return self.inspector().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(self.ConfigObj.Cloud.Bucket),
		Key:    aws.String(self.key(path)),
	})
}

func (self *S3ContractTest) write(path api.FSPathSpec, chunks ...[]byte) {
	writer, err := self.store.WriteFile(path)
	assert.NoError(self.T(), err)
	for _, chunk := range chunks {
		n, err := writer.Write(chunk)
		assert.NoError(self.T(), err)
		assert.Equal(self.T(), len(chunk), n)
	}
	assert.NoError(self.T(), writer.Close())
}

func (self *S3ContractTest) readAll(path api.FSPathSpec) []byte {
	reader, err := self.store.ReadFile(path)
	assert.NoError(self.T(), err)
	data, err := ioutil.ReadAll(reader)
	assert.NoError(self.T(), err)
	return data
}

func isNotFound(err error) bool {
	var api_err smithy.APIError
	return errors.As(err, &api_err) &&
		(api_err.ErrorCode() == "NotFound" || api_err.ErrorCode() == "NoSuchKey")
}

func (self *S3ContractTest) TestObjectKeyAndMetadata() {
	path := path_specs.NewUnsafeFilestorePath("Contract", "small")
	self.write(path, []byte("hello"))

	head, err := self.head(path)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), "application/binary", aws.ToString(head.ContentType))
	assert.Equal(self.T(), int64(5), aws.ToInt64(head.ContentLength))
	assert.Equal(self.T(), "hello", string(self.readAll(path)))
}

func (self *S3ContractTest) TestMultipartRoundTrip() {
	self.ConfigObj.Cloud.S3PartSize = 5 * 1024 * 1024
	store, err := filestore.NewS3Filestore(self.Ctx, self.ConfigObj)
	assert.NoError(self.T(), err)
	self.store = store

	rng := rand.New(rand.NewSource(1))
	var chunks [][]byte
	var all []byte
	for i := 0; i < 12; i++ {
		chunk := make([]byte, 1024*1024)
		rng.Read(chunk)
		chunks = append(chunks, chunk)
		all = append(all, chunk...)
	}
	tail := []byte("tail")
	chunks = append(chunks, tail)
	all = append(all, tail...)

	path := path_specs.NewUnsafeFilestorePath("Contract", "multipart")
	self.write(path, chunks...)

	head, err := self.head(path)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), int64(len(all)), aws.ToInt64(head.ContentLength))
	assert.True(self.T(), strings.HasSuffix(strings.Trim(aws.ToString(head.ETag), `"`), "-3"),
		"expected a 3 part upload, got ETag %v", aws.ToString(head.ETag))

	want := sha256.Sum256(all)
	got := sha256.Sum256(self.readAll(path))
	assert.Equal(self.T(), want, got)
}

func (self *S3ContractTest) TestReadRanges() {
	path := path_specs.NewUnsafeFilestorePath("Contract", "ranges")
	self.write(path, []byte("0123456789"))

	reader, err := self.store.ReadFile(path)
	assert.NoError(self.T(), err)

	var reads []string
	buf := make([]byte, 3)
	for {
		n, err := reader.Read(buf)
		if err == io.EOF {
			assert.Equal(self.T(), 0, n)
			break
		}
		assert.NoError(self.T(), err)
		reads = append(reads, string(buf[:n]))
	}
	assert.Equal(self.T(), []string{"012", "345", "678", "9"}, reads)

	_, err = reader.Seek(8, io.SeekStart)
	assert.NoError(self.T(), err)
	buf = make([]byte, 5)
	n, err := reader.Read(buf)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), "89", string(buf[:n]))

	for _, offset := range []int64{10, 20} {
		_, err = reader.Seek(offset, io.SeekStart)
		assert.NoError(self.T(), err)
		n, err = reader.Read(buf)
		assert.Equal(self.T(), 0, n, "offset %v", offset)
		assert.Equal(self.T(), io.EOF, err, "offset %v", offset)
	}

	stat, err := reader.Stat()
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), int64(10), stat.Size())
	assert.Equal(self.T(), "ranges", stat.Name())
}

func (self *S3ContractTest) TestMissingObject() {
	path := path_specs.NewUnsafeFilestorePath("Contract", "missing")

	reader, err := self.store.ReadFile(path)
	assert.NoError(self.T(), err)

	n, err := reader.Read(make([]byte, 4))
	assert.Equal(self.T(), 0, n)
	assert.Error(self.T(), err)
	assert.NotEqual(self.T(), io.EOF, err)

	_, err = reader.Stat()
	assert.Error(self.T(), err)
}

func (self *S3ContractTest) TestListDirectory() {
	dir := path_specs.NewUnsafeFilestorePath("Contract", "list")
	self.write(dir.AddChild("a"), []byte("a"))
	self.write(dir.AddChild("b"), []byte("bb"))
	self.write(dir.AddChild("sub").AddChild("c"), []byte("ccc"))

	before := time.Now().Add(-time.Hour)
	entries, err := self.store.ListDirectory(dir)
	assert.NoError(self.T(), err)

	var got []string
	for _, entry := range entries {
		got = append(got, fmt.Sprintf("%s:%s:%d", entry.Name(),
			strings.Join(entry.PathSpec().Components(), "/"), entry.Size()))
		assert.Equal(self.T(), api.PATH_TYPE_FILESTORE_JSON, entry.PathSpec().Type())
		assert.True(self.T(), entry.ModTime().After(before))
		assert.False(self.T(), entry.IsDir())
	}
	sort.Strings(got)
	assert.Equal(self.T(), []string{
		"a:Contract/list/a:1",
		"b:Contract/list/b:2",
		"c:Contract/list/c:3",
	}, got)

	entries, err = self.store.ListDirectory(
		path_specs.NewUnsafeFilestorePath("Contract", "empty"))
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 0, len(entries))
}

func (self *S3ContractTest) TestDelete() {
	path := path_specs.NewUnsafeFilestorePath("Contract", "deleted")
	self.write(path, []byte("gone"))

	assert.NoError(self.T(), self.store.Delete(path))
	_, err := self.head(path)
	assert.True(self.T(), isNotFound(err), "expected not found, got %v", err)

	assert.NoError(self.T(), self.store.Delete(path))
}

func (self *S3ContractTest) TestTruncateReplacesObject() {
	path := path_specs.NewUnsafeFilestorePath("Contract", "truncated")
	self.write(path, []byte("old contents"))

	writer, err := self.store.WriteFile(path)
	assert.NoError(self.T(), err)
	assert.NoError(self.T(), writer.Truncate())

	_, err = self.head(path)
	assert.True(self.T(), isNotFound(err), "expected not found, got %v", err)

	_, err = writer.Write([]byte("new"))
	assert.NoError(self.T(), err)
	assert.NoError(self.T(), writer.Close())

	assert.Equal(self.T(), "new", string(self.readAll(path)))
}

func (self *S3ContractTest) TestWriterWithNoData() {
	path := path_specs.NewUnsafeFilestorePath("Contract", "nodata")

	writer, err := self.store.WriteFile(path)
	assert.NoError(self.T(), err)

	n, err := writer.Write(nil)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 0, n)

	assert.Error(self.T(), writer.Close())

	_, err = self.head(path)
	assert.True(self.T(), isNotFound(err), "expected not found, got %v", err)
}

func (self *S3ContractTest) TestUnsupportedOperations() {
	path := path_specs.NewUnsafeFilestorePath("Contract", "unsupported")

	_, err := self.store.StatFile(path)
	assert.Error(self.T(), err)
	assert.Error(self.T(), self.store.Move(path, path))

	writer, err := self.store.WriteFile(path)
	assert.NoError(self.T(), err)
	assert.Error(self.T(), writer.Update([]byte("x"), 0))
	_, err = writer.WriteCompressed([]byte("x"), 0, 1)
	assert.Error(self.T(), err)
	assert.True(self.T(), bytes.Equal(nil, self.readAllOrNil(path)))
}

func (self *S3ContractTest) readAllOrNil(path api.FSPathSpec) []byte {
	reader, err := self.store.ReadFile(path)
	if err != nil {
		return nil
	}
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil
	}
	return data
}

func TestS3Contract(t *testing.T) {
	suite.Run(t, &S3ContractTest{
		CloudTestSuite: &testsuite.CloudTestSuite{OrgId: "test_filestore_contract"},
	})
}
