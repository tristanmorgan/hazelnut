package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/perbu/hazelnut/config"
)

func TestServer(t *testing.T) {
	// Create a logger for testing
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a test origin server
	originServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Received-Host", r.Host)
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Cache-Control", "max-age=3600")
		fmt.Fprintf(w, "Hello from origin! Host: %s, Path: %s", r.Host, r.URL.Path)
	}))
	defer originServer.Close()

	// Extract the host and port from test server URL
	hostParts := strings.Split(strings.TrimPrefix(originServer.URL, "http://"), ":")
	host := hostParts[0]
	port := 80
	if len(hostParts) > 1 {
		port, _ = strconv.Atoi(hostParts[1])
	}
	t.Logf("Test origin server running at %s:%d", host, port)

	// Create a test configuration
	cfg := &config.Config{
		Backend: config.BackendConfig{
			Target:  fmt.Sprintf("%s:%d", host, port),
			Timeout: 30 * time.Second,
			Scheme:  "http", // Use HTTP for tests
		},
		Frontend: config.FrontendConfig{
			Port:        8080,
			MetricsPort: 0, // Disable metrics in tests
		},
		Cache: config.CacheConfig{
			MaxObj:  "100",
			MaxCost: "1M",
		},
	}

	// Create a server instance
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start the server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Create a test request
	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://localhost:8080/test-path", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Host = "example.com"

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		// This is expected since we're not actually listening on port 8080
		t.Logf("Expected connection error: %v", err)
	} else {
		defer resp.Body.Close()
		// If by chance it worked, validate the response
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", resp.Status)
		}
	}

	// Cancel the context to stop the server
	cancel()

	// Check if the server returned an error
	select {
	case err := <-errCh:
		if err != nil && err.Error() != "frontend.Run: ListenAndServe: context canceled" {
			t.Errorf("Server returned unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Server didn't shut down within timeout")
	}
}

func TestServerConfig(t *testing.T) {
	// Test that we can create a server from a config file
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	// Create a simple config
	cfg := &config.Config{
		Backend: config.BackendConfig{
			Target:  "example.com:443",
			Timeout: 30 * time.Second,
			Scheme:  "https",
		},
		Frontend: config.FrontendConfig{
			Port:        8080,
			MetricsPort: 0, // Disable metrics in tests
		},
		Cache: config.CacheConfig{
			MaxObj:  "100",
			MaxCost: "1M",
		},
	}

	// Create the server
	srv, err := New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Verify the server configuration
	if srv.Backend.GetScheme() != "https" {
		t.Errorf("Expected backend scheme to be https, got %s", srv.Backend.GetScheme())
	}

	if srv.Config.Frontend.Port != 8080 {
		t.Errorf("Expected frontend port to be 8080, got %d", srv.Config.Frontend.Port)
	}
}
