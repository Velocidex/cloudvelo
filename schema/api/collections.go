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
	ClientId  string `json:"client_id"`
	SessionId string `json:"session_id"`
	Raw       string `json:"context,omitempty"`
	Tasks     string `json:"tasks,omitempty"`
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
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

func GetDocumentIdForCollection(session_id, client_id, doc_type string) string {
	if doc_type != "" {
		return client_id + "_" + session_id + "_" + doc_type
	}
	return client_id + "_" + session_id
}
