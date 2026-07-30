package frontend

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/perbu/hazelnut/backend"
	"github.com/perbu/hazelnut/cache/lrucache"
	"github.com/perbu/hazelnut/metrics"
)

func TestParseRange(t *testing.T) {
	tests := []struct {
		name          string
		rangeHeader   string
		contentLength int64
		want          []ByteRange
		wantErr       bool
	}{
		{
			name:          "simple range",
			rangeHeader:   "bytes=0-499",
			contentLength: 1000,
			want:          []ByteRange{{Start: 0, End: 499}},
		},
		{
			name:          "suffix range",
			rangeHeader:   "bytes=-500",
			contentLength: 1000,
			want:          []ByteRange{{Start: 500, End: 999}},
		},
		{
			name:          "open-ended range",
			rangeHeader:   "bytes=500-",
			contentLength: 1000,
			want:          []ByteRange{{Start: 500, End: 999}},
		},
		{
			name:          "invalid format",
			rangeHeader:   "bytes=abc-def",
			contentLength: 1000,
			wantErr:       true,
		},
		{
			name:          "out of bounds",
			rangeHeader:   "bytes=0-1500",
			contentLength: 1000,
			wantErr:       true,
		},
		{
			name:          "multiple ranges",
			rangeHeader:   "bytes=0-499,1000-1499",
			contentLength: 2000,
			want:          []ByteRange{{Start: 0, End: 499}, {Start: 1000, End: 1499}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRange(tt.rangeHeader, tt.contentLength)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != len(tt.want) {
				t.Errorf("ParseRange() got %d ranges, want %d", len(got), len(tt.want))
			}
			if !tt.wantErr {
				for i := range got {
					if got[i].Start != tt.want[i].Start || got[i].End != tt.want[i].End {
						t.Errorf("ParseRange() range[%d] = %+v, want %+v", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestByteRangeMethods(t *testing.T) {
	r := ByteRange{Start: 100, End: 199}

	if r.Length() != 100 {
		t.Errorf("Length() = %d, want 100", r.Length())
	}

	contentRange := r.ContentRange(1000)
	expected := "bytes 100-199/1000"
	if contentRange != expected {
		t.Errorf("ContentRange() = %s, want %s", contentRange, expected)
	}
}

func TestRangeRequests(t *testing.T) {
	// Create a logger for testing
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a cache
	c, err := lrucache.New(100, 1024*1024) // 100 objects, 1MB
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Create test content (10KB)
	testContent := make([]byte, 10000)
	for i := range testContent {
		testContent[i] = byte(i % 256)
	}

	// Create a test server that will act as our origin
	originServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")

		if rangeHeader != "" {
			// Backend supports range requests
			ranges, err := ParseRange(rangeHeader, int64(len(testContent)))
			if err != nil {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(testContent)))
				http.Error(w, "Invalid Range", http.StatusRequestedRangeNotSatisfiable)
				return
			}

			if len(ranges) == 1 {
				r := ranges[0]
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Length", fmt.Sprintf("%d", r.Length()))
				w.Header().Set("Content-Range", r.ContentRange(int64(len(testContent))))
				w.Header().Set("Accept-Ranges", "bytes")
				w.WriteHeader(http.StatusPartialContent)
				w.Write(testContent[r.Start : r.End+1])
				return
			}
		}

		// Full content
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Write(testContent)
	}))
	defer originServer.Close()

	// Extract the host and port from test server URL
	hostParts := strings.Split(strings.TrimPrefix(originServer.URL, "http://"), ":")
	host := hostParts[0]
	port := 80
	if len(hostParts) > 1 {
		fmt.Sscanf(hostParts[1], "%d", &port)
	}

	// Create a real backend client pointing to our test server
	b := backend.New(logger, host, port)
	b.SetScheme("http")

	// Create metrics
	m := metrics.New()

	// Create a frontend with our backend and cache
	f := New(logger, c, b, "localhost:8080", m, false)

	// Create a test server with our frontend as handler
	ts := httptest.NewServer(f)
	defer ts.Close()

	// Client for making requests
	client := &http.Client{}

	t.Run("First request caches full content", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/test-file", nil)
		req.Host = "example.com"

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if len(body) != len(testContent) {
			t.Errorf("Expected %d bytes, got %d", len(testContent), len(body))
		}

		if resp.Header.Get("X-Cache") != "miss" {
			t.Errorf("Expected X-Cache: miss, got: %s", resp.Header.Get("X-Cache"))
		}
	})

	t.Run("Range request from cache (first 500 bytes)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/test-file", nil)
		req.Host = "example.com"
		req.Header.Set("Range", "bytes=0-499")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusPartialContent {
			t.Errorf("Expected 206 Partial Content, got %d", resp.StatusCode)
		}

		if resp.Header.Get("X-Cache") != "hit" {
			t.Errorf("Expected X-Cache: hit, got: %s", resp.Header.Get("X-Cache"))
		}

		if resp.Header.Get("Content-Range") != "bytes 0-499/10000" {
			t.Errorf("Wrong Content-Range: %s", resp.Header.Get("Content-Range"))
		}

		body, _ := io.ReadAll(resp.Body)
		if len(body) != 500 {
			t.Errorf("Expected 500 bytes, got %d", len(body))
		}

		// Verify content matches
		for i := 0; i < 500; i++ {
			if body[i] != testContent[i] {
				t.Errorf("Content mismatch at byte %d", i)
				break
			}
		}
	})

	t.Run("Range request from cache (last 500 bytes)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/test-file", nil)
		req.Host = "example.com"
		req.Header.Set("Range", "bytes=-500")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusPartialContent {
			t.Errorf("Expected 206 Partial Content, got %d", resp.StatusCode)
		}

		if resp.Header.Get("X-Cache") != "hit" {
			t.Errorf("Expected X-Cache: hit, got: %s", resp.Header.Get("X-Cache"))
		}

		body, _ := io.ReadAll(resp.Body)
		if len(body) != 500 {
			t.Errorf("Expected 500 bytes, got %d", len(body))
		}

		// Verify content matches last 500 bytes
		for i := 0; i < 500; i++ {
			if body[i] != testContent[9500+i] {
				t.Errorf("Content mismatch at byte %d", i)
				break
			}
		}
	})

	t.Run("Range request from cache (middle range)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/test-file", nil)
		req.Host = "example.com"
		req.Header.Set("Range", "bytes=5000-5999")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusPartialContent {
			t.Errorf("Expected 206 Partial Content, got %d", resp.StatusCode)
		}

		if resp.Header.Get("Content-Range") != "bytes 5000-5999/10000" {
			t.Errorf("Wrong Content-Range: %s", resp.Header.Get("Content-Range"))
		}

		body, _ := io.ReadAll(resp.Body)
		if len(body) != 1000 {
			t.Errorf("Expected 1000 bytes, got %d", len(body))
		}
	})

	t.Run("Range request for uncached content streams from backend", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/new-file", nil)
		req.Host = "example.com"
		req.Header.Set("Range", "bytes=0-499")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusPartialContent {
			t.Errorf("Expected 206 Partial Content, got %d", resp.StatusCode)
		}

		if resp.Header.Get("X-Cache") != "miss" {
			t.Errorf("Expected X-Cache: miss, got: %s", resp.Header.Get("X-Cache"))
		}

		body, _ := io.ReadAll(resp.Body)
		if len(body) != 500 {
			t.Errorf("Expected 500 bytes, got %d", len(body))
		}

		// Verify the content was NOT cached by making another range request
		req2, _ := http.NewRequest("GET", ts.URL+"/new-file", nil)
		req2.Host = "example.com"
		req2.Header.Set("Range", "bytes=500-999")

		resp2, err := client.Do(req2)
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
		defer resp2.Body.Close()

		// Should still be a miss since we didn't cache the range request
		if resp2.Header.Get("X-Cache") != "miss" {
			t.Errorf("Expected X-Cache: miss for second range, got: %s", resp2.Header.Get("X-Cache"))
		}
	})

	t.Run("Invalid range returns 416", func(t *testing.T) {
		// First cache the content
		req1, _ := http.NewRequest("GET", ts.URL+"/test-file", nil)
		req1.Host = "example.com"
		client.Do(req1)

		// Now request invalid range
		req, _ := http.NewRequest("GET", ts.URL+"/test-file", nil)
		req.Host = "example.com"
		req.Header.Set("Range", "bytes=20000-30000") // Out of bounds

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
			t.Errorf("Expected 416, got %d", resp.StatusCode)
		}
	})

	t.Run("Multiple ranges serve full content", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/test-file", nil)
		req.Host = "example.com"
		req.Header.Set("Range", "bytes=0-499,1000-1499")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should serve full content for multi-range
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK for multi-range, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if len(body) != len(testContent) {
			t.Errorf("Expected full content (%d bytes), got %d", len(testContent), len(body))
		}
	})
}
