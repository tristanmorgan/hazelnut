package frontend

import (
	"fmt"
	"github.com/perbu/hazelnut/cache/lrucache"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/perbu/hazelnut/backend"
	"github.com/perbu/hazelnut/metrics"
)

func TestFrontend(t *testing.T) {
	// Create a logger for testing
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a cache
	c, err := lrucache.New(100, 1024*1024) // 100 objects, 1MB
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Create a test service that will act as our origin
	originServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check the path and respond accordingly
		switch r.URL.Path {
		case "/cacheable":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Cache-Control", "max-age=3600")
			fmt.Fprint(w, "Cacheable response")
		case "/non-cacheable":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Cache-Control", "no-store")
			fmt.Fprint(w, "Non-cacheable response")
		default:
			// Echo back host and path for verification
			w.Header().Set("X-Received-Host", r.Host)
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "Path: %s, Host: %s", r.URL.Path, r.Host)
		}
	}))
	defer originServer.Close()

	// Extract the host and port from test service URL
	hostParts := strings.Split(strings.TrimPrefix(originServer.URL, "http://"), ":")
	host := hostParts[0]
	port := 80
	if len(hostParts) > 1 {
		fmt.Sscanf(hostParts[1], "%d", &port)
	}

	// Create a real backend client pointing to our test service
	b := backend.New(logger, host, port)
	// Set the scheme to http since test servers run on http
	b.SetScheme("http")
	t.Logf("Test origin service running at %s:%d", host, port)

	// Create metrics
	m := metrics.New()

	// Create a frontend with our backend and cache, not ignoring host by default
	f := New(logger, c, b, "localhost:8080", m, false)

	// Create a test service with our frontend as handler
	ts := httptest.NewServer(f)
	defer ts.Close()

	// Client for making requests
	client := &http.Client{}

	t.Run("Cacheable request", func(t *testing.T) {
		// First request (cache miss)
		req1, _ := http.NewRequest("GET", ts.URL+"/cacheable", nil)
		req1.Host = "example.com"

		resp1, err := client.Do(req1)
		if err != nil {
			t.Fatalf("First request failed: %v", err)
		}
		defer resp1.Body.Close()

		// Read body to complete the request
		body1, _ := io.ReadAll(resp1.Body)
		if string(body1) != "Cacheable response" {
			t.Errorf("Unexpected response body: %s", body1)
		}

		// Small delay to ensure caching completes
		time.Sleep(100 * time.Millisecond)

		// Second request to same URL (should be cache hit)
		req2, _ := http.NewRequest("GET", ts.URL+"/cacheable", nil)
		req2.Host = "example.com"

		resp2, err := client.Do(req2)
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
		defer resp2.Body.Close()

		// Check cache status header (should be hit)
		if resp2.Header.Get("X-Cache") != "hit" {
			t.Errorf("Expected X-Cache: hit, got: %s", resp2.Header.Get("X-Cache"))
		}

		// Read body
		body2, _ := io.ReadAll(resp2.Body)
		if string(body2) != "Cacheable response" {
			t.Errorf("Unexpected response body: %s", body2)
		}
	})

	t.Run("Non-cacheable request", func(t *testing.T) {
		// First request
		req1, _ := http.NewRequest("GET", ts.URL+"/non-cacheable", nil)
		req1.Host = "example.com"

		resp1, err := client.Do(req1)
		if err != nil {
			t.Fatalf("First request failed: %v", err)
		}
		defer resp1.Body.Close()

		// Read body
		body1, _ := io.ReadAll(resp1.Body)
		if string(body1) != "Non-cacheable response" {
			t.Errorf("Unexpected response body: %s", body1)
		}

		// Small delay
		time.Sleep(100 * time.Millisecond)

		// Second request to same URL (should still be cache miss due to no-store)
		req2, _ := http.NewRequest("GET", ts.URL+"/non-cacheable", nil)
		req2.Host = "example.com"

		resp2, err := client.Do(req2)
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
		defer resp2.Body.Close()

		// Read body
		body2, _ := io.ReadAll(resp2.Body)
		if string(body2) != "Non-cacheable response" {
			t.Errorf("Unexpected response body: %s", body2)
		}
	})

	t.Run("Different hosts use different cache keys", func(t *testing.T) {
		// First request with host1
		req1, _ := http.NewRequest("GET", ts.URL+"/cacheable", nil)
		req1.Host = "host1.example.com"

		resp1, err := client.Do(req1)
		if err != nil {
			t.Fatalf("First request failed: %v", err)
		}
		defer resp1.Body.Close()

		// Read body
		io.ReadAll(resp1.Body)

		// Small delay
		time.Sleep(100 * time.Millisecond)

		// Second request with different host (should be cache miss)
		req2, _ := http.NewRequest("GET", ts.URL+"/cacheable", nil)
		req2.Host = "host2.example.com"

		resp2, err := client.Do(req2)
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
		defer resp2.Body.Close()

		// Should be a cache miss for different host
		if resp2.Header.Get("X-Cache") == "hit" {
			t.Errorf("Expected cache miss for different host, got hit")
		}

		// Third request with first host again (should be cache hit)
		req3, _ := http.NewRequest("GET", ts.URL+"/cacheable", nil)
		req3.Host = "host1.example.com"

		resp3, err := client.Do(req3)
		if err != nil {
			t.Fatalf("Third request failed: %v", err)
		}
		defer resp3.Body.Close()

		// Should be a cache hit for the first host
		if resp3.Header.Get("X-Cache") != "hit" {
			t.Errorf("Expected X-Cache: hit for repeated host, got: %s", resp3.Header.Get("X-Cache"))
		}
	})

	t.Run("Non-GET methods bypass cache", func(t *testing.T) {
		// POST request should bypass cache
		req, _ := http.NewRequest("POST", ts.URL+"/cacheable", strings.NewReader("test data"))
		req.Host = "example.com"

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST request failed: %v", err)
		}
		defer resp.Body.Close()

		// X-Cache should not be set for POST
		if resp.Header.Get("X-Cache") != "" {
			t.Errorf("Expected no X-Cache header for POST, got: %s", resp.Header.Get("X-Cache"))
		}
	})

	t.Run("IgnoreHost option works correctly", func(t *testing.T) {
		// Create a new frontend with ignoreHost = true
		fIgnoreHost := New(logger, c, b, "localhost:8080", m, true)

		// Create a test service with this frontend
		tsIgnore := httptest.NewServer(fIgnoreHost)
		defer tsIgnore.Close()

		// First request with host1
		req1, _ := http.NewRequest("GET", tsIgnore.URL+"/shared-path", nil)
		req1.Host = "host1.example.com"

		resp1, err := client.Do(req1)
		if err != nil {
			t.Fatalf("First request failed: %v", err)
		}
		defer resp1.Body.Close()

		// Read body
		body1, _ := io.ReadAll(resp1.Body)

		// Small delay
		time.Sleep(100 * time.Millisecond)

		// Second request with different host but same path
		req2, _ := http.NewRequest("GET", tsIgnore.URL+"/shared-path", nil)
		req2.Host = "host2.example.com"

		resp2, err := client.Do(req2)
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
		defer resp2.Body.Close()

		// Should be a cache hit despite different host (because ignoreHost=true)
		if resp2.Header.Get("X-Cache") != "hit" {
			t.Errorf("Expected cache hit with ignoreHost=true, got: %s", resp2.Header.Get("X-Cache"))
		}

		// Bodies should match
		body2, _ := io.ReadAll(resp2.Body)
		if string(body1) != string(body2) {
			t.Errorf("Response bodies should match when ignoreHost=true")
		}
	})
}
