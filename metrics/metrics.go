package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"sync"
)

// Metrics contains Prometheus metrics for Hazelnut
type Metrics struct {
	CacheHits   prometheus.Counter
	CacheMisses prometheus.Counter
	Errors      prometheus.Counter
}

var (
	once     sync.Once
	instance *Metrics
)

// New creates a new Metrics instance with initialized Prometheus counters
// Uses a singleton pattern to avoid duplicate registration in tests
func New() *Metrics {
	once.Do(func() {

		instance = &Metrics{
			CacheHits: promauto.NewCounter(prometheus.CounterOpts{
				Name: "hazelnut_cache_hits_total",
				Help: "The total number of cache hits",
			}),
			CacheMisses: promauto.NewCounter(prometheus.CounterOpts{
				Name: "hazelnut_cache_misses_total",
				Help: "The total number of cache misses",
			}),
			Errors: promauto.NewCounter(prometheus.CounterOpts{
				Name: "hazelnut_errors_total",
				Help: "The total number of errors",
			}),
		}
	})
	return instance
}
