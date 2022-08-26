package server

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"sync"
	"time"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto/client"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/crypto/server"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/logging"
)

type ServerCryptoManager struct {
	*server.ServerCryptoManager
}

func NewServerCryptoManager(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) (*ServerCryptoManager, error) {
	if config_obj.Frontend == nil {
		return nil, errors.New("No frontend config")
	}

	cert, err := crypto_utils.ParseX509CertFromPemStr(
		[]byte(config_obj.Frontend.Certificate))
	if err != nil {
		return nil, err
	}

	resolver, err := NewServerPublicKeyResolver(ctx, config_obj, wg)
	if err != nil {
		return nil, err
	}

	base, err := client.NewCryptoManager(config_obj,
		crypto_utils.GetSubjectName(cert),
		[]byte(config_obj.Frontend.PrivateKey), resolver,
		logging.GetLogger(config_obj, &logging.FrontendComponent))
	if err != nil {
		return nil, err
	}

	server_manager := &ServerCryptoManager{&server.ServerCryptoManager{base}}
	return server_manager, nil
}

type serverPublicKeyResolver struct{}

func (self *serverPublicKeyResolver) GetPublicKey(
	config_obj *config_proto.Config,
	client_id string) (*rsa.PublicKey, bool) {

	record, err := cvelo_services.GetElasticRecord(
		context.Background(), config_obj.OrgId,
		"client_key", client_id)
	if err != nil {
		return nil, false
	}

	pem := &crypto_proto.PublicKey{}
	err = json.Unmarshal(record, &pem)
	if err != nil {
		return nil, false
	}

	key, err := crypto_utils.PemToPublicKey(pem.Pem)
	if err != nil {
		return nil, false
	}

	return key, true
}

func (self *serverPublicKeyResolver) SetPublicKey(
	config_obj *config_proto.Config,
	client_id string, key *rsa.PublicKey) error {

	pem := &crypto_proto.PublicKey{
		Pem:        crypto_utils.PublicKeyToPem(key),
		EnrollTime: uint64(time.Now().Unix()),
	}
	return cvelo_services.SetElasticIndex(
		config_obj.OrgId, "client_key", client_id, pem)
}

func (self *serverPublicKeyResolver) Clear() {}

func NewServerPublicKeyResolver(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) (client.PublicKeyResolver, error) {
	result := &serverPublicKeyResolver{}

	return result, nil
}
