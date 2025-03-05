package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/perbu/hazelnut/config"
	"github.com/perbu/hazelnut/server"
)

func main() {
	// Set up a logger
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)

	// Create context with signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Create a configuration programmatically
	cfg := &config.Config{
		Backend: config.BackendConfig{
			Target:  "example.com:443",
			Timeout: 30 * time.Second,
			Scheme:  "https",
		},
		Frontend: config.FrontendConfig{
			Port: 8080,
		},
		Cache: config.CacheConfig{
			MaxObj:  "1M",
			MaxCost: "1G",
		},
	}

	// Create a new Hazelnut server
	hazelnut, err := server.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to create hazelnut server", "error", err)
		os.Exit(1)
	}

	// Start the server in a goroutine
	go func() {
		if err := hazelnut.Run(ctx); err != nil {
			logger.Error("hazelnut server error", "error", err)
		}
	}()

	logger.Info("hazelnut server started at :8080")

	// You can do other things in your application here
	// ...

	// Wait for termination signal
	<-ctx.Done()
	logger.Info("shutting down")

	// We could perform additional cleanup here if needed

	fmt.Println("Goodbye!")
}
