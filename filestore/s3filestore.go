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
	"www.velocidex.com/golang/cloudvelo/elastic_datastore"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

type S3Filestore struct {
	config_obj     *config_proto.Config
	elastic_config *elastic_datastore.ElasticConfiguration
	session        *session.Session
	bucket         string
}

func (self S3Filestore) ReadFile(filename api.FSPathSpec) (api.FileReader, error) {
	downloader := s3manager.NewDownloader(self.session)
	return &S3Reader{
		session:    self.session,
		downloader: downloader,
		key:        PathspecToKey(self.config_obj, filename),
		bucket:     self.bucket,
		filename:   filename,
	}, nil
}

// Async write - same as WriteFileWithCompletion with BackgroundWriter
func (self S3Filestore) WriteFile(filename api.FSPathSpec) (api.FileWriter, error) {
	result := &S3Writer{
		key:            PathspecToKey(self.config_obj, filename),
		elastic_config: self.elastic_config,
		config_obj:     self.config_obj,
		session:        self.session,
		part_number:    1,
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
	return nil, errors.New("Not implemented")
}

func (self S3Filestore) ListDirectory(dirname api.FSPathSpec) ([]api.FileInfo, error) {
	svc := s3.New(self.session)

	// Get the list of items
	resp, err := svc.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(self.bucket),
		Prefix: aws.String(PathspecToKey(self.config_obj,
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

	key := PathspecToKey(self.config_obj, filename)

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
	return errors.New("Not implemented")
}

// Clean up any filestore connections
func (self S3Filestore) Close() error {
	return nil
}

func NewS3Filestore(
	config_obj *config_proto.Config,
	elastic_config_path string) (*S3Filestore, error) {
	elastic_config, session, err := GetS3Session(elastic_config_path)
	return &S3Filestore{
		config_obj:     config_obj,
		session:        session,
		elastic_config: elastic_config,
		bucket:         elastic_config.Bucket,
	}, err
}

func GetS3Session(elastic_config_path string) (
	*elastic_datastore.ElasticConfiguration, *session.Session, error) {
	elastic_config, err := elastic_datastore.LoadConfig(elastic_config_path)
	if err != nil {
		return nil, nil, err
	}

	conf := aws.NewConfig()
	if elastic_config.AWSRegion != "" {
		conf = conf.WithRegion(elastic_config.AWSRegion)
	}

	if elastic_config.CredentialsKey != "" &&
		elastic_config.CredentialsSecret != "" {
		token := ""
		creds := credentials.NewStaticCredentials(
			elastic_config.CredentialsKey, elastic_config.CredentialsSecret, token)
		_, err := creds.Get()
		if err != nil {
			return nil, nil, err
		}

		conf = conf.WithCredentials(creds)
	}

	if elastic_config.Endpoint != "" {
		conf = conf.WithEndpoint(elastic_config.Endpoint).
			WithS3ForcePathStyle(true)

		if elastic_config.NoVerifyCert {
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
		return nil, nil, err
	}

	return elastic_config, sess, nil
}
