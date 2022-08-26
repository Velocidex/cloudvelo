package server

import (
	"www.velocidex.com/golang/cloudvelo/crypto/server"
	"www.velocidex.com/golang/cloudvelo/filestore"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func NewCommunicator(
	config_obj *config_proto.Config,
	elastic_config_path string,
	crypto_manager *server.ServerCryptoManager,
	backend CommunicatorBackend) (*Communicator, error) {

	elastic_config, sess, err := filestore.GetS3Session(elastic_config_path)
	return &Communicator{
		session:        sess,
		elastic_config: elastic_config,
		config_obj:     config_obj,
		backend:        backend,
		crypto_manager: crypto_manager,
	}, err
}
