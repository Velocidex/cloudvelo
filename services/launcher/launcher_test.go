package launcher_test

import (
	"context"
	"testing"
	"time"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/encoding/protojson"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/client_info"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

const (
	getAllItemsQuery = `
{"query": {"match_all" : {}}}
`

	// Query to retrieve all the task queued for a client.
	getClientTasksQuery = `{
  "sort": [
  {
    "timestamp": {"order": "asc"}
  }],
  "query": {
    "bool": {
      "must": [
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

	launcher.SetFlowIdForTests(set_flow_id)

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
		client_id, flow_id)
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
		client_id, flow_id)
	assert.NoError(self.T(), err)

	// Make sure the flow is in the error state
	assert.Equal(self.T(),
		flows_proto.ArtifactCollectorContext_ERROR,
		details.Context.State)

	// Create a second collection
	launcher.SetFlowIdForTests(set_flow_id + "second")
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

func TestLauncher(t *testing.T) {
	suite.Run(t, &LauncherTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{
			Indexes: []string{collections", "transient"},
		},
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
