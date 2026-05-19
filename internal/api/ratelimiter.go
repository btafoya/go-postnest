package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// DynamicRateLimiter is a per-IP token-bucket rate limiter whose limit is read at runtime.
type DynamicRateLimiter struct {
	getLimit func() int
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	lastSeen map[string]time.Time
}

// NewDynamicRateLimiter creates a rate limiter with a runtime-configurable limit.
func NewDynamicRateLimiter(getLimit func() int) *DynamicRateLimiter {
	rl := &DynamicRateLimiter{
		getLimit: getLimit,
		limiters: make(map[string]*rate.Limiter),
		lastSeen: make(map[string]time.Time),
	}
	go rl.cleanup()
	return rl
}

func (rl *DynamicRateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.lastSeen[ip] = time.Now()
	if l, ok := rl.limiters[ip]; ok {
		return l
	}
	limit := rl.getLimit()
	if limit <= 0 {
		limit = 100
	}
	rps := rate.Limit(limit) / 60
	burst := limit
	l := rate.NewLimiter(rps, burst)
	rl.limiters[ip] = l
	return l
}

// Handler returns chi-compatible middleware.
func (rl *DynamicRateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			ip = strings.Split(xf, ",")[0]
		}
		ip = strings.TrimSpace(ip)
		lim := rl.getLimiter(ip)
		if !lim.Allow() {
			w.Header().Set("Retry-After", "60")
			WriteError(w, ErrRateLimited)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *DynamicRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-10 * time.Minute)
		for ip, seen := range rl.lastSeen {
			if seen.Before(cutoff) {
				delete(rl.limiters, ip)
				delete(rl.lastSeen, ip)
			}
		}
		rl.mu.Unlock()
	}
}
