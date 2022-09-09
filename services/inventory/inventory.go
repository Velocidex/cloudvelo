package inventory

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/utils"
)

func NewInventoryDummyService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.Inventory, error) {

	inventory_service := &inventory.Dummy{
		Clock: utils.RealClock{},
		Client: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   300 * time.Second,
					KeepAlive: 300 * time.Second,
					DualStack: true,
				}).DialContext,
				MaxIdleConns:          100,
				IdleConnTimeout:       300 * time.Second,
				TLSHandshakeTimeout:   100 * time.Second,
				ExpectContinueTimeout: 10 * time.Second,
				ResponseHeaderTimeout: 100 * time.Second,
			},
		},
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer inventory_service.Close(config_obj)

		<-ctx.Done()
	}()

	return inventory_service, nil
}
