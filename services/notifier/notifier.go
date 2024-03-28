package notifier

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

// A do nothing notifier - we really can not actively notify clients
// at this stage.

type Nofitier struct {
	poll       time.Duration
	config_obj *config_proto.Config
}

func (self Nofitier) ListenForNotification(id string) (chan bool, func()) {
	output_chan := make(chan bool)

	logger := logging.GetLogger(self.config_obj, &logging.GUIComponent)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer close(output_chan)

		now := time.Now().Unix()

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(self.poll):
				serialized, err := cvelo_services.GetElasticRecord(
					ctx, self.config_obj.OrgId, "persisted", id)
				if err != nil {
					continue
				}

				notifiction_record := &api.NotificationRecord{}
				err = json.Unmarshal(serialized, &notifiction_record)
				if err != nil {
					logger.Error("ListenForNotification: %v", err)
					continue
				}

				// Notify the caller by closing the channel.
				if notifiction_record.Timestamp > now {
					return
				}
			}
		}
	}()

	return output_chan, cancel
}

func (self Nofitier) NotifyListener(
	ctx context.Context,
	config_obj *config_proto.Config, id, tag string) error {
	return cvelo_services.SetElasticIndex(
		context.Background(),
		self.config_obj.OrgId, "persisted",
		id, &api.NotificationRecord{
			Key:       id,
			Timestamp: time.Now().Unix(),
			DocType:   "tasks",
		})
}

// Notify a directly connected listener.
func (self Nofitier) NotifyDirectListener(id string) {}

func (self Nofitier) CountConnectedClients() uint64 {
	return 0
}

// Notify in the near future - no guarantee of delivery.
func (self Nofitier) NotifyListenerAsync(
	ctx context.Context,
	config_obj *config_proto.Config, id, tag string) {
}

// Check if there is someone listening for the specified id. This
// method queries all minion nodes to check if the client is
// connected anywhere - It may take up to 2 seconds to find out.
func (self Nofitier) IsClientConnected(ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, timeout int) bool {
	return false
}

// Returns a list of all clients directly connected at present.
func (self Nofitier) ListClients() []string {
	return nil
}

// Check only the current node if the client is connected.
func (self Nofitier) IsClientDirectlyConnected(client_id string) bool {
	return false
}

func NewNotificationService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (*Nofitier, error) {

	return &Nofitier{
		poll:       time.Second,
		config_obj: config_obj,
	}, nil
}
