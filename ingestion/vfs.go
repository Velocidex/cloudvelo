package ingestion

import (
	"bufio"
	"strings"
	"time"

	"www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/vfs_service"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

// This is what the System.VFS.ListDirectory artifact returns. We use
// it to store the VFS data so the GUI can show it.
type VFSRow struct {
	Components []string `json:"Components"`
	Accessor   string   `json:"_Accessor"`
	Count      uint64   `json:"Count"`
	JSON       string   `json:"_JSON"`
}

func (self ElasticIngestor) HandleSystemVfsListDirectory(
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	if message == nil || message.VQLResponse == nil {
		return nil
	}

	reader := strings.NewReader(message.VQLResponse.JSONLResponse)
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, len(message.VQLResponse.JSONLResponse))
	scanner.Buffer(buf, len(message.VQLResponse.JSONLResponse))

	for scanner.Scan() {
		serialized := scanner.Text()
		row := &VFSRow{}
		err := json.Unmarshal([]byte(serialized), row)
		if err != nil {
			return err
		}

		accessor := row.Accessor
		if accessor == "" {
			accessor = "file"
		}

		components := append([]string{message.Source, accessor}, row.Components...)
		id := services.MakeId(utils.JoinComponents(components, "/"))
		vfs_response := &api_proto.VFSListResponse{
			Response: row.JSON,
			Columns: []string{"_FullPath", "_Accessor", "_Data", "Name",
				"Size", "Mode", "mtime", "atime", "ctime", "btime"},
			TotalRows: row.Count,
			ClientId:  message.Source,
			FlowId:    message.SessionId,
		}

		record := &vfs_service.VFSRecord{
			ClientId:   message.Source,
			Components: components,
			Downloads:  []string{},
			JSONData:   json.MustMarshalString(vfs_response),
		}
		err = services.SetElasticIndex(config_obj.OrgId, "vfs", id, record)
		if err != nil {
			return err
		}
	}

	return nil
}

const (
	updateDownloadQuery = `
{
    "script" : {
        "source": "ctx._source.downloads.addAll(params.download);",
        "lang": "painless",
        "params": {
          "download": %q
       }
    }
}
`
)

func (self ElasticIngestor) HandleSystemVfsUpload(
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	if message == nil || message.VQLResponse == nil {
		return nil
	}

	reader := strings.NewReader(message.VQLResponse.JSONLResponse)
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, len(message.VQLResponse.JSONLResponse))
	scanner.Buffer(buf, len(message.VQLResponse.JSONLResponse))

	downloads := make(map[string][]string)

	for scanner.Scan() {
		serialized := scanner.Text()
		row := &vfs_service.DownloadRow{}
		err := json.Unmarshal([]byte(serialized), row)
		if err != nil {
			return err
		}

		accessor := row.Accessor
		if accessor == "" {
			accessor = "file"
		}

		if len(row.Components) > 0 {
			components := append([]string{message.Source, accessor},
				row.Components[:len(row.Components)-1]...)
			row.Mtime = uint64(time.Now().Unix())
			id := services.MakeId(utils.JoinComponents(components, "/"))
			dir_downloads, _ := downloads[id]
			dir_downloads = append(dir_downloads, json.MustMarshalString(row))
			downloads[id] = dir_downloads
		}
	}

	// Update the downloads in all the VFS records we need to
	for id, downloads := range downloads {
		return services.UpdateIndex(
			config_obj.OrgId, "vfs", id,
			json.Format(updateDownloadQuery, downloads))
	}
	return nil
}
