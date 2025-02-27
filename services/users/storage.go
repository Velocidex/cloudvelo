package users

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/cloudvelo/config"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
	"www.velocidex.com/golang/velociraptor/utils"
)

// The object that is cached in the LRU
type _CachedUserObject struct {
	user_record *api_proto.VelociraptorUser
	gui_options *api_proto.SetGUIOptionsRequest
}

type UserStorageManager struct {
	mu sync.Mutex

	config_obj *config.Config

	lru *ttlcache.Cache
	id  int64
}

func (self *UserStorageManager) GetUserWithHashes(ctx context.Context, username string) (
	*api_proto.VelociraptorUser, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if username == "" {
		return nil, errors.New("Must set a username")
	}

	var cache *_CachedUserObject
	var ok bool

	// Check the LRU for a cache if it is there
	cache_any, err := self.lru.Get(username)
	if err == nil {
		cache, ok = cache_any.(*_CachedUserObject)
		if ok && cache.user_record != nil {
			return proto.Clone(cache.user_record).(*api_proto.VelociraptorUser), nil
		}
	}

	// Otherwise add a new cache
	if cache == nil {
		cache = &_CachedUserObject{}
	}

	err = validateUsername(self.config_obj.VeloConf(), username)
	if err != nil {
		return nil, err
	}

	serialized, err := cvelo_services.GetElasticRecord(ctx,
		services.ROOT_ORG_ID, "users", username)
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

// Update the record in the LRU
func (self *UserStorageManager) SetUser(
	ctx context.Context, user_record *api_proto.VelociraptorUser) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if user_record.Name == "" {
		return errors.New("Must set a username")
	}

	err := validateUsername(self.config_obj.VeloConf(), user_record.Name)
	if err != nil {
		return err
	}

	var cache *_CachedUserObject

	// Check the LRU for a cache if it is there
	cache_any, err := self.lru.Get(user_record.Name)
	if err == nil {
		cache, _ = cache_any.(*_CachedUserObject)
	}
	if cache == nil {
		cache = &_CachedUserObject{}
	}
	cache.user_record = proto.Clone(user_record).(*api_proto.VelociraptorUser)

	serialized, err := protojson.Marshal(user_record)
	if err != nil {
		return err
	}

	return cvelo_services.SetElasticIndex(ctx,
		services.ROOT_ORG_ID,
		"persisted", user_record.Name, &UserRecord{
			Username: user_record.Name,
			Record:   string(serialized),
			DocType:  "users",
		})
}

func (self *UserStorageManager) ListAllUsers(
	ctx context.Context) ([]*api_proto.VelociraptorUser, error) {

	hits, _, err := cvelo_services.QueryElasticRaw(
		ctx, services.ROOT_ORG_ID,
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

func (self *UserStorageManager) SetUserOptions(ctx context.Context,
	username string, options *api_proto.SetGUIOptionsRequest) error {

	// Merge the old options with the new options
	old_options, err := self.GetUserOptions(ctx, username)
	if err != nil {
		old_options = &api_proto.SetGUIOptionsRequest{}
	}

	// For now we do not allow the user to set the links in their
	// profile.
	old_options.Links = nil

	if options.Lang != "" {
		old_options.Lang = options.Lang
	}

	if options.Theme != "" {
		old_options.Theme = options.Theme
	}

	if options.Timezone != "" {
		old_options.Timezone = options.Timezone
	}

	if options.Org != "" {
		old_options.Org = options.Org
	}

	if options.Options != "" {
		old_options.Options = options.Options
	}

	old_options.DefaultPassword = options.DefaultPassword
	old_options.DefaultDownloadsLock = options.DefaultDownloadsLock

	serialized, err := protojson.Marshal(old_options)
	if err != nil {
		return err
	}

	return cvelo_services.SetElasticIndex(ctx,
		services.ROOT_ORG_ID,
		"persisted", username+"_options", &UserGUIOptions{
			Username:   username,
			GUIOptions: string(serialized),
			DocType:    "user_options",
		})
}

const doc_type_query = `
{
  "query": {
     "bool": {
       "must": [{"match": {"_id": %q}}, {"match": {"doc_type": %q}}]
      }
  },
  "sort": [{"timestamp": {"order": "desc"}}],
  "size": 1
}`

func (self *UserStorageManager) GetUserOptions(ctx context.Context, username string) (
	*api_proto.SetGUIOptionsRequest, error) {

	serialized, err := cvelo_services.GetElasticRecordByQuery(ctx,
		services.ROOT_ORG_ID, "persisted",
		json.Format(doc_type_query, username+"_options", "user_options"))
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

func (self *UserStorageManager) GetFavorites(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal, fav_type string) (*api_proto.Favorites, error) {
	return nil, errors.New("UserManager.GetFavorites Not implemented")
}

func (self *UserStorageManager) DeleteUser(ctx context.Context, username string) error {
	logger := logging.GetLogger(self.config_obj.VeloConf(), &logging.Audit)
	if logger != nil {
		logger.Info("Deleted user: %v", username)
	}

	deleteUserCounter.Inc()

	return errors.New("UserManager.DeleteUser not implemented")
}

func NewUserStorageManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config.Config) (*UserStorageManager, error) {
	result := &UserStorageManager{
		config_obj: config_obj,
		lru:        ttlcache.NewCache(),
		id:         utils.GetGUID(),
	}

	result.lru.SetCacheSizeLimit(1000)
	result.lru.SetTTL(time.Minute)

	return result, nil
}

func validateUsername(config_obj *config_proto.Config, name string) error {
	if !validUsernameRegEx.MatchString(name) {
		return fmt.Errorf("Unacceptable username %v", name)
	}

	if config_obj.API != nil &&
		config_obj.API.PinnedGwName == name {
		return fmt.Errorf("Username is reserved: %v", name)
	}

	if config_obj.Client != nil &&
		config_obj.Client.PinnedServerName == name {
		return fmt.Errorf("Username is reserved: %v", name)
	}

	if name == constants.PinnedGwName || name == constants.PinnedServerName {
		return fmt.Errorf("Username is reserved: %v", name)
	}

	return nil
}
