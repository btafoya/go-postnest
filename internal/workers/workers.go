package workers


import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-postnest/postnest/internal/redis"
)

// Job represents a queued background job.
type Job struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Payload  json.RawMessage `json:"payload"`
	Attempts int             `json:"attempts"`
	MaxAttempts int          `json:"max_attempts"`
	CreatedAt int64          `json:"created_at"`
}

// Pool is a worker pool that consumes jobs from Redis.
type Pool struct {
	redis       *redis.Client
	logger      *slog.Logger
	concurrency int
	pollInterval time.Duration
	processors  map[string]Processor
}

// Processor handles a specific job type.
type Processor interface {
	Process(ctx context.Context, job *Job) error
}

// NewPool creates a worker pool.
func NewPool(r *redis.Client, logger *slog.Logger, concurrency int, pollInterval time.Duration) *Pool {
	return &Pool{
		redis:        r,
		logger:       logger,
		concurrency:  concurrency,
		pollInterval: pollInterval,
		processors:   make(map[string]Processor),
	}
}

// Register adds a processor for a job type.
func (p *Pool) Register(jobType string, proc Processor) {
	p.processors[jobType] = proc
}

// Start begins consuming jobs.
func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.concurrency; i++ {
		go p.worker(ctx)
	}
}

func (p *Pool) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		payload, err := p.redis.Dequeue(ctx, "queue:jobs", p.pollInterval)
		if err != nil {
			p.logger.Error("dequeue error", "error", err)
			continue
		}
		if len(payload) == 0 {
			continue
		}
		var job Job
		if err := json.Unmarshal(payload, &job); err != nil {
			p.logger.Error("unmarshal job", "error", err)
			continue
		}
		proc, ok := p.processors[job.Type]
		if !ok {
			p.logger.Warn("no processor for job type", "type", job.Type)
			continue
		}
		if err := proc.Process(ctx, &job); err != nil {
			p.logger.Error("job failed", "type", job.Type, "error", err)
			job.Attempts++
			if job.Attempts < job.MaxAttempts {
				b, _ := json.Marshal(job)
				_ = p.redis.Enqueue(ctx, "queue:jobs", b)
			}
		}
	}
}

// Enqueue sends a job to the worker queue.
func (p *Pool) Enqueue(ctx context.Context, jobType string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	job := Job{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		Type:        jobType,
		Payload:     b,
		MaxAttempts: 3,
		CreatedAt:   time.Now().Unix(),
	}
	jb, _ := json.Marshal(job)
	return p.redis.Enqueue(ctx, "queue:jobs", jb)
}
