package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func setupTestRedis(t *testing.T) (*Client, *miniredis.Miniredis) {
	m := miniredis.RunT(t)
	c, err := New("redis://" + m.Addr())
	if err != nil {
		t.Fatalf("new redis client: %v", err)
	}
	return c, m
}

func TestEnqueueDelayed(t *testing.T) {
	c, _ := setupTestRedis(t)
	ctx := context.Background()

	if err := c.EnqueueDelayed(ctx, "delayed", []byte("job1"), time.Now().Unix()); err != nil {
		t.Fatalf("enqueue delayed: %v", err)
	}

	items, err := c.UniversalClient.ZRangeByScore(ctx, "delayed", &goredis.ZRangeBy{
		Min: "-inf",
		Max: "+inf",
	}).Result()
	if err != nil {
		t.Fatalf("zrange: %v", err)
	}
	if len(items) != 1 || items[0] != "job1" {
		t.Errorf("items = %v, want [job1]", items)
	}
}

func TestPromoteReadyDelayed(t *testing.T) {
	c, _ := setupTestRedis(t)
	ctx := context.Background()

	now := time.Now().Unix()
	_ = c.EnqueueDelayed(ctx, "delayed", []byte("ready"), now-1)
	_ = c.EnqueueDelayed(ctx, "delayed", []byte("future"), now+3600)

	if err := c.PromoteReadyDelayed(ctx, "delayed", "jobs", now); err != nil {
		t.Fatalf("promote: %v", err)
	}

	readyLen, _ := c.UniversalClient.LLen(ctx, "jobs").Result()
	if readyLen != 1 {
		t.Errorf("jobs len = %d, want 1", readyLen)
	}

	delayedLen, _ := c.UniversalClient.ZCard(ctx, "delayed").Result()
	if delayedLen != 1 {
		t.Errorf("delayed len = %d, want 1", delayedLen)
	}
}

func TestEnqueueDead(t *testing.T) {
	c, _ := setupTestRedis(t)
	ctx := context.Background()

	if err := c.EnqueueDead(ctx, "dead", []byte("job1")); err != nil {
		t.Fatalf("enqueue dead: %v", err)
	}

	length, _ := c.UniversalClient.LLen(ctx, "dead").Result()
	if length != 1 {
		t.Errorf("dead len = %d, want 1", length)
	}
}

func TestDequeue(t *testing.T) {
	c, m := setupTestRedis(t)
	ctx := context.Background()

	_ = c.Enqueue(ctx, "jobs", []byte("payload"))
	// Fast-forward miniredis so BRPop doesn't need to block
	m.FastForward(time.Second)
	payload, err := c.Dequeue(ctx, "jobs", time.Second)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if string(payload) != "payload" {
		t.Errorf("payload = %q, want payload", string(payload))
	}
}
