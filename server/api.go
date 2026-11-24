package server

import (
	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/crypto/server"
	"www.velocidex.com/golang/cloudvelo/filestore"
)

func NewCommunicator(
	config_obj *config.Config,
	crypto_manager *server.ServerCryptoManager,
	backend CommunicatorBackend) (*Communicator, error) {

	client, err := filestore.GetS3Client(config_obj)
	return &Communicator{
		s3_client:      client,
		config_obj:     config_obj,
		backend:        backend,
		crypto_manager: crypto_manager,
	}, err
}
