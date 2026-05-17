package webui

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

//go:embed all:dist
var distFS embed.FS

// NewRouter creates a Gin router with proxy, SSE, and static file serving.
func NewRouter(cfg Config) *gin.Engine {
	// Register custom validators
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		_ = v.RegisterValidation("email", emailValidator)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(structuredLogger(cfg.Log))
	r.Use(corsMiddleware(cfg.AllowedOrigins))

	// SSE endpoint for real-time updates
	sseHub := NewSSEHub(cfg.RedisURL, cfg.Log)
	go sseHub.Run()
	r.GET("/events", sseHub.Handler())

	// API proxy to backend
	proxy := NewAPIProxy(cfg.APIBaseURL, cfg.Log)

	// Proxy backend API routes
	apiRoutes := []string{
		"/api/*path",
		"/admin/*path",
		"/webhooks/*path",
		"/auth/*path",
		"/healthz",
		"/.well-known/*path",
		"/dav/*path",
	}
	for _, route := range apiRoutes {
		r.Any(route, proxy.Handle)
	}

	// Serve embedded SPA
	r.NoRoute(func(c *gin.Context) {
		// Don't interfere with API routes
		if strings.HasPrefix(c.Request.URL.Path, "/api/") ||
			strings.HasPrefix(c.Request.URL.Path, "/admin/") ||
			strings.HasPrefix(c.Request.URL.Path, "/auth/") ||
			strings.HasPrefix(c.Request.URL.Path, "/webhooks/") ||
			strings.HasPrefix(c.Request.URL.Path, "/events") ||
			strings.HasPrefix(c.Request.URL.Path, "/dav/") ||
			strings.HasPrefix(c.Request.URL.Path, "/.well-known/") ||
			c.Request.URL.Path == "/healthz" {
			c.Next()
			return
		}

		// Serve index.html for SPA routing
		if strings.Contains(c.Request.Header.Get("Accept"), "text/html") {
			data, err := distFS.ReadFile("dist/index.html")
			if err != nil {
				c.String(http.StatusNotFound, "index.html not found — build the frontend first")
				return
			}
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(http.StatusOK, string(data))
			return
		}

		// Try to serve static file from embed
		path := "dist" + c.Request.URL.Path
		data, err := distFS.ReadFile(path)
		if err != nil {
			// Fallback to index.html for SPA routes
			data, err = distFS.ReadFile("dist/index.html")
			if err != nil {
				c.String(http.StatusNotFound, "index.html not found — build the frontend first")
				return
			}
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(http.StatusOK, string(data))
			return
		}

		// Set content type based on extension
		contentType := "application/octet-stream"
		switch {
		case strings.HasSuffix(path, ".js"):
			contentType = "application/javascript"
		case strings.HasSuffix(path, ".css"):
			contentType = "text/css"
		case strings.HasSuffix(path, ".svg"):
			contentType = "image/svg+xml"
		case strings.HasSuffix(path, ".png"):
			contentType = "image/png"
		case strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"):
			contentType = "image/jpeg"
		case strings.HasSuffix(path, ".woff2"):
			contentType = "font/woff2"
		case strings.HasSuffix(path, ".woff"):
			contentType = "font/woff"
		case strings.HasSuffix(path, ".ttf"):
			contentType = "font/ttf"
		}
		c.Header("Content-Type", contentType)
		c.Status(http.StatusOK)
		c.Writer.Write(data)
	})

	return r
}

func structuredLogger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		log.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency", latency,
			"client_ip", c.ClientIP(),
		)
	}
}

func corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowed := false
		for _, o := range allowedOrigins {
			if o == origin || o == "*" {
				allowed = true
				break
			}
		}
		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, X-Domain-ID, X-CSRF-Token")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func emailValidator(fl validator.FieldLevel) bool {
	email := fl.Field().String()
	return strings.Contains(email, "@") && strings.Contains(email, ".")
}

// DistFS returns the embedded dist filesystem for testing.
func DistFS() fs.FS {
	sub, _ := fs.Sub(distFS, "dist")
	return sub
}
