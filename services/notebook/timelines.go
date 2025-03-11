package notebook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/result_sets/timed"
	"www.velocidex.com/golang/cloudvelo/services"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services/notebook"
	"www.velocidex.com/golang/velociraptor/timelines"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

func NewRecord(
	notebook_id, supertimeline, component, version string,
	timestamp time.Time) *timed.TimedResultSetRecord {
	return &timed.TimedResultSetRecord{
		Type:      "timeline",
		Timestamp: timestamp.UnixNano(),
		VFSPath: fmt.Sprintf("%v/%v/%v/%v",
			notebook_id, supertimeline, component, version),
	}
}

type TimelineWriter struct {
	ctx            context.Context
	config_obj     *config_proto.Config
	stats          *timelines_proto.Timeline
	notebook_id    string
	super_timeline string
	timeline       string

	SuperTimelineStorer timelines.ISuperTimelineStorer
}

func (self *TimelineWriter) Stats() *timelines_proto.Timeline {
	return self.stats
}

func (self *TimelineWriter) updateStats(timestamp time.Time) {
	now := timestamp.Unix()
	if self.stats.EndTime < now {
		self.stats.EndTime = now
	}

	if self.stats.StartTime == 0 {
		self.stats.StartTime = now
	}

}

func (self *TimelineWriter) Write(
	timestamp time.Time, row *ordereddict.Dict) error {

	serialized, err := row.MarshalJSON()
	if err != nil {
		return err
	}

	return self.WriteBuffer(timestamp, serialized)
}

func (self *TimelineWriter) WriteBuffer(
	timestamp time.Time, serialized []byte) error {

	record := NewRecord(self.notebook_id,
		self.super_timeline, self.timeline, self.stats.Version, timestamp)
	record.JSONData = string(serialized)

	err := services.SetElasticIndex(self.ctx,
		utils.GetOrgId(self.config_obj),
		"transient", services.DocIdRandom, record)
	if err != nil {
		return err
	}

	self.updateStats(timestamp)

	return nil
}

func (self *TimelineWriter) Truncate() {
	// Delete the timeline data
}

func (self *TimelineWriter) Close() {
	_, _ = self.SuperTimelineStorer.UpdateTimeline(self.ctx,
		self.notebook_id, self.super_timeline, self.stats)
}

type SuperTimelineWriter struct {
	ctx                 context.Context
	config_obj          *config_proto.Config
	notebook_id         string
	timeline            string
	SuperTimelineStorer timelines.ISuperTimelineStorer
}

func (self *SuperTimelineWriter) AddChild(
	timeline *timelines_proto.Timeline,
	completer func()) (timelines.ITimelineWriter, error) {

	res := &TimelineWriter{
		ctx:        self.ctx,
		config_obj: self.config_obj,
		stats: &timelines_proto.Timeline{
			Id:      timeline.Id,
			Version: fmt.Sprintf("%v", utils.GetGUID()),
		},
		SuperTimelineStorer: self.SuperTimelineStorer,
		notebook_id:         self.notebook_id,
		super_timeline:      self.timeline,
		timeline:            timeline.Id,
	}

	return res, nil
}

func (self *SuperTimelineWriter) Close(ctx context.Context) error {
	return nil
}

func (self *SuperTimelineWriter) New(ctx context.Context,
	config_obj *config_proto.Config,
	storer timelines.ISuperTimelineStorer,
	notebook_id, name string) (timelines.ISuperTimelineWriter, error) {
	return &SuperTimelineWriter{
		ctx:                 ctx,
		config_obj:          config_obj,
		notebook_id:         notebook_id,
		timeline:            name,
		SuperTimelineStorer: storer,
	}, nil
}

type SuperTimelineReader struct {
	config_obj          *config_proto.Config
	SuperTimelineStorer timelines.ISuperTimelineStorer
	timestamp           time.Time
	metadata            map[string]*timelines_proto.Timeline
}

