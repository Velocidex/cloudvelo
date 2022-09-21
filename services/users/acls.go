package users

import (
	"context"
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"github.com/pkg/errors"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ACLRecord struct {
	ACL string `json:"acl"`
}

type ACLManager struct {
	*acls.ACLManager

	ctx context.Context

	lru *ttlcache.Cache
}

func NewACLManager(ctx context.Context) *ACLManager {
	acl_manager := &ACLManager{
		ACLManager: &acls.ACLManager{},
		ctx:        ctx,
		lru:        ttlcache.NewCache(),
	}
	acl_manager.lru.SetTTL(10 * time.Second)
	return acl_manager
}

func (self ACLManager) GetPolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {

	permissions_any, err := self.lru.Get(principal)
	if err == nil {
		permissions, ok := permissions_any.(*acl_proto.ApiClientACL)
		if ok {
			return permissions, nil
		}
	}

	hit, err := cvelo_services.GetElasticRecord(
		context.Background(), config_obj.OrgId,
		"acls", principal)
	if err != nil {
		return nil, err
	}

	record := &ACLRecord{}
	err = json.Unmarshal(hit, &record)
	if err != nil {
		return nil, err
	}

	permissions := &acl_proto.ApiClientACL{}
	err = json.Unmarshal([]byte(record.ACL), &permissions)
	if err != nil {
		return nil, err
	}

	self.lru.Set(principal, permissions)
	return permissions, err
}

func (self ACLManager) GetEffectivePolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {
	permissions, err := self.GetPolicy(config_obj, principal)
	if err != nil {
		return nil, err
	}
	err = acls.GetRolePermissions(config_obj, permissions.Roles, permissions)
	return permissions, err
}

func (self ACLManager) SetPolicy(
	config_obj *config_proto.Config,
	principal string, acl_obj *acl_proto.ApiClientACL) error {

	self.lru.Set(principal, acl_obj)
	return cvelo_services.SetElasticIndex(self.ctx,
		config_obj.OrgId,
		"acls", principal, &ACLRecord{
			ACL: json.MustMarshalString(acl_obj),
		})
}

func (self ACLManager) CheckAccess(
	config_obj *config_proto.Config,
	principal string,
	permissions ...acls.ACL_PERMISSION) (bool, error) {

	// Internal calls from the server are allowed to do anything.
	if config_obj.Client != nil && principal == config_obj.Client.PinnedServerName {
		return true, nil
	}

	if principal == "" {
		return false, nil
	}

	acl_obj, err := self.GetEffectivePolicy(config_obj, principal)
	if err != nil {
		return false, err
	}

	for _, permission := range permissions {
		ok, err := self.CheckAccessWithToken(acl_obj, permission)
		if !ok || err != nil {
			return ok, err
		}
	}

	return true, nil
}

func (self ACLManager) GrantRoles(
	config_obj *config_proto.Config,
	principal string,
	roles []string) error {
	new_policy := &acl_proto.ApiClientACL{}

	for _, role := range roles {
		if !utils.InString(new_policy.Roles, role) {
			if !acls.ValidateRole(role) {
				return errors.Errorf("Invalid role %v", role)
			}
			new_policy.Roles = append(new_policy.Roles, role)
		}
	}
	return self.SetPolicy(config_obj, principal, new_policy)
}
