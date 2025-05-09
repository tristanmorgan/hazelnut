package mapcache

import (
	"github.com/perbu/hazelnut/cache"
	"sync"
	"time"
)

type MAPCache struct {
	mu    sync.RWMutex
	cache map[string]cache.ObjCore
}

func New() *MAPCache {
	return &MAPCache{
		cache: make(map[string]cache.ObjCore),
	}
}

func (s *MAPCache) Get(key string) (cache.ObjCore, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, found := s.cache[key]
	if !found {
		return cache.ObjCore{}, false
	}
	return value, true
}

// Set adds an object to the cache with automatic TTL calculation based on response headers
func (s *MAPCache) Set(key string, value cache.ObjCore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[key] = value
}

// SetWithTTL explicitly sets an object in the cache with a specific TTL
func (s *MAPCache) SetWithTTL(key string, value cache.ObjCore, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[key] = value
}
