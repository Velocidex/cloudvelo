package server

import (
	"context"
	"fmt"

	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type DummyBackend struct{}

func (self DummyBackend) Send(messages []*crypto_proto.VeloMessage) error {
	fmt.Printf("Hello, world from send!")
	return nil
}

func (self DummyBackend) Receive(
	ctx context.Context, client_id, org_id string) (
	message []*crypto_proto.VeloMessage, err error) {
	fmt.Printf("Hello, world from receive!")
	return []*crypto_proto.VeloMessage{}, nil
}
