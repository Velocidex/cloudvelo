package launcher_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	cvelo_api "www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/client_info"
	cvelo_launcher "www.velocidex.com/golang/cloudvelo/services/launcher"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

const (
	getAllItemsQuery = `
{"query": {"match_all" : {}}}
`

	// Query to retrieve all the task queued for a client.
	getClientTasksQuery = `{
  "sort": [{
    "timestamp": {"order": "asc", "unmapped_type" : "long"}
  }],
  "query": {
    "bool": {
      "must": [
 		 {"match": {"doc_type" : "task"}},
         {"match": {"client_id" : %q}}
      ]}
  }
}
`
)

type LauncherTestSuite struct {
	*testsuite.CloudTestSuite
}

func (self *LauncherTestSuite) TestLauncher() {
	config_obj := self.ConfigObj.VeloConf()
	client_id := "C.1234"
	set_flow_id := "F.1234"

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	closer := utils.SetFlowIdForTests(set_flow_id)

	repository := self.loadTestArtifact(config_obj)

	self.seedClient(config_obj, client_id)

	acl_manager := acl_managers.NullACLManager{}
	flow_id, err := launcher.ScheduleArtifactCollection(
		self.Ctx, config_obj, acl_manager,
		repository, &flows_proto.ArtifactCollectorArgs{
			ClientId:  client_id,
			Artifacts: []string{"TestArtifact"},
		}, nil)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), flow_id, flow_id)

	err = cvelo_services.FlushBulkIndexer()
	assert.NoError(self.T(), err)

	// Wait here for the async flow to be written.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		flows, _ := launcher.GetFlows(self.Ctx,
			config_obj, client_id, result_sets.ResultSetOptions{}, 0, 100)
		return len(flows.Items) > 0
	})

	// Get Flows API
	flows, err := launcher.GetFlows(self.Ctx,
		config_obj, client_id, result_sets.ResultSetOptions{}, 0, 100)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 1, len(flows.Items))
	assert.Equal(self.T(), flow_id, flows.Items[0].SessionId)

	details, err := launcher.GetFlowDetails(self.Ctx, config_obj,
		services.GetFlowOptions{}, client_id, flow_id)
	assert.NoError(self.T(), err)

	// Make sure the flow is in the running state
	assert.Equal(self.T(),
		flows_proto.ArtifactCollectorContext_RUNNING,
		details.Context.State)

	// Check the requests are recorded
	requests, err := launcher.Storage().GetFlowRequests(
		self.Ctx, config_obj, client_id, flow_id, 0, 100)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(requests.Items))
	assert.NotNil(self.T(), requests.Items[0].FlowRequest)
	assert.True(self.T(), len(requests.Items[0].FlowRequest.VQLClientActions) > 0)

	// Make sure tasks are scheduled
	tasks, err := PeekClientTasks(self.Ctx, config_obj, client_id)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 1, len(tasks))
	assert.Equal(self.T(), client_id, tasks[0].ClientId)
	assert.Equal(self.T(), flow_id, tasks[0].FlowId)

	// Now cancel the collection
	cancel_response, err := launcher.CancelFlow(
		self.Ctx, config_obj, client_id, flow_id, "admin")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), flow_id, cancel_response.FlowId)

	err = cvelo_services.FlushBulkIndexer()
	assert.NoError(self.T(), err)

	// Check the tasks queue - we just add a cancel message but do not
	// remove the old message.
	tasks, err = PeekClientTasks(self.Ctx, config_obj, client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 2, len(tasks))
	assert.Equal(self.T(), client_id, tasks[1].ClientId)

	message := &crypto_proto.VeloMessage{}
	err = protojson.Unmarshal([]byte(tasks[1].JSONData), message)
	assert.NoError(self.T(), err)
	assert.NotNil(self.T(), message.Cancel)

	// Make sure the collection is marked as cancelled now.
	details, err = launcher.GetFlowDetails(self.Ctx, config_obj,
		services.GetFlowOptions{}, client_id, flow_id)
	assert.NoError(self.T(), err)

	// Make sure the flow is in the error state
	assert.Equal(self.T(),
		flows_proto.ArtifactCollectorContext_ERROR,
		details.Context.State)

	// Create a second collection
	closer()

	closer = utils.SetFlowIdForTests(set_flow_id + "second")
	defer closer()

	flow_id, err = launcher.ScheduleArtifactCollection(
		self.Ctx, config_obj, acl_manager,
		repository, &flows_proto.ArtifactCollectorArgs{
			ClientId:  client_id,
			Artifacts: []string{"TestArtifact"},
		}, nil)
	assert.NoError(self.T(), err)

	err = cvelo_services.FlushBulkIndexer()
	assert.NoError(self.T(), err)

	flows, err = launcher.GetFlows(self.Ctx,
		config_obj, client_id, result_sets.ResultSetOptions{}, 0, 100)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), uint64(2), flows.Total)
	assert.Equal(self.T(), 2, len(flows.Items))

	// Make sure the latest flow is first
	assert.Equal(self.T(), "F.1234second", flows.Items[0].SessionId)
}

