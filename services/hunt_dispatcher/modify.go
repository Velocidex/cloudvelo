package hunt_dispatcher

import (
	"context"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self HuntDispatcher) ModifyHuntObject(hunt_id string,
	cb func(hunt *api_proto.Hunt) services.HuntModificationAction,
) services.HuntModificationAction {

	hunt, pres := self.GetHunt(hunt_id)
	if !pres {
		return services.HuntUnmodified
	}

	old_state := hunt.State

	modification := cb(hunt)
	if modification != services.HuntUnmodified {
		err := self.SetHunt(hunt)
		if err != nil {
			return services.HuntUnmodified
		}
	}

	// Hunt was started - kick it off.
	if old_state == api_proto.Hunt_PAUSED &&
		hunt.State == api_proto.Hunt_RUNNING {

		// Do this in the background because it can take a while.
		go scheduleClientsForHunt(self.ctx, self.config_obj, hunt)
	}

	// Hunt was stopped - FIXME: how do we do this?
	if old_state == api_proto.Hunt_RUNNING &&
		hunt.State != api_proto.Hunt_RUNNING {

	}

	return modification
}

func (self *HuntDispatcher) ModifyHunt(
	ctx context.Context,
	config_obj *config_proto.Config,
	hunt_modification *api_proto.Hunt,
	user string) error {

	self.ModifyHuntObject(hunt_modification.HuntId,
		func(hunt *api_proto.Hunt) services.HuntModificationAction {

			// Is the description changed?
			if hunt_modification.HuntDescription != "" {
				hunt.HuntDescription = hunt_modification.HuntDescription

			} else if hunt_modification.State == api_proto.Hunt_RUNNING {

				// We allow restarting stopped hunts
				// but this may not work as intended
				// because we still have a hunt index
				// - i.e. clients that already
				// scheduled the hunt will not
				// re-schedule (whether they ran it or
				// not). Usually the most reliable way
				// to re-do a hunt is to copy it and
				// do it again.
				hunt.State = api_proto.Hunt_RUNNING
				hunt.StartTime = uint64(time.Now().UnixNano() / 1000)

				// We are trying to pause or stop the hunt.
			} else if hunt_modification.State == api_proto.Hunt_STOPPED ||
				hunt_modification.State == api_proto.Hunt_PAUSED {
				hunt.State = api_proto.Hunt_STOPPED
			}

			return services.HuntPropagateChanges
		})

	return nil
}
