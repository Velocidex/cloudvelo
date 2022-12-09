package inventory

import (
	"context"
	"errors"
	"sync"

	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	notImplementedError = errors.New("Inventory service not supported")
)

type Dummy struct{}

func (self Dummy) Get() *artifacts_proto.ThirdParty {
	return nil
}

func (self Dummy) ProbeToolInfo(name string) (*artifacts_proto.Tool, error) {
	return nil, notImplementedError
}

func (self Dummy) GetToolInfo(ctx context.Context, config_obj *config_proto.Config,
	tool string) (*artifacts_proto.Tool, error) {
	return nil, notImplementedError
}

func (self Dummy) AddTool(config_obj *config_proto.Config,
	tool *artifacts_proto.Tool, opts services.ToolOptions) error {
	return notImplementedError
}

func (self Dummy) RemoveTool(config_obj *config_proto.Config, tool_name string) error {
	return notImplementedError
}

func NewInventoryDummyService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.Inventory, error) {

	return Dummy{}, nil
}
