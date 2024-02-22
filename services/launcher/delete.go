package launcher

import (
	"context"
	"fmt"

	"github.com/Velocidex/ordereddict"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *FlowStorageManager) DeleteFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, flow_id string,
	principal string,
	really_do_it bool) ([]*services.DeleteFlowResponse, error) {

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return nil, err
	}

	collection_details, err := launcher.GetFlowDetails(
		ctx, config_obj, client_id, flow_id)
	if err != nil {
		return nil, err
	}

	collection_context := collection_details.Context
	if collection_context == nil {
		return nil, nil
	}

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)

	upload_metadata_path := flow_path_manager.UploadMetadata()
	r := &reporter{
		really_do_it: really_do_it,
		ctx:          ctx,
		config_obj:   config_obj,
		seen:         make(map[string]bool),
	}
	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, flow_path_manager.UploadMetadata())
	if err == nil {
		for row := range reader.Rows(ctx) {
			upload, pres := row.GetString("vfs_path")
			if pres {
				// Each row is the full filestore path of the upload.
				pathspec := path_specs.NewUnsafeFilestorePath(
					utils.SplitComponents(upload)...).
					SetType(api.PATH_TYPE_FILESTORE_ANY)

				r.delete_index("Upload", "vfs", "vfs_path", pathspec.AsClientPath())

				if really_do_it {
					fmt.Println("Deleting file: ", pathspec)
					err = file_store_factory.Delete(pathspec)
					if err != nil {
						fmt.Println("Failed to delete file: ", err)
						return nil, err
					}
				}
			}
		}
	}

	// Order results to facilitate deletion - container deletion
	// happens after we read its contents.
	r.delete_index("UploadMetadata", "transient", "vfs_path",
		upload_metadata_path.AsClientPath())

	// Remove all result sets from artifacts.
	for _, artifact_name := range collection_context.ArtifactsWithResults {
		path_manager, err := artifact_paths.NewArtifactPathManager(
			ctx, config_obj, client_id, flow_id, artifact_name)
		if err != nil {
			continue
		}

		result_path, err := path_manager.GetPathForWriting()
		if err != nil {
			continue
		}
		r.delete_index("Result", "transient", "vfs_path",
			result_path.AsClientPath())
	}

	r.delete_index("Log", "transient", "vfs_path",
		flow_path_manager.Log().AsClientPath())
	r.delete_index("CollectionContext", "collections", "session_id", flow_id)

	// All notebook and their cells
	notebook_id := fmt.Sprintf("N.%s-%s", flow_id, client_id)
	r.delete_index("Notebook", "persisted", "notebook_id", notebook_id)

	return r.responses, nil
}

type reporter struct {
	ctx          context.Context
	responses    []*services.DeleteFlowResponse
	seen         map[string]bool
	config_obj   *config_proto.Config
	really_do_it bool
}

const (
	deletionQuery = `
{
  "query": {
     "bool": {
       "must": [
         {"match": {%q: %q}}
       ]
     }
  }
}
`
)

func (self *reporter) really_delete_with_query(type_, index, key, prefix, query string) {
	var error_message string

	err := cvelo_services.DeleteByQuery(self.ctx, self.config_obj.OrgId, index, query)
	if err != nil {
		error_message = err.Error()
	}

	self.responses = append(self.responses, &services.DeleteFlowResponse{
		Type: type_,
		Data: ordereddict.NewDict().
			Set("index", index).
			Set(key, prefix),
		Error: error_message,
	})
}

func (self *reporter) really_delete(type_, index, key, prefix string) {
	self.really_delete_with_query(type_, index, key, prefix, json.Format(deletionQuery, key, prefix))
}

func (self *reporter) delete_index(type_, index, key, prefix string) {
	if self.really_do_it {
		self.really_delete(type_, index, key, prefix)
		return
	}

	hits, _, err := cvelo_services.QueryElasticRaw(
		self.ctx, self.config_obj.OrgId, index,
		json.Format(deletionQuery, key, prefix))
	if err != nil {
		self.responses = append(self.responses, &services.DeleteFlowResponse{
			Type: type_,
			Data: ordereddict.NewDict().
				Set("index", index).
				Set(key, prefix),
			Error: err.Error(),
		})
		return
	}

	for _, hit := range hits {
		source := ordereddict.NewDict()
		source.UnmarshalJSON(hit)
		self.responses = append(self.responses, &services.DeleteFlowResponse{
			Type: type_,
			Data: source,
		})
	}
}
