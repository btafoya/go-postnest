package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps go-redis with app-specific helpers.
type Client struct {
	redis.UniversalClient
}

// New creates a Redis client from a URL.
func New(redisURL string) (*Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &Client{UniversalClient: client}, nil
}

// Publish sends a message to a channel.
func (c *Client) Publish(ctx context.Context, channel string, message string) error {
	return c.UniversalClient.Publish(ctx, channel, message).Err()
}

// Enqueue pushes a job onto a Redis list.
func (c *Client) Enqueue(ctx context.Context, queue string, payload []byte) error {
	return c.UniversalClient.LPush(ctx, queue, payload).Err()
}

// Dequeue blocks until a job is available on the queue.
func (c *Client) Dequeue(ctx context.Context, queue string, timeout time.Duration) ([]byte, error) {
	res, err := c.UniversalClient.BRPop(ctx, timeout, queue).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(res) < 2 {
		return nil, nil
	}
	return []byte(res[1]), nil
}
