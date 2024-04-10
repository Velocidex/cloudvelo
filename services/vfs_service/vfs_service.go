package vfs_service

/*
   The VFS index consists of split VFSRecord objects.

   Each VFSRecord is stored in a unique id based on the full path to
   the object (file or directory). Therefore each file or directory in
   the VFS will have a unique document in the index that describes it.

   The Id field in the VFSRecord refers to the document ID of the
   containing directory.

   The VFSRecord corresponding to a directory contains a pointer to
   the flow results of the directory listing that outlines it.

   If a file inside a directory was also downloaded, it's
   corresponding VFSRecord will contain a descriptor about the
   download operation.

   When we list the directory, we query for all VFSRecords that
   contain the same directory ID. This will give both the record
   corresponding to the actual directory as well as multiple records
   corresponding to any downloads in the directory.

   For example:

   Record 1 corresponds to the directory /auto/etc.
   It has a DocId = 72cd427a06bbe03e1bb2cbd496243e98b0625bb4

   {
      Id: "72cd427a06bbe03e1bb2cbd496243e98b0625bb4",  <- the parent dir ID
      ClientId: "C.39a107c4c58c5efa",
      Components: []string len: 3, cap: 4, [  <-- The full path of this record
        "C.39a107c4c58c5efa",
        "auto",
        "etc",
      ],
      Downloads: []string len: 0, cap: 0, [], <- empty download record
      JSONData: "{\"timestamp\":1684288672,\"total_rows\":243,\"client_id\":\"C.39a107c4c58c5efa\",\"flow_id\":\"F.CHI397CB8BK66\",\"artifact\":\"System.VFS.ListDirectory/Listing\",\"end_idx\":243}",}
   }

   Record 2 corresponds to a file inside that had a download /auto/etc/
   It has a DocId "download_XXXX" and contains a serialized DownloadRow object

   {
     Id: "72cd427a06bbe03e1bb2cbd496243e98b0625bb4",
     ClientId: "C.39a107c4c58c5efa",
     Components: []string len: 0, cap: 0, nil,
     Downloads: []string len: 1, cap: 4, [
        "{\"mtime\":1684295452,\"components\":[\"etc\",\".pwd.lock\"],\"flow_id\":\"F.CHI4U5UOB2QH4\",\"in_flight\":true}",
     ],
     JSONData: ""  <- This is empty for file download records.
   }

   Note that the Id field corresponds to the doc id of the parent
   directory is the same for both records but they have different doc
   id.
*/

import (
	"context"
	"errors"
	"sync"
	"time"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type VFSService struct {
	ctx        context.Context
	config_obj *config_proto.Config
}

func (self *VFSService) ListDirectories(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	components []string) (*api_proto.VFSListResponse, error) {

	if len(components) == 0 {
		return renderRootVFS(), nil
	}

	return renderDBVFS(ctx, config_obj, client_id, components)
}

func (self *VFSService) WriteDownloadInfo(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	accessor string,
	client_components []string,
	record *flows_proto.VFSDownloadInfo) error {

	if len(client_components) == 0 {
		return nil
	}

	dir_components := append([]string{client_id, accessor},
		client_components[:len(client_components)-1]...)
	file_components := append([]string{client_id, accessor},
		client_components...)

	download_record := &DownloadRow{
		Mtime:        uint64(utils.GetTime().Now().UnixNano()),
		Components:   client_components,
		FSComponents: file_components,
		InFlight:     record.InFlight,
		FlowId:       record.FlowId,
	}

	dir_id := cvelo_services.MakeId(
		utils.JoinComponents(dir_components, "/"))
	file_id := cvelo_services.MakeId(
		utils.JoinComponents(file_components, "/"))
	stats := &VFSRecord{
		Id:        dir_id,
		DocId:     "download_" + file_id,
		DocType:   "vfs",
		ClientId:  client_id,
		Downloads: []string{json.MustMarshalString(download_record)},
		Timestamp: utils.GetTime().Now().UnixNano(),
	}

	// Write synchronously so the GUI updates the download file right
	// away.
	err := cvelo_services.SetElasticIndex(
		ctx, config_obj.OrgId, "transient", "", stats)

	utils.GetTime().Sleep(time.Second)

	return err
}

func (self *VFSService) StatDirectory(
	config_obj *config_proto.Config,
	client_id string,
	components []string) (*api_proto.VFSListResponse, error) {

	_, stat, err := self.readDirectoryWithDownloads(self.ctx, config_obj,
		client_id, components)
	return stat, err
}

func (self *VFSService) StatDownload(
	config_obj *config_proto.Config,
	client_id string,
	accessor string,
	path_components []string) (*flows_proto.VFSDownloadInfo, error) {
	return nil, errors.New("Not Implemented")
}

func NewVFSService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.VFSService, error) {

	vfs_service := &VFSService{
		config_obj: config_obj,
		ctx:        ctx,
	}

	return vfs_service, nil
}
