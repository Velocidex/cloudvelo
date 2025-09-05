package hunt_dispatcher

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/paths"
)

// availableHuntDownloadFiles returns the prepared zip downloads available to
// be fetched by the user at this moment.
func availableHuntDownloadFiles(config_obj *config_proto.Config,
	hunt_id string) (*api_proto.AvailableDownloads, error) {

	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	download_file := hunt_path_manager.GetHuntDownloadsFile(false, "", false)
	download_path := download_file.Dir()

	return GetAvailableDownloadFiles(config_obj, download_path)
}
