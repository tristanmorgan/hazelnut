package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/perbu/hazelnut/backend"
	"github.com/perbu/hazelnut/cache"
	"github.com/perbu/hazelnut/config"
	"github.com/perbu/hazelnut/frontend"
	"github.com/perbu/hazelnut/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/errgroup"
	"net/http"
)

// Server represents a Hazelnut server instance
type Server struct {
	Config   *config.Config
	Logger   *slog.Logger
	Cache    *cache.Store
	Backend  *backend.Client
	Frontend *frontend.Server
	Metrics  *metrics.Metrics
}

// New creates a new Hazelnut server with the provided configuration
func New(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("initializing hazelnut server")

	// Initialize metrics
	logger.Info("initializing metrics")
	m := metrics.New()

	// Initialize cache
	maxObj := cfg.Cache.GetMaxObjects()
	maxSize := cfg.Cache.GetMaxSize()
	logger.Info("initializing cache", "maxObjects", maxObj, "maxSize", maxSize)
	c, err := cache.New(maxObj, maxSize)
	if err != nil {
		return nil, fmt.Errorf("cache.New: %w", err)
	}

	// Initialize backend
	backendHost, backendPort := cfg.Backend.ParseTarget()
	logger.Info("initializing backend", "host", backendHost, "port", backendPort, "scheme", cfg.Backend.Scheme)
	b := backend.New(logger, backendHost, backendPort)
	b.SetScheme(cfg.Backend.Scheme)

	// Initialize frontend
	listenAddr := cfg.Frontend.GetListenAddr()
	logger.Info("initializing frontend", "listenAddr", listenAddr)
	f := frontend.New(logger, c, b, listenAddr, m)

	// Create metrics HTTP server with a separate mux
	metricsAddr := ":9091" // Default metrics port
	if cfg.Frontend.MetricsPort != 0 {
		metricsAddr = fmt.Sprintf(":%d", cfg.Frontend.MetricsPort)
	}

	// Skip starting metrics server in test environment
	if metricsAddr != ":0" {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())

		metricsServer := &http.Server{
			Addr:    metricsAddr,
			Handler: metricsMux,
		}

		go func() {
			logger.Info("starting metrics server", "addr", metricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("metrics server failed", "error", err)
			}
		}()

		// Ensure metrics server shuts down when context is done
		go func() {
			<-ctx.Done()
			logger.Info("shutting down metrics server")
			_ = metricsServer.Shutdown(context.Background())
		}()
	}

	return &Server{
		Config:   cfg,
		Logger:   logger,
		Cache:    c,
		Backend:  b,
		Frontend: f,
		Metrics:  m,
	}, nil
}

// Run starts the Hazelnut server and blocks until the context is canceled
func (s *Server) Run(ctx context.Context) error {
	eg := new(errgroup.Group)
	eg.Go(func() error {
		return s.Frontend.Run(ctx)
	})

	// Wait for the context to be done
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("frontend.Run: %w", err)
	}
	return nil
}

// LoadAndRun loads a configuration file and runs a Hazelnut server
// This is a convenience function for applications that want to run Hazelnut
// with minimal code
func LoadAndRun(ctx context.Context, configPath string, logger *slog.Logger, stdout, stderr io.Writer) error {
	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// If no logger is provided, create one with the configured log level
	if logger == nil {
		handler := slog.NewTextHandler(stderr, &slog.HandlerOptions{
			Level: cfg.GetLogLevel(),
		})
		logger = slog.New(handler)
	}

	// Create and run server
	srv, err := New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	return srv.Run(ctx)
}
