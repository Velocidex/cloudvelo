package foreman

import (
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/client_monitoring"
	"www.velocidex.com/golang/cloudvelo/services/hunt_dispatcher"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	artifact_definitions = []string{`
name: ArtifactForAll
type: CLIENT_EVENT
sources:
- query: SELECT ArtifactForAll FROM scope()
`, `
name: ArtifactForLabel1
type: CLIENT_EVENT
sources:
- query: SELECT ArtifactForLabel1 FROM scope()
`, `
name: ArtifactForLabel2
type: CLIENT_EVENT
sources:
- query: SELECT ArtifactForLabel2 FROM scope()
`}
)

type ForemanTestSuite struct {
	*testsuite.CloudTestSuite
}

func (self *ForemanTestSuite) getClientRecord(client_id string) *api.ClientInfo {
	serialized, err := cvelo_services.GetElasticRecord(
		self.Ctx, self.ConfigObj.OrgId, "clients", client_id)
	assert.NoError(self.T(), err)

	result := &api.ClientInfo{}
	err = json.Unmarshal(serialized, &result)
	assert.NoError(self.T(), err)

	return result
}

func (self *ForemanTestSuite) TestClientMonitoring() {
	Clock = &utils.MockClock{
		MockNow: time.Unix(1661391000, 0),
	}
	client_monitoring.Clock = Clock

	config_obj := self.ConfigObj.VeloConf()

	// First load some event artifacts
	repository_manager, err := services.GetRepositoryManager(config_obj)
	assert.NoError(self.T(), err)

	client_monitoring_service, err := services.ClientEventManager(config_obj)
	assert.NoError(self.T(), err)

	for _, definition := range artifact_definitions {
		_, err = repository_manager.SetArtifactFile(config_obj, "admin",
			definition, "")
		assert.NoError(self.T(), err)
	}

	// Create some monitoring rules
	err = client_monitoring_service.SetClientMonitoringState(
		self.Ctx, config_obj,
		"user1", &flows_proto.ClientEventTable{
			Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"ArtifactForAll"},
			},
			LabelEvents: []*flows_proto.LabelEvents{
				{Label: "Label1", Artifacts: &flows_proto.ArtifactCollectorArgs{
					Artifacts: []string{"ArtifactForLabel1"},
				}},

				// ProcessCreation for Label2
				{Label: "Label2", Artifacts: &flows_proto.ArtifactCollectorArgs{
					Artifacts: []string{"ArtifactForLabel2"},
				}},
			},
		})
	assert.NoError(self.T(), err)

	// Make some clients
	clients := []api.ClientInfo{
		// This client is currently connected
		{
			ClientId:      "C.ConnectedClient",
			Ping:          uint64(Clock.Now().UnixNano()),
			AssignedHunts: []string{},
			Labels:        []string{},
			LowerLabels:   []string{},
		},

		{
			ClientId:      "C.WithLabel1",
			Ping:          uint64(Clock.Now().UnixNano()),
			AssignedHunts: []string{},
			Labels:        []string{"Label1"},
			LowerLabels:   []string{"label1"},
		},

		{
			ClientId:      "C.WithLabel2",
			Ping:          uint64(Clock.Now().UnixNano()),
			Labels:        []string{"Label2"},
			LowerLabels:   []string{"label2"},
			AssignedHunts: []string{},
		},

		{
			ClientId:      "C.WithLabel1And2",
			Ping:          uint64(Clock.Now().UnixNano()),
			Labels:        []string{"Label1", "Label2"},
			LowerLabels:   []string{"label1", "label2"},
			AssignedHunts: []string{},
		},

		// This client has not been seen in a while
		{
			ClientId:      "C.OfflineClient",
			Ping:          uint64(Clock.Now().Add(-72 * time.Hour).UnixNano()),
			AssignedHunts: []string{},
			Labels:        []string{},
			LowerLabels:   []string{},
		},
	}

	// Add these clients directly into the index.
	for _, c := range clients {
		err := cvelo_services.SetElasticIndex(
			self.Ctx, config_obj.OrgId, "clients", c.ClientId, c)
		assert.NoError(self.T(), err)
	}

	// Now plan to update the clients
	plan := NewPlan()
	err = Foreman{}.CalculateEventTable(self.Ctx, config_obj, plan)
	assert.NoError(self.T(), err)

	// There should be 4 distinct event tables: all, all+label1,
	// all+label2 and all+label1+label2.
	assert.Equal(self.T(), 4, len(plan.MonitoringTablesToClients))
	assert.Equal(self.T(), []string{"C.WithLabel1And2"},
		plan.MonitoringTablesToClients["Label1|Label2"])
	assert.Equal(self.T(), []string{"C.WithLabel1"},
		plan.MonitoringTablesToClients["Label1"])
	assert.Equal(self.T(), []string{"C.WithLabel2"},
		plan.MonitoringTablesToClients["Label2"])

	// Blank means the All label group
	assert.Equal(self.T(), []string{"C.ConnectedClient"},
		plan.MonitoringTablesToClients[""])

	// Check the composed monitoring tables
	assert.Equal(self.T(), 4, len(plan.MonitoringTables))
	golden := ordereddict.NewDict().
		Set("All", plan.MonitoringTables[""]).
		Set("Label1", plan.MonitoringTables["Label1"]).
		Set("Label2", plan.MonitoringTables["Label2"]).
		Set("Label1|Label2", plan.MonitoringTables["Label1|Label2"])

	goldie.Assert(self.T(), "TestClientMonitoring", json.MustMarshalIndent(golden))

	// Now execute this update
	err = plan.ExecuteClientMonitoringUpdate(self.Ctx, config_obj)
	assert.NoError(self.T(), err)

	client := self.getClientRecord("C.WithLabel2")
	assert.Equal(self.T(), client.LastEventTableVersion,
		uint64(Clock.Now().UnixNano()))

	// Plan a second update - no changes are needed now.
	new_plan := NewPlan()
	err = Foreman{}.CalculateEventTable(self.Ctx, config_obj, new_plan)
	assert.NoError(self.T(), err)

	// No updates required.
	assert.Equal(self.T(), 0, len(new_plan.MonitoringTablesToClients))

	// Label a client - this should force an update.
	labeler := services.GetLabeler(config_obj)
	err = labeler.SetClientLabel(self.Ctx, config_obj,
		"C.ConnectedClient", "Label1")
	assert.NoError(self.T(), err)

	new_plan = NewPlan()
	err = Foreman{}.CalculateEventTable(self.Ctx, config_obj, new_plan)
	assert.NoError(self.T(), err)

	// Only one client will be updated
	assert.Equal(self.T(), 1, len(new_plan.MonitoringTablesToClients))
	assert.Equal(self.T(), []string{"C.ConnectedClient"},
		new_plan.MonitoringTablesToClients["Label1"])
}

