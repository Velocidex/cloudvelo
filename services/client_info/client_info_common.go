package client_info

import (
	"context"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"www.velocidex.com/golang/cloudvelo/services"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

var (
	// Query to retrieve all the task queued for a client.
	getClientTasksQuery = `{
  "sort": [{
    "timestamp": {"order": "asc", "unmapped_type" : "long"}
  }],
  "query": {
    "bool": {
      "must": [
 		 {"match": {"doc_type" : "task"}},
         {"match": {"client_id" : %q}}
      ]}
  }
}
`
)

func (self ClientInfoQueuer) QueueMessageForClient(
	ctx context.Context, client_id string,
	req *crypto_proto.VeloMessage,
	notify bool, completion func()) error {

	serialized, err := protojson.Marshal(req)
	if err != nil {
		return err
	}

	// This is problematic because there is no way to remove these
	// from persisted storage.
	return cvelo_services.SetElasticIndex(ctx,
		self.config_obj.OrgId,
		"persisted", services.DocIdRandom,
		&ClientTask{
			ClientId:  client_id,
			FlowId:    req.SessionId,
			Timestamp: time.Now().UnixNano(),
			JSONData:  string(serialized),
			DocType:   "task",
		})
}

type ClientTask struct {
	ClientId  string `json:"client_id"`
	FlowId    string `json:"flow_id"`
	Timestamp int64  `json:"timestamp"`
	JSONData  string `json:"data"`
	DocType   string `json:"doc_type"`
}

// Get the client's tasks and remove them from the queue.
func (self ClientInfoBase) GetClientTasks(
	ctx context.Context, client_id string) ([]*crypto_proto.VeloMessage, error) {

	query := json.Format(getClientTasksQuery, client_id)
	hits, err := cvelo_services.QueryElastic(ctx, self.config_obj.OrgId,
		"persisted", query)
	if err != nil {
		return nil, err
	}

	results := []*crypto_proto.VeloMessage{}
	for _, hit := range hits {
		err = cvelo_services.DeleteDocument(ctx,
			self.config_obj.OrgId,
			"persisted", hit.Id, cvelo_services.NoSync)
		if err != nil {
			return nil, err
		}

		item := &ClientTask{}
		err = json.Unmarshal(hit.JSON, item)
		if err != nil {
			continue
		}

		message := &crypto_proto.VeloMessage{}
		err = protojson.Unmarshal([]byte(item.JSONData), message)
		if err != nil {
			continue
		}
		results = append(results, message)
	}
	return results, nil
}

// Get the client's tasks and remove them from the queue.
func (self ClientInfoBase) PeekClientTasks(
	ctx context.Context, client_id string) ([]*crypto_proto.VeloMessage, error) {

	query := json.Format(getClientTasksQuery, client_id)
	hits, err := cvelo_services.QueryElastic(ctx, self.config_obj.OrgId,
		"persisted", query)
	if err != nil {
		return nil, err
	}

	results := []*crypto_proto.VeloMessage{}
	for _, hit := range hits {
		item := &ClientTask{}
		err = json.Unmarshal(hit.JSON, item)
		if err != nil {
			continue
		}

		message := &crypto_proto.VeloMessage{}
		err = protojson.Unmarshal([]byte(item.JSONData), message)
		if err != nil {
			continue
		}
		results = append(results, message)
	}
	return results, nil
}
