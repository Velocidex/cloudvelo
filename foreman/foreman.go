// The foreman is a batch proces which scans all clients and ensure
// they are assigned all their hunts and are up to date with their
// event tables.

package foreman

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	// Schedule clients that pings this far back on new hunts. This
	// ensures we schedule as many clients as possible in large more
	// efficient operations.
	MAXIMUM_PING_BACKLOG = 3 * time.Hour
)

var (
	huntCountGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "foreman_hunt_gauge",
			Help: "Number of active hunts (per organization).",
		},
		[]string{"orgId"},
	)

	orgCountGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "foreman_org_gauge",
			Help: "Number of orgs managed by Foreman.",
		},
	)

	runCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "foreman_run_counter",
			Help: "Count of Foreman runs.",
		},
	)
)

type Foreman struct {
	last_run_time time.Time
}

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
		ctx, org_config_obj.OrgId, "persisted", hunt.HuntId, stopHuntQuery)
}

// Rather than retrieving the entire client record we only get those
// fields the foreman cares about.
const getMinimalClientInfoQuery = `{
  "query": {
    "bool": {
      "must": [
        {
          "term": {
            "client_id": %q
          }
        },
        {
          "bool": {
            "should": [
              {
                "exists": {
                  "field": "last_event_table_version"
                }
              },
              {
                "exists": {
                  "field": "assigned_hunts"
                }
              },
              {
                "exists": {
                  "field": "labels_timestamp"
                }
              },
              {
                "exists": {
                  "field": "labels"
                }
              },
              {
                "exists": {
                  "field": "system"
                }
              },
              {
                "exists": {
                  "field": "last_label_timestamp"
                }
              }
            ]
          }
        },,
		{
			"match": {
              "doc_type": "clients"
              
            }
		}
      ]
    }
  }
}
`

// Get a minimal client info with only fields relevant to the
// foreman. This allows us to query less documents and it is more
// efficient. We now write multiple documents for assigned_hunts and
// merge them in this function.
func getMinimalClientInfo(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) (*api.ClientRecord, error) {

	query := json.Format(getMinimalClientInfoQuery, client_id)

	result := &api.ClientRecord{
		ClientId: client_id,
		DocType:  "clients",
	}
	hits, err := cvelo_services.QueryChan(ctx,
		config_obj, 1000, config_obj.OrgId,
		"persisted", query, "client_id")
	if err != nil {
		return nil, err
	}

	for hit := range hits {
		h := &api.ClientRecord{
			DocType: "clients",
		}
		err := json.Unmarshal(hit, h)
		if err != nil {
			continue
		}

		if h.LastLabelTimestamp > 0 {
			result.LastLabelTimestamp = h.LastLabelTimestamp
		}
		if h.LastEventTableVersion > 0 {
			result.LastEventTableVersion = h.LastEventTableVersion
		}

		for _, l := range h.Labels {
			if !utils.InString(result.Labels, l) {
				result.Labels = append(result.Labels, l)
				result.LowerLabels = append(result.LowerLabels, strings.ToLower(l))
			}
		}

		for _, l := range h.AssignedHunts {
			if !utils.InString(result.AssignedHunts, l) {
				result.AssignedHunts = append(result.AssignedHunts, l)
			}
		}

		if h.System != "" {
			result.System = h.System
		}
	}

	return result, nil
}

func (self Foreman) planMonitoringForClient(
	ctx context.Context,
	org_config_obj *config_proto.Config,
	client_info *api.ClientRecord, plan *Plan) {

	// When do we need to update the client's monitoring table:
	// 1. The client was recently seen
	// 2. Any of its labels were changed after the last update or
	// 3. The event table is newer than the last update time.
	if !(client_info.LastLabelTimestamp >= uint64(self.last_run_time.UnixNano()) ||
		client_info.LastEventTableVersion < plan.current_monitoring_state.Version) {
		return
	}

	key := labelsKey(client_info.Labels, plan.current_monitoring_state)
	_, pres := plan.MonitoringTables[key]
	if !pres {
		message := GetClientUpdateEventTableMessage(ctx, org_config_obj,
			plan.current_monitoring_state, client_info.Labels)
		plan.MonitoringTables[key] = message
	}

	clients, _ := plan.MonitoringTablesToClients[key]
	if !utils.InString(clients, client_info.ClientId) {
		plan.MonitoringTablesToClients[key] = append(
			[]string{client_info.ClientId}, clients...)
	}

	client_info.LastEventTableVersion = plan.current_monitoring_state.Version
	plan.ClientIdToClientRecords[client_info.ClientId] = client_info
}

