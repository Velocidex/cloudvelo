package exports

import (
	"context"
	"sort"
	"strings"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ExportRecord struct {
	Timestamp  int64  `json:"timestamp"`
	ClientId   string `json:"client_id"`
	VFSPath    string `json:"vfs_path"`
	FlowId     string `json:"flow_id"`
	NotebookId string `json:"notebook_id"`
	HuntId     string `json:"hunt_id"`
	Artifact   string `json:"artifact"`
	Type       string `json:"type"`
	DocType    string `json:"doc_type"`
	JSONData   string `json:"data"` // serialized api_proto.ContainerStats
	ID         string `json:"id"`
}

type ExportManager struct {
	config_obj *config_proto.Config
}

func (self *ExportManager) SetContainerStats(
	ctx context.Context,
	config_obj *config_proto.Config,
	stats *api_proto.ContainerStats,
	opts services.ContainerOptions) error {

	if opts.ContainerFilename == nil {
		return utils.InvalidArgError
	}

	serialized, err := json.Marshal(stats)
	if err != nil {
		return err
	}

	record := &ExportRecord{
		Timestamp: utils.GetTime().Now().UnixNano(),
		VFSPath:   opts.ContainerFilename.AsClientPath(),
		Type:      "export",
		JSONData:  string(serialized),
	}

	switch opts.Type {
	case services.NotebookExport:
		record.NotebookId = opts.NotebookId

	case services.FlowExport:
		record.ClientId = opts.ClientId
		record.FlowId = opts.FlowId

	case services.HuntExport:
		record.HuntId = opts.HuntId

	default:
		return utils.Wrap(utils.InvalidArgError, "Invalid container stat type")
	}

	return cvelo_services.SetElasticIndex(context.Background(),
		utils.GetOrgId(config_obj), "transient", cvelo_services.DocIdRandom, record)
}

const (
	available_export_files = `
{"query": {
     "bool": {
       "must": [
         %s
         {"match": {"type": "export"}}
       ]
     }
 }
}
`
)

func (self *ExportManager) GetAvailableDownloadFiles(
	ctx context.Context,
	config_obj *config_proto.Config,
	opts services.ContainerOptions) (*api_proto.AvailableDownloads, error) {

	cvelo_services.Count("ExportManager: GetAvailableDownloadFiles")

	filter := ""
	switch opts.Type {
	case services.NotebookExport:
		filter = json.Format(`{"match": {"notebook_id": %q}},`, opts.NotebookId)

	case services.FlowExport:
		filter = json.Format(
			`{"match": {"client_id": %q}},{"match": {"flow_id": %q}},`,
			opts.ClientId, opts.FlowId)

	case services.HuntExport:
		filter = json.Format(`{"match": {"hunt_id": %q}},`, opts.HuntId)

	default:
		return nil, utils.Wrap(utils.InvalidArgError, "Invalid container stat type")
	}

	query := json.Format(available_export_files, filter)

	hits, err := cvelo_services.QueryChan(ctx, config_obj, 1000,
		utils.GetOrgId(config_obj), "transient", query, ">timestamp")
	if err != nil {
		return nil, err
	}

	result := &api_proto.AvailableDownloads{}

	// Key is VFSPath
	seen := make(map[string]*api_proto.ContainerStats)

	for hit := range hits {
		record := &ExportRecord{}
		err = json.Unmarshal(hit, &record)
		if err != nil {
			continue
		}

		stats := &api_proto.ContainerStats{}
		err = json.Unmarshal([]byte(record.JSONData), stats)
		if err != nil {
			continue
		}

		_, pres := seen[record.VFSPath]
		if !pres {
			seen[record.VFSPath] = stats
		}
	}

	for _, stats := range seen {
		name := ""
		if len(stats.Components) > 0 {
			name = stats.Components[len(stats.Components)-1]
		}

		result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
			Name:     name,
			Type:     stats.Type,
			Size:     stats.TotalCompressedBytes,
			Path:     strings.Join(stats.Components, "/"),
			Complete: stats.Hash != "",
			Stats:    stats,
		})
	}

	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].Name < result.Files[j].Name
	})

	return result, nil
}

func NewExportManager(config_obj *config_proto.Config) (services.ExportManager, error) {
	return &ExportManager{
		config_obj: config_obj,
	}, nil
}
