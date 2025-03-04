package frontend

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"github.com/perbu/hazelnut/backend"
	"github.com/perbu/hazelnut/cache"
	"io"
	"log/slog"
	"net/http"
	"time"
)

//go:embed .version
var embeddedVersion string

type Server struct {
	cache   *cache.Store
	backend *backend.Client
	srv     *http.Server
	handler http.Handler
	logger  *slog.Logger
}

func New(logger *slog.Logger, cache *cache.Store, backend *backend.Client, addr string) *Server {
	s := &Server{
		cache:   cache,
		backend: backend,
		logger:  logger.With("package", "frontend"),
	}
	s.srv = &http.Server{
		Addr:    addr,
		Handler: s,
	}
	logger.Info("frontend configured", "addr", addr)
	return s
}

func (s *Server) Run(ctx context.Context) error {

	go func() {
		<-ctx.Done()
		s.logger.Info("shutting down server")
		_ = s.srv.Shutdown(ctx)
	}()
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

// makeKey takes a http.Request and returns a 32 byte sha256 hash of the
// host and path of the request.
func makeKey(r *http.Request) []byte {
	sh := sha256.New()
	_, _ = sh.Write([]byte(r.Host))
	_, _ = sh.Write([]byte(r.URL.Path))
	return sh.Sum(nil)
}

// cacheable handles GET and HEAD requests, these can be cached and can have hits
func (s *Server) cacheable(resp http.ResponseWriter, req *http.Request) {
	key := makeKey(req)
	obj, found := s.cache.Get(key)
	if found {
		for k, v := range obj.Headers {
			resp.Header()[k] = v
		}
		resp.Header().Add("X-Cache", "hit")
		resp.WriteHeader(http.StatusOK)
		_, _ = resp.Write(obj.Body) // yolo
		return
	}
	// cache miss. fetch from backend
	beReq := req.Clone(context.Background())
	// clear the URI:
	beReq.RequestURI = ""

	// Ensure we have a scheme and host in the URL for Go's http client
	if beReq.URL.Scheme == "" {
		beReq.URL.Scheme = "http"
	}

	// Use the Host header as the URL host if not already set
	if beReq.URL.Host == "" {
		beReq.URL.Host = beReq.Host
	}

	beResp, cacheable := s.backend.Fetch(beReq)

	defer beResp.Body.Close()
	body, err := io.ReadAll(beResp.Body)
	if err != nil {
		http.Error(resp, err.Error(), http.StatusInternalServerError)
		return
	}
	// clean up headers before inserting into cache:
	for _, h := range headerDenyList() {
		beResp.Header.Del(h)
	}
	// add a Via header to the cached response
	beResp.Header.Add("Via", versionString())

	if cacheable {
		s.cache.Set(key, cache.ObjCore{
			Headers: beResp.Header,
			Body:    body,
		})
	}
	// write the response to the client
	for k, v := range beResp.Header {
		resp.Header()[k] = v
	}
	resp.WriteHeader(beResp.StatusCode)
	if _, err := resp.Write(body); err != nil {
		s.logger.Warn("write beResp.Body", "err", err)
	}
	// Add the X-Cache header to the response
	resp.Header().Add("X-Cache", "miss")

}

// defaultMethod handles all other requests
// no attempt at caching is made
func (s *Server) defaultMethod(resp http.ResponseWriter, req *http.Request) {
	// clone the request to avoid modifying the original
	beReq := req.Clone(context.Background())
	// Clear the URI
	beReq.RequestURI = ""

	// Ensure we have a scheme and host in the URL for Go's http client
	if beReq.URL.Scheme == "" {
		beReq.URL.Scheme = "http"
	}

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
	n, err := io.Copy(resp, beResp.Body)
	if err != nil {
		s.logger.Warn("write beResp.Body", "err", err)
	}
	s.logger.Info("defaultMethod", "bytes", n)
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
