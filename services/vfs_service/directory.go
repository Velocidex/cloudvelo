package vfs_service

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

type VFSRecord struct {
	ClientId   string   `json:"client_id"`
	Components []string `json:"components"`
	Downloads  []string `json:"downloads"`
	JSONData   string   `json:"data"`
}

type DownloadRow struct {
	Accessor     string   `json:"Accessor"`
	Components   []string `json:"_Components"`
	FSComponents []string `json:"FSPath"`
	Size         uint64   `json:"Size"`
	StoredSize   uint64   `json:"StoredSize"`
	Sha256       string   `json:"Sha256"`
	Md5          string   `json:"Md5"`
	Mtime        uint64   `json:"mtime"`
}

// Render the root level pseudo directory. This provides anchor points
// for the other drivers in the navigation.
func renderRootVFS(client_id string) *api_proto.VFSListResponse {
	return &api_proto.VFSListResponse{
		Response: `
   [
    {"Mode": "drwxrwxrwx", "Name": "auto"},
    {"Mode": "drwxrwxrwx", "Name": "ntfs"},
    {"Mode": "drwxrwxrwx", "Name": "registry"}
   ]`,
	}
}

// Render VFS nodes with VQL collection + uploads.
func renderDBVFS(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	components []string) (*api_proto.VFSListResponse, error) {

	result := &api_proto.VFSListResponse{}
	components = append([]string{client_id}, components...)

	id := services.MakeId(utils.JoinComponents(components, "/"))
	record := &VFSRecord{}
	serialized, err := services.GetElasticRecord(ctx,
		config_obj.OrgId, "vfs", id)
	if err != nil {
		// Empty responses mean the directory is empty.
		return result, nil
	}

	err = json.Unmarshal(serialized, record)
	if err != nil {
		return result, nil
	}

	err = json.Unmarshal([]byte(record.JSONData), result)
	if err != nil {
		return nil, err
	}

	// Empty responses mean the directory is empty - no need to
	// worry about downloads.
	if result.TotalRows == 0 {
		return result, nil
	}

	// The artifact that contains the actual data may vary a bit - let
	// the metadata dictate it.
	artifact_name := result.Artifact
	if artifact_name == "" {
		artifact_name = "System.VFS.ListDirectory"
	}

	// Open the original flow result set
	path_manager := artifacts.NewArtifactPathManagerWithMode(
		config_obj, result.ClientId, result.FlowId,
		artifact_name, paths.MODE_CLIENT)

	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, path_manager.Path())
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("Unable to read artifact: %v", err)
		return result, nil
	}
	defer reader.Close()

	err = reader.SeekToRow(int64(result.StartIdx))
	if err != nil {
		return nil, err
	}

	// If the row refers to a downloaded file, we mark it
	// with the download details.
	count := result.StartIdx
	rows := []*ordereddict.Dict{}
	columns := []string{}

	// Filter the files to produce only the directories. This should
	// be a lot less than total files and so should not take too much
	// memory.
	for row := range reader.Rows(ctx) {
		count++
		if count > result.EndIdx {
			break
		}

		// Only return directories here for the tree widget.
		mode, ok := row.GetString("Mode")
		if !ok || mode == "" || mode[0] != 'd' {
			continue
		}

		rows = append(rows, row)

		if len(columns) == 0 {
			columns = row.Keys()
		}

		// Protect the tree widget from being too large.
		if len(rows) > 2000 {
			break
		}
	}

	encoded_rows, err := json.MarshalIndent(rows)
	if err != nil {
		return nil, err
	}

	result.Response = string(encoded_rows)

	// Add a Download column as the first column.
	result.Columns = columns
	return result, nil
}
