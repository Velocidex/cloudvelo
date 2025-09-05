package notebook

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services/notebook"
	"www.velocidex.com/golang/velociraptor/timelines"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type SuperTimelineAnnotator struct {
	config_obj          *config_proto.Config
	SuperTimelineStorer timelines.ISuperTimelineStorer
	SuperTimelineWriter timelines.ISuperTimelineWriter
}

func (self *SuperTimelineAnnotator) AnnotateTimeline(
	ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline string,
	message, principal string,
	timestamp time.Time, event *ordereddict.Dict) error {

	timeline, err := self.SuperTimelineStorer.GetTimeline(ctx, notebook_id,
		supertimeline, constants.TIMELINE_ANNOTATION)
	if err != nil {
		timeline = &timelines_proto.Timeline{
			Id: constants.TIMELINE_ANNOTATION,
		}
	}

	guid, pres := event.GetString(notebook.AnnotationID)
	if !pres {
		guid = notebook.GetGUID()
	}

	writer := &TimelineWriter{
		ctx:                 ctx,
		config_obj:          self.config_obj,
		stats:               timeline,
		SuperTimelineStorer: self.SuperTimelineStorer,
		notebook_id:         notebook_id,
		super_timeline:      supertimeline,
		timeline:            timeline.Id,
	}

	event.Set("Message", message)
	defer writer.Close()

	// An empty timestamp means to delete the event.
	if timestamp.Unix() < 10 {
		original_timestamp, ok := event.Get(notebook.AnnotationOGTime)
		if !ok {
			return utils.Wrap(utils.InvalidArgError, "Original Timestamp not provided for deletion event")
		}

		timestamp, ok = original_timestamp.(time.Time)
		if !ok {
			return utils.Wrap(utils.InvalidArgError, "Original Timestamp invalid for deletion event")
		}

		// Subtrace 1 sec from the timestamp so we can detect the
		// deletion event.
		timestamp = timestamp.Add(-time.Second)
		event.Set("Deletion", true)

	} else {
		event.Update(constants.TIMELINE_DEFAULT_KEY, timestamp).
			Set("Notes", message).
			Set(notebook.AnnotatedBy, principal).
			Set(notebook.AnnotatedAt, utils.GetTime().Now()).
			Set(notebook.AnnotationID, guid)
	}

	return writer.Write(timestamp, event)
}

func NewSuperTimelineAnnotator(
	config_obj *config_proto.Config,
	SuperTimelineStorer timelines.ISuperTimelineStorer) timelines.ISuperTimelineAnnotator {
	return &SuperTimelineAnnotator{
		config_obj:          config_obj,
		SuperTimelineStorer: SuperTimelineStorer,
	}
}
