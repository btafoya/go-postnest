# Deferred Items

## Pre-existing Race Conditions in internal/workers

- **Found during:** Task 3 verification (`go test ./... -race -count=1`)
- **Package:** `github.com/go-postnest/postnest/internal/workers`
- **Tests affected:** `TestWorker_RetryAndDeadLetter`, `TestWorker_PromotesDelayed`
- **Issue:** Data races in worker pool test setup (unrelated to admin package changes)
- **Status:** Out of scope for 01-02 plan. Should be addressed in a dedicated worker stability plan.
