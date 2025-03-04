package backend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

type Client struct {
	httpClient *http.Client
	target     string
	port       int
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
		logger:     logger.With("package", "backend"),
	}
}

// Fetch fetches something from the backend.
func (c *Client) Fetch(beReq *http.Request) (*http.Response, bool) {
	c.logger.Debug("fetching from backend",
		"url", beReq.URL,
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
