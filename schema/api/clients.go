package api

import (
	"context"
	"regexp"

	"github.com/Velocidex/ordereddict"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
)

// This is the record stored in the index. It is similar to
// actions_proto.ClientInfo but contains a couple of extra fields we
// use for better searching.
type ClientRecord struct {
	// Stored in '<client id>'
	ClientId        string `json:"client_id,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	System          string `json:"system,omitempty"`
	FirstSeenAt     uint64 `json:"first_seen_at,omitempty"`
	LastInterrogate string `json:"last_interrogate,omitempty"`

	// Stored in '<client id>_ping'
	Ping uint64 `json:"ping,omitempty"`

	MacAddresses []string `json:"mac_addresses,omitempty"`

	// Stored in '<client id>_hunts'
	AssignedHunts         []string `json:"assigned_hunts,omitempty"`
	LastHuntTimestamp     uint64   `json:"last_hunt_timestamp,omitempty"`
	LastEventTableVersion uint64   `json:"last_event_table_version,omitempty"`

	// Additional fields in the index we use for search

	// "main" for primary key, "ping" for ping, "label" for label.
	Type string `json:"type"`

	// Stored in '<client id>_labels'
	LastLabelTimestamp uint64   `json:"labels_timestamp,omitempty"`
	Labels             []string `json:"labels,omitempty"`
	LowerLabels        []string `json:"lower_labels,omitempty"`
}

func ToClientInfo(record *ClientRecord) *services.ClientInfo {
	return &services.ClientInfo{
		actions_proto.ClientInfo{
			ClientId:              record.ClientId,
			Hostname:              record.Hostname,
			System:                record.System,
			FirstSeenAt:           record.FirstSeenAt,
			Ping:                  record.Ping,
			Labels:                record.Labels,
			MacAddresses:          record.MacAddresses,
			LastHuntTimestamp:     record.LastHuntTimestamp,
			LastEventTableVersion: record.LastEventTableVersion,
			LastInterrogateFlowId: record.LastInterrogate,
		},
	}
}

func GetMultipleClients(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_ids []string) ([]*ClientRecord, error) {

	terms := make([]string, 0, 2*len(client_ids))
	for _, i := range client_ids {
		terms = append(terms, i)
		terms = append(terms, i+"_ping")
		terms = append(terms, i+"_labels")
		terms = append(terms, i+"_hunts")
	}

	hits, err := cvelo_services.GetMultipleElasticRecords(
		ctx, config_obj.OrgId, "clients", terms)
	if err != nil {
		return nil, err
	}

	// Preserve the lookup order
	lookup := ordereddict.NewDict() // make(map[string]*ClientRecord)
	for _, hit := range hits {
		record := &ClientRecord{}
		err = json.Unmarshal(hit, record)
		if err != nil || record.ClientId == "" {
			continue
		}

		first_any, pres := lookup.Get(record.ClientId)
		if !pres {
			lookup.Set(record.ClientId, record)
			continue
		}

		first, ok := first_any.(*ClientRecord)
		if ok {
			mergeClientRecords(first, record)
		}
	}

	result := make([]*ClientRecord, 0, len(client_ids))
	for _, k := range lookup.Keys() {
		v_any, _ := lookup.Get(k)
		v, ok := v_any.(*ClientRecord)
		if ok {
			result = append(result, v)
		}
	}

	return result, nil
}

func mergeClientRecords(first *ClientRecord, second *ClientRecord) {

	if second.Hostname != "" {
		first.Hostname = second.Hostname
	}

	if second.System != "" {
		first.System = second.System
	}

	if second.FirstSeenAt != 0 {
		first.FirstSeenAt = second.FirstSeenAt
	}

	if second.LastInterrogate != "" {
		first.LastInterrogate = second.LastInterrogate
	}

	if second.Ping != 0 {
		first.Ping = second.Ping
	}

	if len(second.Labels) > 0 {
		first.Labels = append(first.Labels, second.Labels...)
		first.LowerLabels = append(first.LowerLabels, second.LowerLabels...)
	}

	if len(second.MacAddresses) > 0 {
		first.MacAddresses = append(first.MacAddresses, second.MacAddresses...)
	}

	if second.LastHuntTimestamp > 0 {
		first.LastHuntTimestamp = second.LastHuntTimestamp
	}

	if second.LastEventTableVersion > 0 {
		first.LastEventTableVersion = second.LastEventTableVersion
	}

	if len(second.AssignedHunts) > 0 {
		first.AssignedHunts = append(first.AssignedHunts, second.AssignedHunts...)
	}

	if second.LastLabelTimestamp > 0 {
		first.LastLabelTimestamp = second.LastLabelTimestamp
	}
}

var doc_id_regex = regexp.MustCompile("(.+)_(labels|ping)")

func GetClientIdFromDocId(doc_id string) string {
	m := doc_id_regex.FindStringSubmatch(doc_id)
	if len(m) > 1 {
		return m[1]
	}
	return doc_id
}
