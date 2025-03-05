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
	"golang.org/x/sync/errgroup"
)

// Server represents a Hazelnut server instance
type Server struct {
	Config   *config.Config
	Logger   *slog.Logger
	Cache    *cache.Store
	Backend  *backend.Client
	Frontend *frontend.Server
}

// New creates a new Hazelnut server with the provided configuration
func New(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("initializing hazelnut server")

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
	f := frontend.New(logger, c, b, listenAddr)

	return &Server{
		Config:   cfg,
		Logger:   logger,
		Cache:    c,
		Backend:  b,
		Frontend: f,
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

	// Create and run server
	srv, err := New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	return srv.Run(ctx)
}
