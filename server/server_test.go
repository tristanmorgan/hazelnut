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

	// Create a test configuration with a random port
	// Using port 0 lets the system allocate an available port
	cfg := &config.Config{
		DefaultBackend: config.BackendConfig{
			Target:  fmt.Sprintf("http://%s:%d", host, port),
			Timeout: 30 * time.Second,
		},
		Frontend: config.FrontendConfig{
			BaseURL:     "http://localhost:0",
			MetricsPort: 0, // Disable metrics in tests
		},
		Cache: config.CacheConfig{
			MaxObj:  "100",
			MaxCost: "1M",
		},
	}

	// Instead of starting the full server, we'll create a test handler
	// Create a server instance to get our handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create the server components (backend, cache, etc.)
	srv, err := New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Instead of starting the full server, create a test server with the same handler
	testServer := httptest.NewServer(srv.Frontend)
	defer testServer.Close()

	t.Logf("Test hazelnut server running at %s", testServer.URL)

	// Create a test request
	client := &http.Client{}
	req, err := http.NewRequest("GET", testServer.URL+"/test-path", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Host = "example.com" // Set the host header for testing

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Validate the response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}

	// Check for cache-related headers
	if resp.Header.Get("X-Cache") != "miss" {
		t.Errorf("Expected X-Cache: miss, got %v", resp.Header.Get("X-Cache"))
	}
}

func TestServerConfig(t *testing.T) {
	// Test that we can create a server from a config file
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	// Create a simple config with port 0 (random port)
	cfg := &config.Config{
		DefaultBackend: config.BackendConfig{
			Target:  "http://example.com:443",
			Timeout: 30 * time.Second,
		},
		Frontend: config.FrontendConfig{
			BaseURL:     "http://example.com:0",
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
	if srv.Backend.GetScheme() != "http" {
		t.Errorf("Expected backend scheme to be https, got %s", srv.Backend.GetScheme())
	}

	if srv.Frontend.ActualPort() != 0 {
		t.Errorf("Expected frontend port to be 0 (random), got %d", srv.Frontend.ActualPort())
	}
}