func (self *ForemanTestSuite) setupAllHunts() {
	config_obj := self.ConfigObj.VeloConf()

	start_request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: []string{"Generic.Client.Info"},
	}

	hunts := []*api_proto.Hunt{
		// This hunt runs everywhere
		{
			HuntId:       "H.AllClients",
			StartRequest: start_request,
			CreateTime:   uint64(Clock.Now().UnixNano() / 1000),
			State:        api_proto.Hunt_RUNNING,
			// Expire in 24 hours. Expires is set in uS
			Expires: uint64(Clock.Now().Add(24*time.Hour).UnixNano() / 1000),
		},

		// This hunt runs only on Label Foo
		{
			HuntId:       "H.OnlyLabelFoo",
			StartRequest: start_request,
			CreateTime:   uint64(Clock.Now().UnixNano() / 1000),
			State:        api_proto.Hunt_RUNNING,
			Expires:      uint64(Clock.Now().Add(24*time.Hour).UnixNano() / 1000),
			Condition: &api_proto.HuntCondition{
				UnionField: &api_proto.HuntCondition_Labels{
					Labels: &api_proto.HuntLabelCondition{
						Label: []string{"Foo"},
					},
				},
			},
		},

		// This hunt has an empty list of required labels
		{
			HuntId:       "H.EmptyLabels",
			StartRequest: start_request,
			CreateTime:   uint64(Clock.Now().UnixNano() / 1000),
			State:        api_proto.Hunt_RUNNING,
			Expires:      uint64(Clock.Now().Add(24*time.Hour).UnixNano() / 1000),
			Condition: &api_proto.HuntCondition{
				UnionField: &api_proto.HuntCondition_Labels{
					Labels: &api_proto.HuntLabelCondition{},
				},
			},
		},

		// This hunt excludes Label Foo
		{
			HuntId:       "H.ExceptLabelFoo",
			StartRequest: start_request,
			CreateTime:   uint64(Clock.Now().UnixNano() / 1000),
			State:        api_proto.Hunt_RUNNING,
			Expires:      uint64(Clock.Now().Add(24*time.Hour).UnixNano() / 1000),
			Condition: &api_proto.HuntCondition{
				ExcludedLabels: &api_proto.HuntLabelCondition{
					Label: []string{"Foo"},
				},
			},
		},
	}

	// Create a bunch of hunts to use
	hunt_service, err := services.GetHuntDispatcher(config_obj)
	assert.NoError(self.T(), err)

	// Set the hunt directly in the database
	for _, h := range hunts {
		err := hunt_service.(*hunt_dispatcher.HuntDispatcher).SetHunt(h)
		assert.NoError(self.T(), err)
	}
}

