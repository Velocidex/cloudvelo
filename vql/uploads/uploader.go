// This uploader communicates with the server's HTTP server to
// implement an upload protocol similar to S3 multi part sequence:

// 1. First call the /start endpoint to establish the multipart upload.
// 2. Next call /put to upload specific parts.
// 3. Finally call /commit to commit the upload.

package uploads

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
	"www.velocidex.com/golang/velociraptor/accessors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
)

const (
	EOF = true
)

// Initial request specifies the destination and receives an upload
// key.
type UploadRequest struct {
	ClientId  string `json:"client_id"`
	SessionId string `json:"session_id"`
	Accessor  string `json:"accessor"`

	// The components of the path on the client.
	Components []string `json:"components"`

	// Type idx is an index.
	Type string `json:"type"`
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

type VeloCloudUploader struct {
	mu sync.Mutex

	client     *http.Client
	ctx        context.Context
	config_obj *config_proto.Config

	// The key for multipart uploads.
	key       string
	upload_id string

	// The number of the upload in the collection.
	upload_number int64

	commit bool

	start_url, put_url, commit_url string

	md5_sum hash.Hash
	sha_sum hash.Hash
	size    int64
	offset  uint64
	part    uint64

	parts []*s3.CompletedPart

	Responder responder.Responder

	// The token is used to authenticate to the upload endpoints. It
	// consists of a normal empty cipher_text just like the regular
	// POST data but the server can use it to verify the caller.
	token string

	path *accessors.OSPath

	client_id string
	accessor  string
	mtime     time.Time
	atime     time.Time
	ctime     time.Time
	btime     time.Time

	// This indicates which type of file it is:
	// 1. "idx" is an index file
	// empty is a regular file.
	uploader_type string

	response *uploads.UploadResponse
	closed   bool

	manager crypto.ICryptoManager
	index   *actions_proto.Index
}

func (self *VeloCloudUploader) SetIndex(index *actions_proto.Index) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if index == nil || len(index.Ranges) == 0 {
		return
	}

	self.index = index

	// Update the size to be the end of the last range
	last_run := index.Ranges[len(index.Ranges)-1]
	self.size = last_run.OriginalOffset + last_run.Length
}

func (self *VeloCloudUploader) GetVQLResponse() *uploads.UploadResponse {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.response != nil {
		return self.response
	}

	return &uploads.UploadResponse{
		Path:       self.path.String(),
		StoredName: self.path.String(),
		Size:       self.offset,
		StoredSize: self.offset,
		Sha256:     hex.EncodeToString(self.sha_sum.Sum(nil)),
		Md5:        hex.EncodeToString(self.md5_sum.Sum(nil)),
	}
}

func (self *VeloCloudUploader) New(
	ctx context.Context,
	scope vfilter.Scope,
	// How the file will be stored on the server
	dest *accessors.OSPath,

	// What file we will open and send
	accessor string,
	name *accessors.OSPath,

	mtime, atime, ctime, btime time.Time,
	size int64, // Expected size.
	uploader_type string) (CloudUploader, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	if dest == nil {
		dest = name
	}

	// We need a responder as we will be sending FileBuffer messages
	// directly.
	responder_any, pres := scope.GetContext("_Responder")
	if !pres {
		return nil, errors.New("Responder not found")
	}

	responder, ok := responder_any.(responder.Responder)
	if !ok {
		return nil, errors.New("Responder not found")
	}

	// Use a standard VeloMessage to create a token with which we can
	// communicate with the server. The server will be able to verify
	// this token using its private key.
	server_name := "VelociraptorServer"
	if self.config_obj.Client != nil &&
		self.config_obj.Client.PinnedServerName != "" {
		server_name = self.config_obj.Client.PinnedServerName
	}

	cipher_text, err := self.manager.Encrypt(
		nil, crypto_proto.PackedMessageList_UNCOMPRESSED,
		self.config_obj.Client.Nonce, server_name)
	if err != nil {
		return nil, err
	}

	// Create a new uploader based on the present one but initialized
	// for a new upload.
	result := &VeloCloudUploader{
		// Carry over old information
		config_obj: self.config_obj,
		start_url:  self.start_url,
		put_url:    self.put_url,
		commit_url: self.commit_url,
		client:     self.client,
		client_id:  self.client_id,

		// Manage the new file
		md5_sum:       md5.New(),
		sha_sum:       sha256.New(),
		part:          1,
		ctx:           ctx,
		mtime:         mtime,
		atime:         atime,
		ctime:         ctime,
		btime:         btime,
		accessor:      accessor,
		path:          dest,
		uploader_type: uploader_type,
		Responder:     responder,
		size:          size,
		token:         base64.StdEncoding.EncodeToString(cipher_text),
	}

	return result, result.Start(ctx)
}

