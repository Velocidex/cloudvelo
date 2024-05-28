package users

import (
	"context"
	"regexp"
	"sync"

	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
)

var (
	validUsernameRegEx = regexp.MustCompile("^[a-zA-Z0-9@.\\-_#+]+$")
)

// The record stored in the elastic index
type UserRecord struct {
	Username string `json:"username"`
	Record   string `json:"record"` // An encoded api_proto.VelociraptorUser
	DocType  string `json:"doc_type"`
}

type UserGUIOptions struct {
	Username   string `json:"username"`
	GUIOptions string `json:"gui_options"` // An endoded api_proto.SetGUIOptionsRequest
	DocType    string `json:"doc_type"`
}

func StartUserManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config.Config) error {

	storage, err := NewUserStorageManager(ctx, wg, config_obj)
	if err != nil {
		return err
	}

	service := users.NewUserManager(config_obj.VeloConf(), storage)
	services.RegisterUserManager(service)

	return nil
}
