package hunt_dispatcher

import (
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
)

// availableHuntDownloadFiles returns the prepared zip downloads available to
// be fetched by the user at this moment.
func availableHuntDownloadFiles(config_obj *config_proto.Config,
	hunt_id string) (*api_proto.AvailableDownloads, error) {

	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	download_file := hunt_path_manager.GetHuntDownloadsFile(false, "", false)
	download_path := download_file.Dir()

	return getAvailableDownloadFiles(config_obj, download_path)
}

func getAvailableDownloadFiles(config_obj *config_proto.Config,
	download_dir api.FSPathSpec) (*api_proto.AvailableDownloads, error) {
	result := &api_proto.AvailableDownloads{}

	file_store_factory := file_store.GetFileStore(config_obj)
	files, err := file_store_factory.ListDirectory(download_dir)
	if err != nil {
		return nil, err
	}

	is_complete := func(name string) bool {
		for _, item := range files {
			ps := item.PathSpec()
			// If there is a lock file we are not done.
			if ps.Base() == name &&
				ps.Type() == api.PATH_TYPE_FILESTORE_LOCK {
				return false
			}
		}
		return true
	}

	for _, item := range files {
		ps := item.PathSpec()

		// Skip lock files
		if ps.Type() == api.PATH_TYPE_FILESTORE_LOCK {
			continue
		}

		result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
			Name:     item.Name(),
			Type:     api.GetExtensionForFilestore(ps),
			Path:     ps.AsClientPath(),
			Size:     uint64(item.Size()),
			Date:     item.ModTime().UTC().Format(time.RFC3339),
			Complete: is_complete(ps.Base()),
		})
	}

	return result, nil
}