func (self *LauncherTestSuite) TestLoadCollectionContextFallback() {
	config_obj := self.ConfigObj.VeloConf()
	client_id := "C.5678"
	set_flow_id := "F.5678"

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	self.seedClient(config_obj, client_id)

	_, err = launcher.GetFlows(self.Ctx, config_obj, client_id,
		result_sets.ResultSetOptions{}, 0, 100)
	assert.NoError(self.T(), err)

	closer := utils.SetFlowIdForTests(set_flow_id)
	defer closer()

	flow_id, err := launcher.ScheduleArtifactCollection(
		self.Ctx, config_obj, acl_managers.NullACLManager{},
		self.loadTestArtifact(config_obj),
		&flows_proto.ArtifactCollectorArgs{
			ClientId:  client_id,
			Artifacts: []string{"TestArtifact"},
		}, nil)
	assert.NoError(self.T(), err)

	err = cvelo_services.FlushBulkIndexer()
	assert.NoError(self.T(), err)

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		collection_context, err := launcher.Storage().LoadCollectionContext(
			self.Ctx, config_obj, client_id, flow_id)
		return err == nil && collection_context != nil &&
			collection_context.SessionId == flow_id
	})

	collection_context, err := launcher.Storage().LoadCollectionContext(
		self.Ctx, config_obj, client_id, flow_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), flow_id, collection_context.SessionId)
}

// The server is a pseudo client with no record in the index - collections
// against it must still be scheduled.
func (self *LauncherTestSuite) TestScheduleServerCollection() {
	config_obj := self.ConfigObj.VeloConf()

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	closer := utils.SetFlowIdForTests("F.server")
	defer closer()

	repository := self.loadTestArtifact(config_obj)

	flow_id, err := launcher.ScheduleArtifactCollection(
		self.Ctx, config_obj, acl_managers.NullACLManager{},
		repository, &flows_proto.ArtifactCollectorArgs{
			ClientId:  "server",
			Artifacts: []string{"TestArtifact"},
		}, nil)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), "F.server", flow_id)
}

// A client id that is well formed but has no record must still be refused,
// and the error must remain recognisable as a not found error.
func (self *LauncherTestSuite) TestScheduleUnknownClientRefused() {
	config_obj := self.ConfigObj.VeloConf()

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	closer := utils.SetFlowIdForTests("F.unknown")
	defer closer()

	repository := self.loadTestArtifact(config_obj)

	_, err = launcher.ScheduleArtifactCollection(
		self.Ctx, config_obj, acl_managers.NullACLManager{},
		repository, &flows_proto.ArtifactCollectorArgs{
			ClientId:  "C.deadbeef",
			Artifacts: []string{"TestArtifact"},
		}, nil)
	assert.Error(self.T(), err)
	assert.True(self.T(), utils.IsNotFound(err))
}

// A flow created after the client's flow list has been read must still be
// readable. The flow cache is keyed by client id and holds a point in time
// snapshot of that client's flows, so the new flow is absent from it.
func (self *LauncherTestSuite) TestGetFlowDetailsForFlowCreatedAfterCacheWarmed() {
	// The server pseudo client and a real agent are scheduled through
	// different branches, so both are covered.
	self.assertFlowReadableAfterCacheWarmed(
		"server", "F.serverwarm", "F.serverlate")
	self.assertFlowReadableAfterCacheWarmed(
		"C.cachewarm", "F.agentwarm", "F.agentlate")
}

