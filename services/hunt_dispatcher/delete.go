package hunt_dispatcher

import (
	"context"

	"www.velocidex.com/golang/velociraptor/utils"
)

// We do not keep a local index cache so there is no need to flush it.
func (self *HuntStorageManagerImpl) FlushIndex(ctx context.Context) error {
	return nil
}

func (self *HuntStorageManagerImpl) DeleteHunt(
	ctx context.Context, hunt_id string) error {
	return utils.NotImplementedError
}
