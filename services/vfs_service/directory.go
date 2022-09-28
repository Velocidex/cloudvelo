package vfs_service

import (
	"www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
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

func (self *VFSService) renderDBVFS(
	config_obj *config_proto.Config,
	client_id string,
	components []string) (*api_proto.VFSListResponse, error) {

	result := &api_proto.VFSListResponse{}
	components = append([]string{client_id}, components...)
	id := services.MakeId(utils.JoinComponents(components, "/"))
	record := &VFSRecord{}

	serialized, err := services.GetElasticRecord(self.ctx,
		self.config_obj.OrgId, "vfs", id)
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

	if len(record.Downloads) == 0 {
		return result, nil
	}

	json.Dump(record.Downloads)

	// When there are downloads we need to merge them into the
	// results.
	downloads := make(map[string]*flows_proto.VFSDownloadInfo)
	for _, serialized := range record.Downloads {
		row := &DownloadRow{}
		err = json.Unmarshal([]byte(serialized), row)
		if err == nil && len(row.Components) > 0 {
			name := row.Components[len(row.Components)-1]
			downloads[name] = &flows_proto.VFSDownloadInfo{
				Components: row.FSComponents,
				Size:       row.Size,
				MD5:        row.Md5,
				SHA256:     row.Sha256,
				Mtime:      row.Mtime,
			}
		}
	}

	rows, err := utils.ParseJsonToDicts([]byte(result.Response))
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		name, pres := row.GetString("Name")
		if !pres {
			continue
		}

		download_info, pres := downloads[name]
		if pres {
			row.Set("Download", download_info)
			json.Dump(row)
		}
	}

	result.Response = json.MustMarshalString(rows)

	return result, nil
}
