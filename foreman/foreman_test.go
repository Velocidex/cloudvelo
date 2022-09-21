package foreman

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/client_info"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type ForemanTestSuite struct {
	*testsuite.CloudTestSuite
}

func (self *ForemanTestSuite) TestHuntsAllClients() {
	Clock = &utils.MockClock{
		MockNow: time.Unix(1661391000, 0),
	}

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
			Expires:      uint64(Clock.Now().Add(24 * time.Hour).UnixNano()),
		},

		// This hunt runs only on Label Foo
		{
			HuntId:       "H.OnlyLabelFoo",
			StartRequest: start_request,
			CreateTime:   uint64(Clock.Now().UnixNano() / 1000),
			State:        api_proto.Hunt_RUNNING,
			Expires:      uint64(Clock.Now().Add(24 * time.Hour).UnixNano()),
			Condition: &api_proto.HuntCondition{
				UnionField: &api_proto.HuntCondition_Labels{
					Labels: &api_proto.HuntLabelCondition{
						Label: []string{"Foo"},
					},
				},
			},
		},

		// This hunt excludes Label Foo
		{
			HuntId:       "H.ExceptLabelFoo",
			StartRequest: start_request,
			CreateTime:   uint64(Clock.Now().UnixNano() / 1000),
			State:        api_proto.Hunt_RUNNING,
			Expires:      uint64(Clock.Now().Add(24 * time.Hour).UnixNano()),
			Condition: &api_proto.HuntCondition{
				ExcludedLabels: &api_proto.HuntLabelCondition{
					Label: []string{"Foo"},
				},
			},
		},
	}

	clients := []client_info.ClientInfo{
		// This client is currently connected
		{
			ClientId:      "C.ConnectedClient",
			Ping:          uint64(Clock.Now().UnixNano()),
			AssignedHunts: []string{},
		},

		// This client already ran on hunt H.1234 it will not be
		// chosen again.
		{
			ClientId:      "C.AlreadyRanAllClients",
			Ping:          uint64(Clock.Now().UnixNano()),
			AssignedHunts: []string{"H.AllClients"},
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
		},
	}

	// Add these clients directory into the index.
	for _, c := range clients {
		err := cvelo_services.SetElasticIndex(
			self.Ctx, self.ConfigObj.OrgId, "clients", c.ClientId, c)
		assert.NoError(self.T(), err)
	}

	// See the hunt update plan.
	plan, err := Foreman{}.CalculateHuntUpdatePlan(
		self.Ctx, self.ConfigObj.VeloConf(), hunts)
	assert.NoError(self.T(), err)

	// The connected client should run the all client hunt.
	assert.True(self.T(),
		huntPresent("H.AllClients", plan["C.ConnectedClient"]))

	// The connected client does not have the label so should get the
	// label hunt.
	assert.True(self.T(),
		huntPresent("H.ExceptLabelFoo", plan["C.ConnectedClient"]))

	// The OfflineClient should not receive any hunts since it is
	// offline.
	assert.True(self.T(),
		!huntPresent("H.AllClients", plan["C.OfflineClient"]))

	// The C.WithLabelFoo client should receive the hunt targeting the
	// label
	assert.True(self.T(),
		huntPresent("H.OnlyLabelFoo", plan["C.WithLabelFoo"]))

	// But should **NOT** receive the hunt that excludes the label.
	assert.True(self.T(),
		!huntPresent("H.ExceptLabelFoo", plan["C.WithLabelFoo"]))

	// The client that already ran the H.AllClients hunt should not
	// receive it again.
	assert.True(self.T(),
		!huntPresent("H.AllClients", plan["C.AlreadyRanAllClients"]))

	// The C.AlreadyRanAllClients client has not label Foo so will not
	// receive the hunt targeting that label.
	assert.True(self.T(),
		!huntPresent("H.OnlyLabelFoo", plan["C.AlreadyRanAllClients"]))

	// Now execute the plan
	err = Foreman{}.ExecuteHuntUpdatePlan(
		self.Ctx, self.ConfigObj.VeloConf(), plan)
	assert.NoError(self.T(), err)

	// Calculating the plan again should produce nothing to do.
	new_plan, err := Foreman{}.CalculateHuntUpdatePlan(
		self.Ctx, self.ConfigObj.VeloConf(), hunts)
	assert.NoError(self.T(), err)

	// The plan should be empty as there is nothing to do!
	assert.True(self.T(), len(new_plan) == 0)
}

func TestForeman(t *testing.T) {
	suite.Run(t, &ForemanTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{
			Indexes: []string{"clients", "hunts"},
		},
	})
}

func huntPresent(hunt_id string, hunts []*api_proto.Hunt) bool {
	for _, h := range hunts {
		if h.HuntId == hunt_id {
			return true
		}
	}
	return false
}
