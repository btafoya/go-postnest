package webhook

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-postnest/postnest/internal/redis"
)

func TestDedup_NewMessage(t *testing.T) {
	m := miniredis.RunT(t)
	c, _ := redis.New("redis://" + m.Addr())
	h := &Handler{redis: c, secret: "test"}

	payload := map[string]any{"MessageID": "msg-123"}
	if !h.dedup(context.Background(), payload) {
		t.Error("expected dedup to return true for new message")
	}
}

func TestDedup_DuplicateMessage(t *testing.T) {
	m := miniredis.RunT(t)
	c, _ := redis.New("redis://" + m.Addr())
	h := &Handler{redis: c, secret: "test"}

	payload := map[string]any{"MessageID": "msg-456"}
	if !h.dedup(context.Background(), payload) {
		t.Fatal("expected first dedup to return true")
	}
	if h.dedup(context.Background(), payload) {
		t.Error("expected duplicate dedup to return false")
	}
}

func TestDedup_NoMessageID(t *testing.T) {
	m := miniredis.RunT(t)
	c, _ := redis.New("redis://" + m.Addr())
	h := &Handler{redis: c, secret: "test"}

	payload := map[string]any{"Other": "value"}
	if !h.dedup(context.Background(), payload) {
		t.Error("expected dedup to return true when MessageID is missing")
	}
}

func TestDedup_TTLEviction(t *testing.T) {
	m := miniredis.RunT(t)
	c, _ := redis.New("redis://" + m.Addr())
	h := &Handler{redis: c, secret: "test"}

	payload := map[string]any{"MessageID": "msg-ttl"}
	_ = h.dedup(context.Background(), payload)

	// Fast-forward past TTL (5 minutes)
	m.FastForward(6 * time.Minute)

	if !h.dedup(context.Background(), payload) {
		t.Error("expected dedup to return true after TTL expiry")
	}
}
