package cache

import (
	"crypto/sha256"
	"github.com/dgraph-io/ristretto/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
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

// Set adds an object to the cache with automatic TTL calculation based on response headers
func (s *Store) Set(key []byte, value ObjCore) {
	ttl := calculateTTL(value.Headers)
	if ttl == 0 {
		// Default behavior, no expiration
		s.cache.Set(key, value, int64(len(value.Body)))
	} else {
		s.cache.SetWithTTL(key, value, int64(len(value.Body)), ttl)
	}
}

// SetWithTTL explicitly sets an object in the cache with a specific TTL
func (s *Store) SetWithTTL(key []byte, value ObjCore, ttl time.Duration) {
	s.cache.SetWithTTL(key, value, int64(len(value.Body)), ttl)
}

// calculateTTL determines appropriate cache lifetime from response headers
// Returns 0 for objects that should use the default cache behavior (no expiration)
// Considers:
// - Cache-Control: max-age, s-maxage, no-cache, no-store, private, must-revalidate
// - Expires header
// - Age header
func calculateTTL(headers http.Header) time.Duration {
	// Check for Cache-Control directives that prevent caching
	cacheControl := headers.Get("Cache-Control")
	if cacheControl != "" {
		directives := strings.Split(cacheControl, ",")
		for _, directive := range directives {
			directive = strings.TrimSpace(directive)

			// Check for no-store directive - don't cache at all
			if directive == "no-store" {
				return -1 // Negative value means don't cache
			}

			// Check for private directive - typically shouldn't be cached by shared cache
			if directive == "private" {
				return -1
			}

			// Check for no-cache directive - can be stored but must be revalidated
			if directive == "no-cache" {
				return -1
			}

			// Check for must-revalidate
			if directive == "must-revalidate" {
				// We'll still allow caching but with caution
			}

			// Check for s-maxage (takes precedence over max-age for shared caches)
			if strings.HasPrefix(directive, "s-maxage=") {
				seconds, err := strconv.Atoi(strings.TrimPrefix(directive, "s-maxage="))
				if err == nil && seconds > 0 {
					return time.Duration(seconds) * time.Second
				}
			}

			// Check for max-age
			if strings.HasPrefix(directive, "max-age=") {
				seconds, err := strconv.Atoi(strings.TrimPrefix(directive, "max-age="))
				if err == nil && seconds > 0 {
					return time.Duration(seconds) * time.Second
				}
			}
		}
	}

	// Check Expires header if no max-age was found
	expires := headers.Get("Expires")
	if expires != "" {
		// Parse the expires header in various formats
		formats := []string{
			time.RFC1123,
			time.RFC1123Z,
			time.RFC850,
			time.ANSIC,
		}

		var expiresTime time.Time
		var err error

		// Try each format until we find one that works
		for _, format := range formats {
			expiresTime, err = time.Parse(format, expires)
			if err == nil {
				break
			}
		}

		if err == nil {
			// Calculate TTL as difference between expiration time and now
			ttl := time.Until(expiresTime)
			if ttl > 0 {
				// Account for Age header if present
				age := headers.Get("Age")
				if age != "" {
					ageSeconds, err := strconv.Atoi(age)
					if err == nil && ageSeconds > 0 {
						ttl -= time.Duration(ageSeconds) * time.Second
						if ttl <= 0 {
							return -1 // Already expired
						}
					}
				}
				return ttl
			}
			return -1 // Already expired
		}
	}

	// Default case: use default cache behavior
	return 0
}

// MakeKey takes a http.Request and a flag indicating whether to ignore the host,
// and returns a 32 byte sha256 hash of the request.
func MakeKey(r *http.Request, ignoreHost bool) []byte {
	sh := sha256.New()

	// Only include the host in the key if we're not ignoring it
	if !ignoreHost {
		_, _ = sh.Write([]byte(r.Host))
	}

	// Always include the path in the key
	_, _ = sh.Write([]byte(r.URL.Path))

	return sh.Sum(nil)
}