func (self *ForemanTestSuite) TestHuntsAllClients() {
	Clock = &utils.MockClock{
		MockNow: time.Unix(1661391000, 0),
	}

	config_obj := self.ConfigObj.VeloConf()

	self.setupAllHunts()

	clients := []api.ClientInfo{
		// This client is currently connected
		{
			ClientId:      "C.ConnectedClient",
			Ping:          uint64(Clock.Now().UnixNano()),
			AssignedHunts: []string{},
			Labels:        []string{},
			LowerLabels:   []string{},
		},

		// This client already ran on hunt H.1234 it will not be
		// chosen again.
		{
			ClientId:      "C.AlreadyRanAllClients",
			Ping:          uint64(Clock.Now().UnixNano()),
			AssignedHunts: []string{"H.AllClients"},
			Labels:        []string{},
			LowerLabels:   []string{},
		},

		{
			ClientId:      "C.WithLabelFoo",
			Ping:          uint64(Clock.Now().UnixNano()),
			Labels:        []string{"Foo"},
			LowerLabels:   []string{"foo"},
			AssignedHunts: []string{},
		},

		// This client has not been seen in a while
		{
			ClientId:      "C.OfflineClient",
			Ping:          uint64(Clock.Now().Add(-72 * time.Hour).UnixNano()),
			AssignedHunts: []string{},
			Labels:        []string{},
			LowerLabels:   []string{},
		},
	}

	// Add these clients directly into the index.
	for _, c := range clients {
		err := cvelo_services.SetElasticIndex(
			self.Ctx, config_obj.OrgId, "clients", c.ClientId, c)
		assert.NoError(self.T(), err)
	}

	// See the hunt update plan.
	plan := NewPlan()
	err := Foreman{}.UpdateHuntMembership(self.Ctx, config_obj, plan)
	assert.NoError(self.T(), err)

	// The connected client should run the all client hunt.
	assert.True(self.T(),
		huntPresent("H.AllClients", plan.ClientIdToHunts["C.ConnectedClient"]))

	// The connected client does not have the label so should get the
	// label hunt.
	assert.True(self.T(),
		huntPresent("H.ExceptLabelFoo", plan.ClientIdToHunts["C.ConnectedClient"]))

	// The OfflineClient should not receive any hunts since it is
	// offline.
	assert.True(self.T(),
		!huntPresent("H.AllClients", plan.ClientIdToHunts["C.OfflineClient"]))

	// The C.WithLabelFoo client should receive the hunt targeting the
	// label
	assert.True(self.T(),
		huntPresent("H.OnlyLabelFoo", plan.ClientIdToHunts["C.WithLabelFoo"]))

	// But should **NOT** receive the hunt that excludes the label.
	assert.True(self.T(),
		!huntPresent("H.ExceptLabelFoo", plan.ClientIdToHunts["C.WithLabelFoo"]))

	// The client that already ran the H.AllClients hunt should not
	// receive it again.
	assert.True(self.T(),
		!huntPresent("H.AllClients", plan.ClientIdToHunts["C.AlreadyRanAllClients"]))

	// The C.AlreadyRanAllClients client has not label Foo so will not
	// receive the hunt targeting that label.
	assert.True(self.T(),
		!huntPresent("H.OnlyLabelFoo", plan.ClientIdToHunts["C.AlreadyRanAllClients"]))

	// All online clients should have the empty label hunt
	assert.True(self.T(),
		huntPresent("H.EmptyLabels", plan.ClientIdToHunts["C.ConnectedClient"]))
	assert.True(self.T(),
		huntPresent("H.EmptyLabels", plan.ClientIdToHunts["C.WithLabelFoo"]))
	assert.True(self.T(),
		huntPresent("H.EmptyLabels", plan.ClientIdToHunts["C.AlreadyRanAllClients"]))
	// Offline client doesn't receive it
	assert.True(self.T(),
		!huntPresent("H.EmptyLabels", plan.ClientIdToHunts["C.OfflineClient"]))

	// Close the plan to finish updating the clients
	err = plan.Close(self.Ctx, config_obj)
	assert.NoError(self.T(), err)

	// Check the client records
	client := self.getClientRecord("C.ConnectedClient")

	// The client should be assigned **BOTH** H.AllClients and H.ExceptLabelFoo
	assert.Contains(self.T(), client.AssignedHunts, "H.AllClients")
	assert.Contains(self.T(), client.AssignedHunts, "H.ExceptLabelFoo")

	// Make sure the timestamp is updated
	assert.Equal(self.T(), client.LastHuntTimestamp, uint64(Clock.Now().UnixNano()))

	// This client did not get scheduled for anything
	client = self.getClientRecord("C.OfflineClient")
	assert.Equal(self.T(), client.AssignedHunts, []string{})

	// Calculating the plan again should produce nothing to do.
	new_plan := NewPlan()
	err = Foreman{}.UpdateHuntMembership(self.Ctx, config_obj, new_plan)
	assert.NoError(self.T(), err)

	// The plan should be empty as there is nothing to do!
	assert.True(self.T(), len(new_plan.ClientIdToHunts) == 0)

	// Now label C.ConnectedClient with the Foo label.
	labeler := services.GetLabeler(config_obj)
	err = labeler.SetClientLabel(self.Ctx, config_obj, "C.ConnectedClient", "Foo")
	assert.NoError(self.T(), err)

	// Get the plan again - this hunt should now be scheduled on the
	// client.
	new_plan = NewPlan()
	err = Foreman{}.UpdateHuntMembership(self.Ctx, config_obj, new_plan)
	assert.NoError(self.T(), err)

	// Only one client will be scheduled now.
	assert.True(self.T(), len(new_plan.ClientIdToHunts) == 1)

	// Hunt targeting Foo label will be scheduled on the client now.
	assert.True(self.T(),
		huntPresent("H.OnlyLabelFoo", new_plan.ClientIdToHunts["C.ConnectedClient"]))

	self.testHuntsExpireInFuture()
}

