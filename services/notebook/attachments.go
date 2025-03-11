package notebook

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services/notebook"
)

type AttachmentManagerImpl struct {
	notebook.AttachmentManager
}

// TODO - need to find a better way to store download (export) files.
func (self *AttachmentManagerImpl) GetAvailableDownloadFiles(
	ctx context.Context, notebook_id string) (*api_proto.AvailableDownloads, error) {
	return &api_proto.AvailableDownloads{}, nil
}

func NewAttachmentManager(
	config_obj *config_proto.Config,
	NotebookStore notebook.NotebookStore) notebook.AttachmentManager {
	return &AttachmentManagerImpl{
		AttachmentManager: notebook.NewAttachmentManager(
			config_obj, NotebookStore),
	}
}
