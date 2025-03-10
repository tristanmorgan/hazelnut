package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/perbu/hazelnut/config"
	"github.com/perbu/hazelnut/server"
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

	// Load configuration first to get log level
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	var handler slog.Handler
	// Initialize logger with configured log level
	switch cfg.Logging.Format {
	case "text":
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: cfg.GetLogLevel(),
		})
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: cfg.GetLogLevel(),
		})
	}
	logger := slog.New(handler)
	logger.Info("starting hazelnut", "version", embeddedVersion,
		"logLevel", cfg.Logging.Level, "config", configPath)

	// Use the server package to run the server with loaded config
	srv, err := server.New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	return srv.Run(ctx)
}
