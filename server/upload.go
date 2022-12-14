package server

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"www.velocidex.com/golang/cloudvelo/filestore"
	"www.velocidex.com/golang/cloudvelo/vql/uploads"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	maxRetries = 10
)

func (self *Communicator) log(message string, args ...interface{}) {
	return

	logger := logging.GetLogger(self.config_obj.VeloConf(),
		&logging.FrontendComponent)
	logger.Info(message, args...)
}

// All communication with the server is secured by the same underlying
// crypto manager. Verification is essentially free because the cipher
// protobuf is cached on both ends. An RSA operation is only needed to
// verify it once.
func (self *Communicator) verifyToken(r *http.Request) (org_id string, err error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", errors.New("No token provided")
	}

	decoded, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return "", err
	}

	msg_info, err := self.crypto_manager.Decrypt(decoded)
	if err != nil {
		return "", err
	}

	return utils.OrgIdFromClientId(msg_info.Source), err
}

// Receive a POST from the client to start the upload.
//
func (self *Communicator) StartMultipartUpload(
	w http.ResponseWriter, r *http.Request) {
	org_id, err := self.verifyToken(r)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	serialized, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
		return
	}
	defer r.Body.Close()

	request := &uploads.UploadRequest{}
	err = json.Unmarshal(serialized, &request)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
		return
	}

	// Formulate the filestore path from the upload request.
	svc := s3.New(self.session)
	key := filestore.S3KeyForClientUpload(org_id, request)
	s3_request := &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(self.config_obj.Cloud.Bucket),
		Key:         aws.String(key),
		ContentType: aws.String("application/binary"),
	}

	resp, err := svc.CreateMultipartUpload(s3_request)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
		return
	}

	self.log("StartMultipartUpload %v", s3_request)

	if resp.UploadId == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
		return
	}

	response := uploads.UploadResponse{
		Key:      key,
		UploadId: *resp.UploadId,
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(json.MustMarshalString(response)))
}

func extractUploadRequest(in url.Values) (
	*uploads.UploadPutRequest, error) {

	// Pull the query from the URL
	result := &uploads.UploadPutRequest{}
	for k, v := range in {
		if k == "payload" && len(v) > 0 {
			serialized, err := base64.StdEncoding.DecodeString(v[0])
			if err != nil {
				continue
			}
			err = json.Unmarshal(serialized, result)
			if err != nil {
				return nil, err
			}
			return result, nil
		}
	}

	return nil, errors.New("Invalid Request")
}

func (self *Communicator) GetUploadPart(
	w http.ResponseWriter, r *http.Request) {
	_, err := self.verifyToken(r)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	req, err := extractUploadRequest(r.URL.Query())
	if err != nil || req.Part < 0 ||
		req.UploadId == "" || req.Key == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid request"))
		return
	}

	serialized, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	defer r.Body.Close()

	part_data, err := self.uploadPart(req.Key, req.UploadId, req.Part, serialized)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	completed_part := &s3.CompletedPart{
		ETag:       part_data.ETag,
		PartNumber: aws.Int64(int64(req.Part)),
	}

	w.WriteHeader(http.StatusOK)
	w.Write(json.MustMarshalIndent(completed_part))
}

func (self *Communicator) uploadPart(
	key, upload_id string, part int, data []byte) (
	resp *s3.UploadPartOutput, err error) {
	svc := s3.New(self.session)
	partInput := &s3.UploadPartInput{
		Body:          bytes.NewReader(data),
		Bucket:        aws.String(self.config_obj.Cloud.Bucket),
		Key:           aws.String(key),
		PartNumber:    aws.Int64(int64(part)),
		UploadId:      aws.String(upload_id),
		ContentLength: aws.Int64(int64(len(data))),
	}

	for i := 0; i <= maxRetries; i++ {
		resp, err = svc.UploadPart(partInput)
		if err == nil {
			return resp, nil
		}

		logger := logging.GetLogger(
			self.config_obj.VeloConf(), &logging.FrontendComponent)
		logger.Error("While uploading %v: %v", upload_id, err)
		if i >= maxRetries {
			return nil, err
		}
	}

	return nil, err
}

func (self *Communicator) completeUpload(
	key, upload_id string, parts []*s3.CompletedPart) error {
	svc := s3.New(self.session)
	completeInput := &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(self.config_obj.Cloud.Bucket),
		Key:      aws.String(key),
		UploadId: aws.String(upload_id),
		MultipartUpload: &s3.CompletedMultipartUpload{
			Parts: parts,
		},
	}

	self.log("CompleteMultipartUpload %v", completeInput)

	_, err := svc.CompleteMultipartUpload(completeInput)
	return err
}

func (self *Communicator) CompleteMultipartUpload(
	w http.ResponseWriter, r *http.Request) {
	_, err := self.verifyToken(r)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	serialized, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
		return
	}
	defer r.Body.Close()

	request := &uploads.UploadCompletionRequest{}
	err = json.Unmarshal(serialized, &request)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
		return
	}

	if request.UploadId == "" || request.Key == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid request"))
		return
	}

	err = self.completeUpload(request.Key, request.UploadId, request.Parts)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (self Communicator) AbortMultipartUpload(
	w http.ResponseWriter, r *http.Request) {

}