func GetURL(config_obj *config_proto.Config) (string, error) {
	if config_obj.Client == nil ||
		len(config_obj.Client.ServerUrls) == 0 {
		return "", errors.New("No server URLs configured")
	}

	// Choose a random url to upload to from the configured URLs.
	idx := http_comms.GetRand()(len(config_obj.Client.ServerUrls))
	return config_obj.Client.ServerUrls[idx], nil
}

// In the client's VFS we store files as subdirectories of the
// root. This means we can not represent a proper pathspec (because
// navigation is tree based). For very complex OSPath paths, we encode
// them as JSON and use a single component.
func (self *VeloCloudUploader) getComponents(path *accessors.OSPath) []string {
	if path.DelegatePath() != "" {
		return []string{path.String()}
	}

	return path.Components
}

// Initiate the upload request and get a key for the parts.
func (self *VeloCloudUploader) Start(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", self.start_url,
		strings.NewReader(json.MustMarshalString(&UploadRequest{
			ClientId:   self.client_id,
			SessionId:  self.Responder.FlowContext().SessionId(),
			Accessor:   self.accessor,
			Components: self.getComponents(self.path),
			Type:       self.uploader_type,
		})))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", self.token)

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

	self.upload_number = self.Responder.NextUploadId()

	return nil
}

func (self *VeloCloudUploader) Put(buf []byte) error {
	self.md5_sum.Write(buf)
	self.sha_sum.Write(buf)

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

	req.Header.Set("Authorization", self.token)

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

	// Send the server an update that we uploaded a part.
	self.updateServerStat(!EOF, uint64(len(buf)))

	// First update must be at offset 0, so increment offset after.
	self.offset += uint64(len(buf))

	return nil
}

func (self *VeloCloudUploader) Commit() {
	self.commit = true
}

func (self *VeloCloudUploader) updateServerStat(eof bool, buffer_size uint64) {
	// Send the server a message that this file was uploaded.
	message := &crypto_proto.VeloMessage{
		SessionId: self.Responder.FlowContext().SessionId(),
		FileBuffer: &actions_proto.FileBuffer{
			Pathspec: &actions_proto.PathSpec{
				// This is the string form that will be shown in the
				// GUI.
				Path:       self.path.String(),
				Accessor:   self.accessor,
				Components: self.getComponents(self.path),
			},
			Offset:       self.offset,
			Size:         uint64(self.size),
			StoredSize:   self.offset + buffer_size,
			DataLength:   buffer_size,
			Eof:          eof,
			UploadNumber: self.upload_number,
		},
	}

	if self.index != nil {
		message.FileBuffer.IsSparse = true
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

	select {
	case <-self.ctx.Done():
		return

	default:
		// Send the packet to the server.
		self.Responder.AddResponse(message)
	}
}

func (self *VeloCloudUploader) Close() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.closed {
		return nil
	}
	self.closed = true

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
		self.response = &uploads.UploadResponse{Error: err.Error()}
		return err
	}

	req.Header.Set("Authorization", self.token)

	resp, err := self.client.Do(req)
	if err != nil {
		self.response = &uploads.UploadResponse{Error: err.Error()}
		return err
	}
	defer resp.Body.Close()

	serialized, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		self.response = &uploads.UploadResponse{Error: err.Error()}
		return fmt.Errorf("Unable to put part: %v: %v\n",
			resp.Status, string(serialized))
	}

	// Send the final status update to the server.
	self.updateServerStat(EOF, 0)

	return nil
}

// Install a new VeloCloudUploader into the factory.
func InstallVeloCloudUploader(
	config_obj *config_proto.Config,
	client_id string,
	manager crypto.ICryptoManager) error {

	http_client, err := networking.GetDefaultHTTPClient(
		config_obj.Client, "")
	if err != nil {
		return err
	}

	// Choose a random URL to upload to
	base_url, err := GetURL(config_obj)
	if err != nil {
		return err
	}

	return SetUploaderService(&VeloCloudUploader{
		config_obj: config_obj,
		start_url:  fmt.Sprintf("%supload/start", base_url),
		put_url:    fmt.Sprintf("%supload/put", base_url),
		commit_url: fmt.Sprintf("%supload/commit", base_url),
		client:     http_client,
		client_id:  client_id,
		manager:    manager,
	})
}
