package workers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-postnest/postnest/internal/redis"
)

func setupTestPool(t *testing.T) (*Pool, *miniredis.Miniredis, *time.Time) {
	m := miniredis.RunT(t)
	c, err := redis.New("redis://" + m.Addr())
	if err != nil {
		t.Fatalf("new redis: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	p := NewPool(c, logger, 1, 100*time.Millisecond)
	now := time.Now()
	p.nowFunc = func() time.Time { return now }
	return p, m, &now
}

type testProcessor struct {
	called int
	fail   bool
}

func (tp *testProcessor) Process(ctx context.Context, job *Job) error {
	tp.called++
	if tp.fail {
		return errors.New("process error")
	}
	return nil
}

func TestEnqueue_JobHasUUID(t *testing.T) {
	p, _, _ := setupTestPool(t)
	_ = p
	ctx := context.Background()

	if err := p.Enqueue(ctx, "test", map[string]string{"key": "value"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	payload, err := p.redis.Dequeue(ctx, queueJobs, time.Second)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	var job Job
	if err := json.Unmarshal(payload, &job); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if job.ID == "" {
		t.Error("job ID is empty")
	}
	if job.Type != "test" {
		t.Errorf("type = %q, want test", job.Type)
	}
	if job.ScheduledAt == 0 {
		t.Error("ScheduledAt not set")
	}
}

func TestWorker_RetryAndDeadLetter(t *testing.T) {
	p, m, nowPtr := setupTestPool(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tp := &testProcessor{fail: true}
	p.Register("failjob", tp)
	p.Start(ctx)

	// Enqueue a job that will fail 2 times (max attempts = 2)
	job := Job{
		ID:          "retry-test",
		Type:        "failjob",
		Payload:     []byte("{}"),
		MaxAttempts: 2,
		CreatedAt:   p.now().Unix(),
		ScheduledAt: p.now().Unix(),
	}
	b, _ := json.Marshal(job)
	_ = p.redis.Enqueue(ctx, queueJobs, b)

	// Give first attempt time to run and be delayed (miniredis BRPop min 1s)
	time.Sleep(1500 * time.Millisecond)

	// Advance clock past backoff (5s) and miniredis time
	*nowPtr = nowPtr.Add(10 * time.Second)
	m.FastForward(10 * time.Second)
	time.Sleep(1500 * time.Millisecond)

	// Second attempt should have run and dead-lettered
	deadLen, _ := p.redis.UniversalClient.LLen(ctx, queueDead).Result()
	if deadLen != 1 {
		t.Errorf("dead len = %d, want 1", deadLen)
	}

	delayedLen, _ := p.redis.UniversalClient.ZCard(ctx, queueDelayed).Result()
	if delayedLen != 0 {
		t.Errorf("delayed len = %d, want 0", delayedLen)
	}

	p.Stop(context.Background())
}

func TestWorker_PromotesDelayed(t *testing.T) {
	p, m, _ := setupTestPool(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tp := &testProcessor{}
	p.Register("delayedjob", tp)
	p.Start(ctx)

	// Enqueue a delayed job scheduled in the past so it promotes immediately
	job := Job{
		ID:          "delayed-1",
		Type:        "delayedjob",
		Payload:     []byte("{}"),
		MaxAttempts: 3,
		CreatedAt:   p.now().Unix(),
		ScheduledAt: p.now().Add(-time.Hour).Unix(),
	}
	b, _ := json.Marshal(job)
	_ = p.redis.EnqueueDelayed(ctx, queueDelayed, b, job.ScheduledAt)

	// Advance both clocks
	m.FastForward(time.Second)
	time.Sleep(300 * time.Millisecond)

	found := false
	for i := 0; i < 30; i++ {
		if tp.called > 0 {
			found = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !found {
		t.Error("delayed job was not processed")
	}

	p.Stop(context.Background())
}
