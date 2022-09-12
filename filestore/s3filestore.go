package filestore

import (
	"crypto/tls"
	"errors"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

type S3Filestore struct {
	config_obj *config.Config
	session    *session.Session
	bucket     string
}

func (self S3Filestore) ReadFile(filename api.FSPathSpec) (api.FileReader, error) {
	downloader := s3manager.NewDownloader(self.session)
	return &S3Reader{
		session:    self.session,
		downloader: downloader,
		key:        PathspecToKey(filename),
		bucket:     self.bucket,
		filename:   filename,
	}, nil
}

// Async write - same as WriteFileWithCompletion with BackgroundWriter
func (self S3Filestore) WriteFile(filename api.FSPathSpec) (api.FileWriter, error) {
	result := &S3Writer{
		key:         PathspecToKey(filename),
		config_obj:  self.config_obj,
		session:     self.session,
		part_number: 1,
	}

	return result, result.start()
}

// Completion function will be called when the file is committed.
func (self S3Filestore) WriteFileWithCompletion(
	filename api.FSPathSpec,
	completion func()) (api.FileWriter, error) {
	return self.WriteFile(filename)
}

func (self S3Filestore) StatFile(filename api.FSPathSpec) (api.FileInfo, error) {
	return nil, errors.New("S3Filestore.StatFile Not implemented")
}

func (self S3Filestore) ListDirectory(dirname api.FSPathSpec) ([]api.FileInfo, error) {
	svc := s3.New(self.session)

	// Get the list of items
	resp, err := svc.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(self.bucket),
		Prefix: aws.String(PathspecToKey(
			dirname.SetType(api.PATH_TYPE_DATASTORE_DIRECTORY))),
	})
	if err != nil {
		return nil, err
	}

	var result []api.FileInfo
	for _, object := range resp.Contents {
		components := strings.Split(*object.Key, "/")
		name := components[len(components)-1]

		name_type, name := api.GetFileStorePathTypeFromExtension(name)
		result = append(result, &S3FileInfo{
			pathspec: dirname.AddUnsafeChild(
				utils.UnsanitizeComponent(name)).
				SetType(name_type),
			size:     *object.Size,
			mod_time: *object.LastModified,
		})
	}

	return result, nil
}

func (self S3Filestore) Delete(filename api.FSPathSpec) error {

	key := PathspecToKey(filename)
	svc := s3.New(self.session)
	_, err := svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(self.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}

	return svc.WaitUntilObjectNotExists(&s3.HeadObjectInput{
		Bucket: aws.String(self.bucket),
		Key:    aws.String(key),
	})
}

func (self S3Filestore) Move(src, dest api.FSPathSpec) error {
	return errors.New("S3Filestore.Move Not implemented")
}

// Clean up any filestore connections
func (self S3Filestore) Close() error {
	return nil
}

func NewS3Filestore(
	config_obj *config.Config) (*S3Filestore, error) {

	session, err := GetS3Session(config_obj)
	return &S3Filestore{
		config_obj: config_obj,
		session:    session,
		bucket:     config_obj.Cloud.Bucket,
	}, err
}

func GetS3Session(config_obj *config.Config) (*session.Session, error) {
	conf := aws.NewConfig()
	if config_obj.Cloud.AWSRegion != "" {
		conf = conf.WithRegion(config_obj.Cloud.AWSRegion)
	}

	if config_obj.Cloud.CredentialsKey != "" &&
		config_obj.Cloud.CredentialsSecret != "" {
		token := ""
		creds := credentials.NewStaticCredentials(
			config_obj.Cloud.CredentialsKey, config_obj.Cloud.CredentialsSecret, token)
		_, err := creds.Get()
		if err != nil {
			return nil, err
		}

		conf = conf.WithCredentials(creds)
	}

	if config_obj.Cloud.Endpoint != "" {
		conf = conf.WithEndpoint(config_obj.Cloud.Endpoint).
			WithS3ForcePathStyle(true)

		if config_obj.Cloud.NoVerifyCert {
			tr := &http.Transport{
				Proxy:           networking.GetProxy(),
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}

			client := &http.Client{Transport: tr}
			conf = conf.WithHTTPClient(client)
		}
	}

	sess, err := session.NewSessionWithOptions(
		session.Options{
			Config:            *conf,
			SharedConfigState: session.SharedConfigEnable,
		})
	if err != nil {
		return nil, err
	}

	return sess, nil
}
