package frontend

import (
	"context"
	"net/url"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services/frontend"
	"www.velocidex.com/golang/velociraptor/utils"
)

type FrontendService struct{}

func (self FrontendService) GetMinionCount() int {
	return 0
}

func (self FrontendService) GetMasterAPIClient(ctx context.Context) (
	api_proto.APIClient, func() error, error) {
	return nil, nil, utils.NotImplementedError
}

func (self FrontendService) GetBaseURL(
	config_obj *config_proto.Config) (res *url.URL, err error) {
	return frontend.GetBaseURL(config_obj)
}

func (self FrontendService) GetPublicUrl(
	config_obj *config_proto.Config) (res *url.URL, err error) {
	return frontend.GetPublicUrl(config_obj)
}
