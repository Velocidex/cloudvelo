package launcher

import (
	"context"
	"fmt"
	"sort"
	"time"

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

	// The index of flows in the GUI.
	defer cvelo_services.Count("WriteFlowIndex")

	return self.buildIndex(ctx, config_obj, flow.ClientId)
}

func (self *FlowStorageManager) buildIndex(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) error {

	cvelo_services.Count("FlowStorageManager: buildIndex")

	// Do not allow the index rebuild to be cancelled or we will end
	// up with a broken index.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

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

const getLatestFlowRecord = `
{
  "sort": [
   {"timestamp": {"order": "desc"}}
  ],
  "query": {
     "bool": {
       "must": [
         {"match": {"client_id" : %q}},
         {"match": {"doc_type" : "collection"}}
      ]}
  },
  "size": 1
}
`

// We only need to rebuild the index if the latest flow document is
// newer than the index.
func (self *FlowStorageManager) shouldRebuildIndex(
	ctx context.Context, config_obj *config_proto.Config,
	client_id string,

	// The rs reader of the index.
	rs_reader result_sets.ResultSetReader) bool {

	if rs_reader == nil || rs_reader.TotalRows() <= 0 {
		return true
	}

	// Within 1 minute we do not rebuild the index - we are ok with
	// the index being 1 minute out.
	now := utils.GetTime().Now()
	if now.Sub(rs_reader.MTime()) < time.Minute {
		return false
	}

	hit, err := cvelo_services.GetElasticRecordByQuery(ctx,
		config_obj.OrgId, cvelo_services.TRANSIENT, json.Format(
			getLatestFlowRecord, client_id))

	// If there are no flows at all in this client, we dont need to
	// build any indexes.
	if err != nil || len(hit) == 0 {
		return false
	}

	item := &cvelo_schema_api.ArtifactCollectorRecord{}
	err = json.Unmarshal(hit, item)
	if err == nil {
		last_modified := time.Unix(0, item.Timestamp)
		if rs_reader.MTime().After(last_modified) {
			// Skip the update if the result set is newer than the
			// last_modified record.
			return false
		}
	}
	return true
}
