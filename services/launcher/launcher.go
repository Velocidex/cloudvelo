package launcher

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"www.velocidex.com/golang/cloudvelo/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
)

type Launcher struct {
	services.Launcher
	config_obj *config_proto.Config
}

func NewLauncherService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	cloud_config *config.ElasticConfiguration) (services.Launcher, error) {

	lru := ttlcache.NewCache()
	lru.SetCacheSizeLimit(20000)
	lru.SkipTTLExtensionOnHit(true)
	lru.SetTTL(5 * time.Minute)

	go func() {
		<-ctx.Done()
		lru.Close()
	}()

	launcher_service := &launcher.Launcher{
		Storage_: &FlowStorageManager{
			cache:        lru,
			cloud_config: cloud_config,
		},
	}

	return &Launcher{
		Launcher:   launcher_service,
		config_obj: config_obj,
	}, nil
}
