package filestore

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
)

const (
	maxRetries = 10
)

type S3Writer struct {
	config_obj  *config.Config
	client      *s3.Client
	key         string
	buf         []byte
	parts       []types.CompletedPart
	size        int64
	upload_id   string
	part_number int32

	part_size uint64

	ctx       context.Context
	path_spec api.FSPathSpec
}

func (self *S3Writer) start() error {
	resp, err := self.client.CreateMultipartUpload(
		context.Background(), &s3.CreateMultipartUploadInput{
			Bucket:      aws.String(self.config_obj.Cloud.Bucket),
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
	defer Instrument("S3Reader.Write")()

	if len(data) == 0 {
		return 0, nil
	}

	// Make sure we started. This is needed to ensure we start
	// creating the multipart upload **after** any truncation is
	// needed.
	if self.upload_id == "" {
		err := self.start()
		if err != nil {
			return 0, err
		}
	}

	self.buf = append(self.buf, data...)
	if uint64(len(self.buf)) > self.part_size {
		_, err := self.writeBuf()
		if err != nil {
			return 0, err
		}
	}

	s3_counter_upload.Add(float64(len(data)))

	return len(data), nil
}

// Writing compressed data requires a chunk index sidecar file so the
// chunks can be located and inflated on read. The S3 filestore has no
// concept of that sidecar, and nothing in cloudvelo produces compressed
// result sets, so fail loudly rather than store data that could never be
// read back.
func (self *S3Writer) WriteCompressed(
	data []byte, offset uint64, uncompressed_size int) (int, error) {
	return 0, errors.New("Writing compressed filestore objects is not supported")
}

func (self *S3Writer) Update(data []byte, offset int64) error {
	return errors.New("Updating filestore objects is not implemented yet")
}

func (self *S3Writer) writeBuf() (size int, err error) {
	data := self.buf[:]
	self.buf = nil

	var resp *s3.UploadPartOutput
	for i := 0; i <= maxRetries; i++ {
		resp, err = self.client.UploadPart(context.Background(),
			&s3.UploadPartInput{
				Body:          bytes.NewReader(data),
				Bucket:        aws.String(self.config_obj.Cloud.Bucket),
				Key:           aws.String(self.key),
				PartNumber:    aws.Int32(self.part_number),
				UploadId:      aws.String(self.upload_id),
				ContentLength: aws.Int64(int64(len(data))),
			})
		if err == nil {
			self.parts = append(self.parts, types.CompletedPart{
				ETag:       resp.ETag,
				PartNumber: aws.Int32(self.part_number),
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

// Note that on S3 truncate is the default behavior anyway as AWS does
// not support appending to an S3 object.
func (self *S3Writer) Truncate() error {
	self.buf = nil

	// Wait here for a reasonable time but not forever!
	subctx, cancel := context.WithTimeout(self.ctx, 10*time.Second)
	defer cancel()

	_, err := self.client.DeleteObject(subctx,
		&s3.DeleteObjectInput{
			Bucket: aws.String(self.config_obj.Cloud.Bucket),
			Key:    aws.String(self.key),
		})
	return err
}

func (self *S3Writer) Close() error {
	err := self.Flush()
	if err != nil {
		logger := logging.GetLogger(
			self.config_obj.VeloConf(), &logging.FrontendComponent)
		logger.Error("S3Writer %v Close error: %v", self.key, err)
	}
	return err
}

// Force the writer to be flushed to disk immediately.
func (self *S3Writer) Flush() error {
	if len(self.buf) > 0 {
		_, err := self.writeBuf()
		if err != nil {
			return err
		}
	}

	completeInput := &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(self.config_obj.Cloud.Bucket),
		Key:      aws.String(self.key),
		UploadId: aws.String(self.upload_id),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: self.parts,
		},
	}

	_, err := self.client.CompleteMultipartUpload(
		context.Background(), completeInput)
	return err
}