func (self *SuperTimelineReader) Stat() *timelines_proto.SuperTimeline {
	return &timelines_proto.SuperTimeline{}
}

func (self *SuperTimelineReader) Close() {}
func (self *SuperTimelineReader) SeekToTime(timestamp time.Time) {
	self.timestamp = timestamp
}

const (
	timeline_query = `
{
  "query": {
    "bool": {
      "must": [
        {
          "bool": {
            "should": [
             %s
          ]}
        },
        {"range": {"timestamp": {"gte": %q}}},
        {"match": {"type": "timeline"}}
      ]}
  }
}
`
)

func (self *SuperTimelineReader) Read(ctx context.Context) <-chan timelines.TimelineItem {
	output_chan := make(chan timelines.TimelineItem)

	// Get all the components
	component_query := []string{}
	for vfs := range self.metadata {
		component_query = append(component_query,
			json.Format(`{"match": {"vfs_path": %q}}`, vfs))
	}

	go func() {
		defer close(output_chan)

		deletions := make(map[string]bool)

		start := self.timestamp.UnixNano()
		query := json.Format(timeline_query,
			strings.Join(component_query, ",\n"), start)

		hits, err := cvelo_services.QueryChan(ctx, self.config_obj, 1000,
			utils.GetOrgId(self.config_obj), "transient", query, "timestamp")
		if err != nil {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Error("SuperTimelineReader.Read: %v", err)
			return
		}

		for hit := range hits {
			record := &timed.TimedResultSetRecord{}
			err := json.Unmarshal(hit, record)
			if err != nil {
				continue
			}

			md, pres := self.metadata[record.VFSPath]
			if !pres {
				continue
			}

			row := ordereddict.NewDict()
			err = json.Unmarshal([]byte(record.JSONData), &row)
			if err != nil {
				continue
			}

			// Because we write data to the transient index it can not
			// be deleted. Instead we write a deletion event just
			// before the deleted event and intentionally omit the
			// event from reading.

			// Identify the deletion events.
			_, pres = row.Get("Deletion")
			if pres {
				guid, pres := row.GetString(notebook.AnnotationID)
				if pres {
					deletions[guid] = true
				}
			}

			guid, pres := row.GetString(notebook.AnnotationID)
			if pres {
				// Drop deleted items.
				_, pres = deletions[guid]
				if pres {
					continue
				}
			}

			// Get the message
			message_field := md.MessageColumn
			if message_field == "" {
				message_field = "Message"
			}

			message, ok := row.GetString(message_field)
			if !ok {
				message = record.JSONData
			}

			item := timelines.TimelineItem{
				Row:     row,
				Time:    time.Unix(0, record.Timestamp),
				Message: message,
			}

			parts := strings.Split(record.VFSPath, "/")
			if len(parts) > 2 {
				item.Source = parts[len(parts)-2]
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- item:
			}
		}
	}()

	return output_chan
}

func shouldInclude(id string, include_components, exclude_components []string) bool {
	if len(include_components) > 0 && !utils.InString(include_components, id) {
		return false
	}

	if utils.InString(exclude_components, id) {
		return false
	}

	return true
}

func (self *SuperTimelineReader) New(
	ctx context.Context,
	config_obj *config_proto.Config,
	storer timelines.ISuperTimelineStorer,
	notebook_id, super_timeline string,
	include_components []string, exclude_components []string) (
	timelines.ISuperTimelineReader, error) {

	timeline, err := storer.Get(ctx, notebook_id, super_timeline)
	if err != nil {
		return nil, err
	}

	metadata := make(map[string]*timelines_proto.Timeline)
	for _, t := range timeline.Timelines {
		if shouldInclude(t.Id, include_components, exclude_components) {
			key := fmt.Sprintf("%v/%v/%v/%v",
				notebook_id, super_timeline, t.Id, t.Version)
			metadata[key] = t
		}
	}

	return &SuperTimelineReader{
		config_obj:          config_obj,
		SuperTimelineStorer: storer,
		metadata:            metadata,
	}, nil
}
