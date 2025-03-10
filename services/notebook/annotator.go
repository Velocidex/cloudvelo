package notebook

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/timelines"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type SuperTimelineAnnotator struct {
	config_obj          *config_proto.Config
	SuperTimelineStorer timelines.ISuperTimelineStorer
}

func (self *SuperTimelineAnnotator) AnnotateTimeline(
	ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline string,
	message, principal string,
	timestamp time.Time, event *ordereddict.Dict) error {
	return utils.NotImplementedError
}

func NewSuperTimelineAnnotator(
	config_obj *config_proto.Config,
	SuperTimelineStorer timelines.ISuperTimelineStorer) timelines.ISuperTimelineAnnotator {
	return &SuperTimelineAnnotator{
		config_obj:          config_obj,
		SuperTimelineStorer: SuperTimelineStorer,
	}
}
