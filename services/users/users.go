package users

import (
	"context"
	"crypto/x509"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/cloudvelo/config"
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
	config_obj *config.Config
	ctx        context.Context

	lru *ttlcache.Cache
}

func (self *UserManager) SetUser(
	ctx context.Context, user_record *api_proto.VelociraptorUser) error {
	serialized, err := protojson.Marshal(user_record)
	if err != nil {
		return err
	}

	self.lru.Set(user_record.Name, user_record)

	return cvelo_services.SetElasticIndex(ctx,
		self.config_obj.OrgId,
		"users", user_record.Name, &UserRecord{
			Username: user_record.Name,
			Record:   string(serialized),
		})
}

func (self *UserManager) ListUsers(ctx context.Context) (
	[]*api_proto.VelociraptorUser, error) {
	hits, err := cvelo_services.QueryElasticRaw(self.ctx, services.ROOT_ORG_ID,
		"users", `{"query": {"match_all": {}}}`)
	if err != nil {
		return nil, err
	}

	result := make([]*api_proto.VelociraptorUser, 0, len(hits))
	for _, hit := range hits {
		record := &UserRecord{}
		err := json.Unmarshal(hit, record)
		if err != nil {
			continue
		}

		user_record := &api_proto.VelociraptorUser{}
		err = protojson.Unmarshal([]byte(record.Record), user_record)
		if err == nil {
			result = append(result, user_record)
		}
	}

	return result, nil
}

func (self *UserManager) GetUserFromContext(ctx context.Context) (
	*api_proto.VelociraptorUser, *config_proto.Config, error) {

	grpc_user_info := users.GetGRPCUserInfo(
		self.config_obj.VeloConf(), ctx, self.ca_pool)
	user_record, err := self.GetUser(ctx, grpc_user_info.Name)
	if err != nil {
		return nil, nil, err
	}

	user_record.CurrentOrg = grpc_user_info.CurrentOrg
	if len(grpc_user_info.Orgs) > 0 {
		user_record.Orgs = grpc_user_info.Orgs
	}

	// Fetch the appropriate config file fro the org manager.
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, nil, err
	}

	org_config_obj, err := org_manager.GetOrgConfig(user_record.CurrentOrg)
	return user_record, org_config_obj, err
}

func (self *UserManager) GetUserWithHashes(
	ctx context.Context, username string) (*api_proto.VelociraptorUser, error) {

	cached_any, err := self.lru.Get(username)
	if err == nil {
		cached, ok := cached_any.(*api_proto.VelociraptorUser)
		if ok {
			// Return a copy so the cached version does not get
			// changed.
			return proto.Clone(cached).(*api_proto.VelociraptorUser), nil
		}
	}

	serialized, err := cvelo_services.GetElasticRecord(self.ctx,
		self.config_obj.OrgId, "users", username)
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

	err = protojson.Unmarshal(
		[]byte(user_record.Record), result)
	if err != nil {
		return nil, err
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	for _, org := range result.Orgs {
		org_config_obj, err := org_manager.GetOrgConfig(org.Id)
		if err == nil {
			org.Name = org_config_obj.OrgName
		}
	}

	self.lru.Set(username, result)

	return result, err
}

func (self *UserManager) GetUser(ctx context.Context, username string) (
	*api_proto.VelociraptorUser, error) {
	result, err := self.GetUserWithHashes(ctx, username)
	if err != nil {
		return nil, err
	}
	result.PasswordHash = nil
	result.PasswordSalt = nil

	return result, nil
}

func (self *UserManager) SetUserOptions(
	ctx context.Context,
	username string,
	options *api_proto.SetGUIOptionsRequest) error {

	user_record, err := self.GetUserOptions(ctx, username)
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

	serialized, err := protojson.Marshal(user_record)
	if err != nil {
		return err
	}

	return cvelo_services.SetElasticIndex(ctx,
		self.config_obj.OrgId,
		"user_options", username, &UserGUIOptions{
			Username:   username,
			GUIOptions: string(serialized),
		})
}

func (self *UserManager) GetUserOptions(ctx context.Context, username string) (
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
	err = protojson.Unmarshal(
		[]byte(user_record.GUIOptions), result)

	if result.Customizations == nil {
		result.Customizations = &api_proto.GUICustomizations{}
	}

	result.Customizations.DisableServerEvents = true

	// Add any links in the config file to the user's preferences.
	if self.config_obj.GUI != nil {
		result.Links = users.MergeGUILinks(result.Links, self.config_obj.GUI.Links)
	}

	// Add the defaults.
	result.Links = users.MergeGUILinks(result.Links, users.DefaultLinks)

	return result, err
}

func (self *UserManager) GetFavorites(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal, fav_type string) (*api_proto.Favorites, error) {
	return nil, errors.New("UserManager.GetFavorites Not implemented")
}

func StartUserManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config.Config) error {

	CA_Pool := x509.NewCertPool()
	if config_obj.Client != nil {
		CA_Pool.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))
	}

	service := &UserManager{
		ca_pool:    CA_Pool,
		config_obj: config_obj,
		ctx:        ctx,
		lru:        ttlcache.NewCache(),
	}
	service.lru.SetTTL(10 * time.Second)

	services.RegisterUserManager(service)

	// Register our new acl manager.
	acl_manager := NewACLManager(ctx)
	acls.SetACLManager(acl_manager)

	return nil
}
