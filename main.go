package main

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/perbu/hazelnut/backend"
	"github.com/perbu/hazelnut/cache"
	"github.com/perbu/hazelnut/frontend"
	"golang.org/x/sync/errgroup"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

const (
	// DefaultMaxObj is the default maximum number of objects in the cache.
	DefaultMaxObj = 1000 * 1000 // 1M objects
	// DefaultMaxSize is the default maximum size of the cache.
	DefaultMaxSize = 1000 * 1000 * 1000 // 1GB
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
	logger := slog.Default()
	logger.Info("starting hazelnut", "version", embeddedVersion)
	c, err := cache.New(DefaultMaxObj, DefaultMaxSize)
	if err != nil {
		return fmt.Errorf("cache.New: %w", err)
	}
	b := backend.New(logger, "www.varnish-software.com", 443)
	s := frontend.New(logger, c, b, ":8080")
	// set up an errgroup to handle the running
	eg := new(errgroup.Group)
	eg.Go(func() error {
		return s.Run(ctx)
	})
	// wait for the context to be done
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("frontend.Run: %w", err)
	}
	return nil

}
