package backend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// Fetcher is an interface that both Client and Router implement
type Fetcher interface {
	Fetch(req *http.Request) (*http.Response, bool)
}

type Client struct {
	httpClient *http.Client
	target     string
	port       int
	scheme     string
	logger     *slog.Logger
}

// New creates a new backend Client that forces connections to the specified target host and port,
// while leaving the HTTP Host header and URL intact.
func New(logger *slog.Logger, target string, port int) *Client {
	dialer := &net.Dialer{
		Timeout: 30 * time.Second,
	}

	transport := &http.Transport{
		// Override the DialContext to always dial our fixed target and port.
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Instead of using the provided addr, use our target.
			fixedAddr := fmt.Sprintf("%s:%d", target, port)
			logger.Info("dialing backend", "addr", fixedAddr)
			return dialer.DialContext(ctx, network, fixedAddr)
		},
	}

	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	return &Client{
		httpClient: httpClient,
		target:     target,
		port:       port,
		scheme:     "https", // default scheme
		logger:     logger.With("package", "backend"),
	}
}

// SetScheme sets the scheme (http/https) to use for backend requests
func (c *Client) SetScheme(scheme string) {
	if scheme == "http" || scheme == "https" {
		c.scheme = scheme
	}
}

// GetScheme returns the current scheme
func (c *Client) GetScheme() string {
	return c.scheme
}

// Fetch fetches something from the backend.
func (c *Client) Fetch(beReq *http.Request) (*http.Response, bool) {
	// Set the URL scheme if not already set
	if beReq.URL.Scheme == "" {
		beReq.URL.Scheme = c.scheme
	}

	c.logger.Debug("fetching from backend",
		"url", beReq.URL.String(),
		"host", beReq.Host,
		"target", fmt.Sprintf("%s:%d", c.target, c.port))

	beResp, err := c.httpClient.Do(beReq)
	if err != nil {
		c.logger.Error("backend request failed, serving nuts",
			"error", err,
			"url", beReq.URL,
			"host", beReq.Host,
			"target", fmt.Sprintf("%s:%d", c.target, c.port))
		return nuts(), false
	}
	return beResp, true
}

// Router manages multiple backend clients based on virtual hosts
type Router struct {
	defaultBackend *Client
	backends       map[string]*Client
	mu             sync.RWMutex
	logger         *slog.Logger
}

// NewRouter creates a new backend router with the specified default backend
func NewRouter(logger *slog.Logger, defaultBackend *Client) *Router {
	return &Router{
		defaultBackend: defaultBackend,
		backends:       make(map[string]*Client),
		logger:         logger.With("package", "backend.router"),
	}
}

// AddBackend adds a backend for a specific virtual host
func (r *Router) AddBackend(host string, backend *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backends[host] = backend
	r.logger.Info("added backend for host", "host", host, "target", backend.target)
}

// GetBackend returns the backend for the specified host or the default backend if not found
func (r *Router) GetBackend(host string) *Client {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if backend, exists := r.backends[host]; exists {
		return backend
	}
	return r.defaultBackend
}

// Fetch routes the request to the appropriate backend based on the Host header
func (r *Router) Fetch(beReq *http.Request) (*http.Response, bool) {
	backend := r.GetBackend(beReq.Host)
	r.logger.Debug("routing request", "host", beReq.Host, "backend", backend.target)
	return backend.Fetch(beReq)
}

// GetScheme returns the scheme of the default backend
// This is needed for compatibility with tests that access this method
func (r *Router) GetScheme() string {
	return r.defaultBackend.GetScheme()
}

func nuts() *http.Response {
	header := http.Header{}
	header.Add("Content-Type", "text/plain")
	header.Add("X-Backend-Name", "nuts")

	bodyBytes := []byte("<html><body><h1>I have a confuse</h1></body></html>")
	body := io.NopCloser(bytes.NewBuffer(bodyBytes))

	return &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     header,
		Body:       body,
	}
}