// The datastore fallback must work even when nothing invalidates the cache.
// WriteFlow stores the record without rebuilding the index or purging the
// snapshot, so the read can only succeed by querying the datastore.
func (self *LauncherTestSuite) TestLoadCollectionContextFallsBackToDatastore() {
	config_obj := self.ConfigObj.VeloConf()
	client_id := "C.fallback"
	flow_id := "F.datastoreonly"

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	self.warmFlowCache(config_obj, client_id, "F.fallbackwarm")

	err = launcher.Storage().WriteFlow(self.Ctx, config_obj,
		&flows_proto.ArtifactCollectorContext{
			ClientId:      client_id,
			SessionId:     flow_id,
			State:         flows_proto.ArtifactCollectorContext_RUNNING,
			TotalRequests: 1,
		}, utils.SyncCompleter)
	assert.NoError(self.T(), err)

	err = cvelo_services.FlushBulkIndexer()
	assert.NoError(self.T(), err)

	collection_context, err := launcher.Storage().LoadCollectionContext(
		self.Ctx, config_obj, client_id, flow_id)
	assert.NoError(self.T(), err)
	assert.NotNil(self.T(), collection_context)
	assert.Equal(self.T(), flow_id, collection_context.SessionId)
}

// A flow that exists nowhere must still be reported as a not found error,
// because that is what maps to a 404 rather than a 503.
func (self *LauncherTestSuite) TestLoadCollectionContextUnknownFlowIsNotFound() {
	config_obj := self.ConfigObj.VeloConf()

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	_, err = launcher.Storage().LoadCollectionContext(
		self.Ctx, config_obj, "C.noflows", "F.doesnotexist")
	assert.Error(self.T(), err)
	assert.True(self.T(), utils.IsNotFound(err))
}

// A flow cached while it was still running must not keep being served in
// that state. Completion is written by whichever process observes it, and
// the snapshot is per process with no cross instance invalidation.
func (self *LauncherTestSuite) TestLoadCollectionContextRefreshesNonTerminalFlow() {
	config_obj := self.ConfigObj.VeloConf()
	client_id := "C.staleflow"
	flow_id := "F.stalerunning"

	self.seedClient(config_obj, client_id)

	// Independent snapshots over one datastore - a stand in for two replicas.
	warmed := self.newLauncherInstance()
	cold := self.newLauncherInstance()

	self.seedCollectionRecord(config_obj, runningFlow(client_id, flow_id))

	self.assertFlowState(warmed, client_id, flow_id,
		flows_proto.ArtifactCollectorContext_RUNNING)

	// The completion lands from elsewhere, invalidating nothing.
	self.seedCollectionRecord(config_obj, completedFlow(client_id, flow_id))

	self.assertFlowState(warmed, client_id, flow_id,
		flows_proto.ArtifactCollectorContext_FINISHED)
	self.assertFlowState(cold, client_id, flow_id,
		flows_proto.ArtifactCollectorContext_FINISHED)
}

// A terminal collection must still be served from the snapshot, which is
// what keeps GetFlowDetails cheap while the GUI polls it once a second.
// Two reads returning the identical pointer is the assertion - the
// datastore path builds a fresh context every call.
func (self *LauncherTestSuite) TestLoadCollectionContextServesTerminalFlowFromSnapshot() {
	config_obj := self.ConfigObj.VeloConf()
	client_id := "C.terminalflow"
	flow_id := "F.alreadyfinished"

	self.seedClient(config_obj, client_id)

	instance := self.newLauncherInstance()

	self.seedCollectionRecord(config_obj, completedFlow(client_id, flow_id))

	first, err := instance.Storage().LoadCollectionContext(
		self.Ctx, config_obj, client_id, flow_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(),
		flows_proto.ArtifactCollectorContext_FINISHED, first.State)

	second, err := instance.Storage().LoadCollectionContext(
		self.Ctx, config_obj, client_id, flow_id)
	assert.NoError(self.T(), err)
	assert.True(self.T(), first == second)
}

