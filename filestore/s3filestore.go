package filestore

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

type S3Filestore struct {
	config_obj *config.Config
	client     *s3.Client
	bucket     string
	ctx        context.Context
}

func (self S3Filestore) ReadFile(filename api.FSPathSpec) (api.FileReader, error) {
	downloader := manager.NewDownloader(self.client)
	return &S3Reader{
		client:     self.client,
		downloader: downloader,
		key:        PathspecToKey(self.config_obj, filename),
		bucket:     self.bucket,
		filename:   filename,
	}, nil
}

// Async write - same as WriteFileWithCompletion with BackgroundWriter
func (self S3Filestore) WriteFile(filename api.FSPathSpec) (api.FileWriter, error) {
	part_size := uint64(6000000) // Minimum size is 5mb
	if self.config_obj.Cloud.S3PartSize > 0 {
		part_size = self.config_obj.Cloud.S3PartSize
	}

	result := &S3Writer{
		key:         PathspecToKey(self.config_obj, filename),
		path_spec:   filename,
		config_obj:  self.config_obj,
		client:      self.client,
		part_number: 1,
		part_size:   part_size,
		ctx:         self.ctx,
	}

	return result, nil
}

// Completion function will be called when the file is committed.
func (self S3Filestore) WriteFileWithCompletion(
	filename api.FSPathSpec,
	completion func()) (api.FileWriter, error) {
	return self.WriteFile(filename)
}

func (self S3Filestore) StatFile(filename api.FSPathSpec) (api.FileInfo, error) {
	return nil, errors.New("S3Filestore.StatFile is not implemented")
}

func (self S3Filestore) ListDirectory(dirname api.FSPathSpec) ([]api.FileInfo, error) {
	// Get the list of items
	resp, err := self.client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
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
			size:     aws.ToInt64(object.Size),
			mod_time: aws.ToTime(object.LastModified),
		})
	}

	return result, nil
}

func (self S3Filestore) Delete(filename api.FSPathSpec) error {

	// Wait here for a reasonable time but not forever!
	subctx, cancel := context.WithTimeout(self.ctx, 100*time.Second)
	defer cancel()

	key := PathspecToKey(self.config_obj, filename)
	_, err := self.client.DeleteObject(
		subctx, &s3.DeleteObjectInput{
			Bucket: aws.String(self.bucket),
			Key:    aws.String(key),
		})
	return err
}

func (self S3Filestore) Move(src, dest api.FSPathSpec) error {
	return errors.New("S3Filestore.Move is not implemented")
}

// Clean up any filestore connections
func (self S3Filestore) Close() error {
	return nil
}

func NewS3Filestore(
	ctx context.Context,
	config_obj *config.Config) (*S3Filestore, error) {

	client, err := GetS3Client(config_obj)
	return &S3Filestore{
		config_obj: config_obj,
		client:     client,
		bucket:     config_obj.Cloud.Bucket,
		ctx:        ctx,
	}, err
}

func GetS3Client(config_obj *config.Config) (*s3.Client, error) {
	options := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRequestChecksumCalculation(
			aws.RequestChecksumCalculationWhenRequired),
		awsconfig.WithResponseChecksumValidation(
			aws.ResponseChecksumValidationWhenRequired),
		awsconfig.WithRetryMaxAttempts(4),
	}

	if config_obj.Cloud.AWSRegion != "" {
		options = append(options,
			awsconfig.WithRegion(config_obj.Cloud.AWSRegion))
	}

	if config_obj.Cloud.CredentialsKey != "" &&
		config_obj.Cloud.CredentialsSecret != "" {
		options = append(options, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				config_obj.Cloud.CredentialsKey,
				config_obj.Cloud.CredentialsSecret, "")))
	}

	endpoint := config_obj.Cloud.Endpoint
	if endpoint != "" && config_obj.Cloud.NoVerifyCert {
		tr := &http.Transport{
			Proxy:           networking.GetProxy(),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		options = append(options,
			awsconfig.WithHTTPClient(&http.Client{Transport: tr}))
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), options...)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		}
	}), nil
}
