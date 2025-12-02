package hunt_dispatcher

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/codesoap/lineworker"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/indexing"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type job_t struct {
	ClientId string
	FlowId   string
	Details  *api_proto.FlowDetails
}

const (
	getHuntsFlowsQuery = `{ "from": %q,
  "query": {
    "bool": {
      "must": [
         {"match": {"hunt_id" : %q}},
         {"match": {"doc_type" : "hunt_flow"}}
      ]}
  }
}
`
	getLatestHuntFlowForHuntId = `{
  "sort": [{
     "timestamp": { "order": "desc" }
  }],
  "size": 1,
  "query": {
    "bool": {
      "must": [
         {"match": {"hunt_id" : %q}},
         {"match": {"doc_type" : "hunt_flow"}}
      ]}
  }
}`
)

type HuntFlowEntry struct {
	HuntId    string `json:"hunt_id"`
	Timestamp int64  `json:"timestamp"`
	ClientId  string `json:"client_id"`
	FlowId    string `json:"flow_id"`
	Status    string `json:"status"`
	Type      string `json:"type"`
	DocType   string `json:"doc_type"`
}

func (self *HuntDispatcher) shouldRebuildIndex(
	ctx context.Context,
	config_obj *config_proto.Config,
	hunt_id string) bool {

	file_store_factory := file_store.GetFileStore(config_obj)
	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	table_to_query := hunt_path_manager.EnrichedClients()

	rs_reader, err := result_sets.NewResultSetReader(
		file_store_factory, table_to_query)
	if err != nil {
		return true
	}

	now := utils.GetTime().Now()

	// If the index is too old, then rebuild it anyway. TODO: This can
	// be relaxed when the indexing gets more stable.
	if now.Sub(rs_reader.MTime()) > time.Hour*12 {
		return true
	}

	// First get the latest hunt_flow document. This indicates the
	// last time the hunt was seen.
	hit, err := cvelo_services.GetElasticRecordByQuery(ctx,
		config_obj.OrgId, cvelo_services.TRANSIENT, json.Format(
			getLatestHuntFlowForHuntId, hunt_id))
	if err != nil || len(hit) == 0 {
		return true
	}

	entry := &HuntFlowEntry{}
	err = json.Unmarshal(hit, entry)
	if err != nil {
		return true
	}

	// This is the last modified time of any flow in the hunt.
	last_modified := time.Unix(entry.Timestamp, 0)

	// Now check the last modified time of the result set.
	if rs_reader.MTime().Before(last_modified) {
		return true
	}

	// Skip the update if the result set is newer than the
	// last_modified record.
	return false
}

