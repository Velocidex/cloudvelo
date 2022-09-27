package client_info

import (
	"context"
	"errors"
	"strings"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	NotFoundError = errors.New("Not found")
)

type ClientInfo struct {
	ClientId              string   `json:"client_id"`
	Hostname              string   `json:"hostname"`
	System                string   `json:"system"`
	FirstSeenAt           uint64   `json:"first_seen_at"`
	Ping                  uint64   `json:"ping"`
	Labels                []string `json:"labels"`
	LowerLabels           []string `json:"lower_labels"`
	MacAddresses          []string `json:"mac_addresses"`
	LastHuntTimestamp     uint64   `json:"last_hunt_timestamp"`
	LastEventTableVersion uint64   `json:"last_event_table_version"`
	LastLabelTimestamp    uint64   `json:"labels_timestamp"`
	AssignedHunts         []string `json:"assigned_hunts"`
}

type ClientInfoManager struct {
	config_obj *config_proto.Config
}

func (self *ClientInfoManager) Set(
	ctx context.Context, client_info *services.ClientInfo) error {
	lower_labels := make([]string, 0, len(client_info.Labels))
	for _, label := range client_info.Labels {
		lower_labels = append(lower_labels, strings.ToLower(label))
	}

	return cvelo_services.SetElasticIndex(ctx,
		self.config_obj.OrgId,
		"clients", client_info.ClientId, &ClientInfo{
			ClientId:              client_info.ClientId,
			Hostname:              client_info.Hostname,
			System:                client_info.System,
			FirstSeenAt:           client_info.FirstSeenAt,
			Ping:                  client_info.Ping,
			Labels:                client_info.Labels,
			LowerLabels:           lower_labels,
			MacAddresses:          client_info.MacAddresses,
			LastHuntTimestamp:     client_info.LastHuntTimestamp,
			LastEventTableVersion: client_info.LastEventTableVersion,
			LastLabelTimestamp:    client_info.LabelsTimestamp,
			AssignedHunts:         []string{},
		})
}

func (self ClientInfoManager) Remove(
	ctx context.Context, client_id string) {
	cvelo_services.DeleteDocument(ctx, self.config_obj.OrgId,
		"clients", client_id, true)
}

// Get a single entry from a client id
func (self ClientInfoManager) Get(
	ctx context.Context, client_id string) (
	*services.ClientInfo, error) {
	hit, err := cvelo_services.GetElasticRecord(
		context.Background(), self.config_obj.OrgId,
		"clients", client_id)
	if err != nil {
		return nil, err
	}

	result := actions_proto.ClientInfo{}
	err = json.Unmarshal(hit, &result)
	if err != nil {
		return nil, err
	}

	// Ping times in Velociraptor are in microseconds
	result.Ping /= 1000
	result.FirstSeenAt /= 1000

	return &services.ClientInfo{result}, nil
}

func (self ClientInfoManager) GetStats(
	ctx context.Context, client_id string) (*services.Stats, error) {
	return nil, errors.New("ClientInfoManager.GetStats Not implemented")
}

func (self ClientInfoManager) UpdateStats(
	ctx context.Context, client_id string, stats *services.Stats) error {
	return errors.New("ClientInfoManager.UpdateStats Not implemented")
}

func (self ClientInfoManager) QueueMessagesForClient(
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

func (self ClientInfoManager) QueueMessageForMultipleClients(
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

func (self ClientInfoManager) UnQueueMessageForClient(
	ctx context.Context,
	client_id string, req *crypto_proto.VeloMessage) error {
	return nil
}

func (self ClientInfoManager) Flush(ctx context.Context, client_id string) {
}

func NewClientInfoManager(config_obj *config_proto.Config) (*ClientInfoManager, error) {

	service := &ClientInfoManager{
		config_obj: config_obj,
	}
	return service, nil
}
