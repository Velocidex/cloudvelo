package hunt_dispatcher

import (
	"context"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services/hunt_manager"
)

// This is only used by the GUI so we do it inline.
func (self *HuntDispatcher) MutateHunt(
	ctx context.Context, config_obj *config_proto.Config,
	mutation *api_proto.HuntMutation) error {

	hunt_manager, err := hunt_manager.MakeHuntManager(config_obj)
	if err != nil {
		return err
	}
	return hunt_manager.ProcessMutation(ctx, config_obj,
		ordereddict.NewDict().
			Set("hunt_id", mutation.HuntId).
			Set("mutation", mutation))
}
