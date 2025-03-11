package exports

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ExportManager struct {
	config_obj *config_proto.Config
}

func (self *ExportManager) SetContainerStats(
	ctx context.Context,
	config_obj *config_proto.Config,
	stats *api_proto.ContainerStats,
	opts services.ContainerOptions) error {
	return utils.NotImplementedError
}

func (self *ExportManager) GetAvailableDownloadFiles(
	ctx context.Context,
	config_obj *config_proto.Config,
	opts services.ContainerOptions) (*api_proto.AvailableDownloads, error) {
	return nil, utils.NotImplementedError
}

func NewExportManager(config_obj *config_proto.Config) (services.ExportManager, error) {
	return &ExportManager{
		config_obj: config_obj,
	}, nil
}
