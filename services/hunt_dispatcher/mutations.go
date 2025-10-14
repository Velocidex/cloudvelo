package hunt_dispatcher

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services/hunt_manager"
	velo_utils "www.velocidex.com/golang/velociraptor/utils"
)

// This is only used by the GUI so we do it inline.
func (self *HuntDispatcher) MutateHunt(
	ctx context.Context, config_obj *config_proto.Config,
	mutation *api_proto.HuntMutation) error {

	hunt_manager, err := hunt_manager.MakeHuntManager(config_obj)
	if err != nil {
		return err
	}
	err = hunt_manager.ProcessMutation(ctx, config_obj,
		ordereddict.NewDict().
			Set("hunt_id", mutation.HuntId).
			Set("mutation", mutation))

	if err != nil {
		return err
	}

	if mutation.Assignment != nil {
		hunt_flow_entry := &HuntFlowEntry{
			HuntId:    mutation.HuntId,
			ClientId:  mutation.Assignment.ClientId,
			FlowId:    mutation.Assignment.FlowId,
			Timestamp: velo_utils.GetTime().Now().Unix(),
			Status:    "started",
			DocType:   "hunt_flow",
		}
		return services.SetElasticIndex(ctx,
			config_obj.OrgId,
			"transient", services.DocIdRandom,
			hunt_flow_entry)
	}
	return nil
}
