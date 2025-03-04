package cache

import (
	"github.com/dgraph-io/ristretto/v2"
	"net/http"
)

type Store struct {
	cache *ristretto.Cache[[]byte, ObjCore]
}

type ObjCore struct {
	Headers http.Header
	Body    []byte
}

func New(maxObj, maxSize int64) (*Store, error) {
	config := &ristretto.Config[[]byte, ObjCore]{
		// A rule-of-thumb is to set NumCounters to 10× the capacity.
		NumCounters: maxObj * 10,
		// MaxCost is the total cost allowed in the cache.
		MaxCost: maxSize,
		// BufferItems should be a power-of-two, a common choice is 64.
		BufferItems: 64,
		// Cost function: here we use the length of the Body as the cost.
		// You could customize this further if needed.
		Cost: func(value ObjCore) int64 {
			return int64(len(value.Body))
		},
		// You can set TtlTickerDurationInSec if needed.
	}

	// Create the ristretto cache using generics.
	rCache, err := ristretto.NewCache(config)
	if err != nil {
		return nil, err
	}

	return &Store{
		cache: rCache,
	}, nil
}

func (s *Store) Get(key []byte) (ObjCore, bool) {
	value, found := s.cache.Get(key)
	if !found {
		return ObjCore{}, false
	}
	return value, true
}

func (s *Store) Set(key []byte, value ObjCore) {
	s.cache.Set(key, value, int64(len(value.Body)))
}
