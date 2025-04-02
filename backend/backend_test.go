package backend

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBackendRequest(t *testing.T) {
	// Create a logger for testing
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a test service that will echo back the request information
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Host: %s\nPath: %s\nMethod: %s\n", r.Host, r.URL.Path, r.Method)
		for name, values := range r.Header {
			for _, value := range values {
				fmt.Fprintf(w, "Header: %s: %s\n", name, value)
			}
		}
	}))
	defer ts.Close()

	// Extract the host and port from the test service
	hostParts := strings.Split(strings.TrimPrefix(ts.URL, "http://"), ":")
	host := hostParts[0]
	port := 80
	if len(hostParts) > 1 {
		fmt.Sscanf(hostParts[1], "%d", &port)
	}
	t.Logf("Test service running at %s:%d", host, port)

	// Create the backend pointing to our test service
	b := New(logger, host, port)

	t.Run("Preserves Host header", func(t *testing.T) {
		// Create a request with a custom Host header
		reqURL, _ := url.Parse("http://example.com/test-path")
		req := &http.Request{
			Method: "GET",
			URL:    reqURL,
			Host:   "example.com",
			Header: make(http.Header),
		}
		req.Header.Set("X-Custom-Header", "test-value")

		// Make the request through the backend
		resp, ok := b.Fetch(req)
		if !ok {
			t.Fatalf("Backend request failed, unexpected failure")
		}
		defer resp.Body.Close()

		// Read the response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		// Check that the Host header was preserved
		bodyStr := string(body)
		t.Logf("Response body: %s", bodyStr)

		if !strings.Contains(bodyStr, "Host: example.com") {
			t.Errorf("Host header not preserved. Response body: %s", bodyStr)
		}

		if !strings.Contains(bodyStr, "Path: /test-path") {
			t.Errorf("Path not preserved. Response body: %s", bodyStr)
		}

		if !strings.Contains(bodyStr, "Header: X-Custom-Header: test-value") {
			t.Errorf("Custom header not preserved. Response body: %s", bodyStr)
		}
	})

	t.Run("Handles backend errors", func(t *testing.T) {
		// Create a backend pointing to a non-existent service
		badBackend := New(logger, "localhost", 9999) // This port should not be in use

		// Create a request
		reqURL, _ := url.Parse("http://example.com/test-path")
		req := &http.Request{
			Method: "GET",
			URL:    reqURL,
			Host:   "example.com",
			Header: make(http.Header),
		}

		// Make the request through the backend
		resp, ok := badBackend.Fetch(req)
		if ok {
			t.Errorf("Expected failed backend request (ok=false), got success")
		}
		defer resp.Body.Close()

		// Verify we got the fallback response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		if !strings.Contains(string(body), "I have a confuse") {
			t.Errorf("Expected fallback response with 'I have a confuse', got: %s", body)
		}
	})
}
