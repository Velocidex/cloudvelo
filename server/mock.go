package server

import (
	"context"
	"fmt"
	"io"

	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

type MockBackend struct {
	Tasks  [][]*crypto_proto.VeloMessage
	idx    int
	Logger *logging.LogContext
}

func (self MockBackend) Send(messages []*crypto_proto.VeloMessage) error {
	fmt.Printf("Sending to backend %v\n", messages)
	return nil
}

func (self MockBackend) Receive(ctx context.Context, client_id, org_id string) (
	message []*crypto_proto.VeloMessage, err error) {
	if self.idx > len(self.Tasks)-1 {
		return nil, io.EOF
	}

	res := self.Tasks[self.idx]
	self.idx++
	self.Logger.Info(
		"Serving a mock response with %v messages", len(res))

	return res, nil
}
