package client_info

import (
	"context"
	"errors"
	"regexp"

	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	NotFoundError = errors.New("Not found")
)

type ClientInfoManager struct {
	ClientInfoBase
	ClientInfoQueuer
	config_obj *config_proto.Config
}

type ClientInfoBase struct {
	config_obj *config_proto.Config
}

type ClientInfoQueuer struct {
	config_obj *config_proto.Config
}

var (
	client_id_regex              = regexp.MustCompile("(?i)^(C\\.[a-z0-9]+|server)")
	client_id_not_provided_error = errors.New("ClientId not provided")
	client_id_not_valid_error    = errors.New("ClientId is not valid")
)

func (self *ClientInfoBase) ValidateClientId(client_id string) error {
	if client_id == "" {
		return client_id_not_provided_error
	}
	if !client_id_regex.MatchString(client_id) {
		return client_id_not_valid_error
	}
	return nil
}

// TODO: This implementation is a susceptible to a race but it does not
// matter because the cloud does not have server events so client
// records are not updated that much.
func (self *ClientInfoBase) Modify(ctx context.Context, client_id string,
	modifier func(client_info *services.ClientInfo) (new_record *services.ClientInfo, err error)) error {
	record, err := self.Get(ctx, client_id)
	if err != nil {
		return err
	}

	new_record, err := modifier(record)
	if err != nil {
		return err
	}

	// Callback can indicate no change is needed by returning a nil
	// for client_info.
	if new_record == nil {
		return nil
	}

	return self.Set(ctx, new_record)
}

func (self *ClientInfoBase) ListClients(ctx context.Context) <-chan string {
	output_chan := make(chan string)

	go func() {
		defer close(output_chan)

		indexer, err := services.GetIndexer(self.config_obj)
		if err != nil {
			return
		}

		scope := vql_subsystem.MakeScope()
		record_chan, err := indexer.SearchClientsChan(ctx, scope, self.config_obj, "all", "")
		if err != nil {
			return
		}

		for record := range record_chan {
			output_chan <- record.ClientId
		}
	}()

	return output_chan
}

func (self *ClientInfoBase) Set(
	ctx context.Context, client_info *services.ClientInfo) error {
	now := utils.GetTime().Now().Unix()

	return cvelo_services.SetElasticIndex(ctx,
		self.config_obj.OrgId,
		"persisted", client_info.ClientId,
		&api.ClientRecord{
			ClientId:              client_info.ClientId,
			Hostname:              client_info.Hostname,
			System:                client_info.System,
			FirstSeenAt:           client_info.FirstSeenAt,
			Type:                  "main",
			MacAddresses:          client_info.MacAddresses,
			LastHuntTimestamp:     client_info.LastHuntTimestamp,
			LastEventTableVersion: client_info.LastEventTableVersion,
			DocType:               "clients",
			Timestamp:             uint64(now),
		})
}

func (self ClientInfoBase) Remove(
	ctx context.Context, client_id string) {
	for _, suffix := range []string{"", "_ping", "_hunts", "_labels"} {
		cvelo_services.DeleteDocument(ctx, self.config_obj.OrgId,
			"persisted", client_id+suffix, true)
	}
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

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Debug("QueueMessageForMultipleClients: Queue message for %v clients",
		len(client_ids))

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
		config_obj:       config_obj,
	}
	return service, nil
}
