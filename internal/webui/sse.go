package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// SSEHub manages Server-Sent Events connections and broadcasts messages.
type SSEHub struct {
	clients    map[chan string]bool
	register   chan chan string
	unregister chan chan string
	broadcast  chan string
	redis      *redis.Client
	log        *slog.Logger
}

// NewSSEHub creates an SSE hub connected to Redis.
func NewSSEHub(redisURL string, log *slog.Logger) *SSEHub {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Warn("invalid redis url, sse will run without redis", "url", redisURL, "error", err)
		opt = &redis.Options{Addr: "localhost:6379"}
	}
	rdb := redis.NewClient(opt)

	return &SSEHub{
		clients:    make(map[chan string]bool),
		register:   make(chan chan string),
		unregister: make(chan chan string),
		broadcast:  make(chan string),
		redis:      rdb,
		log:        log,
	}
}

// Run starts the SSE hub goroutines.
func (h *SSEHub) Run() {
	go h.manageClients()
	go h.subscribeRedis()
}

func (h *SSEHub) manageClients() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.log.Info("sse client connected", "clients", len(h.clients))
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client)
				h.log.Info("sse client disconnected", "clients", len(h.clients))
			}
		case msg := <-h.broadcast:
			for client := range h.clients {
				select {
				case client <- msg:
				default:
					// Client is slow, skip
				}
			}
		}
	}
}

func (h *SSEHub) subscribeRedis() {
	ctx := context.Background()
	// Subscribe to channels that the backend publishes to
	pubsub := h.redis.Subscribe(ctx, "mailbox:updates", "message:new", "delivery:events")
	defer pubsub.Close()

	ch := pubsub.Channel()
	for msg := range ch {
		event := map[string]any{
			"type":    msg.Channel,
			"payload": msg.Payload,
			"time":    time.Now().UTC().Format(time.RFC3339),
		}
		data, _ := json.Marshal(event)
		h.broadcast <- string(data)
	}
}

// Handler returns a Gin handler for SSE connections.
func (h *SSEHub) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			c.String(http.StatusInternalServerError, "streaming not supported")
			return
		}

		// Send initial connection event
		fmt.Fprintf(c.Writer, "event: connected\ndata: %s\n\n", `{"status":"ok"}`)
		flusher.Flush()

		msgChan := make(chan string)
		h.register <- msgChan
		defer func() { h.unregister <- msgChan }()

		// Send heartbeat every 30s to keep connection alive
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case msg := <-msgChan:
				fmt.Fprintf(c.Writer, "data: %s\n\n", msg)
				flusher.Flush()
			case <-ticker.C:
				fmt.Fprintf(c.Writer, "event: heartbeat\ndata: %s\n\n", `{"time":"`+time.Now().UTC().Format(time.RFC3339)+`"}`)
				flusher.Flush()
			case <-c.Request.Context().Done():
				return
			}
		}
	}
}

// Broadcast sends a message to all connected SSE clients.
func (h *SSEHub) Broadcast(eventType string, payload any) {
	data, _ := json.Marshal(map[string]any{
		"type":    eventType,
		"payload": payload,
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
	h.broadcast <- string(data)
}
