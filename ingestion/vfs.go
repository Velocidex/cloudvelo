package ingestion

import (
	"bufio"
	"context"
	"strings"

	"www.velocidex.com/golang/cloudvelo/filestore"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	cvelo_vfs_service "www.velocidex.com/golang/cloudvelo/services/vfs_service"
	"www.velocidex.com/golang/cloudvelo/vql/uploads"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self Ingestor) HandleSystemVfsListDirectory(
	ctx context.Context,
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
		row := &services.VFSListRow{}
		err := json.Unmarshal([]byte(serialized), row)
		if err != nil || row.Stats == nil {
			continue
		}

		accessor := row.Accessor
		if accessor == "" {
			accessor = "auto"
		}

		components := append([]string{message.Source, accessor}, row.Components...)
		id := cvelo_services.MakeId(utils.JoinComponents(components, "/"))

		stats := &api_proto.VFSListResponse{
			Timestamp: uint64(utils.GetTime().Now().UnixNano()),
			ClientId:  message.Source,
			FlowId:    message.SessionId,
			TotalRows: row.Stats.EndIdx - row.Stats.StartIdx,
			Artifact:  "System.VFS.ListDirectory/Listing",
			StartIdx:  row.Stats.StartIdx,
			EndIdx:    row.Stats.EndIdx,
		}

		record := &cvelo_vfs_service.VFSRecord{
			Id:         id,
			ClientId:   message.Source,
			Components: components,
			Downloads:  []string{},
			JSONData:   json.MustMarshalString(stats),
			DocType:    "vfs",
			DocId:      id,
			Timestamp:  utils.GetTime().Now().UnixNano(),
		}

		err = cvelo_services.SetElasticIndexAsync(
			config_obj.OrgId, "transient", "",
			cvelo_services.BulkUpdateCreate, record)

		if err != nil {
			return err
		}
	}

	return nil
}

func (self Ingestor) HandleSystemVfsUpload(
	ctx context.Context,
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
		row := &cvelo_vfs_service.DownloadRow{}
		err := json.Unmarshal([]byte(serialized), row)
		if err != nil {
			return err
		}

		accessor := row.Accessor
		if accessor == "" {
			accessor = "auto"
		}

		// Here row.Components are the client's VFS components, we
		// need to convert it into a VFSDownloadInfoPath proto. Inside
		// that the Components field is the full filestore path
		if len(row.Components) > 0 {
			// Record where the actual file was stored.
			row.FSComponents = filestore.S3ComponentsForClientUpload(
				&uploads.UploadRequest{
					ClientId:   message.Source,
					SessionId:  message.SessionId,
					Accessor:   accessor,
					Components: row.Components,
				})

			// The containing directory will contain an additional
			// download record.
			dir_components := append([]string{message.Source, accessor},
				row.Components[:len(row.Components)-1]...)
			file_components := append([]string{message.Source, accessor},
				row.Components...)

			row.Mtime = uint64(utils.GetTime().Now().Unix())
			dir_id := cvelo_services.MakeId(
				utils.JoinComponents(dir_components, "/"))
			file_id := cvelo_services.MakeId(
				utils.JoinComponents(file_components, "/"))
			stats := &cvelo_vfs_service.VFSRecord{
				Id:        dir_id,
				ClientId:  message.Source,
				FlowId:    message.SessionId,
				DocId:     "download_" + file_id,
				DocType:   "vfs",
				Downloads: []string{json.MustMarshalString(row)},
				Timestamp: utils.GetTime().Now().UnixNano(),
			}

			cvelo_services.SetElasticIndexAsync(
				config_obj.OrgId, "transient", "",
				cvelo_services.BulkUpdateCreate, stats)
		}
	}
	return nil
}
