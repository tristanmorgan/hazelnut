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

	// Initialize logger
	logger := slog.Default()
	logger.Info("starting hazelnut", "version", embeddedVersion)

	// Use the server package to load config and run the server
	return server.LoadAndRun(ctx, configPath, logger, stdout, stderr)
}