// A flow index written by an older build has no _Flow column, so its
// rows load with a nil context. Such an entry must fall back to the
// datastore rather than be dereferenced.
func (self *LauncherTestSuite) TestLoadCollectionContextIgnoresIndexRowWithoutFlow() {
	config_obj := self.ConfigObj.VeloConf()
	client_id := "C.legacyindex"
	flow_id := "F.legacyrow"

	self.seedClient(config_obj, client_id)
	self.seedCollectionRecord(config_obj, completedFlow(client_id, flow_id))
	self.writeFlowIndexWithoutFlow(config_obj, client_id, flow_id)

	instance := self.newLauncherInstance()

	collection_context, err := instance.Storage().LoadCollectionContext(
		self.Ctx, config_obj, client_id, flow_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(),
		flows_proto.ArtifactCollectorContext_FINISHED, collection_context.State)
}

// Resuming a terminal collection must leave it exactly as stored and
// queue nothing for the client.
func (self *LauncherTestSuite) TestResumeFlowIsRefused() {
	config_obj := self.ConfigObj.VeloConf()

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	for _, tc := range []struct {
		name string
		flow *flows_proto.ArtifactCollectorContext
	}{{
		name: "finished",
		flow: &flows_proto.ArtifactCollectorContext{
			ClientId:  "C.resumefinished",
			SessionId: "F.resumefinished",
			Request:   &flows_proto.ArtifactCollectorArgs{Timeout: 600},
			State:     flows_proto.ArtifactCollectorContext_FINISHED,
			QueryStats: []*crypto_proto.VeloStatus{{
				Status:     crypto_proto.VeloStatus_OK,
				ResultRows: 77,
			}},
		},
	}, {
		name: "error",
		flow: &flows_proto.ArtifactCollectorContext{
			ClientId:  "C.resumeerror",
			SessionId: "F.resumeerror",
			Request:   &flows_proto.ArtifactCollectorArgs{Timeout: 600},
			State:     flows_proto.ArtifactCollectorContext_ERROR,
			Status:    "query failed",
			QueryStats: []*crypto_proto.VeloStatus{{
				Status:       crypto_proto.VeloStatus_GENERIC_ERROR,
				ErrorMessage: "query failed",
				ResultRows:   3,
			}},
		},
	}} {
		self.Run(tc.name, func() {
			client_id := tc.flow.ClientId
			flow_id := tc.flow.SessionId

			self.seedClient(config_obj, client_id)

			err := launcher.Storage().WriteFlow(
				self.Ctx, config_obj, tc.flow, utils.SyncCompleter)
			assert.NoError(self.T(), err)

			err = cvelo_services.FlushBulkIndexer()
			assert.NoError(self.T(), err)

			before, err := launcher.GetFlowDetails(self.Ctx, config_obj,
				services.GetFlowOptions{}, client_id, flow_id)
			assert.NoError(self.T(), err)
			assert.Equal(self.T(), tc.flow.State, before.Context.State)

			transactions, err := launcher.ResumeFlow(
				self.Ctx, config_obj, client_id, flow_id)
			assert.Error(self.T(), err)
			assert.Equal(self.T(), codes.Unimplemented, status.Code(err))
			assert.Nil(self.T(), transactions)

			err = cvelo_services.FlushBulkIndexer()
			assert.NoError(self.T(), err)

			// A fresh instance has no snapshot, so this reads storage.
			after, err := self.newLauncherInstance().GetFlowDetails(
				self.Ctx, config_obj, services.GetFlowOptions{},
				client_id, flow_id)
			assert.NoError(self.T(), err)

			assert.Equal(self.T(), before.Context.State, after.Context.State)
			assert.Equal(self.T(), before.Context.Status, after.Context.Status)
			assert.Equal(self.T(),
				len(before.Context.QueryStats), len(after.Context.QueryStats))
			for i := range before.Context.QueryStats {
				assert.True(self.T(), proto.Equal(
					before.Context.QueryStats[i], after.Context.QueryStats[i]))
			}

			tasks, err := PeekClientTasks(self.Ctx, config_obj, client_id)
			assert.NoError(self.T(), err)
			assert.Equal(self.T(), 0, len(tasks))
		})
	}
}

// Writes the index row shape an older build produces. The index is
// fresh, so it is read as is rather than rebuilt.
func (self *LauncherTestSuite) writeFlowIndexWithoutFlow(
	config_obj *config_proto.Config, client_id, flow_id string) {

	rs_writer, err := result_sets.NewResultSetWriter(
		file_store.GetFileStore(config_obj),
		paths.NewClientPathManager(client_id).FlowIndex(),
		json.DefaultEncOpts(), utils.SyncCompleter, result_sets.TruncateMode)
	assert.NoError(self.T(), err)

	rs_writer.Write(ordereddict.NewDict().
		Set("FlowId", flow_id).
		Set("Artifacts", []string{"TestArtifact"}).
		Set("Created", 0).
		Set("Creator", "admin"))
	rs_writer.Close()
}

