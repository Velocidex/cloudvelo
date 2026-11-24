package filestore

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type S3Reader struct {
	client     *s3.Client
	downloader *manager.Downloader
	offset     int64
	bucket     string
	key        string
	filename   api.FSPathSpec
}

func (self *S3Reader) Read(buff []byte) (int, error) {
	defer Instrument("S3Reader.Read")()

	n, err := self.downloader.Download(context.Background(),
		manager.NewWriteAtBuffer(buff),
		&s3.GetObjectInput{
			Bucket: aws.String(self.bucket),
			Key:    aws.String(self.key),
			Range: aws.String(
				fmt.Sprintf("bytes=%d-%d", self.offset,
					self.offset+int64(len(buff)-1))),
		})

	if err != nil {
		var api_err smithy.APIError
		if errors.As(err, &api_err) && api_err.ErrorCode() == "InvalidRange" {
			// Not really an error - this happens at the end of
			// the file, just return EOF
			return 0, io.EOF
		}
		return 0, err
	}
	self.offset += n

	s3_counter_download.Add(float64(n))

	return int(n), nil
}

func (self *S3Reader) Seek(offset int64, whence int) (int64, error) {
	self.offset = offset
	return self.offset, nil
}

func (self *S3Reader) Stat() (api.FileInfo, error) {
	defer Instrument("S3Reader.Read")()

	headObj := s3.HeadObjectInput{
		Bucket: aws.String(self.bucket),
		Key:    aws.String(self.key),
	}
	result, err := self.client.HeadObject(context.Background(), &headObj)
	if err != nil {
		return nil, err
	}

	return &vtesting.MockFileInfo{
		Name_:     self.filename.Base(),
		PathSpec_: self.filename,
		Size_:     aws.ToInt64(result.ContentLength),
	}, nil
}

func (self *S3Reader) Close() error {
	return nil
}
