package notebook

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/timelines"
	"www.velocidex.com/golang/velociraptor/utils"
)

type NotebookRecord struct {
	NotebookId        string   `json:"notebook_id"`
	CellId            string   `json:"cell_id"`
	AvailableVersions []string `json:"available_versions"`
	CurrentVersion    string   `json:"current_version"`
	Notebook          string   `json:"notebook"`
	NotebookCell      string   `json:"notebook_cell"`
	Creator           string   `json:"creator"`
	Public            bool     `json:"public"`
	SharedWith        []string `json:"shared"`
	Timestamp         int64    `json:"timestamp"`

	// For timelines this contains the JSON encoded timeline.
	Timeline          string `json:"timeline"`
	SupertimelineName string `json:"supertimeline_name"`
	Type              string `json:"type"`
	DocType           string `json:"doc_type"`
}

type NotebookStoreImpl struct {
	ctx                 context.Context
	config_obj          *config_proto.Config
	SuperTimelineStorer timelines.ISuperTimelineStorer
}

func NewNotebookStore(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	SuperTimelineStorer timelines.ISuperTimelineStorer,
) *NotebookStoreImpl {
	return &NotebookStoreImpl{
		ctx:                 ctx,
		config_obj:          config_obj,
		SuperTimelineStorer: SuperTimelineStorer,
	}
}

func getType(notebook_id string) string {
	if strings.HasPrefix(notebook_id, "N.F.") {
		return "Flow"
	}

	if strings.HasPrefix(notebook_id, "N.H.") {
		return "Hunt"
	}

	if strings.HasPrefix(notebook_id, "N.E.") {
		return "Event"
	}

	return "User"
}

func (self *NotebookStoreImpl) Version() int64 {
	return utils.GetTime().Now().Unix()
}

func (self *NotebookStoreImpl) SetNotebook(in *api_proto.NotebookMetadata) error {
	return cvelo_services.SetElasticIndex(self.ctx,
		self.config_obj.OrgId,
		"persisted", in.NotebookId,
		&NotebookRecord{
			NotebookId: in.NotebookId,
			Notebook:   json.MustMarshalString(in),
			Creator:    in.Creator,
			Public:     in.Public,
			Timestamp:  time.Now().UnixNano(),
			SharedWith: append([]string{},
				in.Collaborators...),
			Type:    getType(in.NotebookId),
			DocType: "notebooks",
		})
}

func (self *NotebookStoreImpl) GetNotebook(notebook_id string) (
	*api_proto.NotebookMetadata, error) {

	serialized, err := cvelo_services.GetElasticRecord(
		self.ctx, self.config_obj.OrgId, "persisted", notebook_id)
	if err != nil {
		return nil, err
	}

	// Somethig is wrong with this notebook, just report it as not
	// existing.
	entry := &NotebookRecord{}
	err = json.Unmarshal(serialized, entry)
	if err != nil || entry.NotebookId != notebook_id {
		return nil, os.ErrNotExist
	}

	result := &api_proto.NotebookMetadata{}
	err = json.Unmarshal([]byte(entry.Notebook), result)
	return result, err
}

func (self *NotebookStoreImpl) SetNotebookCell(
	notebook_id string, in *api_proto.NotebookCell) error {

	// Store the actual cell in a new document.
	err := cvelo_services.SetElasticIndex(self.ctx,
		self.config_obj.OrgId,
		"persisted", in.CellId+in.CurrentVersion,
		&NotebookRecord{
			NotebookId:        notebook_id,
			CellId:            in.CellId,
			AvailableVersions: in.AvailableVersions,
			CurrentVersion:    in.CurrentVersion,
			Timestamp:         time.Now().UnixNano(),
			NotebookCell:      json.MustMarshalString(in),
			DocType:           "notebooks",
		})
	if err != nil {
		return err
	}

	// Open the notebook and update the cell's timestamp.
	notebook, err := self.GetNotebook(notebook_id)
	if err != nil {
		return err
	}

	// Update the cell's timestamp so the gui will refresh it.
	new_cell_md := []*api_proto.NotebookCell{}
	cell_summary := &api_proto.NotebookCell{
		CellId:            in.CellId,
		AvailableVersions: in.AvailableVersions,
		CurrentVersion:    in.CurrentVersion,
		Timestamp:         time.Now().UnixNano(),
		Type:              in.Type,
	}

	existing := false
	for _, cell_md := range notebook.CellMetadata {
		// Replace the cell with the new one
		if cell_md.CellId == in.CellId {
			new_cell_md = append(new_cell_md, cell_summary)
			existing = true
			continue
		}
		// Copy the old cell back
		new_cell_md = append(new_cell_md, cell_md)
	}

	// This is a new cell add to the end.
	if !existing {
		new_cell_md = append(new_cell_md, cell_summary)
	}

	// Update the notebook record.
	notebook.CellMetadata = new_cell_md

	return self.SetNotebook(notebook)

}

