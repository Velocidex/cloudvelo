package hunt_dispatcher

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *HuntStorageManagerImpl) GetTags(
	ctx context.Context) (res []string) {
	// Fixme
	return nil
}

func (self *HuntStorageManagerImpl) Refresh(ctx context.Context,
	config_obj *config_proto.Config) error {
	return utils.NotImplementedError
}

func (self *HuntStorageManagerImpl) ModifyHuntObject(
	ctx context.Context,
	hunt_id string,
	cb func(hunt *hunt_dispatcher.HuntRecord) services.HuntModificationAction) services.HuntModificationAction {
	// Fixme
	return services.HuntUnmodified
}

func NewHuntStorageManagerImpl(
	ctx context.Context,
	config_obj *config_proto.Config) hunt_dispatcher.HuntStorageManager {
	result := &HuntStorageManagerImpl{
		config_obj: config_obj,
		ctx:        ctx,
	}

	return result
}
