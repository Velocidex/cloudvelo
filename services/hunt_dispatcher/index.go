package hunt_dispatcher

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

type HuntIndexEntry struct {
	HuntId      string `json:"HuntId"`
	Description string `json:"Description"`
	Created     uint64 `json:"Created"`
	Started     uint64 `json:"Started"`
	Expires     uint64 `json:"Expires"`
	Creator     string `json:"Creator"`

	// The hunt object is serialized into JSON here to make it quicker
	// to write the index if nothing is changed.
	Hunt string `json:"Hunt"`
	Tags string `json:"Tags"`
}

func (self *HuntStorageManagerImpl) FlushIndex(
	ctx context.Context) error {

	start_row := 0
	length := 1000

	hits, _, err := cvelo_services.QueryElasticRaw(
		ctx, self.config_obj.OrgId,
		"persisted", json.Format(getAllHuntsQuery, start_row, length))
	if err != nil {
		return err
	}

	hunt_path_manager := paths.NewHuntPathManager("")
	file_store_factory := file_store.GetFileStore(self.config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		hunt_path_manager.HuntIndex(), json.DefaultEncOpts(),

		// We need the index to be written immediately so it is
		// visible in the GUI.
		utils.SyncCompleter,
		result_sets.TruncateMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	seen_tags := make(map[string]bool)

	for _, hit := range hits {
		entry := &HuntEntry{}
		err = json.Unmarshal(hit, entry)
		if err != nil {
			continue
		}

		hunt_info, err := entry.GetHunt()
		if err != nil {
			continue
		}

		for _, tag := range hunt_info.Tags {
			seen_tags[tag] = true
		}

		rs_writer.Write(ordereddict.NewDict().
			Set("HuntId", hunt_info.HuntId).
			Set("Description", hunt_info.HuntDescription).
			// Store the tags in the index so we can search for them.
			Set("Tags", strings.Join(hunt_info.Tags, "\n")).
			Set("Created", hunt_info.CreateTime).
			Set("Started", hunt_info.StartTime).
			Set("Expires", hunt_info.Expires).
			Set("Creator", hunt_info.Creator).
			Set("Hunt", entry.Hunt))
	}

	record := &HuntEntry{}
	for tag := range seen_tags {
		record.Labels = append(record.Labels, tag)
	}

	return cvelo_services.SetElasticIndex(ctx,
		self.config_obj.OrgId, "persisted", TAGS_ID, record)
}
