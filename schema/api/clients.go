package api

import (
	"context"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
)

// This is the record stored in the index. It is similar to
// actions_proto.ClientInfo but contains a couple of extra fields we
// use for better searching.
type ClientInfo struct {
	ClientId              string   `json:"client_id"`
	Hostname              string   `json:"hostname"`
	System                string   `json:"system"`
	FirstSeenAt           uint64   `json:"first_seen_at"`
	Ping                  uint64   `json:"ping"`
	Labels                []string `json:"labels"`
	MacAddresses          []string `json:"mac_addresses"`
	LastHuntTimestamp     uint64   `json:"last_hunt_timestamp"`
	LastEventTableVersion uint64   `json:"last_event_table_version"`

	// Additional fields in the index we use for search

	// "primary" for primary key, "ping" for ping, "label" for label.
	Type               string   `json:"type"`
	LastLabelTimestamp uint64   `json:"labels_timestamp"`
	AssignedHunts      []string `json:"assigned_hunts"`
	LowerLabels        []string `json:"lower_labels"`
}

func GetMultipleClients(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_ids []string) ([]*services.ClientInfo, error) {

	terms := make([]string, 0, 2*len(client_ids))
	for _, i := range client_ids {
		terms = append(terms, i)
		terms = append(terms, i+"_ping")
		terms = append(terms, i+"_labels")
	}

	hits, err := cvelo_services.GetMultipleElasticRecords(
		ctx, config_obj.OrgId, "clients", terms)
	if err != nil {
		return nil, err
	}

	lookup := make(map[string]*services.ClientInfo)
	for _, hit := range hits {
		record := actions_proto.ClientInfo{}
		err = json.Unmarshal(hit, &record)
		if err != nil || record.ClientId == "" {
			continue
		}

		// Ping times in Velociraptor are in microseconds
		record.Ping /= 1000
		record.FirstSeenAt /= 1000

		first, pres := lookup[record.ClientId]
		if !pres {
			lookup[record.ClientId] = &services.ClientInfo{record}
			continue
		}

		mergeClientRecords(first, &record)
	}

	result := make([]*services.ClientInfo, 0, len(client_ids))
	for _, v := range lookup {
		result = append(result, v)
	}

	return result, nil
}

func mergeClientRecords(
	first *services.ClientInfo,
	second *actions_proto.ClientInfo) {

	if second.Hostname != "" {
		first.Hostname = second.Hostname
	}

	if second.System != "" {
		first.System = second.System
	}

	if second.FirstSeenAt != 0 {
		first.FirstSeenAt = second.FirstSeenAt
	}

	if second.Ping != 0 {
		first.Ping = second.Ping
	}

	if len(second.Labels) > 0 {
		first.Labels = append(first.Labels, second.Labels...)
	}

	if len(second.MacAddresses) > 0 {
		first.MacAddresses = append(first.MacAddresses, second.MacAddresses...)
	}

	if second.LastHuntTimestamp > 0 {
		first.LastHuntTimestamp = second.LastHuntTimestamp
		first.LastEventTableVersion = second.LastEventTableVersion
	}
}
