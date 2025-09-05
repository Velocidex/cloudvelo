package notebook

import (
	"context"

	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type PathManagerWrapper struct {
	path api.FSPathSpec
}

func (self *PathManagerWrapper) GetPathForWriting() (api.FSPathSpec, error) {
	return self.path, nil
}

func (self *PathManagerWrapper) GetQueueName() string {
	return self.path.Base()
}

func (self *PathManagerWrapper) GetAvailableFiles(
	ctx context.Context) []*api.ResultSetFileProperties {
	return []*api.ResultSetFileProperties{{
		Path: self.path,
	}}
}