func (self Foreman) planHuntForClient(
	ctx context.Context,
	org_config_obj *config_proto.Config,
	client_info *api.ClientRecord,
	hunt *api_proto.Hunt,
	plan *Plan) {

	// Hunt is already assigned, skip it
	if utils.InString(client_info.AssignedHunts, hunt.HuntId) {
		return
	}

	// Hunt is unconditional, assign it.
	if hunt.Condition == nil {
		plan.assignClientToHunt(client_info, hunt)
		return
	}

	// Hunt specifies labels. Does the client have these labels?
	labels := hunt.Condition.GetLabels()
	if labels != nil && len(labels.Label) > 0 {

		// Client has none of the specified labels.
		if !clientHasLabels(client_info, labels.Label) {
			return
		}
	}

	if hunt.Condition.ExcludedLabels != nil &&
		len(hunt.Condition.ExcludedLabels.Label) > 0 {
		// Client has an exclude label ignore it.
		if clientHasLabels(client_info, hunt.Condition.ExcludedLabels.Label) {
			return
		}
	}

	// Handle OS target
	os_condition := hunt.Condition.GetOs()
	if os_condition != nil &&
		os_condition.Os != api_proto.HuntOsCondition_ALL {
		os_name := ""
		switch os_condition.Os {
		case api_proto.HuntOsCondition_WINDOWS:
			os_name = "windows"
		case api_proto.HuntOsCondition_LINUX:
			os_name = "linux"
		case api_proto.HuntOsCondition_OSX:
			os_name = "darwin"
		}

		if os_name != "" && client_info.System != os_name {
			return
		}
	}

	// If we get here we assign the hunt to the client
	plan.assignClientToHunt(client_info, hunt)
}

// Find clients that polled back up to 3 hours and schedule them for
// new hunts. This function spawns a goroutine to do this in the
// background as it can take a while.
func (self Foreman) scheduleClientsWithBacklog(
	ctx context.Context,
	wg *sync.WaitGroup,
	org_config_obj *config_proto.Config,
	hunts []*api_proto.Hunt) {

	if len(hunts) == 0 {
		return
	}

	new_plan, err := NewPlan(org_config_obj)
	if err != nil {
		return
	}

	// Schedule started hunts in the background because it could take
	// a while.
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Consider all clients that were active in the last 3 hours
		// for scheduling.
		early_time_range := utils.GetTime().Now().
			Add(-MAXIMUM_PING_BACKLOG).UnixNano()
		if early_time_range < 0 {
			early_time_range = 0
		}

		hunt_ids := make([]string, 0, len(hunts))
		for _, h := range hunts {
			hunt_ids = append(hunt_ids, h.HuntId)
		}

		for client_id := range self.getClientsSeenAfter(
			ctx, org_config_obj, early_time_range) {

			// Need to deal with hunts.
			seen_hunts, err := self.getClientHuntMembership(
				ctx, org_config_obj, client_id, hunt_ids)
			if err != nil {
				return
			}

			for _, h := range hunts {
				_, pres := seen_hunts[h.HuntId]
				if !pres {
					client_info, err := getMinimalClientInfo(
						ctx, org_config_obj, client_id)
					if err != nil {
						continue
					}

					self.planHuntForClient(ctx, org_config_obj, client_info, h, new_plan)
				}
			}
		}

		err = new_plan.ExecuteHuntUpdate(ctx, org_config_obj)
		if err != nil {
			logging.GetLogger(org_config_obj, &logging.ClientComponent).
				Error("Foreman ExecuteHuntUpdate: %v", err)
		}
	}()
}