func (self *HuntDispatcher) syncFlowTables(
	ctx context.Context,
	config_obj *config_proto.Config,
	hunt_id string) error {

	defer cvelo_services.Summarize("HuntDispatcher: syncFlowTables")()

	count := 0
	seen := make(map[string]bool)

	laucher_manager, err := services.GetLauncher(config_obj)
	if err != nil {
		return err
	}

	err = indexing.PopulateClientInfoCache(ctx, config_obj)
	if err != nil {
		return err
	}

	// Needs to be immediately available because we will query it
	// right away.
	defer cvelo_services.FlushIndex(ctx, self.config_obj.OrgId, "transient")

	file_store_factory := file_store.GetFileStore(config_obj)
	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	table_to_query := hunt_path_manager.EnrichedClients()

	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		table_to_query, json.DefaultEncOpts(),
		utils.SyncCompleter, result_sets.TruncateMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	query := json.Format(getHuntsFlowsQuery, 0, hunt_id)
	hits, err := cvelo_services.QueryChan(
		ctx, config_obj, 1000, self.config_obj.OrgId,
		cvelo_services.TRANSIENT, query, "timestamp")
	if err != nil {
		return err
	}

	// Get the flow details in a worker pool
	pool := lineworker.NewWorkerPool(30, func(entry *job_t) (*job_t, error) {
		flow, err := laucher_manager.GetFlowDetails(
			ctx, config_obj, services.GetFlowOptions{},
			entry.ClientId, entry.FlowId)
		if err == nil {
			entry.Details = flow
		}
		return entry, err
	})
	go func() {
		defer pool.Stop()

		for hit := range hits {
			entry := &HuntFlowEntry{}
			err = json.Unmarshal(hit, entry)
			if err != nil {
				continue
			}

			key := entry.FlowId + entry.ClientId
			if seen[key] {
				continue
			}
			seen[key] = true
			pool.Process(&job_t{
				ClientId: entry.ClientId,
				FlowId:   entry.FlowId,
			})
		}
	}()

	for {
		entry, err := pool.Next()
		if err == lineworker.EOS {
			break
		}
		if err != nil {
			continue
		}
		count++

		flow := entry.Details
		rs_writer.WriteJSONL([]byte(
			json.Format(`{"ClientId": %q, "Hostname": %q, "FlowId": %q, "StartedTime": %q, "State": %q, "Duration": %q, "TotalBytes": %q, "TotalRows": %q}
`,
				entry.ClientId,
				services.GetHostname(ctx, config_obj, entry.ClientId),
				entry.FlowId,
				flow.Context.StartTime/1000,
				flow.Context.State.String(),
				flow.Context.ExecutionDuration/1000000000,
				flow.Context.TotalUploadedBytes,
				flow.Context.TotalCollectedRows)), 1)
	}

	return nil
}

func (self HuntDispatcher) GetFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	options services.FlowSearchOptions, scope vfilter.Scope,
	hunt_id string, start int) (chan *api_proto.FlowDetails, int64, error) {

	output_chan := make(chan *api_proto.FlowDetails)

	if self.shouldRebuildIndex(ctx, config_obj, hunt_id) {
		err := self.syncFlowTables(ctx, config_obj, hunt_id)
		if err != nil {
			close(output_chan)
			return output_chan, 0, err
		}
	}

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		close(output_chan)
		return output_chan, 0, err
	}

	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	table_to_query := hunt_path_manager.EnrichedClients()

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_reader, err := result_sets.NewResultSetReaderWithOptions(
		ctx, self.config_obj, file_store_factory,
		table_to_query, options.ResultSetOptions)
	if err != nil {
		close(output_chan)
		return output_chan, 0, err
	}

	// Seek to the row we need.
	err = rs_reader.SeekToRow(int64(start))
	if errors.Is(err, io.EOF) {
		close(output_chan)
		rs_reader.Close()

		return output_chan, 0, nil
	}

	if err != nil {
		close(output_chan)
		rs_reader.Close()
		return output_chan, 0, err
	}

	go func() {
		defer close(output_chan)
		defer rs_reader.Close()

		defer cvelo_services.Summarize("HuntDispatcher: GetFlows")()

		for row := range rs_reader.Rows(ctx) {
			client_id, pres := row.GetString("ClientId")
			if !pres {
				client_id, pres = row.GetString("client_id")
				if !pres {
					continue
				}
			}
			flow_id, pres := row.GetString("FlowId")
			if !pres {
				flow_id, pres = row.GetString("flow_id")
				if !pres {
					continue
				}
			}

			var collection_context *api_proto.FlowDetails
			if options.BasicInformation {
				collection_context = &api_proto.FlowDetails{
					Context: &flows_proto.ArtifactCollectorContext{
						ClientId:  client_id,
						SessionId: flow_id,
					},
				}
				// If the user wants detailed flow information we need
				// to fetch this now. For many uses this is not
				// necessary so we can get away with very basic
				// information.
			} else {
				collection_context, err = launcher.GetFlowDetails(
					ctx, config_obj, services.GetFlowOptions{},
					client_id, flow_id)
				if err != nil {
					continue
				}
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- collection_context:
			}
		}
	}()

	return output_chan, rs_reader.TotalRows(), nil
}
