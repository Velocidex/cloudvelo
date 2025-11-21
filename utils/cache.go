package utils

import (
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/utils"
)

type Cache struct {
	mu   sync.Mutex
	Dict *ordereddict.Dict

	timestamp time.Time
}

func (self *Cache) Get(key string) (
	value interface{}, pres bool, age time.Time) {

	// Cache is currently empty.
	if self.Dict == nil {
		return nil, false, time.Time{}
	}

	v, pres := self.Dict.Get(key)
	return v, pres, self.timestamp
}

func (self *Cache) Set(key string, value interface{}) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.Dict == nil {
		self.Dict = ordereddict.NewDict()
		self.timestamp = utils.GetTime().Now()

		// schedule a cleanup for a bit
		self.cleanup()
	}

	self.Dict.Set(key, value)
}

func (self *Cache) cleanup() {
	go func() {
		<-time.After(time.Minute)

		self.mu.Lock()
		defer self.mu.Unlock()

		// Free memory completely.
		self.Dict = nil
		self.timestamp = time.Time{}
	}()
}

func NewCache() *Cache {
	res := &Cache{}
	return res
}
