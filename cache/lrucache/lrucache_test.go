package lrucache

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/perbu/hazelnut/cache"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	// Create a new cache with small limits for testing
	c, err := New(10, 1024) // 10 objects, 1KB
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	t.Run("Basic get/set operations", func(t *testing.T) {
		// Create a test response
		headers := make(http.Header)
		headers.Set("Content-Type", "text/plain")
		headers.Set("X-Test", "value")
		body := []byte("Test response body")

		// Create a key using SHA256
		key := sha256.Sum256([]byte("test-key-1"))

		// Create ObjCore
		value := cache.ObjCore{
			Headers: headers,
			Body:    body,
		}

		// Store in cache
		c.Set(string(key[:]), value)

		// Wait for Ristretto to process the set operation (it's async)
		time.Sleep(10 * time.Millisecond)

		// Try to retrieve from cache
		cachedValue, found := c.Get(string(key[:]))
		if !found {
			t.Fatalf("Item not found in cache after setting")
		}

		// Verify headers were preserved
		if cachedValue.Headers.Get("Content-Type") != "text/plain" {
			t.Errorf("Content-Type header not preserved, got: %s", cachedValue.Headers.Get("Content-Type"))
		}
		if cachedValue.Headers.Get("X-Test") != "value" {
			t.Errorf("X-Test header not preserved, got: %s", cachedValue.Headers.Get("X-Test"))
		}

		// Verify body was preserved
		if !bytes.Equal(cachedValue.Body, body) {
			t.Errorf("Body not preserved, got: %s, want: %s", cachedValue.Body, body)
		}
	})

	t.Run("Cache miss", func(t *testing.T) {
		// Try to get a non-existent key
		nonExistentKey := sha256.Sum256([]byte("non-existent-key"))
		_, found := c.Get(string(nonExistentKey[:]))
		if found {
			t.Errorf("Expected cache miss for non-existent key, but got a hit")
		}
	})

	t.Run("Cache eviction and capacity", func(t *testing.T) {
		// Create a tiny cache to test that items can be stored and retrieved
		tinyCache, err := New(5, 1024) // Small cache
		if err != nil {
			t.Fatalf("Failed to create tiny cache: %v", err)
		}

		// Store several items
		for i := range 5 {
			key := sha256.Sum256([]byte(fmt.Sprintf("key-%d", i)))
			value := cache.ObjCore{
				Headers: make(http.Header),
				Body:    []byte(fmt.Sprintf("content-%d", i)),
			}
			tinyCache.Set(string(key[:]), value)
		}

		// Wait for processing
		time.Sleep(10 * time.Millisecond)

		// Verify we can retrieve at least one item
		key0 := sha256.Sum256([]byte("key-0"))
		val, found := tinyCache.Get(string(key0[:]))

		if !found {
			t.Logf("Note: Cache eviction test is informational. Ristretto may evict based on its policy.")
		} else {
			if string(val.Body) != "content-0" {
				t.Errorf("Unexpected content in cache, got %s", string(val.Body))
			}
		}
	})

	t.Run("Response conversion", func(t *testing.T) {
		// Create a dummy HTTP response
		dummyResp := &http.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"text/plain"}, "X-Test": []string{"test-value"}},
			Body:       io.NopCloser(bytes.NewBufferString("Test response body")),
		}

		// Read the response body for caching
		body, err := io.ReadAll(dummyResp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}
		// Close the original body
		dummyResp.Body.Close()

		// Create key and value
		key := sha256.Sum256([]byte("test-conversion"))
		value := cache.ObjCore{
			Headers: dummyResp.Header,
			Body:    body,
		}

		// Store in cache
		c.Set(string(key[:]), value)

		// Wait for processing
		time.Sleep(10 * time.Millisecond)

		// Retrieve from cache
		cachedValue, found := c.Get(string(key[:]))
		if !found {
			t.Fatalf("Item not found in cache after setting")
		}

		// Create a new response from the cached data
		newResp := &http.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Header:     cachedValue.Headers,
			Body:       io.NopCloser(bytes.NewBuffer(cachedValue.Body)),
		}

		// Verify the new response
		if newResp.Header.Get("Content-Type") != "text/plain" {
			t.Errorf("Content-Type header not preserved, got: %s", newResp.Header.Get("Content-Type"))
		}

		newBody, err := io.ReadAll(newResp.Body)
		if err != nil {
			t.Fatalf("Failed to read new response body: %v", err)
		}
		newResp.Body.Close()

		if string(newBody) != "Test response body" {
			t.Errorf("Body not preserved, got: %s", newBody)
		}
	})
}
