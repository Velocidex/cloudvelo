package filestore

import (
	"io/fs"
	"time"

	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type S3FileInfo struct {
	pathspec api.FSPathSpec
	size     int64
	mod_time time.Time
}

func (self S3FileInfo) Name() string {
	return self.pathspec.Base()
}
func (self S3FileInfo) Size() int64 {
	return self.size
}
func (self S3FileInfo) Mode() fs.FileMode {
	return 0666
}
func (self S3FileInfo) ModTime() time.Time {
	return self.mod_time
}
func (self S3FileInfo) IsDir() bool {
	return false
}
func (self S3FileInfo) Sys() any {
	return nil
}

func (self S3FileInfo) PathSpec() api.FSPathSpec {
	return self.pathspec
}
