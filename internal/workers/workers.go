package workers

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/go-postnest/postnest/internal/redis"
	"github.com/google/uuid"
)

const (
	queueJobs       = "queue:jobs"
	queueDelayed    = "queue:jobs:delayed"
	queueDead       = "queue:jobs:dead"
)

// Job represents a queued background job.
type Job struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"max_attempts"`
	CreatedAt   int64           `json:"created_at"`
	ScheduledAt int64           `json:"scheduled_at"`
}

// Pool is a worker pool that consumes jobs from Redis.
type Pool struct {
	redis        *redis.Client
	logger       *slog.Logger
	concurrency  int
	pollInterval time.Duration
	processors   map[string]Processor
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	nowFunc      func() time.Time
}

func (p *Pool) now() time.Time {
	if p.nowFunc != nil {
		return p.nowFunc()
	}
	return time.Now()
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
	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.worker(ctx)
		}()
	}
}

// Stop signals workers to stop and waits for in-flight jobs to finish.
func (p *Pool) Stop(ctx context.Context) error {
	if p.cancel != nil {
		p.cancel()
	}
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Pool) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Promote any delayed jobs whose time has come.
		if err := p.redis.PromoteReadyDelayed(ctx, queueDelayed, queueJobs, p.now().Unix()); err != nil {
			p.logger.Error("promote delayed jobs failed", "error", err)
		}

		payload, err := p.redis.Dequeue(ctx, queueJobs, p.pollInterval)
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

		// Skip if job is scheduled for the future.
		if job.ScheduledAt > p.now().Unix() {
			b, _ := json.Marshal(job)
			if err := p.redis.EnqueueDelayed(ctx, queueDelayed, b, job.ScheduledAt); err != nil {
				p.logger.Error("re-delay job failed", "error", err)
			}
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
				backoff := time.Duration(job.Attempts) * 5 * time.Second
				job.ScheduledAt = p.now().Add(backoff).Unix()
				b, _ := json.Marshal(job)
				if err := p.redis.EnqueueDelayed(ctx, queueDelayed, b, job.ScheduledAt); err != nil {
					p.logger.Error("enqueue delayed job failed", "error", err)
				}
			} else {
				b, _ := json.Marshal(job)
				if err := p.redis.EnqueueDead(ctx, queueDead, b); err != nil {
					p.logger.Error("enqueue dead job failed", "error", err)
				}
				p.logger.Error("job dead-lettered", "type", job.Type, "id", job.ID, "attempts", job.Attempts)
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
		ID:          uuid.Must(uuid.NewV7()).String(),
		Type:        jobType,
		Payload:     b,
		MaxAttempts: 3,
		CreatedAt:   p.now().Unix(),
		ScheduledAt: p.now().Unix(),
	}
	jb, _ := json.Marshal(job)
	return p.redis.Enqueue(ctx, queueJobs, jb)
}
