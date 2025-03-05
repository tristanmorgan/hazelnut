package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"github.com/perbu/hazelnut/backend"
	"github.com/perbu/hazelnut/cache"
	"github.com/perbu/hazelnut/config"
	"github.com/perbu/hazelnut/frontend"
	"golang.org/x/sync/errgroup"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

//go:embed .version
var embeddedVersion string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("clean exit")
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	// Parse command line flags
	var configPath string
	fs := flag.NewFlagSet("hazelnut", flag.ExitOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	// Initialize logger
	logger := slog.Default()
	logger.Info("starting hazelnut", "version", embeddedVersion)

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	logger.Info("configuration loaded", "path", configPath)

	// Initialize cache
	maxObj := cfg.Cache.GetMaxObjects()
	maxSize := cfg.Cache.GetMaxSize()
	logger.Info("initializing cache", "maxObjects", maxObj, "maxSize", maxSize)
	c, err := cache.New(maxObj, maxSize)
	if err != nil {
		return fmt.Errorf("cache.New: %w", err)
	}

	// Initialize backend
	backendHost, backendPort := cfg.Backend.ParseTarget()
	logger.Info("initializing backend", "host", backendHost, "port", backendPort, "scheme", cfg.Backend.Scheme)
	b := backend.New(logger, backendHost, backendPort)
	// Set scheme in the backend
	b.SetScheme(cfg.Backend.Scheme)

	// Initialize frontend
	listenAddr := cfg.Frontend.GetListenAddr()
	logger.Info("initializing frontend", "listenAddr", listenAddr)
	s := frontend.New(logger, c, b, listenAddr)

	// Set up an errgroup to handle the running
	eg := new(errgroup.Group)
	eg.Go(func() error {
		return s.Run(ctx)
	})

	// Wait for the context to be done
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("frontend.Run: %w", err)
	}
	return nil
}
