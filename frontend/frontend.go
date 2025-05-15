package frontend

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"github.com/perbu/hazelnut/backend"
	"github.com/perbu/hazelnut/cache"
	"github.com/perbu/hazelnut/metrics"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed .version
var embeddedVersion string

const (
	defaultTTL = 5 * time.Minute
)

type Cache interface {
	Get(key string) (cache.ObjCore, bool)
	Set(key string, value cache.ObjCore)
}

type Server struct {
	cache      Cache
	backend    backend.Fetcher
	srv        *http.Server
	logger     *slog.Logger
	metrics    *metrics.Metrics
	ignoreHost bool // Flag to determine if host should be ignored in cache keys
}

func New(logger *slog.Logger, cache Cache, backend backend.Fetcher, addr string, metrics *metrics.Metrics, ignoreHost bool) *Server {
	s := &Server{
		cache:      cache,
		backend:    backend,
		logger:     logger.With("package", "frontend"),
		metrics:    metrics,
		ignoreHost: ignoreHost,
	}
	s.srv = &http.Server{
		Addr:    addr,
		Handler: s,
	}
	logger.Info("frontend configured", "addr", addr, "ignoreHost", ignoreHost)
	return s
}

// ActualPort returns the actual port the service is listening on.
// Only works after service is started and when using port 0 to get a random port.
// this is useful for testing when the service is started with port 0.
func (s *Server) ActualPort() int {
	if s.srv == nil || s.srv.Addr == "" {
		return 0
	}
	// If the service has a listener, get the actual port
	if listener := s.srv.BaseContext; listener != nil {
		if addr, ok := s.srv.BaseContext(nil).Value(http.LocalAddrContextKey).(net.Addr); ok {
			if tcpAddr, ok := addr.(*net.TCPAddr); ok {
				return tcpAddr.Port
			}
		}
	}
	return 0
}

func (s *Server) Run(ctx context.Context) error {
	// Setup service shutdown when context is done
	go func() {
		<-ctx.Done()
		s.logger.Info("shutting down service")
		_ = s.srv.Shutdown(ctx)
	}()

	// Start the service
	if err := s.srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("ListenAndServe: %w", err)
	}
	return nil
}

func (s *Server) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	t0 := time.Now()
	switch req.Method {
	case http.MethodGet:
		s.cacheable(resp, req)
	case http.MethodHead:
		s.cacheable(resp, req)
	default:
		s.defaultMethod(resp, req)
	}
	s.logger.Info("request", "method", req.Method, "path", req.URL.Path, "duration", time.Since(t0))
}

// cacheable handles GET and HEAD requests, these can be cached and can have hits
func (s *Server) cacheable(resp http.ResponseWriter, req *http.Request) {
	t0 := time.Now()
	key := cache.MakeKey(req, s.ignoreHost)
	obj, found := s.cache.Get(key)
	if found {
		// Increment cache hit counter
		s.metrics.CacheHits.Inc()

		for k, v := range obj.Headers {
			resp.Header()[k] = v
		}
		resp.Header().Add("X-Cache", "hit")
		resp.Header().Add("X-Cache-Latency", asciiFormat(time.Since(t0)))
		resp.WriteHeader(http.StatusOK)
		_, _ = resp.Write(obj.Body) // yolo
		s.logger.Info("cache hit", "key", key, "duration", time.Since(t0), "path", req.URL.Path, "ignoreHost", s.ignoreHost)
		return
	}

	// Increment cache miss counter
	s.metrics.CacheMisses.Inc()

	// cache miss. fetch from backend
	beReq := req.Clone(context.Background())
	// clear the URI:
	beReq.RequestURI = ""

	// If original request is HEAD, convert to GET for backend fetch
	if req.Method == http.MethodHead {
		beReq.Method = http.MethodGet
	}

	// URL scheme will be set by the backend

	// Use the Host header as the URL host if not already set
	if beReq.URL.Host == "" {
		beReq.URL.Host = beReq.Host
	}

	beResp, cacheable := s.backend.Fetch(beReq)

	defer beResp.Body.Close()
	body, err := io.ReadAll(beResp.Body)
	if err != nil {
		s.metrics.Errors.Inc()
		http.Error(resp, err.Error(), http.StatusInternalServerError)
		return
	}
	// body dump for debugging purposes:
	// _, _ = fmt.Fprintln(os.Stdout, string(body))

	// clean up headers before inserting into cache:
	for _, h := range headerDenyList() {
		beResp.Header.Del(h)
	}
	// add a Via header to the cached response
	beResp.Header.Add("Via", versionString())

	if cacheable {
		objCore := cache.ObjCore{
			Headers: beResp.Header,
			Body:    body,
		}

		// Calculate cache TTL based on response headers
		ttl := calculateTTL(beResp.Header)
		if ttl > 0 {
			resp.Header().Add("X-Cache-TTL", ttl.String())
			s.cache.Set(key, objCore)
			s.logger.Debug("caching response with TTL", "ttl", ttl.String(), "contentLength", len(body))
		} else {
			s.logger.Debug("not caching response", "reason", "fetch said so")
		}
	}
	// write the response to the client
	for k, v := range beResp.Header {
		resp.Header()[k] = v
	}
	resp.Header().Add("X-Cache", "miss")
	resp.Header().Add("X-Cache-Latency", asciiFormat(time.Since(t0)))
	resp.WriteHeader(beResp.StatusCode)
	if _, err := resp.Write(body); err != nil {
		s.metrics.Errors.Inc()
		s.logger.Warn("write beResp.Body", "err", err)
	}
	s.logger.Info("cache miss", "key", key, "duration", time.Since(t0), "path", req.URL.Path, "ignoreHost", s.ignoreHost, "cacheable", cacheable)
	// Add the X-Cache header to the response
}

