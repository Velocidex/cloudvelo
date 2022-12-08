package uploads

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"hash"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
	"www.velocidex.com/golang/velociraptor/accessors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vql/networking"
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

type Uploader struct {
	client     *http.Client
	ctx        context.Context
	config_obj *config_proto.Config

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
	// The token is used to authenticate to the upload endpoints. It
	// consists of a normal empty cipher_text just like the regular
	// POST data but the server can use it to verify the caller.
	token string

	path *accessors.OSPath

	client_id  string
	session_id string
	accessor   string
	mtime      time.Time
	atime      time.Time
	ctime      time.Time
	btime      time.Time

	session_tracker *SessionTracker

	// This indicates which type of file it is:
	// 1. "idx" is an index file
	// empty is a regular file.
	uploader_type string
	owner         *UploaderFactory
}

// In the client's VFS we store files as subdirectories of the
// root. This means we can not represent a proper pathspec (because
// navigation is tree based). For very complex OSPath paths, we encode
// them as JSON and use a single component.
func (self *Uploader) getComponents(path *accessors.OSPath) []string {
	if path.DelegatePath() != "" {
		return []string{path.String()}
	}

	return path.Components
}

// Initiate the upload request and get a key for the parts.
func (self *Uploader) Start(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", self.start_url,
		strings.NewReader(json.MustMarshalString(&UploadRequest{
			ClientId:   self.client_id,
			SessionId:  self.session_id,
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

	// Send the server a message that this file was uploaded.
	message := &crypto_proto.VeloMessage{
		SessionId: self.session_id,
		FileBuffer: &actions_proto.FileBuffer{
			Pathspec: &actions_proto.PathSpec{
				// This is the string form that will be shown in the
				// GUI.
				Path:       self.path.String(),
				Accessor:   self.accessor,
				Components: self.getComponents(self.path),
			},
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

func (self *UploaderFactory) GetURL() (string, error) {
	if len(self.config_obj.Client.ServerUrls) == 0 {
		return "", errors.New("No server URLs configured")
	}

	// Choose a random url to upload to from the configured URLs.
	idx := http_comms.GetRand()(len(self.config_obj.Client.ServerUrls))
	return self.config_obj.Client.ServerUrls[idx], nil
}

func (self *UploaderFactory) NewUploader(
	ctx context.Context,
	session_id, accessor, uploader_type string,
	path *accessors.OSPath) (*Uploader, error) {

	// Choose a random URL to upload to
	base_url, err := self.GetURL()
	if err != nil {
		return nil, err
	}

	var cipher_text []byte
	if self.manager != nil {
		server_name := "VelociraptorServer"
		if self.config_obj.Client != nil &&
			self.config_obj.Client.PinnedServerName != "" {
			server_name = self.config_obj.Client.PinnedServerName
		}

		cipher_text, err = self.manager.Encrypt(
			nil, crypto_proto.PackedMessageList_UNCOMPRESSED,
			self.config_obj.Client.Nonce, server_name)
		if err != nil {
			return nil, err
		}
	}

	http_client, err := networking.GetDefaultHTTPClient(
		self.config_obj.Client, "")
	if err != nil {
		return nil, err
	}

	result := &Uploader{
		config_obj:      self.config_obj,
		start_url:       fmt.Sprintf("%supload/start", base_url),
		put_url:         fmt.Sprintf("%supload/put", base_url),
		commit_url:      fmt.Sprintf("%supload/commit", base_url),
		client:          http_client,
		md5_sum:         md5.New(),
		sha_sum:         sha256.New(),
		part:            1,
		ctx:             ctx,
		exe:             self.exe,
		session_tracker: self.GetTracker(session_id),
		session_id:      session_id,
		client_id:       self.client_id,
		owner:           self,
		accessor:        accessor,
		path:            path,
		uploader_type:   uploader_type,
		token:           base64.StdEncoding.EncodeToString(cipher_text),
	}

	return result, result.Start(ctx)
}