func (self *ForemanTestSuite) testHuntsExpireInFuture() {
	// Test that the hunt expires - move time forward by 25 hours.
	Clock = &utils.MockClock{
		MockNow: time.Unix(1661391000+25*60*60, 0),
	}

	config_obj := self.ConfigObj.VeloConf()

	// Add a new client
	c := &api.ClientInfo{
		ClientId:      "C.NewClient",
		Ping:          uint64(Clock.Now().UnixNano()),
		AssignedHunts: []string{},
		Labels:        []string{},
		LowerLabels:   []string{},
	}
	err := cvelo_services.SetElasticIndex(
		self.Ctx, config_obj.OrgId, "clients", c.ClientId, c)
	assert.NoError(self.T(), err)

	plan := NewPlan()
	err = Foreman{}.UpdateHuntMembership(self.Ctx, config_obj, plan)
	assert.NoError(self.T(), err)

	// No hunts scheduled
	assert.True(self.T(), len(plan.ClientIdToHunts) == 0)

	hunt_service, err := services.GetHuntDispatcher(config_obj)
	assert.NoError(self.T(), err)

	// All the hunts are stopped now because they all expired.
	hunt_list, err := hunt_service.ListHunts(
		self.Ctx, config_obj, &api_proto.ListHuntsRequest{
			Count: 1000,
		})
	assert.NoError(self.T(), err)

	for _, hunt := range hunt_list.Items {
		assert.Equal(self.T(), api_proto.Hunt_STOPPED, hunt.State)
	}
}

