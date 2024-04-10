package indexing

import (
	"time"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// Keeps track of users->client id used so we can populate the MRU
// search
type MRUItem struct {
	Username  string `json:"username"`
	ClientId  string `json:"client_id"`
	Timestamp int64  `json:"timestamp"`
	DocType   string `json:"doc_type"`
}

func (self Indexer) UpdateMRU(
	config_obj *config_proto.Config,
	user_name string, client_id string) error {
	return cvelo_services.SetElasticIndex(self.ctx,
		self.config_obj.OrgId,
		"persisted", user_name+":"+client_id,
		&MRUItem{
			Username:  user_name,
			ClientId:  client_id,
			Timestamp: time.Now().Unix(),
			DocType:   "user_mru",
		})
}
