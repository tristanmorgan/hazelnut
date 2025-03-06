package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/perbu/hazelnut/backend"
	"github.com/perbu/hazelnut/cache"
	"github.com/perbu/hazelnut/frontend"
	"github.com/perbu/hazelnut/metrics"
)

func TestProxy(t *testing.T) {
	// Create a logger for testing
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a test server that will act as our origin
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo the host header to verify it's preserved
		w.Header().Set("X-Received-Host", r.Host)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Hello from origin! Host: %s, Path: %s", r.Host, r.URL.Path)
	}))
	defer ts.Close()

	// Extract the host and port from test server URL
	// Format is http://127.0.0.1:port
	hostParts := strings.Split(strings.TrimPrefix(ts.URL, "http://"), ":")
	host := hostParts[0]
	port := 80
	if len(hostParts) > 1 {
		port, _ = strconv.Atoi(hostParts[1])
	}
	t.Logf("Test server running at %s:%d", host, port)

	// Configure the cache
	c, err := cache.New(100, 1024*1024) // 100 objects, 1MB
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Configure the backend to point to our test server
	b := backend.New(logger, host, port)
	// Set the scheme to http since test servers run on http
	b.SetScheme("http")

	// Create metrics
	m := metrics.New()

	// Configure the frontend with our backend and cache
	f := frontend.New(logger, c, b, "localhost:8080", m, false)

	// Start the proxy server
	proxyServer := httptest.NewServer(f)
	t.Logf("Test proxy running at %s", proxyServer.URL)
	defer proxyServer.Close()

	// Run tests
	t.Run("Basic proxy functionality", func(t *testing.T) {
		// Make a request to the proxy
		req, err := http.NewRequest("GET", proxyServer.URL+"/test-path", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		// Set a custom Host header to test preservation
		req.Host = "example.com"

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		// Read the response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		// Verify the Host header was preserved
		receivedHost := resp.Header.Get("X-Received-Host")
		if receivedHost != "example.com" {
			t.Errorf("Host header not preserved. Expected 'example.com', got %q", receivedHost)
		}

		// Check response content
		expected := "Hello from origin! Host: example.com, Path: /test-path"
		if !strings.Contains(string(body), expected) {
			t.Errorf("Unexpected response body: %s", body)
		}
	})

	t.Run("Cache functionality", func(t *testing.T) {
		// Make first request which should be a cache miss
		req, _ := http.NewRequest("GET", proxyServer.URL+"/cached-path", nil)
		req.Host = "example.com"

		client := &http.Client{}
		resp1, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make first request: %v", err)
		}
		defer resp1.Body.Close()

		// Read the response to complete the request
		body1, _ := io.ReadAll(resp1.Body)
		t.Logf("First response body: %s", body1)

		// Small delay to ensure caching completes
		time.Sleep(100 * time.Millisecond)

		// Make second request which should be a cache hit
		resp2, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make second request: %v", err)
		}
		defer resp2.Body.Close()

		// We expect the same content in both responses
		body2, _ := io.ReadAll(resp2.Body)
		if string(body1) != string(body2) {
			t.Errorf("Response bodies differ: first=%q, second=%q", string(body1), string(body2))
		}
	})
}