func (self *ForemanTestSuite) setupOSHunts() {
	config_obj := self.ConfigObj.VeloConf()

	start_request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: []string{"Generic.Client.Info"},
	}

	hunts := []*api_proto.Hunt{
		{
			HuntId:       "H.AllOSes",
			StartRequest: start_request,
			CreateTime:   uint64(Clock.Now().UnixNano() / 1000),
			State:        api_proto.Hunt_RUNNING,
			// Expire in 24 hours. Expires is set in uS
			Expires: uint64(Clock.Now().Add(24*time.Hour).UnixNano() / 1000),
		},

		// This hunt runs only on Windows machines
		{
			HuntId:       "H.WindowsOnly",
			StartRequest: start_request,
			CreateTime:   uint64(Clock.Now().UnixNano() / 1000),
			State:        api_proto.Hunt_RUNNING,
			Expires:      uint64(Clock.Now().Add(24*time.Hour).UnixNano() / 1000),
			Condition: &api_proto.HuntCondition{
				UnionField: &api_proto.HuntCondition_Os{
					Os: &api_proto.HuntOsCondition{
						Os: api_proto.HuntOsCondition_WINDOWS,
					},
				},
			},
		},

		// This hunt runs only on Linux machines
		{
			HuntId:       "H.LinuxOnly",
			StartRequest: start_request,
			CreateTime:   uint64(Clock.Now().UnixNano() / 1000),
			State:        api_proto.Hunt_RUNNING,
			Expires:      uint64(Clock.Now().Add(24*time.Hour).UnixNano() / 1000),
			Condition: &api_proto.HuntCondition{
				UnionField: &api_proto.HuntCondition_Os{
					Os: &api_proto.HuntOsCondition{
						Os: api_proto.HuntOsCondition_LINUX,
					},
				},
			},
		},
	}

	// Create a bunch of hunts to use
	hunt_service, err := services.GetHuntDispatcher(config_obj)
	assert.NoError(self.T(), err)

	// Set the hunt directly in the database
	for _, h := range hunts {
		err := hunt_service.(*hunt_dispatcher.HuntDispatcher).SetHunt(h)
		assert.NoError(self.T(), err)
	}
}