func (self *NotebookStoreImpl) GetNotebookCell(notebook_id, cell_id, version string) (
	*api_proto.NotebookCell, error) {

	// Get the cell's record.
	serialized, err := cvelo_services.GetElasticRecord(
		self.ctx, self.config_obj.OrgId, "persisted",
		cell_id+version)
	if err != nil {
		return nil, err
	}

	entry := &NotebookRecord{}
	err = json.Unmarshal(serialized, entry)
	if err != nil {
		return nil, err
	}

	result := &api_proto.NotebookCell{}
	err = json.Unmarshal([]byte(entry.NotebookCell), result)
	return result, err
}

// We dont need to explicitely update the index - it is part of the
// Elastic index anyway.
func (self *NotebookStoreImpl) UpdateShareIndex(
	notebook *api_proto.NotebookMetadata) error {
	return nil
}

const (
	query_for_shared_notebooks = `
{
  "sort": [
  {
    "timestamp": {"order": "desc"}
  }],
  "query": {
    "bool": {
      "must": [
        {
          "bool": {
            "should": [
              {"match": {"creator" : %q}},
              {"match": {"shared": %q}},
              {"match": {"public": true}}
           ]}
        },
        {"match": {"doc_type" : "notebooks"}},
        {"match": {"type": "User"}}
      ]}
  },
  "size": %q,
  "from": %q
}
`

	// Very rarely used
	query_for_all_notebooks = `
{
  "sort": [
  {
    "timestamp": {"order": "desc"}
  }],
  "query": {
    "bool": {
      "must": [
        {"match": {"doc_type" : "notebooks"}},
        {"match": {"type": "User"}}
      ]}
  },
  "size": %q,
  "from": %q
}
`
)

func (self *NotebookStoreImpl) GetAllNotebooks(
	ctx context.Context, opts services.NotebookSearchOptions) (
	[]*api_proto.NotebookMetadata, error) {

	var query string
	count := 1000
	offset := 0

	if opts.Username != "" {
		query = json.Format(query_for_shared_notebooks, opts.Username, opts.Username,
			count, offset)
	} else {
		query = json.Format(query_for_all_notebooks, count, offset)
	}

	hits, _, err := cvelo_services.QueryElasticRaw(
		self.ctx, self.config_obj.OrgId, "persisted",
		json.Format(query, 1000, 0))
	if err != nil {
		return nil, err
	}

	result := []*api_proto.NotebookMetadata{}
	for _, hit := range hits {
		entry := &NotebookRecord{}
		err = json.Unmarshal(hit, entry)
		if err != nil {
			continue
		}

		item := &api_proto.NotebookMetadata{}
		err = json.Unmarshal([]byte(entry.Notebook), item)
		if err != nil {
			continue
		}

		if item.Hidden {
			continue
		}

		if opts.Username != "" && !checkNotebookAccess(item, opts.Username) {
			continue
		}

		if opts.Timelines {
			utils.DlvBreak()
			timelines, err := self.SuperTimelineStorer.List(ctx, item.NotebookId)
			if err == nil {
				for _, t := range timelines {
					if !utils.InString(item.Timelines, t.Name) {
						item.Timelines = append(item.Timelines, t.Name)
					}
				}

			}
		}

		result = append(result, item)
	}
	return result, nil
}

func (self *NotebookStoreImpl) RemoveNotebookCell(
	ctx context.Context, config_obj *config_proto.Config,
	notebook_id, cell_id, version string, output_chan chan *ordereddict.Dict) error {

	// Notebook cells contain result sets which are stored in the
	// transient index. Therefore we can not really delete them. We
	// just delete the cell record instead from the persisted index.

	// TODO: We also probably need to delete the s3 objects or maybe
	// we let them expire too?
	return cvelo_services.DeleteDocument(ctx, config_obj.OrgId,
		"persisted", cell_id+version, cvelo_services.AsyncDelete)
}
