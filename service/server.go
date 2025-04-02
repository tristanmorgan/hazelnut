package service

import (
	"context"
	"fmt"
	"github.com/perbu/hazelnut/cache"
	"github.com/perbu/hazelnut/cache/mapcache"
	"io"
	"log/slog"

	"github.com/perbu/hazelnut/backend"
	"github.com/perbu/hazelnut/config"
	"github.com/perbu/hazelnut/frontend"
	"github.com/perbu/hazelnut/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/errgroup"
	"net/http"
)

// Server represents a Hazelnut service instance
type Server struct {
	Config   *config.Config
	Logger   *slog.Logger
	Cache    Cache
	Backend  *backend.Router
	Frontend *frontend.Server
	Metrics  *metrics.Metrics
}

type Cache interface {
	Get(key string) (cache.ObjCore, bool)
	Set(key string, value cache.ObjCore)
}

// New creates a new Hazelnut service with the provided configuration
func New(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("initializing hazelnut service")

	// Initialize metrics
	logger.Info("initializing metrics")
	m := metrics.New()

	// Initialize cache
	/*
		maxObj := cfg.Cache.GetMaxObjects()
		maxSize := cfg.Cache.GetMaxSize()
		logger.Info("initializing cache", "maxObjects", maxObj, "maxSize", maxSize)

	*/
	c := mapcache.New()
	/*
		if err != nil {
			return nil, fmt.Errorf("cache.New: %w", err)
		}
	*/

	// Initialize default backend
	scheme, backendHost, backendPort, err := cfg.DefaultBackend.ParseTarget()
	if err != nil {
		return nil, fmt.Errorf("parsing default backend target: %w", err)
	}
	logger.Info("initializing default backend", "scheme", scheme, "host", backendHost, "port", backendPort)
	defaultBackend := backend.New(logger, backendHost, backendPort)
	defaultBackend.SetScheme(scheme)

	// Create the backend router with the default backend
	backendRouter := backend.NewRouter(logger, defaultBackend)

	// Add virtual host backends if configured
	for host, backendCfg := range cfg.VirtualHosts {
		scheme, vHost, vPort, err := backendCfg.ParseTarget()
		if err != nil {
			return nil, fmt.Errorf("parsing virtual host backend target: %w", err)
		}
		logger.Info("initializing virtual host backend",
			"virtualHost", host,
			"target", vHost,
			"port", vPort,
			"scheme", scheme)

		vBackend := backend.New(logger, vHost, vPort)
		vBackend.SetScheme(scheme)
		backendRouter.AddBackend(host, vBackend)
	}

	// Initialize frontend
	listenAddr := cfg.Frontend.GetListenAddr()
	logger.Info("initializing frontend", "listenAddr", listenAddr, "ignoreHost", cfg.Cache.IgnoreHost)
	f := frontend.New(logger, c, backendRouter, listenAddr, m, cfg.Cache.IgnoreHost)

	// Create metrics HTTP service with a separate mux
	metricsAddr := ":9091" // Default metrics port
	if cfg.Frontend.MetricsPort != 0 {
		metricsAddr = fmt.Sprintf(":%d", cfg.Frontend.MetricsPort)
	}

	// Skip starting metrics service in test environment
	if metricsAddr != ":0" {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())

		metricsServer := &http.Server{
			Addr:    metricsAddr,
			Handler: metricsMux,
		}

		go func() {
			logger.Info("starting metrics service", "addr", metricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("metrics service failed", "error", err)
			}
		}()

		// Ensure metrics service shuts down when context is done
		go func() {
			<-ctx.Done()
			logger.Info("shutting down metrics service")
			_ = metricsServer.Shutdown(context.Background())
		}()
	}

	return &Server{
		Config:   cfg,
		Logger:   logger,
		Cache:    c,
		Backend:  backendRouter,
		Frontend: f,
		Metrics:  m,
	}, nil
}

// GetActualPort returns the actual port the service is listening on
func (s *Server) GetActualPort() int {
	return s.Frontend.ActualPort()
}

// Run starts the Hazelnut service and blocks until the context is canceled
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

// LoadAndRun loads a configuration file and runs a Hazelnut service
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

	// Create and run service
	srv, err := New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("creating service: %w", err)
	}

	return srv.Run(ctx)
}