func (self *LauncherTestSuite) newLauncherInstance() services.Launcher {
	instance, err := cvelo_launcher.NewLauncherService(
		self.Ctx, self.Sm.Wg, self.ConfigObj.VeloConf(),
		&self.ConfigObj.Cloud)
	assert.NoError(self.T(), err)

	return instance
}

func (self *LauncherTestSuite) assertFlowState(
	instance services.Launcher, client_id, flow_id string,
	expected flows_proto.ArtifactCollectorContext_State) {

	collection_context, err := instance.Storage().LoadCollectionContext(
		self.Ctx, self.ConfigObj.VeloConf(), client_id, flow_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), expected, collection_context.State)
}

func runningFlow(
	client_id, flow_id string) *flows_proto.ArtifactCollectorContext {
	return &flows_proto.ArtifactCollectorContext{
		ClientId:  client_id,
		SessionId: flow_id,
		Request: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact"},
			Creator:   "admin",
		},
		QueryStats: []*crypto_proto.VeloStatus{{
			Status: crypto_proto.VeloStatus_PROGRESS,
		}},
	}
}

func completedFlow(
	client_id, flow_id string) *flows_proto.ArtifactCollectorContext {
	return &flows_proto.ArtifactCollectorContext{
		ClientId:  client_id,
		SessionId: flow_id,
		QueryStats: []*crypto_proto.VeloStatus{{
			Status:     crypto_proto.VeloStatus_OK,
			ResultRows: 77,
		}},
	}
}

func (self *LauncherTestSuite) assertFlowReadableAfterCacheWarmed(
	client_id, first_flow_id, second_flow_id string) {

	config_obj := self.ConfigObj.VeloConf()

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	// Populates the snapshot, which then holds the first flow but not the second.
	self.warmFlowCache(config_obj, client_id, first_flow_id)

	closer := utils.SetFlowIdForTests(second_flow_id)
	defer closer()

	flow_id, err := launcher.ScheduleArtifactCollection(
		self.Ctx, config_obj, acl_managers.NullACLManager{},
		self.loadTestArtifact(config_obj),
		&flows_proto.ArtifactCollectorArgs{
			ClientId:  client_id,
			Artifacts: []string{"TestArtifact"},
		}, nil)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), second_flow_id, flow_id)

	err = cvelo_services.FlushBulkIndexer()
	assert.NoError(self.T(), err)

	details, err := launcher.GetFlowDetails(self.Ctx, config_obj,
		services.GetFlowOptions{}, client_id, flow_id)
	assert.NoError(self.T(), err)
	assert.NotNil(self.T(), details.Context)
	assert.Equal(self.T(), flow_id, details.Context.SessionId)
}

// Schedule one flow for the client and read the flow list, which is what
// populates the client keyed cache entry.
func (self *LauncherTestSuite) warmFlowCache(
	config_obj *config_proto.Config, client_id, flow_id string) {

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	if client_id != "server" {
		self.seedClient(config_obj, client_id)
	}

	closer := utils.SetFlowIdForTests(flow_id)
	defer closer()

	_, err = launcher.ScheduleArtifactCollection(
		self.Ctx, config_obj, acl_managers.NullACLManager{},
		self.loadTestArtifact(config_obj),
		&flows_proto.ArtifactCollectorArgs{
			ClientId:  client_id,
			Artifacts: []string{"TestArtifact"},
		}, nil)
	assert.NoError(self.T(), err)

	err = cvelo_services.FlushBulkIndexer()
	assert.NoError(self.T(), err)

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		flows, _ := launcher.GetFlows(self.Ctx, config_obj,
			client_id, result_sets.ResultSetOptions{}, 0, 100)
		return len(flows.Items) > 0
	})

	flows, err := launcher.GetFlows(self.Ctx, config_obj,
		client_id, result_sets.ResultSetOptions{}, 0, 100)
	assert.NoError(self.T(), err)
	assert.True(self.T(), len(flows.Items) > 0)
}

