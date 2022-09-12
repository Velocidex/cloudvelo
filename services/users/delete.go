package users

import (
	"context"
	"errors"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func (self *UserManager) DeleteUser(
	ctx context.Context, org_config_obj *config_proto.Config, username string) error {
	return errors.New("UserManager.DeleteUser not implemented")
}
