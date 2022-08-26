package filestore

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type S3Reader struct {
	session    *session.Session
	downloader *s3manager.Downloader
	offset     int64
	bucket     string
	key        string
	filename   api.FSPathSpec
}

func (self *S3Reader) Read(buff []byte) (int, error) {
	n, err := self.downloader.Download(aws.NewWriteAtBuffer(buff),
		&s3.GetObjectInput{
			Bucket: aws.String(self.bucket),
			Key:    aws.String(self.key),
			Range: aws.String(
				fmt.Sprintf("bytes=%d-%d", self.offset,
					self.offset+int64(len(buff)-1))),
		})
	if err == nil {
		self.offset += n
	}
	return int(n), err
}

func (self *S3Reader) Seek(offset int64, whence int) (int64, error) {
	self.offset = offset
	return self.offset, nil
}

func (self *S3Reader) Stat() (api.FileInfo, error) {
	svc := s3.New(self.session)
	headObj := s3.HeadObjectInput{
		Bucket: aws.String(self.bucket),
		Key:    aws.String(self.key),
	}
	result, err := svc.HeadObject(&headObj)
	if err != nil {
		return nil, err
	}

	return &vtesting.MockFileInfo{
		Name_:     self.filename.Base(),
		PathSpec_: self.filename,
		Size_:     aws.Int64Value(result.ContentLength),
	}, nil
}

func (self *S3Reader) Close() error {
	return nil
}