func (self Foreman) CalculateUpdate(
	ctx context.Context,
	wg *sync.WaitGroup,
	org_config_obj *config_proto.Config,
	hunts []*api_proto.Hunt, plan *Plan) error {

	// Get all clients that pinged within the last period
	now := utils.GetTime().Now().UnixNano()
	early_time_range := self.last_run_time.UnixNano()
	if early_time_range < 0 {
		early_time_range = 0
	}

	// Split hunts into two sets
	// 1. Hunts that were started between the last sweep time and now.
	// 2. Hunts that were started before the last sweep time.
	//
	// For the first case we need to schedule clients that were active
	// in the recent past. In the second case we only need to schedule
	// clients that were active since the last sweep time.
	var started_hunts []*api_proto.Hunt
	var running_hunts []*api_proto.Hunt
	var running_hunt_id []string

	for _, hunt := range hunts {
		hunt_start := int64(hunt.StartTime) * 1000
		if hunt_start >= early_time_range && hunt_start < now {
			started_hunts = append(started_hunts, hunt)
		} else {
			running_hunts = append(running_hunts, hunt)
			running_hunt_id = append(running_hunt_id, hunt.HuntId)
		}
	}

	// Schedule new hunts with backlog of client pings. This is
	// expensive as we need to consider all clients that checked in
	// within the last 3 hours. But this only happens when a hunt is
	// created so not that often.
	if len(started_hunts) > 0 {
		// We must wait for this to complete before we run again to
		// make sure the client's AssignedHunts are up to date.
		self.scheduleClientsWithBacklog(
			ctx, wg, org_config_obj, started_hunts)
	}

	for client_id := range self.getClientsSeenAfter(ctx, org_config_obj, early_time_range) {
		client_info, err := getMinimalClientInfo(ctx, org_config_obj, client_id)
		if err != nil {
			return err
		}

		// Need to deal with hunts.
		if len(running_hunt_id) > 0 {
			seen_hunts, err := self.getClientHuntMembership(ctx, org_config_obj, client_id, running_hunt_id)
			if err != nil {
				return err
			}

			for _, h := range running_hunts {
				_, pres := seen_hunts[h.HuntId]
				if !pres {
					self.planHuntForClient(ctx, org_config_obj, client_info, h, plan)
				}
			}
		}

		self.planMonitoringForClient(ctx, org_config_obj, client_info, plan)
	}

	return nil
}

const (
	getRecentClientsQuery = `{
  "query": {
    "bool": {
      "must": [
        {
          "range": {
            "ping": {
              "gt": %q
            }
          }
        },
		{
			"match": {
              "doc_type": "clients"
              
            }
		}
      ]
    }
  }
}
`
	clientMembershipQuery = `
{
  "query": {
    "bool": {
      "must": [
        {
          "terms": {
             "assigned_hunts": %q
          }
        },
        {
          "term": {
            "client_id": {
               "value": %q
            }
          }
        },
		{
			"match": {
              "doc_type": "clients"
              
            }
		}
      ]
    }
  }
}
`
)

// Get all the hunts that this client is a member of
func (self Foreman) getClientHuntMembership(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, hunts []string) (map[string]bool, error) {

	query := json.Format(clientMembershipQuery, hunts, client_id)
	seen := make(map[string]bool)
	hits, err := cvelo_services.QueryChan(ctx,
		config_obj, 1000, config_obj.OrgId,
		"persisted", query, "client_id")
	if err != nil {
		return nil, err
	}

	for hit := range hits {
		client_record := &api.ClientRecord{
			DocType: "clients",
		}
		err = json.Unmarshal(hit, client_record)
		if err != nil {
			continue
		}

		for _, hunt_id := range client_record.AssignedHunts {
			seen[hunt_id] = true
		}
	}

	return seen, nil
}

