package api

import (
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

// The source of truth for this record is
// flows_proto.ArtifactCollectorContext but we extract some of the
// fields into the Elastic schema so they can be searched on.

// We use the database to manipulate exposed fields.
type ArtifactCollectorRecord struct {
	ClientId  string `json:"client_id,omitempty"`
	SessionId string `json:"session_id,omitempty"`
	Raw       string `json:"context,omitempty"`
	Tasks     string `json:"tasks,omitempty"`
	Type      string `json:"type,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

func (self *ArtifactCollectorRecord) ToProto() (
	*flows_proto.ArtifactCollectorContext, error) {

	result := &flows_proto.ArtifactCollectorContext{}
	err := json.Unmarshal([]byte(self.Raw), result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func ArtifactCollectorRecordFromProto(
	in *flows_proto.ArtifactCollectorContext) *ArtifactCollectorRecord {
	self := &ArtifactCollectorRecord{}
	self.ClientId = in.ClientId
	self.SessionId = in.SessionId
	self.Raw = json.MustMarshalString(in)

	return self
}
