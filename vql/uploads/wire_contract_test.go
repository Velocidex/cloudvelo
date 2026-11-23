package uploads

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

// The bytes the v1 upload server sends back for each part.
const v1PartResponse = `{
 "ChecksumCRC32": null,
 "ChecksumCRC32C": null,
 "ChecksumSHA1": null,
 "ChecksumSHA256": null,
 "ETag": "\"etag-PART\"",
 "PartNumber": PART
}`

type fakeUploadServer struct {
	mu     sync.Mutex
	parts  int
	commit []byte
}

func (self *fakeUploadServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	self.mu.Lock()
	defer self.mu.Unlock()

	body, _ := ioutil.ReadAll(r.Body)
	switch r.URL.Path {
	case "/upload/start":
		w.Write([]byte(`{"key":"orgs/O1/clients/C.1/k","upload_id":"U1"}`))
	case "/upload/put":
		self.parts++
		part := string(rune('0' + self.parts))
		w.Write([]byte(strings.ReplaceAll(v1PartResponse, "PART", part)))
	case "/upload/commit":
		self.commit = body
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestUploaderWireContract(t *testing.T) {
	fake := &fakeUploadServer{}
	server := httptest.NewServer(fake)
	defer server.Close()

	config_obj := config.GetDefaultConfig()
	ctx := context.Background()
	resp := responder.TestResponderWithFlowId(config_obj, "F.1")
	defer resp.Close()

	uploader := &VeloCloudUploader{
		client:     http.DefaultClient,
		ctx:        ctx,
		config_obj: config_obj,
		start_url:  server.URL + "/upload/start",
		put_url:    server.URL + "/upload/put",
		commit_url: server.URL + "/upload/commit",
		md5_sum:    md5.New(),
		sha_sum:    sha256.New(),
		Responder:  resp,
		path:       accessors.MustNewLinuxOSPath("/etc/hostname"),
		accessor:   "file",
		client_id:  "C.1",
	}

	assert.NoError(t, uploader.Start(ctx))
	assert.NoError(t, uploader.Put([]byte("part one")))
	assert.NoError(t, uploader.Put([]byte("part two")))
	assert.NoError(t, uploader.Close())

	var commit struct {
		Key      string                       `json:"key"`
		UploadId string                       `json:"upload_id"`
		Parts    []map[string]json.RawMessage `json:"parts"`
	}
	assert.NoError(t, json.Unmarshal(fake.commit, &commit), string(fake.commit))
	assert.Equal(t, "orgs/O1/clients/C.1/k", commit.Key)
	assert.Equal(t, "U1", commit.UploadId)
	assert.Equal(t, 2, len(commit.Parts), string(fake.commit))

	for i, part := range commit.Parts {
		number := string(rune('1' + i))
		assert.Equal(t, `"\"etag-`+number+`\""`, string(part["ETag"]), string(fake.commit))
		assert.Equal(t, number, string(part["PartNumber"]), string(fake.commit))
		for name, value := range part {
			if name != "ETag" && name != "PartNumber" {
				assert.Equal(t, "null", string(value),
					"unexpected non-null field %v in %s", name, fake.commit)
			}
		}
	}
}
