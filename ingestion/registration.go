package ingestion

import (
	"bufio"
	"context"
	"strings"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
)

type ClientInfoUpdate struct {
	ClientId      string `json:"client_id"`
	Hostname      string `json:"hostname"`
	Release       string `json:"release"`
	Architecture  string `json:"architecture"`
	ClientVersion string `json:"client_version"`
	BuildTime     string `json:"build_time"`
	System        string `json:"system"`
	InstallTime   uint64 `json:"install_time"`
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
		err = client_info_manager.Set(ctx,
			&services.ClientInfo{actions_proto.ClientInfo{
				ClientId:      message.Source,
				Hostname:      row.Hostname,
				Fqdn:          row.Hostname,
				ClientVersion: row.ClientVersion,
				BuildTime:     row.BuildTime,
				System:        row.System,
				Architecture:  row.Architecture,
				FirstSeenAt:   row.InstallTime,
			}})
		if err != nil {
			return err
		}
	}

	return nil
}
