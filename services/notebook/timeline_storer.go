package notebook

import (
	"context"
	"errors"
	"os"

	"github.com/Velocidex/ordereddict"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/timelines"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

/*
  How are timelines implemented?

  - Each notebook logically contains a number of "Super timelines".
  - Each super timeline contains a number of "timelines" or child
    timelines.

  In this implementation there are two types of records:

  1. type: Supertimeline - this is a record of type
     timelines_proto.SuperTimeline - mostly containing the name of the
     super timeline. We do not manage the list of child timelines in
     this object since we can easily get it from the database.

  2. type: Timeline - this is a child timeline - it belongs to a super
     timeline and a notebook id.

*/

const (
	query_for_supertimelines = `
{
  "sort": [
  {
    "timestamp": {"order": "desc"}
  }],
  "query": {
    "bool": {
      "must": [
        {"match": {"notebook_id": %q}},
        {"match": {"type": "Supertimeline"}}
      ]}
  },
  "size": %q,
  "from": %q
}
`

	query_for_specific_supertimeline = `
{
  "sort": [
  {
    "timestamp": {"order": "desc"}
  }],
  "query": {
    "bool": {
      "must": [
        {"match": {"notebook_id": %q}},
        {"match": {"supertimeline_name": %q}},
        {"match": {"type": "Supertimeline"}}
      ]}
  }
}
`
)

type SuperTimelineStorer struct {
	config_obj *config_proto.Config
}

func NewSuperTimelineStorer(
	config_obj *config_proto.Config) timelines.ISuperTimelineStorer {
	return &SuperTimelineStorer{
		config_obj: config_obj,
	}
}

func (self SuperTimelineStorer) Get(
	ctx context.Context, notebook_id string, name string) (
	*timelines_proto.SuperTimeline, error) {

	doc_id := cvelo_services.MakeId(notebook_id + name)
	hit, err := cvelo_services.GetElasticRecord(ctx,
		self.config_obj.OrgId, "persisted", doc_id)
	if err != nil {
		return nil, err
	}

	entry := &NotebookRecord{}
	err = json.Unmarshal(hit, entry)
	if err != nil {
		return nil, utils.NotFoundError
	}

	item := &timelines_proto.SuperTimeline{}
	err = json.Unmarshal([]byte(entry.Timeline), item)
	if err != nil {
		return nil, utils.NotFoundError
	}

	return item, nil
}

func (self SuperTimelineStorer) Set(
	ctx context.Context, notebook_id string,
	timeline *timelines_proto.SuperTimeline) error {

	serialized, err := json.Marshal(timeline)
	if err != nil {
		return err
	}

	entry := &NotebookRecord{
		NotebookId:        notebook_id,
		Type:              "Supertimeline",
		SupertimelineName: timeline.Name,
		DocType:           "Notebook",
		Timeline:          string(serialized),
	}

	doc_id := cvelo_services.MakeId(notebook_id + timeline.Name)
	return cvelo_services.SetElasticIndex(ctx,
		self.config_obj.OrgId, "persisted", doc_id, entry)
}

func (self SuperTimelineStorer) List(ctx context.Context,
	notebook_id string) ([]*timelines_proto.SuperTimeline, error) {

	count := 1000
	offset := 0

	query := json.Format(query_for_supertimelines, notebook_id, count, offset)
	hits, _, err := cvelo_services.QueryElasticRaw(
		ctx, self.config_obj.OrgId, "persisted", query)
	if err != nil {
		return nil, err
	}

	// Deduplicate timelines
	seen := ordereddict.NewDict()
	for _, hit := range hits {
		entry := &NotebookRecord{}
		err = json.Unmarshal(hit, entry)
		if err != nil {
			continue
		}

		item := &timelines_proto.SuperTimeline{}
		err = json.Unmarshal([]byte(entry.Timeline), item)
		if err != nil {
			continue
		}

		seen.Update(item.Name, item)
	}

	result := []*timelines_proto.SuperTimeline{}
	for _, k := range seen.Keys() {
		v, pres := seen.Get(k)
		if !pres {
			continue
		}

		item, ok := v.(*timelines_proto.SuperTimeline)
		if ok {
			result = append(result, item)
		}
	}

	return result, nil
}

func (self SuperTimelineStorer) GetAvailableTimelines(
	ctx context.Context, notebook_id string) (res []string) {
	timelines, err := self.List(ctx, notebook_id)
	if err == nil {
		for _, t := range timelines {
			res = append(res, t.Name)
		}
	}
	return res
}

func (self SuperTimelineStorer) DeleteComponent(ctx context.Context,
	notebook_id string, super_timeline, del_component string) error {

	supertimeline, err := self.Get(ctx, notebook_id, super_timeline)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		supertimeline = &timelines_proto.SuperTimeline{}
	}

	new_timelines := make([]*timelines_proto.Timeline, 0, len(supertimeline.Timelines))
	for _, t := range supertimeline.Timelines {
		if t.Id == del_component {
			continue
		}
		new_timelines = append(new_timelines, t)
	}

	supertimeline.Timelines = new_timelines

	return self.Set(ctx, notebook_id, supertimeline)
}

func (self SuperTimelineStorer) GetTimeline(
	ctx context.Context, notebook_id string,
	super_timeline, component string) (*timelines_proto.Timeline, error) {

	supertimeline, err := self.Get(ctx, notebook_id, super_timeline)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		supertimeline = &timelines_proto.SuperTimeline{}
	}

	for _, t := range supertimeline.Timelines {
		if t.Id == component {
			return t, nil
		}
	}
	return nil, utils.Wrap(utils.NotFoundError, component)
}

// Add or update the super timeline record in the data store.
func (self SuperTimelineStorer) UpdateTimeline(ctx context.Context,
	notebook_id string, supertimeline string,
	timeline *timelines_proto.Timeline) (*timelines_proto.SuperTimeline, error) {

	super_timeline, err := self.Get(ctx, notebook_id, supertimeline)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		super_timeline = &timelines_proto.SuperTimeline{}
	}
	super_timeline.Name = supertimeline

	// Find the existing timeline or add a new one.
	var existing_timeline *timelines_proto.Timeline
	for _, component := range super_timeline.Timelines {
		if component.Id == timeline.Id {
			existing_timeline = component
			break
		}
	}

	if existing_timeline == nil {
		existing_timeline = timeline
		super_timeline.Timelines = append(super_timeline.Timelines, timeline)
	} else {
		// Make a copy
		*existing_timeline = *timeline
	}

	// Now delete the actual record.
	return super_timeline, self.Set(ctx, notebook_id, super_timeline)
}