func (self *ForemanTestSuite) TestHuntsByOS() {
	Clock = &utils.MockClock{
		MockNow: time.Unix(1661391000, 0),
	}

	config_obj := self.ConfigObj.VeloConf()

	self.setupOSHunts()

	clients := []api.ClientInfo{
		{
			ClientId:      "C.Windows1",
			Ping:          uint64(Clock.Now().UnixNano()),
			AssignedHunts: []string{},
			Labels:        []string{},
			LowerLabels:   []string{},
			System:        "windows",
		},
		{
			ClientId:      "C.Windows2",
			Ping:          uint64(Clock.Now().UnixNano()),
			AssignedHunts: []string{},
			Labels:        []string{},
			LowerLabels:   []string{},
			System:        "windows",
		},
		{
			ClientId:      "C.Linux1",
			Ping:          uint64(Clock.Now().UnixNano()),
			AssignedHunts: []string{},
			Labels:        []string{},
			LowerLabels:   []string{},
			System:        "linux",
		},
		{
			ClientId:      "C.Linux2",
			Ping:          uint64(Clock.Now().UnixNano()),
			AssignedHunts: []string{},
			Labels:        []string{},
			LowerLabels:   []string{},
			System:        "linux",
		},
	}

	// Add these clients directly into the index.
	for _, c := range clients {
		err := cvelo_services.SetElasticIndex(
			self.Ctx, config_obj.OrgId, "clients", c.ClientId, c)
		assert.NoError(self.T(), err)
	}

	// See the hunt update plan.
	plan := NewPlan()
	err := Foreman{}.UpdateHuntMembership(self.Ctx, config_obj, plan)
	assert.NoError(self.T(), err)

	windowsExpected := []string{"H.AllOSes", "H.WindowsOnly"}
	linuxExpected := []string{"H.AllOSes", "H.LinuxOnly"}
	self.checkPlannedHunts(plan, "C.Windows1", windowsExpected)
	self.checkPlannedHunts(plan, "C.Windows2", windowsExpected)
	self.checkPlannedHunts(plan, "C.Linux1", linuxExpected)
	self.checkPlannedHunts(plan, "C.Linux2", linuxExpected)

	// Close the plan to finish updating the clients
	err = plan.Close(self.Ctx, config_obj)
	assert.NoError(self.T(), err)

	self.checkAssignedHunts("C.Windows1", windowsExpected)
	self.checkAssignedHunts("C.Windows2", windowsExpected)
	self.checkAssignedHunts("C.Linux1", linuxExpected)
	self.checkAssignedHunts("C.Linux2", linuxExpected)

	// Calculating the plan again should produce nothing to do.
	new_plan := NewPlan()
	err = Foreman{}.UpdateHuntMembership(self.Ctx, config_obj, new_plan)
	assert.NoError(self.T(), err)

	// The plan should be empty as there is nothing to do!
	assert.True(self.T(), len(new_plan.ClientIdToHunts) == 0)
}

func (self *ForemanTestSuite) checkPlannedHunts(plan *Plan, clientId string, expectedHunts []string) {
	plannedHunts := plan.ClientIdToHunts[clientId]
	assert.True(self.T(), len(plannedHunts) == len(expectedHunts))

	for _, hunt := range expectedHunts {
		assert.True(self.T(), huntPresent(hunt, plannedHunts))
	}
}

func (self *ForemanTestSuite) checkAssignedHunts(clientId string, expectedHunts []string) {
	client := self.getClientRecord(clientId)
	assert.True(self.T(), len(client.AssignedHunts) == len(expectedHunts))

	for _, hunt := range expectedHunts {
		assert.Contains(self.T(), client.AssignedHunts, hunt)
	}
}

/* TODO: Foreman is currently broken due to the multiple client records split.

func TestForeman(t *testing.T) {
	suite.Run(t, &ForemanTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{
			Indexes: []string{"clients", "hunts", "repository", "tasks"},
		},
	})
}
*/

func huntPresent(hunt_id string, hunts []*api_proto.Hunt) bool {
	for _, h := range hunts {
		if h.HuntId == hunt_id {
			return true
		}
	}
	return false
}