// LoadCollectionContext serves flows out of a 5 minute cache, and
// DeleteFlow's own GetFlowDetails call populates that cache before the
// documents are removed. Delete-then-read is the GUI's own flow.
func (self *LauncherTestSuite) TestDeleteFlowPurgesFlowCache() {
	config_obj := self.ConfigObj.VeloConf()
	client_id := "C.1234delete"
	set_flow_id := "F.todelete"

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	closer := utils.SetFlowIdForTests(set_flow_id)
	defer closer()

	repository := self.loadTestArtifact(config_obj)
	self.seedClient(config_obj, client_id)

	flow_id, err := launcher.ScheduleArtifactCollection(
		self.Ctx, config_obj, acl_managers.NullACLManager{},
		repository, &flows_proto.ArtifactCollectorArgs{
			ClientId:  client_id,
			Artifacts: []string{"TestArtifact"},
		}, nil)
	assert.NoError(self.T(), err)

	err = cvelo_services.FlushBulkIndexer()
	assert.NoError(self.T(), err)

	// Read the flow before deleting it, which is what puts it in the
	// cache.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		details, err := launcher.GetFlowDetails(self.Ctx, config_obj,
			services.GetFlowOptions{}, client_id, flow_id)
		return err == nil && details.Context != nil &&
			details.Context.SessionId == flow_id
	})

	responses, err := launcher.Storage().DeleteFlow(self.Ctx, config_obj,
		client_id, flow_id, "admin",
		services.DeleteFlowOptions{ReallyDoIt: true})
	assert.NoError(self.T(), err)

	// A per-index failure is reported in the response rather than
	// returned, so a silently failed delete would otherwise look clean.
	for _, response := range responses {
		assert.Equal(self.T(), "", response.Error)
	}

	// DeleteByQuery refreshes the index, so the delete is already
	// visible. A retry loop here would hide a cache that is never
	// purged, because every miss repopulates it.
	_, err = launcher.GetFlowDetails(self.Ctx, config_obj,
		services.GetFlowOptions{}, client_id, flow_id)
	assert.Error(self.T(), err)
	assert.True(self.T(), utils.IsNotFound(err))
}

// buildIndex reads Artifacts off flow.Request, falling back to
// ArtifactsWithResults when the flow has no Request. Flows written by the
// ingestion path do not always carry one.
func (self *LauncherTestSuite) TestBuildIndexHandlesFlowWithoutRequest() {
	config_obj := self.ConfigObj.VeloConf()
	client_id := "C.1234norequest"
	flow_id := "F.norequest"

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	self.seedClient(config_obj, client_id)
	self.seedCollectionRecord(config_obj, &flows_proto.ArtifactCollectorContext{
		ClientId:             client_id,
		SessionId:            flow_id,
		ArtifactsWithResults: []string{"Custom.WithResults"},
		State:                flows_proto.ArtifactCollectorContext_FINISHED,
	})

	flows, total, err := launcher.Storage().ListFlows(self.Ctx, config_obj,
		client_id, result_sets.ResultSetOptions{}, 0, 100)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), int64(1), total)
	assert.Equal(self.T(), 1, len(flows))
	assert.Equal(self.T(), flow_id, flows[0].FlowId)

	// The Request is nil, so the artifact names have to come from
	// ArtifactsWithResults rather than being dropped.
	assert.Equal(self.T(), []string{"Custom.WithResults"}, flows[0].Artifacts)
	assert.Equal(self.T(), "", flows[0].Creator)
}

