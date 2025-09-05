package launcher

import (
	"context"
	"fmt"
	"sort"

	"github.com/Velocidex/ordereddict"
	cvelo_schema_api "www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	getCollectionsQuery = `{
  "query": {
    "bool": {
      "must": [
        {
          "match": {
            "doc_type": "collection"
          }
        }, {
          "match": {
            "client_id": %q
          }
        }, {
          "match": {
            "type": "main"
          }
        }
      ]
    }
  },
  "size": 10000
}
`
)

func (self *FlowStorageManager) WriteFlowIndex(
	ctx context.Context,
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext) error {

	return self.buildIndex(ctx, config_obj, flow.ClientId)
}

func (self *FlowStorageManager) buildIndex(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) error {

	query := fmt.Sprintf(getCollectionsQuery, client_id)
	records, _, err := cvelo_services.QueryElasticRaw(ctx,
		config_obj.OrgId, "transient", query)
	if err != nil {
		return err
	}

	seen := make(map[string]bool)

	var flows []*flows_proto.ArtifactCollectorContext
	for _, record := range records {
		item := &cvelo_schema_api.ArtifactCollectorRecord{}
		err = json.Unmarshal(record, &item)
		if err != nil {
			continue
		}

		_, pres := seen[item.SessionId]
		if pres {
			continue
		}

		seen[item.SessionId] = true

		flow_context, err := item.ToProto()
		if err != nil {
			continue
		}

		flows = append(flows, flow_context)
	}

	sort.Slice(flows, func(i, j int) bool {
		return flows[i].SessionId > flows[j].SessionId
	})

	// Now write the index to storage.
	client_path_manager := paths.NewClientPathManager(client_id)
	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		client_path_manager.FlowIndex(), json.DefaultEncOpts(),
		// We need the index to be written immediately so it is
		// visible in the GUI.
		utils.SyncCompleter,
		result_sets.TruncateMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	for _, flow := range flows {
		summary := ordereddict.NewDict().
			Set("FlowId", flow.SessionId).
			Set("Artifacts", flow.Request.Artifacts).
			Set("Created", flow.StartTime).
			Set("Creator", flow.Request.Creator)

		rs_writer.Write(summary)
	}

	return nil
}
