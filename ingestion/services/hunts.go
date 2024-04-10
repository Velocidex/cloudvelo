package services

/*

  During a hunt we need to update the hunt statistics record very
  frequently. This causes a lot of traffic to the database and locking
  issues because many threads are trying to update the same hunt stats
  record.

  This service mediates updates to the hunt record by combining the
  stats from many updates into single mutations. The reduces lock
  contention on the same record and improves efficiency.

*/

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
)

var (
	HuntStatsManager *HuntStatsManager_t
)

type HuntStatsManager_t struct {
	config_obj *config_proto.Config
	lru        *ttlcache.Cache
}

func (self *HuntStatsManager_t) Update(hunt_id string) *HuntStatsUpdater {

	result_any, err := self.lru.Get(hunt_id)
	if err == nil {
		return result_any.(*HuntStatsUpdater)
	}

	result := &HuntStatsUpdater{
		config_obj: self.config_obj,
		hunt_id:    hunt_id,
	}
	self.lru.Set(hunt_id, result)

	return result
}

type HuntStatsUpdater struct {
	mu         sync.Mutex
	config_obj *config_proto.Config

	hunt_id string

	// Total hunts scheduled
	scheduled uint64

	// Total flows with errors
	errors uint64

	// Total collections completed
	completed uint64
}

func (self *HuntStatsUpdater) IncScheduled() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.scheduled++
}

func (self *HuntStatsUpdater) IncError() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.errors++
}

func (self *HuntStatsUpdater) IncCompleted() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.completed++
}

const (
	updatedPainlessQuery = `
ctx._source.scheduled += params.scheduled ;
ctx._source.completed += params.completed ;
ctx._source.errors += params.errors ;
`

	updateQuery = `
{
  "script" : {
    "source": %q,
    "lang": "painless",
    "params": {
      "scheduled": %q,
      "completed": %q,
      "errors": %q
    }
  }
}
`
)

func (self *HuntStatsUpdater) Flush(ctx context.Context) error {

	// Kepp the lock tight because elastic call can take a while.
	self.mu.Lock()
	query := json.Format(updateQuery,
		updatedPainlessQuery,
		self.scheduled,
		self.completed,
		self.errors)

	hunt_id := self.hunt_id

	self.scheduled = 0
	self.completed = 0
	self.errors = 0
	self.mu.Unlock()

	return services.UpdateIndex(
		ctx, self.config_obj.OrgId, "persisted", hunt_id, query)
}

func StartHuntStatsUpdater(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	HuntStatsManager = &HuntStatsManager_t{
		config_obj: config_obj,
		lru:        ttlcache.NewCache(),
	}

	// Flush the stat records every second
	HuntStatsManager.lru.SetTTL(time.Second)
	HuntStatsManager.lru.SetExpirationCallback(
		func(hunt_id string, value interface{}) {
			updater := value.(*HuntStatsUpdater)
			err := updater.Flush(ctx)
			if err != nil {
				logger.Error("HuntStatsUpdater: %v", err)
			}
		})

	// Handle shutdown
	wg.Add(1)
	go func() {
		defer wg.Done()

		<-ctx.Done()

		// Flush everything before shutdown.
		HuntStatsManager.lru.Purge()
	}()

	return nil
}
