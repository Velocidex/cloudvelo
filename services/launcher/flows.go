package launcher

import (
	"errors"
	"sort"
	"time"

	cvelo_schema_api "www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
)

var (
	NotFoundError = errors.New("Not found")
)

// Get all the flows for this client. TODO: implement paging
const getFlowsQuery = `
{
  "sort": [{
    "create_time": {"order": "desc"}
  }],
  "query": {
     "match": {"client_id" : %q}
  },
  "from": %q,
  "size": %q
}
`

func (self Launcher) GetFlows(
	config_obj *config_proto.Config,
	client_id string, include_archived bool,
	flow_filter func(flow *flows_proto.ArtifactCollectorContext) bool,
	offset uint64, length uint64) (*api_proto.ApiFlowResponse, error) {

	if length == 0 {
		length = 1000
	}

	hits, err := cvelo_services.QueryElasticRaw(self.ctx,
		self.config_obj.OrgId, "collections",
		json.Format(getFlowsQuery, client_id, offset, length))
	if err != nil {
		return nil, err
	}

	lookup := make(map[string]*cvelo_schema_api.ArtifactCollectorContext)

	for _, hit := range hits {
		item := &cvelo_schema_api.ArtifactCollectorContext{}
		err = json.Unmarshal(hit, &item)
		if err == nil {
			record, pres := lookup[item.SessionId]
			if !pres {
				record = &cvelo_schema_api.ArtifactCollectorContext{}
			}
			mergeRecords(record, item)
			lookup[item.SessionId] = record
		}
	}

	result := &api_proto.ApiFlowResponse{}
	for _, record := range lookup {
		flow, err := cvelo_schema_api.ArtifactCollectorContextToProto(record)
		if err == nil {
			cleanUpContext(flow)
			result.Items = append(result.Items, flow)
		}
	}

	// Show newer sessions before older sessions.
	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].SessionId > result.Items[j].SessionId
	})

	return result, nil
}

const getFlowDetailsQuery = `
{
  "sort": [{"timestamp": {"order": "asc"}}],
  "query": {
     "bool": {
       "must": [
         {"match": {"client_id" : %q}},
         {"match": {"session_id" : %q}}
      ]}
  }
}
`

func (self *Launcher) GetFlowDetails(
	config_obj *config_proto.Config,
	client_id string, flow_id string) (*api_proto.FlowDetails, error) {
	if flow_id == "" || client_id == "" {
		return &api_proto.FlowDetails{}, nil
	}

	hits, err := cvelo_services.QueryElasticRaw(self.ctx,
		self.config_obj.OrgId, "collections",
		json.Format(getFlowDetailsQuery, client_id, flow_id))
	if err != nil {
		return nil, err
	}

	if len(hits) == 0 {
		return nil, NotFoundError
	}

	result := &cvelo_schema_api.ArtifactCollectorContext{}

	for _, hit := range hits {
		item := &cvelo_schema_api.ArtifactCollectorContext{}
		err = json.Unmarshal(hit, item)
		if err != nil {
			return nil, err
		}

		mergeRecords(result, item)
	}

	flow, err := cvelo_schema_api.ArtifactCollectorContextToProto(result)
	if err != nil {
		return nil, err
	}
	cleanUpContext(flow)

	availableDownloads, _ := availableDownloadFiles(config_obj, client_id, flow_id)
	return &api_proto.FlowDetails{
		Context:            flow,
		AvailableDownloads: availableDownloads,
	}, nil
}

func mergeRecords(output, input *cvelo_schema_api.ArtifactCollectorContext) {
	if len(input.QueryStats) > 0 {
		output.QueryStats = append(output.QueryStats, input.QueryStats...)
	}

	if input.Raw != "" {
		output.Raw = input.Raw
	}

	if input.SessionId != "" {
		output.SessionId = input.SessionId
		output.ClientId = input.ClientId
	}

	if input.CreateTime > 0 {
		output.CreateTime = input.CreateTime
	}

	if input.LastActive > 0 {
		output.LastActive = input.LastActive
	}
}

// availableDownloads returns the prepared zip downloads available to
// be fetched by the user at this moment.
func availableDownloadFiles(config_obj *config_proto.Config,
	client_id string, flow_id string) (*api_proto.AvailableDownloads, error) {

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	download_dir := flow_path_manager.GetDownloadsDirectory()

	return getAvailableDownloadFiles(config_obj, download_dir)
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

func cleanUpContext(item *flows_proto.ArtifactCollectorContext) {
	// Remove empty string from the ArtifactsWithResults
	results := []string{}
	for _, i := range item.ArtifactsWithResults {
		if i != "" {
			results = append(results, i)
		}
	}

	item.ArtifactsWithResults = results
}
