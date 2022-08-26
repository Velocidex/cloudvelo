package ingestion

import (
	"bufio"
	"context"
	"strings"
	"time"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
)

type ClientInfoUpdate struct {
	Name         string
	BuildTime    string
	Labels       []string
	Hostname     string
	OS           string
	Architecture string
	Platform     string
	MACAddresses []string
}

// Register a new client - update the client record and update it's
// client event table.
func (self ElasticIngestor) HandleClientInfoUpdates(
	ctx context.Context,
	message *crypto_proto.VeloMessage) error {
	if message == nil || message.VQLResponse == nil {
		return nil
	}

	reader := strings.NewReader(message.VQLResponse.JSONLResponse)
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, len(message.VQLResponse.JSONLResponse))
	scanner.Buffer(buf, len(message.VQLResponse.JSONLResponse))

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	org_config_obj, err := org_manager.GetOrgConfig(message.OrgId)
	if err != nil {
		return err
	}

	client_info_manager, err := services.GetClientInfoManager(org_config_obj)
	if err != nil {
		return err
	}

	for scanner.Scan() {
		serialized := scanner.Text()
		row := &ClientInfoUpdate{}
		err := json.Unmarshal([]byte(serialized), row)
		if err != nil {
			return err
		}

		old_client_info, err := client_info_manager.Get(ctx, message.Source)
		if err != nil {
			old_client_info = &services.ClientInfo{actions_proto.ClientInfo{
				Labels:       []string{},
				MacAddresses: []string{},
			}}
		}

		err = client_info_manager.Set(ctx,
			&services.ClientInfo{actions_proto.ClientInfo{
				ClientId:              message.Source,
				Hostname:              row.Hostname,
				Fqdn:                  row.Hostname,
				System:                row.OS,
				Architecture:          row.Architecture,
				Ping:                  uint64(time.Now().Unix()),
				MacAddresses:          row.MACAddresses,
				FirstSeenAt:           old_client_info.FirstSeenAt,
				Labels:                old_client_info.Labels,
				LastHuntTimestamp:     old_client_info.LastHuntTimestamp,
				LastEventTableVersion: old_client_info.LastEventTableVersion,
			}})
		if err != nil {
			return err
		}
	}

	return nil
}
