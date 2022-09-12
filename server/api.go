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

	sess, err := filestore.GetS3Session(config_obj)
	return &Communicator{
		session:        sess,
		config_obj:     config_obj,
		backend:        backend,
		crypto_manager: crypto_manager,
	}, err
}
