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

// PromoteReadyDelayed moves jobs whose score is <= maxScore from a sorted set back to a list.
func (c *Client) PromoteReadyDelayed(ctx context.Context, delayedQueue, targetQueue string, maxScore int64) error {
	ready, err := c.UniversalClient.ZRangeByScore(ctx, delayedQueue, &redis.ZRangeBy{
		Min: "-inf",
		Max: fmt.Sprintf("%d", maxScore),
	}).Result()
	if err != nil || len(ready) == 0 {
		return err
	}
	for _, item := range ready {
		if err := c.UniversalClient.LPush(ctx, targetQueue, item).Err(); err != nil {
			return err
		}
		if err := c.UniversalClient.ZRem(ctx, delayedQueue, item).Err(); err != nil {
			return err
		}
	}
	return nil
}

// EnqueueDelayed adds a job to a delayed sorted-set queue.
func (c *Client) EnqueueDelayed(ctx context.Context, queue string, payload []byte, score int64) error {
	return c.UniversalClient.ZAdd(ctx, queue, redis.Z{Score: float64(score), Member: string(payload)}).Err()
}

// EnqueueDead pushes a job to a dead-letter queue.
func (c *Client) EnqueueDead(ctx context.Context, queue string, payload []byte) error {
	return c.UniversalClient.LPush(ctx, queue, payload).Err()
}
