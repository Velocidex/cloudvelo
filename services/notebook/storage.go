package notebook

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
)

type NotebookRecord struct {
	NotebookId   string   `json:"notebook_id"`
	CellId       string   `json:"cell_id"`
	Notebook     string   `json:"notebook"`
	NotebookCell string   `json:"notebook_cell"`
	Creator      string   `json:"creator"`
	Public       bool     `json:"public"`
	SharedWith   []string `json:"shared"`
	Timestamp    int64    `json:"timestamp"`
	Type         string   `json:"type"`
}

type NotebookStoreImpl struct {
	ctx        context.Context
	config_obj *config_proto.Config
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

func (self *NotebookStoreImpl) SetNotebook(in *api_proto.NotebookMetadata) error {
	return cvelo_services.SetElasticIndex(self.ctx,
		self.config_obj.OrgId,
		"notebooks", in.NotebookId, &NotebookRecord{
			NotebookId: in.NotebookId,
			Notebook:   json.MustMarshalString(in),
			Creator:    in.Creator,
			Public:     in.Public,
			Timestamp:  time.Now().Unix(),
			SharedWith: append([]string{},
				in.Collaborators...),
			Type: getType(in.NotebookId),
		})
}

func (self *NotebookStoreImpl) GetNotebook(notebook_id string) (
	*api_proto.NotebookMetadata, error) {

	serialized, err := cvelo_services.GetElasticRecord(
		self.ctx, self.config_obj.OrgId, "notebooks", notebook_id)
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

	err := cvelo_services.SetElasticIndex(self.ctx,
		self.config_obj.OrgId,
		"notebooks", in.CellId, &NotebookRecord{
			NotebookId:   notebook_id,
			CellId:       in.CellId,
			Timestamp:    time.Now().Unix(),
			NotebookCell: json.MustMarshalString(in),
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
	for _, cell_md := range notebook.CellMetadata {
		if cell_md.CellId == in.CellId {
			new_cell_md = append(new_cell_md, &api_proto.NotebookCell{
				CellId:    in.CellId,
				Timestamp: time.Now().Unix(),
			})
			continue
		}
		new_cell_md = append(new_cell_md, cell_md)
	}
	notebook.CellMetadata = new_cell_md

	return self.SetNotebook(notebook)

}

func (self *NotebookStoreImpl) GetNotebookCell(notebook_id, cell_id string) (
	*api_proto.NotebookCell, error) {

	serialized, err := cvelo_services.GetElasticRecord(
		self.ctx, self.config_obj.OrgId, "notebooks", cell_id)
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

func (self *NotebookStoreImpl) StoreAttachment(notebook_id, filename string, data []byte) (api.FSPathSpec, error) {
	full_path := paths.NewNotebookPathManager(notebook_id).
		Attachment(filename)
	file_store_factory := file_store.GetFileStore(self.config_obj)
	fd, err := file_store_factory.WriteFile(full_path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	_, err = fd.Write(data)
	return full_path, err
}

func (self *NotebookStoreImpl) GetAvailableTimelines(notebook_id string) []string {
	return nil
}

func (self *NotebookStoreImpl) GetAvailableDownloadFiles(
	notebook_id string) (*api_proto.AvailableDownloads, error) {
	return &api_proto.AvailableDownloads{}, nil
}

func (self *NotebookStoreImpl) RemoveAttachment(ctx context.Context, notebook_id string, components []string) error {
	return errors.New("Not implemented")
}

func (self *NotebookStoreImpl) GetAvailableUploadFiles(notebook_id string) (
	*api_proto.AvailableDownloads, error) {
	return &api_proto.AvailableDownloads{}, nil
}

// We dont need to explicitely update the index - it is part of the
// Elastic index anyway.
func (self *NotebookStoreImpl) UpdateShareIndex(
	notebook *api_proto.NotebookMetadata) error {
	return nil
}
