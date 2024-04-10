package vfs_service

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/api/tables"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
)

// This record is mapped to the results index (see
// schema/templates/transient.json).
type VFSRecord struct {
	Id         string   `json:"id"`
	ClientId   string   `json:"client_id"`
	FlowId     string   `json:"flow_id"`
	Components []string `json:"components"`
	Downloads  []string `json:"downloads"`
	JSONData   string   `json:"data"`
	DocType    string   `json:"doc_type"`
	DocId      string   `json:"doc_id"`
	Timestamp  int64    `json:"timestamp"`
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
	InFlight     bool     `json:"in_flight"`
	FlowId       string   `json:"flow_id"`
}

// Render the root level pseudo directory. This provides anchor points
// for the other drivers in the navigation.
func renderRootVFS() *api_proto.VFSListResponse {
	return &api_proto.VFSListResponse{
		Response: `
   [
    {"Mode": "drwxrwxrwx", "Name": "auto"},
    {"Mode": "drwxrwxrwx", "Name": "ntfs"},
    {"Mode": "drwxrwxrwx", "Name": "registry"}
   ]`,
	}
}

// Render VFS tree node - only returns directories contained within
// this tree node.
func renderDBVFS(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	components []string) (*api_proto.VFSListResponse, error) {

	result, err := getLatestVFSListResponse(ctx, config_obj, client_id, components)
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

// Render all files within the tree node. Enrich with available
// downloads.
func (self *VFSService) ListDirectoryFiles(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	downloads, stat, err := self.readDirectoryWithDownloads(
		ctx, config_obj, in.ClientId, in.VfsComponents)
	if err != nil {
		return nil, err
	}

	table_request := proto.Clone(in).(*api_proto.GetTableRequest)
	table_request.Artifact = stat.Artifact
	if table_request.Artifact == "" {
		table_request.Artifact = "System.VFS.ListDirectory"
	}

	// Transform the table into a subsection of the main table.
	table_request.StartIdx = stat.StartIdx
	table_request.EndIdx = stat.EndIdx

	// Get the table possibly applying any table transformations.
	result, err := tables.GetTable(ctx, config_obj, table_request)
	if err != nil {
		return nil, err
	}

	index_of_Name := -1
	for idx, column_name := range result.Columns {
		if column_name == "Name" {
			index_of_Name = idx
			break
		}
	}

	// Should not happen - Name is missing from results.
	if index_of_Name < 0 {
		return result, nil
	}

	// Merge uploaded file info with the VFSListResponse.
	lookup := make(map[string]*DownloadRow)
	for _, download := range downloads {
		components := download.Components
		if len(components) == 0 {
			continue
		}
		basename := components[len(components)-1]
		lookup[basename] = download
	}

	for _, row := range result.Rows {
		if len(row.Cell) <= index_of_Name {
			continue
		}

		// Find the Name column entry in each cell.
		name := row.Cell[index_of_Name]

		// Insert a Download columns in the begining.
		row.Cell = append([]string{""}, row.Cell...)

		download, pres := lookup[name]
		if !pres {
			continue
		}

		row.Cell[0] = json.MustMarshalString(&flows_proto.VFSDownloadInfo{
			Size:       download.Size,
			Components: download.FSComponents,
			Mtime:      download.Mtime,
			SHA256:     download.Sha256,
			MD5:        download.Md5,
			InFlight:   download.InFlight,
			FlowId:     download.FlowId,
		})
	}
	result.Columns = append([]string{"Download"}, result.Columns...)
	return result, nil
}
