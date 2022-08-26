package vfs_service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type VFSService struct {
	ctx        context.Context
	config_obj *config_proto.Config
}

func (self *VFSService) ListDirectory(
	config_obj *config_proto.Config,
	client_id string,
	components []string) (*api_proto.VFSListResponse, error) {

	if len(components) == 0 {
		return renderRootVFS(client_id), nil
	}

	return self.renderDBVFS(config_obj, client_id, components)
}

func (self *VFSService) StatDirectory(
	config_obj *config_proto.Config,
	client_id string,
	components []string) (*api_proto.VFSListResponse, error) {

	result := &api_proto.VFSListResponse{}
	components = append([]string{client_id}, components...)
	id := cvelo_services.MakeId(utils.JoinComponents(components, "/"))
	record := &VFSRecord{}

	serialized, err := cvelo_services.GetElasticRecord(
		self.ctx, self.config_obj.OrgId, "vfs", id)
	if err != nil {
		// Empty responses mean the directory is empty.
		return result, nil
	}

	err = json.Unmarshal(serialized, record)
	if err != nil {
		return result, nil
	}

	err = json.Unmarshal([]byte(record.JSONData), result)
	if err != nil {
		return nil, err
	}

	result.Response = ""
	return result, nil
}

func (self *VFSService) StatDownload(
	config_obj *config_proto.Config,
	client_id string,
	accessor string,
	path_components []string) (*flows_proto.VFSDownloadInfo, error) {
	return nil, errors.New("Not Implemented")
}

func NewVFSService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.VFSService, error) {

	vfs_service := &VFSService{
		config_obj: config_obj,
		ctx:        ctx,
	}

	return vfs_service, nil
}
