package orgs

import (
	"errors"

	"www.velocidex.com/golang/cloudvelo/schema"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Remove the org and all its data.
func (self *OrgManager) DeleteOrg(org_id string) error {

	if utils.IsRootOrg(org_id) {
		return errors.New("Can not remove root org.")
	}

	err := orgs.RemoveOrgFromUsers(org_id)
	if err != nil {
		return err
	}

	// Remove the org from the index.
	err = cvelo_services.DeleteDocument(services.ROOT_ORG_ID, "orgs",
		org_id, cvelo_services.SyncDelete)
	if err != nil {
		return err
	}

	self.mu.Lock()
	delete(self.orgs, org_id)
	delete(self.org_id_by_nonce, org_id)
	self.mu.Unlock()

	// Drop all the org's indexes
	return schema.Delete(self.ctx, self.config_obj,
		org_id, services.ROOT_ORG_ID)
}
