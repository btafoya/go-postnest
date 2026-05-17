package webui

import (
	"log/slog"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
)

// APIProxy proxies requests to the backend API server.
type APIProxy struct {
	target *url.URL
	proxy  *httputil.ReverseProxy
	log    *slog.Logger
}

// NewAPIProxy creates a reverse proxy to the backend.
func NewAPIProxy(apiBaseURL string, log *slog.Logger) *APIProxy {
	target, err := url.Parse(apiBaseURL)
	if err != nil {
		log.Error("invalid api base url", "url", apiBaseURL, "error", err)
		panic(err)
	}
	return &APIProxy{
		target: target,
		proxy:  httputil.NewSingleHostReverseProxy(target),
		log:    log,
	}
}

// Handle proxies the request to the backend.
func (p *APIProxy) Handle(c *gin.Context) {
	// Preserve the original request path
	c.Request.URL.Scheme = p.target.Scheme
	c.Request.URL.Host = p.target.Host
	c.Request.Host = p.target.Host

	// Ensure cookies are forwarded properly
	if cookie := c.Request.Header.Get("Cookie"); cookie != "" {
		c.Request.Header.Set("Cookie", cookie)
	}

	// Pass original client IP for rate limiting and logging
	if clientIP := c.ClientIP(); clientIP != "" {
		c.Request.Header.Set("X-Forwarded-For", clientIP)
	}

	p.log.Debug("proxying request",
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"target", p.target.String(),
	)

	p.proxy.ServeHTTP(c.Writer, c.Request)
}
