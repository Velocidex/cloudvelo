package filestore_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/filestore"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type recordedRequest struct {
	Method        string
	Path          string
	Range         string
	Authorization string
}

type fakeS3 struct {
	mu       sync.Mutex
	requests []recordedRequest
	body     string
}

func (self *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	self.mu.Lock()
	self.requests = append(self.requests, recordedRequest{
		Method:        r.Method,
		Path:          r.URL.Path,
		Range:         r.Header.Get("Range"),
		Authorization: r.Header.Get("Authorization"),
	})
	self.mu.Unlock()

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Range", "bytes 0-3/4")
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusPartialContent)
		io.WriteString(w, self.body)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (self *fakeS3) last() recordedRequest {
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.requests[len(self.requests)-1]
}

func newConfig(endpoint string) *config.Config {
	endpoint = strings.Replace(endpoint, "127.0.0.1", "localhost", 1)
	result := &config.Config{}
	result.Config = config_proto.Config{OrgId: "O123"}
	result.Cloud.Endpoint = endpoint
	result.Cloud.Bucket = "contract-bucket"
	result.Cloud.AWSRegion = "eu-west-3"
	result.Cloud.CredentialsKey = "AKIDCONTRACT"
	result.Cloud.CredentialsSecret = "secret"
	return result
}

func readFrom(t *testing.T, config_obj *config.Config) (string, error) {
	store, err := filestore.NewS3Filestore(context.Background(), config_obj)
	assert.NoError(t, err)

	reader, err := store.ReadFile(path_specs.NewUnsafeFilestorePath("a", "b"))
	assert.NoError(t, err)

	buf := make([]byte, 4)
	n, err := reader.Read(buf)
	return string(buf[:n]), err
}

func TestS3ConfigRequestShape(t *testing.T) {
	fake := &fakeS3{body: "data"}
	server := httptest.NewServer(fake)
	defer server.Close()

	got, err := readFrom(t, newConfig(server.URL))
	assert.NoError(t, err)
	assert.Equal(t, "data", got)

	request := fake.last()
	assert.Equal(t, http.MethodGet, request.Method)
	assert.Equal(t, "/contract-bucket/orgs/O123/a/b.json", request.Path)
	assert.Equal(t, "bytes=0-3", request.Range)
	assert.True(t, strings.HasPrefix(request.Authorization, "AWS4-HMAC-SHA256 "),
		"not SigV4: %v", request.Authorization)
	assert.True(t, strings.Contains(request.Authorization,
		"Credential=AKIDCONTRACT/"), "wrong key: %v", request.Authorization)
	assert.True(t, strings.Contains(request.Authorization,
		"/eu-west-3/s3/aws4_request"), "wrong region: %v", request.Authorization)

	store, err := filestore.NewS3Filestore(context.Background(), newConfig(server.URL))
	assert.NoError(t, err)
	assert.NoError(t, store.Delete(path_specs.NewUnsafeFilestorePath("a", "b")))

	request = fake.last()
	assert.Equal(t, http.MethodDelete, request.Method)
	assert.Equal(t, "/contract-bucket/orgs/O123/a/b.json", request.Path)
}

func TestS3ConfigNoVerifyCert(t *testing.T) {
	fake := &fakeS3{body: "data"}
	server := httptest.NewTLSServer(fake)
	defer server.Close()

	config_obj := newConfig(server.URL)
	config_obj.Cloud.NoVerifyCert = true
	got, err := readFrom(t, config_obj)
	assert.NoError(t, err)
	assert.Equal(t, "data", got)

	config_obj = newConfig(server.URL)
	_, err = readFrom(t, config_obj)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "certificate"),
		"expected a certificate error, got %v", err)
}

func TestS3ConfigDefaultCredentialChain(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDFROMENV")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_CONFIG_FILE", "/nonexistent")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/nonexistent")

	fake := &fakeS3{body: "data"}
	server := httptest.NewServer(fake)
	defer server.Close()

	config_obj := newConfig(server.URL)
	config_obj.Cloud.CredentialsKey = ""
	config_obj.Cloud.CredentialsSecret = ""

	_, err := readFrom(t, config_obj)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(fake.last().Authorization,
		"Credential=AKIDFROMENV/"), "env credentials not used: %v",
		fake.last().Authorization)
}

func TestS3ReadServerErrorIsReturned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
		}))
	defer server.Close()

	got, err := readFrom(t, newConfig(server.URL))
	assert.Equal(t, "", got)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "AccessDenied"),
		"expected AccessDenied, got %v", err)
}

type fakeMultipartS3 struct {
	mu       sync.Mutex
	partHdrs []http.Header
}

func (self *fakeMultipartS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.Copy(io.Discard, r.Body)
	query := r.URL.Query()
	switch {
	case r.Method == http.MethodPost && query.Has("uploads"):
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<InitiateMultipartUploadResult><Bucket>b</Bucket><Key>k</Key><UploadId>U1</UploadId></InitiateMultipartUploadResult>`)
	case r.Method == http.MethodPut && query.Has("partNumber"):
		self.mu.Lock()
		self.partHdrs = append(self.partHdrs, r.Header.Clone())
		self.mu.Unlock()
		w.Header().Set("ETag", `"abc"`)
	case r.Method == http.MethodPost && query.Has("uploadId"):
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<CompleteMultipartUploadResult><Location>l</Location><Bucket>b</Bucket><Key>k</Key><ETag>"abc-1"</ETag></CompleteMultipartUploadResult>`)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func TestS3WriterSendsNoChecksumHeaders(t *testing.T) {
	fake := &fakeMultipartS3{}
	server := httptest.NewServer(fake)
	defer server.Close()

	store, err := filestore.NewS3Filestore(context.Background(), newConfig(server.URL))
	assert.NoError(t, err)

	writer, err := store.WriteFile(path_specs.NewUnsafeFilestorePath("a", "b"))
	assert.NoError(t, err)
	_, err = writer.Write([]byte("some data"))
	assert.NoError(t, err)
	assert.NoError(t, writer.Close())

	assert.Equal(t, 1, len(fake.partHdrs))
	for name := range fake.partHdrs[0] {
		lower := strings.ToLower(name)
		assert.False(t, strings.HasPrefix(lower, "x-amz-checksum") ||
			lower == "x-amz-sdk-checksum-algorithm" || lower == "x-amz-trailer",
			"unexpected checksum header %v", name)
	}
	assert.NotEqual(t, "aws-chunked", fake.partHdrs[0].Get("Content-Encoding"))
}
