package hunt_dispatcher

import (
	"context"

	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *HuntStorageManagerImpl) FlushIndex(ctx context.Context) error {
	return utils.NotImplementedError
}

func (self *HuntStorageManagerImpl) DeleteHunt(
	ctx context.Context, hunt_id string) error {
	return utils.NotImplementedError
}
