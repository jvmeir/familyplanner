package server

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a small per-key token-bucket limiter used to throttle
// passphrase attempts on /login and /pair. It is in-memory (single-process,
// which matches this app's single-container deployment) and self-prunes idle
// buckets so the map can't grow unbounded.
type rateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*tokenBucket
	burst     float64
	refill    float64 // tokens regained per second
	now       func() time.Time
	lastPrune time.Time
}

type tokenBucket struct {
	tokens float64
	seen   time.Time
}

// newRateLimiter permits up to burst attempts, regaining one token every
// per/burst — i.e. a sustained rate of burst attempts per per window.
func newRateLimiter(burst int, per time.Duration, now func() time.Time) *rateLimiter {
	if now == nil {
		now = time.Now
	}
	return &rateLimiter{
		buckets: map[string]*tokenBucket{},
		burst:   float64(burst),
		refill:  float64(burst) / per.Seconds(),
		now:     now,
	}
}

// allow consumes a token for key, returning false when the bucket is empty.
func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.prune(now)
	b := l.buckets[key]
	if b == nil {
		b = &tokenBucket{tokens: l.burst, seen: now}
		l.buckets[key] = b
	}
	b.tokens += l.refill * now.Sub(b.seen).Seconds()
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.seen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// prune drops idle buckets (checked at most once a minute) to bound memory.
func (l *rateLimiter) prune(now time.Time) {
	if now.Sub(l.lastPrune) < time.Minute {
		return
	}
	l.lastPrune = now
	for k, b := range l.buckets {
		if now.Sub(b.seen) > 10*time.Minute {
			delete(l.buckets, k)
		}
	}
}

// limit is middleware that rejects requests from a key over its budget with 429.
func (l *rateLimiter) limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "30")
			http.Error(w, "Te veel pogingen. Probeer het straks opnieuw.", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP is the request's source IP (host part of RemoteAddr).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
