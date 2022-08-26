package filestore

import (
	"bytes"
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"www.velocidex.com/golang/cloudvelo/elastic_datastore"
	"www.velocidex.com/golang/velociraptor/json"
)

const (
	maxRetries = 10
)

type S3Writer struct {
	elastic_config *elastic_datastore.ElasticConfiguration
	session        *session.Session
	key            string
	parts          []*s3.CompletedPart
	size           int64
	upload_id      string
	part_number    int64
}

func (self *S3Writer) start() error {
	svc := s3.New(self.session)
	resp, err := svc.CreateMultipartUpload(
		&s3.CreateMultipartUploadInput{
			Bucket:      aws.String(self.elastic_config.Bucket),
			Key:         aws.String(self.key),
			ContentType: aws.String("application/binary"),
		})
	if err != nil {
		return err
	}

	if resp.UploadId == nil {
		return errors.New("Unknown UploadId")
	}

	self.upload_id = *resp.UploadId
	return nil
}

func (self *S3Writer) Size() (int64, error) {
	return self.size, nil
}

func (self *S3Writer) Write(data []byte) (size int, err error) {
	if len(data) == 0 {
		return 0, nil
	}

	svc := s3.New(self.session)
	partInput := &s3.UploadPartInput{
		Body:          bytes.NewReader(data),
		Bucket:        aws.String(self.elastic_config.Bucket),
		Key:           aws.String(self.key),
		PartNumber:    aws.Int64(self.part_number),
		UploadId:      aws.String(self.upload_id),
		ContentLength: aws.Int64(int64(len(data))),
	}

	json.Dump(partInput)

	var resp *s3.UploadPartOutput
	for i := 0; i <= maxRetries; i++ {
		resp, err = svc.UploadPart(partInput)
		if err == nil {
			self.parts = append(self.parts, &s3.CompletedPart{
				ETag:       resp.ETag,
				PartNumber: aws.Int64(self.part_number),
			})
			self.part_number++
			return len(data), nil
		}

		if i >= maxRetries {
			return 0, err
		}
	}

	return
}

func (self *S3Writer) Truncate() error {
	return errors.New("Not implemented")
}

func (self *S3Writer) Close() error {
	return self.Flush()
}

// Force the writer to be flushed to disk immediately.
func (self *S3Writer) Flush() error {
	svc := s3.New(self.session)
	completeInput := &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(self.elastic_config.Bucket),
		Key:      aws.String(self.key),
		UploadId: aws.String(self.upload_id),
		MultipartUpload: &s3.CompletedMultipartUpload{
			Parts: self.parts,
		},
	}

	_, err := svc.CompleteMultipartUpload(completeInput)
	return err
}
