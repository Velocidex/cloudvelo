package services

import (
	"context"

	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

// Can send same message to multiple clients efficiently
type MultiClientMessageQueuer interface {
	QueueMessageForMultipleClients(
		ctx context.Context,
		client_ids []string,
		req *crypto_proto.VeloMessage,
		notify bool) error
}
