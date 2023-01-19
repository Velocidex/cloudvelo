package ingestion

import (
	"bufio"
	"context"
	"strings"

	"www.velocidex.com/golang/cloudvelo/utils"
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
// client event table. NOTE: This happens automatically every time the
// client starts up so we get to refresh the record each time. This
// way there is no need to run an interrogation flow specifically - it
// just happens automatically.
func (self Ingestor) HandleClientInfoUpdates(
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

		// TODO: This should be an update operation but it does not
		// happen too often (only when client is started) and we do
		// not need to lock the record so it might be ok for now.
		old_client_info, err := client_info_manager.Get(ctx, message.Source)
		if err != nil {
			old_client_info = &services.ClientInfo{actions_proto.ClientInfo{
				Labels:       []string{},
				MacAddresses: []string{},
			}}
		}

		first_seen_at := old_client_info.FirstSeenAt
		if first_seen_at == 0 {
			first_seen_at = uint64(utils.Clock.Now().UnixNano())
		}

		err = client_info_manager.Set(ctx,
			&services.ClientInfo{actions_proto.ClientInfo{
				ClientId:              message.Source,
				Hostname:              row.Hostname,
				Fqdn:                  row.Hostname,
				System:                row.OS,
				Architecture:          row.Architecture,
				MacAddresses:          row.MACAddresses,
				FirstSeenAt:           first_seen_at,
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
