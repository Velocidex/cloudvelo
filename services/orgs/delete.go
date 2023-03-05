package orgs

import (
	"context"
	"errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/cloudvelo/schema"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	deleteOrgCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "velociraptor_delete_org_count",
			Help: "Count of organizations deleted from the Velociraptor system.",
		})
)

// Remove the org and all its data.
func (self *OrgManager) DeleteOrg(
	ctx context.Context, org_id string) error {

	if utils.IsRootOrg(org_id) {
		return errors.New("Can not remove root org.")
	}

	logger := logging.GetLogger(self.config_obj, &logging.Audit)
	if logger != nil {
		logger.Info("Deleted organization: %v", org_id)
	}

	err := orgs.RemoveOrgFromUsers(ctx, org_id)
	if err != nil {
		return err
	}

	// Remove the org from the index.
	err = cvelo_services.DeleteDocument(ctx,
		services.ROOT_ORG_ID, "orgs",
		org_id, cvelo_services.SyncDelete)
	if err != nil {
		return err
	}

	self.mu.Lock()
	delete(self.orgs, org_id)
	delete(self.org_id_by_nonce, org_id)
	self.mu.Unlock()

	deleteOrgCounter.Inc()

	// Drop all the org's indexes
	return schema.Delete(self.ctx, self.config_obj,
		org_id, services.ROOT_ORG_ID)
}
