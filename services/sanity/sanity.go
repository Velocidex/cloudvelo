package sanity

import (
	"context"
	"sync"

	"www.velocidex.com/golang/cloudvelo/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// This service checks the running server environment for sane
// conditions.
type SanityChecks struct{}

// Check sanity of general server state - this is only done for the root org.
func (self *SanityChecks) CheckRootOrg(
	ctx context.Context, config_obj *config_proto.Config) error {

	// Make sure the initial user accounts are created with the
	// administrator roles.
	if config_obj.GUI != nil && config_obj.GUI.Authenticator != nil {
		// Create initial orgs
		org_manager, err := services.GetOrgManager()
		if err != nil {
			return err
		}

		for _, org := range config_obj.GUI.InitialOrgs {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Info("<green>Creating initial org for</> %v", org.Name)
			_, err := org_manager.CreateNewOrg(org.Name, org.OrgId, services.RandomNonce)
			if err != nil {
				return err
			}
		}

		err = createInitialUsers(ctx, config_obj, config_obj.GUI.InitialUsers)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *SanityChecks) Check(
	ctx context.Context, config_obj *config_proto.Config) error {
	if utils.IsRootOrg(config_obj.OrgId) {
		err := self.CheckRootOrg(ctx, config_obj)
		if err != nil {
			return err
		}
	}

	return nil
}

func NewSanityCheckService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config.Config) error {

	result := &SanityChecks{}
	return result.Check(ctx, config_obj.VeloConf())
}