// buildIndex pages through the collections with QueryChan sorted on
// timestamp, and QueryChan abandons the remaining pages when the last row
// of a page has no value for the sort field. What stops that from
// truncating the flow index is the transient index being a data stream,
// which refuses a document without a timestamp outright. Pin that, because
// turning transient back into a plain index would reintroduce the hazard
// silently.
func (self *LauncherTestSuite) TestTransientIndexRejectsRecordWithoutTimestamp() {
	config_obj := self.ConfigObj.VeloConf()
	client_id := "C.1234notimestamp"

	launcher, err := services.GetLauncher(config_obj)
	assert.NoError(self.T(), err)

	self.seedClient(config_obj, client_id)

	for _, flow_id := range []string{"F.good1", "F.good2"} {
		self.seedCollectionRecord(config_obj,
			&flows_proto.ArtifactCollectorContext{
				ClientId:  client_id,
				SessionId: flow_id,
				Request: &flows_proto.ArtifactCollectorArgs{
					Artifacts: []string{"TestArtifact"},
					Creator:   "admin",
				},
				State: flows_proto.ArtifactCollectorContext_FINISHED,
			})
	}

	err = self.writeCollectionRecordWithoutTimestamp(config_obj,
		&flows_proto.ArtifactCollectorContext{
			ClientId:  client_id,
			SessionId: "F.notimestamp",
			Request: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"TestArtifact"},
				Creator:   "admin",
			},
			State: flows_proto.ArtifactCollectorContext_FINISHED,
		})
	assert.Error(self.T(), err)
	assert.True(self.T(), strings.Contains(err.Error(), "timestamp"))

	// The well formed flows are unaffected.
	flows, total, err := launcher.Storage().ListFlows(self.Ctx, config_obj,
		client_id, result_sets.ResultSetOptions{}, 0, 100)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), int64(2), total)
	assert.Equal(self.T(), 2, len(flows))

	seen := make(map[string]bool)
	for _, flow := range flows {
		seen[flow.FlowId] = true
	}
	assert.True(self.T(), seen["F.good1"])
	assert.True(self.T(), seen["F.good2"])
}

// Mirrors FlowStorageManager.WriteFlow.
func (self *LauncherTestSuite) seedCollectionRecord(
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext) {

	doc_id := cvelo_api.GetDocumentIdForCollection(
		flow.ClientId, flow.SessionId, "")

	record := cvelo_api.ArtifactCollectorRecordFromProto(flow, doc_id)
	record.Type = "main"
	record.Timestamp = utils.GetTime().Now().UnixNano()

	err := cvelo_services.SetElasticIndex(self.Ctx, config_obj.OrgId,
		"transient", cvelo_services.DocIdRandom, record)
	assert.NoError(self.T(), err)
}

// api.ArtifactCollectorRecord has no omitempty on Timestamp, so it cannot
// express a document that lacks the field at all.
type collectionRecordWithoutTimestamp struct {
	ClientId  string `json:"client_id"`
	SessionId string `json:"session_id"`
	Raw       string `json:"context,omitempty"`
	Type      string `json:"type"`
	Doc_Type  string `json:"doc_type"`
	ID        string `json:"id"`
}

func (self *LauncherTestSuite) writeCollectionRecordWithoutTimestamp(
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext) error {

	record := &collectionRecordWithoutTimestamp{
		ClientId:  flow.ClientId,
		SessionId: flow.SessionId,
		Raw:       json.MustMarshalString(flow),
		Type:      "main",
		Doc_Type:  "collection",
		ID: cvelo_api.GetDocumentIdForCollection(
			flow.ClientId, flow.SessionId, ""),
	}

	return cvelo_services.SetElasticIndex(self.Ctx, config_obj.OrgId,
		"transient", cvelo_services.DocIdRandom, record)
}

func (self *LauncherTestSuite) loadTestArtifact(
	config_obj *config_proto.Config) services.Repository {

	repository_manager, err := services.GetRepositoryManager(config_obj)
	assert.NoError(self.T(), err)

	repository := repository_manager.NewRepository()
	_, err = repository.LoadYaml(`
name: TestArtifact
sources:
- query: SELECT * FROM info()
`, services.ArtifactOptions{
		ValidateArtifact:  true,
		ArtifactIsBuiltIn: true})
	assert.NoError(self.T(), err)

	return repository
}

func (self *LauncherTestSuite) seedClient(
	config_obj *config_proto.Config, client_id string) {

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	assert.NoError(self.T(), err)

	err = client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{ClientId: client_id}})
	assert.NoError(self.T(), err)
}

func TestLauncher(t *testing.T) {
	suite.Run(t, &LauncherTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{},
	})
}

func PeekClientTasks(ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) (
	[]*client_info.ClientTask, error) {

	query := json.Format(getClientTasksQuery, client_id)
	hits, err := cvelo_services.QueryElastic(ctx, config_obj.OrgId,
		"persisted", query)
	if err != nil {
		return nil, err
	}

	results := []*client_info.ClientTask{}
	for _, hit := range hits {
		item := &client_info.ClientTask{}
		err = json.Unmarshal(hit.JSON, item)
		if err != nil {
			continue
		}
		results = append(results, item)
	}
	return results, nil
}
