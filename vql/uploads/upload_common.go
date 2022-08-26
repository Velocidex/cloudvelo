package uploads

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
	"www.velocidex.com/golang/velociraptor/accessors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/json"
)

// Initial request specifies the destination and receives an upload
// key.
type UploadRequest struct {
	// A path where the file should be stored inside the bucket.
	Components []string `json:"components"`
}

// Key to be used for subsequent requests.
type UploadResponse struct {
	Key      string `json:"key"`
	UploadId string `json:"upload_id"`
}

// PUT HTTP operations:
type UploadPutRequest struct {
	Key      string `json:"key"`
	UploadId string `json:"upload_id"`
	Part     int    `json:"partNumber"`
}

type UploadCompletionRequest struct {
	Key      string              `json:"key"`
	UploadId string              `json:"upload_id"`
	Parts    []*s3.CompletedPart `json:"parts"`
}

type Uploader struct {
	client *http.Client
	ctx    context.Context

	// The key for multipart uploads.
	key       string
	upload_id string

	commit bool

	start_url, put_url, commit_url string

	md5_sum hash.Hash
	sha_sum hash.Hash
	offset  uint64
	part    uint64

	parts []*s3.CompletedPart

	exe *executor.ClientExecutor

	file_store_path api.FSPathSpec

	session_id string
	mtime      time.Time
	atime      time.Time
	ctime      time.Time
	btime      time.Time

	session_tracker *SessionTracker

	owner *UploaderFactory
}

// Initiate the upload request and get a key for the parts.
func (self *Uploader) Start(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", self.start_url,
		strings.NewReader(json.MustMarshalString(&UploadRequest{
			Components: self.file_store_path.Components(),
		})))
	if err != nil {
		return err
	}

	req.Header["Content-Type"] = []string{"application/json"}

	resp, err := self.client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	serialized, err := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return fmt.Errorf("Unable to connect: %v: %v\n",
			resp.Status, string(serialized))
	}

	upload_response := &UploadResponse{}
	err = json.Unmarshal(serialized, upload_response)
	if err != nil {
		return err
	}

	// Remember the key
	self.key = upload_response.Key
	self.upload_id = upload_response.UploadId

	return nil
}

func (self *Uploader) Put(buf []byte) error {
	self.md5_sum.Write(buf)
	self.sha_sum.Write(buf)
	self.offset += uint64(len(buf))

	request := &UploadPutRequest{
		Key:      self.key,
		UploadId: self.upload_id,
		Part:     int(self.part),
	}

	// Write the buffer using a PUT request.
	req, err := http.NewRequestWithContext(
		self.ctx, http.MethodPut,
		fmt.Sprintf("%s?payload=%s",
			self.put_url,
			base64.StdEncoding.EncodeToString(
				json.MustMarshalIndent(request))),
		bytes.NewBuffer(buf))
	if err != nil {
		return err
	}

	resp, err := self.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	serialized, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("Unable to put part: %v: %v\n",
			resp.Status, string(serialized))
	}

	self.part++

	completed_part := &s3.CompletedPart{}
	err = json.Unmarshal(serialized, completed_part)
	if err != nil {
		return err
	}
	self.parts = append(self.parts, completed_part)
	return nil
}

func (self *Uploader) Commit() {
	self.commit = true
}

func (self *Uploader) Close() error {

	// Write the buffer using a PUT request.
	req, err := http.NewRequestWithContext(
		self.ctx, http.MethodPost,
		self.commit_url,
		strings.NewReader(json.MustMarshalString(
			&UploadCompletionRequest{
				Key:      self.key,
				UploadId: self.upload_id,
				Parts:    self.parts,
			})))
	if err != nil {
		return err
	}

	resp, err := self.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	serialized, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("Unable to put part: %v: %v\n",
			resp.Status, string(serialized))
	}

	// Send the server a message that this file was uploaded.
	message := &crypto_proto.VeloMessage{
		SessionId: self.session_id,
		FileBuffer: &actions_proto.FileBuffer{
			Pathspec: &actions_proto.PathSpec{
				Path: self.file_store_path.AsClientPath(),
			},
			// This is the s3 path that was used.
			Reference:    self.file_store_path.AsClientPath(),
			Size:         self.offset,
			StoredSize:   self.offset,
			Eof:          true,
			UploadNumber: int64(self.owner.ReturnTracker(self.session_tracker)),
		},
	}

	if !self.mtime.IsZero() {
		message.FileBuffer.Mtime = self.mtime.Unix()
	}

	if !self.atime.IsZero() {
		message.FileBuffer.Atime = self.atime.Unix()
	}

	if !self.ctime.IsZero() {
		message.FileBuffer.Ctime = self.ctime.Unix()
	}

	if !self.btime.IsZero() {
		message.FileBuffer.Btime = self.btime.Unix()
	}

	if self.exe != nil {
		self.exe.SendToServer(message)
	}

	json.Dump(message)

	return nil
}

func (self *UploaderFactory) NewUploader(
	ctx context.Context,
	session_id, accessor string,
	path *accessors.OSPath) (*Uploader, error) {
	result := &Uploader{
		start_url:  fmt.Sprintf("http://localhost:%d/upload/start", self.port),
		put_url:    fmt.Sprintf("http://localhost:%d/upload/put", self.port),
		commit_url: fmt.Sprintf("http://localhost:%d/upload/commit", self.port),
		client: &http.Client{
			Timeout: time.Duration(60) * time.Second,
		},
		md5_sum:         md5.New(),
		sha_sum:         sha256.New(),
		part:            1,
		ctx:             ctx,
		exe:             self.exe,
		session_tracker: self.GetTracker(session_id),
		owner:           self,
		file_store_path: path_specs.NewUnsafeFilestorePath(
			"clients", self.client_id, "collections",
			session_id, "uploads", accessor).
			AddChild(path.Components...).
			SetType(api.PATH_TYPE_FILESTORE_ANY),
	}

	return result, result.Start(ctx)
}
