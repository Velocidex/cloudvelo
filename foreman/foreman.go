// The foreman is a batch proces which scans all clients and ensure
// they are assigned all their hunts and are up to date with their
// event tables.

package foreman

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	cvelo_indexing "www.velocidex.com/golang/cloudvelo/services/indexing"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
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
		ctx, org_config_obj.OrgId, "hunts", hunt.HuntId, stopHuntQuery)
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

	// Hunt specifies labels does the client have these labels?
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

func (self Foreman) planForClient(
	ctx context.Context,
	org_config_obj *config_proto.Config,
	client_info *api.ClientRecord,
	hunts []*api_proto.Hunt, plan *Plan) {

	for _, h := range hunts {
		self.planHuntForClient(ctx, org_config_obj, client_info, h, plan)
	}
	self.planMonitoringForClient(ctx, org_config_obj, client_info, plan)
}

func (self Foreman) CalculateUpdate(
	ctx context.Context,
	org_config_obj *config_proto.Config,
	hunts []*api_proto.Hunt, plan *Plan) error {

	// Get all clients that pinged within the last period
	indexer, err := services.GetIndexer(org_config_obj)
	if err != nil {
		return err
	}

	clients_chan, err := indexer.(*cvelo_indexing.Indexer).SearchClientRecordsChan(
		ctx, nil, org_config_obj,
		fmt.Sprintf("after:%d", self.last_run_time.UnixNano()), "")
	if err != nil {
		return err
	}

	for client_info := range clients_chan {
		self.planForClient(ctx, org_config_obj, client_info, hunts, plan)
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
	org_config_obj *config_proto.Config, plan *Plan) error {

	hunts, err := self.GetActiveHunts(ctx, org_config_obj)
	if err != nil {
		return err
	}

	huntCountGauge.WithLabelValues(org_config_obj.OrgId).Set(float64(len(hunts)))

	// Get an update plan
	err = self.CalculateUpdate(ctx, org_config_obj, hunts, plan)
	if err != nil {
		return err
	}

	// Now execute the plan.
	err = plan.ExecuteHuntUpdate(ctx, org_config_obj)
	if err != nil {
		return err
	}

	return plan.ExecuteClientMonitoringUpdate(ctx, org_config_obj)
}

func (self Foreman) RunOnce(
	ctx context.Context,
	org_config_obj *config_proto.Config) error {

	logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	orgs := org_manager.ListOrgs()
	orgCountGauge.Set(float64(len(orgs)))

	for _, org := range orgs {
		org_config_obj.OrgId = org.OrgId
		if logger != nil {
			logger.Debug("Foreman RunOnce, org: %v", org_config_obj.OrgId)
		}
		plan, err := NewPlan(org_config_obj)
		if err != nil {
			continue
		}
		err = self.UpdatePlan(ctx, org_config_obj, plan)
		if err != nil {
			if logger != nil {
				logger.Error("UpdatePlan, orgId=%v: %v", org_config_obj.OrgId, err)
			}
			continue
		}

		err = plan.Close(ctx, org_config_obj)
		if err != nil {
			if logger != nil {
				logger.Error("Close, orgId=%v: %v", org_config_obj.OrgId, err)
			}
			continue
		}
	}

	return nil
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