func (self Foreman) getClientsSeenAfter(
	ctx context.Context,
	config_obj *config_proto.Config,
	early_time_range int64) chan string {
	output_chan := make(chan string)

	go func() {
		defer close(output_chan)

		query := json.Format(getRecentClientsQuery, early_time_range)

		hits, err := cvelo_services.QueryChan(
			ctx, config_obj, 1000,
			config_obj.OrgId, "persisted", query, "ping")
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("getClientsSeenAfter: %v", err)
			return
		}

		for hit := range hits {
			client_record := &api.ClientRecord{
				DocType: "clients",
			}
			err := json.Unmarshal(hit, client_record)
			if err != nil || client_record.ClientId == "" {
				continue
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- client_record.ClientId:
			}
		}

	}()

	return output_chan
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
		if hunt.State != api_proto.Hunt_RUNNING ||
			hunt.StartTime == 0 {
			continue
		}

		// Check if the hunt is expired and stop it if it is
		if hunt.Expires < uint64(utils.GetTime().Now().UnixNano()/1000) {
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

func (self Foreman) UpdatePlan(
	ctx context.Context,
	wg *sync.WaitGroup,
	org_config_obj *config_proto.Config, plan *Plan) error {

	hunts, err := self.GetActiveHunts(ctx, org_config_obj)
	if err != nil {
		return err
	}

	huntCountGauge.WithLabelValues(org_config_obj.OrgId).Set(float64(len(hunts)))

	// Get an update plan
	err = self.CalculateUpdate(ctx, wg, org_config_obj, hunts, plan)
	if err != nil {
		return err
	}

	// Now execute the plan.
	err = plan.ExecuteHuntUpdate(ctx, org_config_obj)
	if err != nil {
		return err
	}

	err = plan.ExecuteClientMonitoringUpdate(ctx, org_config_obj)
	if err != nil {
		return err
	}

	return plan.closePlan(ctx, org_config_obj)
}

func (self Foreman) RunOnce(
	ctx context.Context,
	config_obj *config_proto.Config) error {

	// Wait for all jobs to finish before we exit to avoid a race
	// condition.
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	orgs := org_manager.ListOrgs()
	orgCountGauge.Set(float64(len(orgs)))

	for _, org := range orgs {
		org_config_obj, err := org_manager.GetOrgConfig(org.OrgId)
		if err != nil {
			continue
		}

		if logger != nil {
			logger.Debug("Foreman RunOnce, org: %v", org_config_obj.OrgId)
		}

		plan, err := NewPlan(org_config_obj)
		if err != nil {
			continue
		}

		err = self.UpdatePlan(ctx, wg, org_config_obj, plan)
		if err != nil {
			if logger != nil {
				logger.Error("UpdatePlan, orgId=%v: %v", org_config_obj.OrgId, err)
			}
			continue
		}
	}

	// Make sure to flush out any outstanding writes so the data is
	// fresh.
	return cvelo_services.FlushBulkIndexer()
}

func (self *Foreman) Start(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config.Config) error {

	// Run once inline to trap any errors.
	self.last_run_time = utils.GetTime().Now()
	err := self.RunOnce(ctx, config_obj.VeloConf())
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger := logging.GetLogger(config_obj.VeloConf(), &logging.FrontendComponent)

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Duration(config_obj.Cloud.ForemanIntervalSeconds) * time.Second):
				err := self.RunOnce(ctx, config_obj.VeloConf())
				if err != nil {
					logger.Error("Foreman: %v", err)
				} else {
					runCounter.Inc()
				}
				self.last_run_time = utils.GetTime().Now()
			}
		}
	}()

	return nil
}

func StartForemanService(ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config.Config) error {
	service := NewForeman()

	return service.Start(ctx, wg, config_obj)
}

func slice(a []string, length int) []string {
	if length > len(a) {
		length = len(a)
	}
	return a[:length]
}

// Return true if any of the labels match
func clientHasLabels(client_info *api.ClientRecord, labels []string) bool {
	for _, label := range labels {
		if utils.InString(client_info.LowerLabels, strings.ToLower(label)) {
			return true
		}
	}
	return false
}

func huntsContain(hunts []*api_proto.Hunt, hunt_id string) bool {
	for _, h := range hunts {
		if h.HuntId == hunt_id {
			return true
		}
	}
	return false
}

func NewForeman() *Foreman {
	return &Foreman{
		last_run_time: utils.GetTime().Now(),
	}
}
