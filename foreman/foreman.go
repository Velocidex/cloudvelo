// The foreman is a batch proces which scans all clients and ensure
// they are assigned all their hunts and are up to date with their
// event tables.

package foreman

import (
	"context"
	"sync"
	"time"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	Clock utils.Clock = &utils.RealClock{}
)

const (
	// Get all recent clients that have not had the hunt applied to
	// them.
	clientsLaterThanHuntQuery = `
  "query": {"bool": {
    "must_not": [
      %s
      {"terms": {"assigned_hunts": [%q]}}
    ],
    "filter": [
      %s
      {"range": {"last_hunt_timestamp": {"lte": %q}}},
      {"range": {"ping": {"gte": %q}}}
    ]}
  }
`

	updateAllClientHuntId = `
{
 "query": {
   "terms": {
    "_id": %q
 }},
 "script": {
   "source": "ctx._source.last_hunt_timestamp = params.last_hunt_timestamp; ctx._source.assigned_hunts.add(params.hunt_id);",
   "lang": "painless",
   "params": {
     "hunt_id": %q,
     "last_hunt_timestamp": %q
   }
  }
 }
}
`
)

type Foreman struct{}

func (self Foreman) stopHunt(
	ctx context.Context,
	org_config_obj *config_proto.Config, hunt *api_proto.Hunt) error {
	stopHuntQuery := `
{
  "script": {
     "source": "ctx._source.state='STOPPED';",
     "lang": "painless"
  }
}
`

	return cvelo_services.UpdateIndex(
		ctx, org_config_obj.OrgId, "hunts", hunt.HuntId, stopHuntQuery)
}

func (self Foreman) getClientQueryForHunt(hunt *api_proto.Hunt) string {
	extra_conditions := ""
	must_not_condition := ""
	if hunt.Condition != nil {
		labels := hunt.Condition.GetLabels()
		if labels != nil {
			extra_conditions += json.Format(
				`{"terms": {"labels": %q}},`, labels.Label)
		}

		if hunt.Condition.ExcludedLabels != nil &&
			len(hunt.Condition.ExcludedLabels.Label) > 0 {
			must_not_condition += json.Format(
				`{"terms": {"labels": %q}},`,
				hunt.Condition.ExcludedLabels.Label)
		}
	}

	hunt_create_time_ns := hunt.CreateTime * 1000

	// Get all clients that were active in the last hour that need
	// to get the hunt.
	return json.Format(clientsLaterThanHuntQuery,
		must_not_condition, hunt.HuntId,
		extra_conditions,
		hunt_create_time_ns,
		Clock.Now().Add(-time.Hour).UnixNano())
}

// Query the backend for the list of all clients which have not
// received this hunt.
func (self Foreman) CalculateHuntUpdate(
	ctx context.Context,
	org_config_obj *config_proto.Config,
	hunts []*api_proto.Hunt, plan *Plan) error {

	// Prepare a plan of all the hunts we are going to launch right
	// now.
	for _, hunt := range hunts {
		query := json.Format(`{%s, "_source": {"includes": ["client_id"]}}`,
			self.getClientQueryForHunt(hunt))
		hits_chan, err := cvelo_services.QueryChan(ctx, org_config_obj,
			1000, org_config_obj.OrgId, "clients",
			query, "client_id")
		if err != nil {
			return err
		}

		for hit := range hits_chan {
			client_info := &actions_proto.ClientInfo{}
			err := json.Unmarshal(hit, client_info)
			if err != nil {
				continue
			}

			planned_hunts, _ := plan.ClientIdToHunts[client_info.ClientId]
			plan.ClientIdToHunts[client_info.ClientId] = append(planned_hunts, hunt)
		}
	}

	return nil
}

func (self Foreman) GetActiveHunts(
	ctx context.Context,
	org_config_obj *config_proto.Config) ([]*api_proto.Hunt, error) {

	hunt_dispatcher, err := services.GetHuntDispatcher(org_config_obj)
	if err != nil {
		return nil, err
	}

	hunts, err := hunt_dispatcher.ListHunts(
		ctx, org_config_obj, &api_proto.ListHuntsRequest{
			Count: 1000,
		})
	if err != nil {
		return nil, err
	}

	result := make([]*api_proto.Hunt, 0, len(hunts.Items))
	for _, hunt := range hunts.Items {
		if hunt.State != api_proto.Hunt_RUNNING {
			continue
		}

		// Check if the hunt is expired and stop it if it is
		if hunt.Expires < uint64(Clock.Now().UnixNano()/1000) {
			err := self.stopHunt(ctx, org_config_obj, hunt)
			if err != nil {
				return nil, err
			}
			continue
		}

		result = append(result, hunt)
	}

	return result, nil
}

func (self Foreman) UpdateHuntMembership(
	ctx context.Context,
	org_config_obj *config_proto.Config, plan *Plan) error {
	hunts, err := self.GetActiveHunts(ctx, org_config_obj)
	if err != nil {
		return err
	}

	// Get an update plan
	err = self.CalculateHuntUpdate(ctx, org_config_obj, hunts, plan)
	if err != nil {
		return err
	}

	// Now execute the plan.
	return plan.ExecuteHuntUpdate(ctx, org_config_obj)
}

func (self Foreman) UpdateMonitoringTable(
	ctx context.Context,
	org_config_obj *config_proto.Config, plan *Plan) error {

	err := self.CalculateEventTable(ctx, org_config_obj, plan)
	if err != nil {
		return err
	}

	// Now execute the plan.
	return plan.ExecuteClientMonitoringUpdate(ctx, org_config_obj)
}

func (self Foreman) RunOnce(
	ctx context.Context,
	org_config_obj *config_proto.Config) error {

	plan := NewPlan()
	err := self.UpdateHuntMembership(ctx, org_config_obj, plan)
	if err != nil {
		return err
	}

	err = self.UpdateMonitoringTable(ctx, org_config_obj, plan)
	if err != nil {
		return err
	}

	return plan.Close(ctx, org_config_obj)
}

func (self Foreman) Start(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(5 * time.Second):
				err := self.RunOnce(ctx, config_obj)
				if err != nil {
					logger.Error("Foreman: %v", err)
				}
			}
		}
	}()

	return nil
}

func StartForemanService(ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	return Foreman{}.Start(ctx, wg, config_obj)
}

func slice(a []string, length int) []string {
	if length > len(a) {
		length = len(a)
	}
	return a[:length]
}
