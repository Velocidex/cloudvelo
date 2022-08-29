package users

import (
	"context"
	"crypto/x509"
	"errors"
	"os"
	"sync"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
)

// The record stored in the elastic index
type UserRecord struct {
	Username string `json:"username"`
	Record   string `json:"record"` // An encoded api_proto.VelociraptorUser
}

type UserGUIOptions struct {
	Username   string `json:"username"`
	GUIOptions string `json:"gui_options"` // An endoded api_proto.SetGUIOptionsRequest
}

type UserManager struct {
	ca_pool    *x509.CertPool
	config_obj *config_proto.Config
	ctx        context.Context
}

func (self *UserManager) SetUser(user_record *api_proto.VelociraptorUser) error {
	serialized, err := json.Marshal(user_record)
	if err != nil {
		return err
	}

	return cvelo_services.SetElasticIndex(self.config_obj.OrgId,
		"users", user_record.Name, &UserRecord{
			Username: user_record.Name,
			Record:   string(serialized),
		})
}

func (self *UserManager) ListUsers() ([]*api_proto.VelociraptorUser, error) {
	return nil, errors.New("Not implemented")
}

func (self *UserManager) GetUserFromContext(ctx context.Context) (
	*api_proto.VelociraptorUser, *config_proto.Config, error) {

	grpc_user_info := users.GetGRPCUserInfo(self.config_obj, ctx, self.ca_pool)
	user_record, err := self.GetUser(grpc_user_info.Name)
	if err != nil {
		return nil, nil, err
	}

	user_record.CurrentOrg = grpc_user_info.CurrentOrg

	// Fetch the appropriate config file fro the org manager.
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, nil, err
	}

	org_config_obj, err := org_manager.GetOrgConfig(user_record.CurrentOrg)
	return user_record, org_config_obj, err
}

func (self *UserManager) GetUserWithHashes(username string) (
	*api_proto.VelociraptorUser, error) {

	serialized, err := cvelo_services.GetElasticRecord(self.ctx,
		self.config_obj.OrgId, "users", username)
	if err == os.ErrNotExist {
		// User is not found, create an empty user record.  The
		// existance of a user record depends on the header having the
		// correct username - we always trust the headers.
		return &api_proto.VelociraptorUser{
			Name: username,
		}, nil
	}

	if err != nil {
		return nil, err
	}

	user_record := &UserRecord{}
	err = json.Unmarshal(serialized, user_record)
	if err != nil {
		return nil, err
	}

	result := &api_proto.VelociraptorUser{
		Name: user_record.Username,
	}

	return result, json.Unmarshal(
		[]byte(user_record.Record), result)
}

func (self *UserManager) GetUser(username string) (
	*api_proto.VelociraptorUser, error) {
	result, err := self.GetUserWithHashes(username)
	if err != nil {
		return nil, err
	}
	result.PasswordHash = nil
	result.PasswordSalt = nil

	return result, nil
}

func (self *UserManager) SetUserOptions(
	username string,
	options *api_proto.SetGUIOptionsRequest) error {

	user_record, err := self.GetUserOptions(username)
	if err != nil {
		return err
	}

	if options.Theme != "" {
		user_record.Theme = options.Theme
	}

	if options.Timezone != "" {
		user_record.Timezone = options.Timezone
	}

	if options.Lang != "" {
		user_record.Lang = options.Lang
	}

	if options.Options != "" {
		user_record.Options = options.Options
	}

	if options.Org != "" {
		user_record.Org = options.Org
	}

	serialized, err := json.Marshal(user_record)
	if err != nil {
		return err
	}

	return cvelo_services.SetElasticIndex(
		self.config_obj.OrgId,
		"user_options", username, &UserGUIOptions{
			Username:   username,
			GUIOptions: string(serialized),
		})
}

func (self *UserManager) GetUserOptions(username string) (
	*api_proto.SetGUIOptionsRequest, error) {

	serialized, err := cvelo_services.GetElasticRecord(self.ctx,
		self.config_obj.OrgId, "user_options", username)
	if err == os.ErrNotExist {
		return &api_proto.SetGUIOptionsRequest{}, nil
	}

	if err != nil {
		return nil, err
	}

	user_record := &UserGUIOptions{}
	err = json.Unmarshal(serialized, user_record)
	if err != nil {
		return nil, err
	}

	result := &api_proto.SetGUIOptionsRequest{}
	if user_record.GUIOptions == "" {
		return result, nil
	}
	err = json.Unmarshal(
		[]byte(user_record.GUIOptions), result)

	if result.Customizations == nil {
		result.Customizations = &api_proto.GUICustomizations{}
	}

	result.Customizations.DisableServerEvents = true
	return result, err
}

func (self *UserManager) GetFavorites(
	config_obj *config_proto.Config,
	principal, fav_type string) (*api_proto.Favorites, error) {
	return nil, errors.New("Not implemented")
}

func StartUserManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	CA_Pool := x509.NewCertPool()
	if config_obj.Client != nil {
		CA_Pool.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))
	}

	service := &UserManager{
		ca_pool:    CA_Pool,
		config_obj: config_obj,
		ctx:        ctx,
	}
	services.RegisterUserManager(service)

	// Register our new acl manager.
	acls.SetACLManager(&ACLManager{&acls.ACLManager{}})

	return nil
}
