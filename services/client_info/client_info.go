package client_info

import (
	"context"
	"errors"

	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	NotFoundError = errors.New("Not found")
)

type ClientInfoManager struct {
	ClientInfoBase
	ClientInfoQueuer
}

type ClientInfoBase struct {
	config_obj *config_proto.Config
}

type ClientInfoQueuer struct {
	config_obj *config_proto.Config
}

func (self *ClientInfoBase) Set(
	ctx context.Context, client_info *services.ClientInfo) error {

	return cvelo_services.SetElasticIndex(ctx,
		self.config_obj.OrgId,
		"clients", client_info.ClientId, &api.ClientRecord{
			ClientId:              client_info.ClientId,
			Hostname:              client_info.Hostname,
			System:                client_info.System,
			FirstSeenAt:           client_info.FirstSeenAt,
			Type:                  "main",
			MacAddresses:          client_info.MacAddresses,
			LastHuntTimestamp:     client_info.LastHuntTimestamp,
			LastEventTableVersion: client_info.LastEventTableVersion,
		})
}

func (self ClientInfoBase) Remove(
	ctx context.Context, client_id string) {
	cvelo_services.DeleteDocument(ctx, self.config_obj.OrgId,
		"clients", client_id, true)
}

// Get a single entry from a client id
func (self ClientInfoBase) Get(
	ctx context.Context, client_id string) (
	*services.ClientInfo, error) {

	hits, err := api.GetMultipleClients(
		ctx, self.config_obj, []string{client_id})
	if err != nil {
		return nil, err
	}

	if len(hits) == 0 {
		return nil, errors.New("Client ID not found")
	}

	return api.ToClientInfo(hits[0]), nil
}

func (self ClientInfoBase) GetStats(
	ctx context.Context, client_id string) (*services.Stats, error) {
	return nil, errors.New("ClientInfoManager.GetStats Not implemented")
}

func (self ClientInfoBase) UpdateStats(
	ctx context.Context, client_id string, stats *services.Stats) error {
	return errors.New("ClientInfoManager.UpdateStats Not implemented")
}

func (self ClientInfoQueuer) QueueMessagesForClient(
	ctx context.Context,
	client_id string,
	req []*crypto_proto.VeloMessage,
	notify bool) error {
	for _, item := range req {
		err := self.QueueMessageForClient(ctx, client_id, item, notify, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self ClientInfoQueuer) QueueMessageForMultipleClients(
	ctx context.Context,
	client_ids []string,
	req *crypto_proto.VeloMessage,
	notify bool) error {
	for _, client_id := range client_ids {
		err := self.QueueMessageForClient(ctx, client_id, req, notify, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self ClientInfoQueuer) UnQueueMessageForClient(
	ctx context.Context,
	client_id string, req *crypto_proto.VeloMessage) error {
	return nil
}

func (self ClientInfoBase) Flush(ctx context.Context, client_id string) {
}

func NewClientInfoBase(config_obj *config_proto.Config) ClientInfoBase {
	return ClientInfoBase{config_obj: config_obj}
}

func NewClientInfoManager(config_obj *config_proto.Config) (*ClientInfoManager, error) {

	service := &ClientInfoManager{
		ClientInfoBase:   NewClientInfoBase(config_obj),
		ClientInfoQueuer: ClientInfoQueuer{config_obj: config_obj},
	}
	return service, nil
}
