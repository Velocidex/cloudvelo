package services

import (
	"context"
	"errors"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

type HuntSearchOptions int

const (
	AllHunts HuntSearchOptions = iota

	// Only visit non expired hunts
	OnlyRunningHunts
)

// Add new methods that will be merged in Velociraptor 0.72 sync but
// for now they are separate.
type HuntDispatcherV2 interface {
	ApplyFuncOnHuntsWithOptions(ctx context.Context, options HuntSearchOptions,
		cb func(hunt *api_proto.Hunt) error) error
}

// TODO: Refactor when we merge with upstream.
func ApplyFuncOnHuntsWithOptions(
	dispatcher services.IHuntDispatcher,
	ctx context.Context, options HuntSearchOptions,
	cb func(hunt *api_proto.Hunt) error) error {

	v2, ok := dispatcher.(HuntDispatcherV2)
	if !ok {
		return errors.New("Hunt Dispatcher is not a V2")
	}

	return v2.ApplyFuncOnHuntsWithOptions(ctx, options, cb)
}
