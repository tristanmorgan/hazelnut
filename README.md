# Hazelnut - A caching reverse proxy

Hazelnut is a lightweight caching reverse proxy written in Go. It can be used as a standalone server or embedded into your Go applications.

## Features

- HTTP caching based on standard Cache-Control headers
- Configurable backend targets
- Support for both HTTP and HTTPS
- High-performance Ristretto-based cache
- Simple configuration via YAML
- Prometheus metrics for monitoring cache performance

## Usage

### As a standalone server

```bash
# Run with default config file (config.yaml)
./hazelnut

# Run with a specific config file
./hazelnut -config path/to/config.yaml
```

### Embedded in your Go application

Hazelnut can be easily embedded in your Go application:
#### Code example
```go
package main

import (
    "github.com/perbu/hazelnut/config"
    "github.com/perbu/hazelnut/server"
)

// Create configuration
cfg := &config.Config{
    Backend: config.BackendConfig{
        Target:  "example.com:443",
        Scheme:  "https",
    },
    Frontend: config.FrontendConfig{
        Port: 8080,
        MetricsPort: 9091,
    },
    Cache: config.CacheConfig{
        MaxObj:  "1M",
        MaxCost: "1G",
    },
}

// Create and run server
hazelnut, err := server.New(ctx, cfg, logger)
if err != nil {
    // handle error
}
// ..
// Run hazelnut. You might wanna check the return value if you actually care.
go hazelnut.Run(ctx)
```

See the `examples` directory for more detailed examples.

## Metrics

Hazelnut exposes Prometheus metrics at `/metrics` on the configured metrics port (default: 9091):

- `hazelnut_cache_hits_total`: Counter for the total number of cache hits
- `hazelnut_cache_misses_total`: Counter for the total number of cache misses
- `hazelnut_errors_total`: Counter for the total number of errors

You can configure these metrics in Prometheus by adding the following to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'hazelnut'
    static_configs:
      - targets: ['localhost:9091']
```

## Configuration

Configuration is done via YAML file:

```yaml
frontend:
  port: 8080
  metricsport: 9091  # Port for Prometheus metrics (optional)
  cert: ""  # TLS cert file (optional)
  key: ""   # TLS key file (optional)

backend:
  target: example.com:443
  timeout: 10s
  scheme: https

cache:
  maxobj: 1M     # Maximum number of objects
  maxcost: 1G    # Maximum cache size
```