// asciiFormat returns a human-readable string representation of a duration in ASCII format (header-safe)
func asciiFormat(since time.Duration) string {
	if since > time.Second {
		return fmt.Sprintf("%.3fs", since.Seconds())
	} else if since > time.Millisecond {
		return fmt.Sprintf("%.3fms", float64(since.Microseconds())/1000.0)
	} else if since > time.Microsecond {
		return fmt.Sprintf("%.3fus", float64(since.Nanoseconds())/1000.0)
	} else {
		return fmt.Sprintf("%.3dns", since.Nanoseconds())
	}
}

// defaultMethod handles all other requests
// no attempt at caching is made
func (s *Server) defaultMethod(resp http.ResponseWriter, req *http.Request) {
	// clone the request to avoid modifying the original
	beReq := req.Clone(context.Background())
	// Clear the URI
	beReq.RequestURI = ""

	// URL scheme will be set by the backend

	// Use the Host header as the URL host if not already set
	if beReq.URL.Host == "" {
		beReq.URL.Host = beReq.Host
	}

	beResp, _ := s.backend.Fetch(beReq)
	defer beResp.Body.Close()
	for k, v := range beResp.Header {
		resp.Header()[k] = v
	}
	resp.WriteHeader(beResp.StatusCode)
	if req.Method != http.MethodHead {
		n, err := io.Copy(resp, beResp.Body)
		if err != nil {
			s.metrics.Errors.Inc()
			s.logger.Warn("write beResp.Body", "err", err)
		}
		s.logger.Info("body response written", "bytes", n)
	}
}

func versionString() string {
	return fmt.Sprintf("hazelnut %s", embeddedVersion)
}

func headerDenyList() []string {
	return []string{
		"connection",
		"keep-alive",
		"proxy-authenticate",
		"proxy-authorization",
		"te",
		"trailers",
		"transfer-encoding",
		"upgrade",
	}
}

// calculateTTL determines appropriate cache lifetime from response headers
// Returns 0 for objects that should use the default cache behavior (no expiration)
// Considers:
// - Cache-Control: max-age, s-maxage, no-cache, no-store, private, must-revalidate
// - Expires header
// - Age header
func calculateTTL(headers http.Header) time.Duration {
	// Check for Cache-Control directives that prevent caching
	cacheControl := headers.Get("Cache-Control")
	if cacheControl != "" {
		directives := strings.Split(cacheControl, ",")
		for _, directive := range directives {
			directive = strings.TrimSpace(directive)

			// Check for no-store directive - don't cache at all
			if directive == "no-store" {
				return 0 // Don't cache
			}

			// Check for private directive - typically shouldn't be cached by shared cache
			if directive == "private" {
				return 0
			}

			// Check for no-cache directive - can be stored but must be revalidated
			if directive == "no-cache" {
				return 0
			}

			// Check for must-revalidate
			if directive == "must-revalidate" {
				// We'll still allow caching but with caution
			}

			// Check for s-maxage (takes precedence over max-age for shared caches)
			if strings.HasPrefix(directive, "s-maxage=") {
				seconds, err := strconv.Atoi(strings.TrimPrefix(directive, "s-maxage="))
				if err == nil && seconds > 0 {
					return time.Duration(seconds) * time.Second
				}
			}

			// Check for max-age
			if strings.HasPrefix(directive, "max-age=") {
				seconds, err := strconv.Atoi(strings.TrimPrefix(directive, "max-age="))
				if err == nil && seconds > 0 {
					return time.Duration(seconds) * time.Second
				}
			}
		}
	}

	// Check Expires header if no max-age was found
	expires := headers.Get("Expires")
	if expires != "" {
		// Parse the expires header in various formats
		formats := []string{
			time.RFC1123,
			time.RFC1123Z,
			time.RFC850,
			time.ANSIC,
		}

		var expiresTime time.Time
		var err error

		// Try each format until we find one that works
		for _, format := range formats {
			expiresTime, err = time.Parse(format, expires)
			if err == nil {
				break
			}
		}

		if err == nil {
			// Calculate TTL as difference between expiration time and now
			ttl := time.Until(expiresTime)
			if ttl > 0 {
				// Account for Age header if present
				age := headers.Get("Age")
				if age != "" {
					ageSeconds, err := strconv.Atoi(age)
					if err == nil && ageSeconds > 0 {
						ttl -= time.Duration(ageSeconds) * time.Second
						if ttl <= 0 {
							return 0 // Already expired
						}
					}
				}
				return ttl
			}
			return 0 // Already expired
		}
	}

	// Default case: use default cache behavior
	return defaultTTL
}
