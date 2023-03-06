package client_info

import (
	"context"
	"errors"

	"github.com/Velocidex/ordereddict"
)

func (self ClientInfoManager) GetMetadata(ctx context.Context,
	client_id string) (*ordereddict.Dict, error) {
	return nil, errors.New("Not implemented")
}

func (self ClientInfoManager) SetMetadata(ctx context.Context,
	client_id string, metadata *ordereddict.Dict, principal string) error {
	return errors.New("Not implemented")
}
