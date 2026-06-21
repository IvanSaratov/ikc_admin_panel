package admin

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
)

// auditRecorder is the minimal audit surface LoginRateLimitMiddleware
// needs. The concrete *audit.Service satisfies it; tests pass a stub.
type auditRecorder interface {
	Record(ctx context.Context, in audit.RecordInput) error
}

// RateLimiter is an in-memory per-IP token bucket. Tokens are refilled
// continuously based on elapsed wall-clock time, up to maxTokens per
// window. On Allow() the limiter decrements one token if any remain
// and returns true; otherwise it returns false along with the duration
// until the next token is available.
//
// State is process-local: counters reset on restart. For a single-host
// internal admin panel this is acceptable; if Mintrud Admin ever grows
// to multiple replicas, swap this for a shared store (SQLite row or
// Redis) — the Allow/retryAfter surface is what callers depend on.
type RateLimiter struct {
	maxTokens   int
	refillPerNs float64 // tokens added per nanosecond of elapsed time
	now         func() time.Time

	mu      sync.Mutex
	buckets map[string]*tokenBucket
}

// tokenBucket is the per-key refill state. tokens is a fractional
// counter so we can keep the bucket math continuous.
type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
}

// NewRateLimiter constructs a RateLimiter that allows maxTokens requests
// per window (a sliding window: tokens drain on demand and refill at
// maxTokens/window rate). Pass nil for nowFn to default to time.Now.
func NewRateLimiter(maxTokens int, window time.Duration, nowFn func() time.Time) *RateLimiter {
	if nowFn == nil {
		nowFn = time.Now
	}
	if maxTokens < 1 {
		maxTokens = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{
		maxTokens:   maxTokens,
		refillPerNs: float64(maxTokens) / float64(window),
		now:         nowFn,
		buckets:     make(map[string]*tokenBucket),
	}
}

// Allow consumes one token for key. Returns (true, 0) if the request
// is permitted; (false, retryAfter) if the bucket is empty, where
// retryAfter is the time the caller must wait for at least one token
// to be available.
func (r *RateLimiter) Allow(key string) (bool, time.Duration) {
	now := r.now()

	r.mu.Lock()
	defer r.mu.Unlock()

	bucket, ok := r.buckets[key]
	if !ok {
		bucket = &tokenBucket{tokens: float64(r.maxTokens), lastRefill: now}
		r.buckets[key] = bucket
	}

	// Refill: add tokens proportional to elapsed time, capped at maxTokens.
	elapsed := now.Sub(bucket.lastRefill).Nanoseconds()
	if elapsed > 0 {
		bucket.tokens += float64(elapsed) * r.refillPerNs
		if bucket.tokens > float64(r.maxTokens) {
			bucket.tokens = float64(r.maxTokens)
		}
		bucket.lastRefill = now
	}

	if bucket.tokens >= 1 {
		bucket.tokens -= 1
		return true, 0
	}

	// Compute how long until tokens >= 1.
	missing := 1.0 - bucket.tokens
	nsNeeded := int64(missing / r.refillPerNs)
	if nsNeeded < 1 {
		nsNeeded = 1
	}
	return false, time.Duration(nsNeeded)
}

// Reset clears all bucket state. Used by tests; production code does
// not call this.
func (r *RateLimiter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buckets = make(map[string]*tokenBucket)
}

// LoginRateLimitMiddleware enforces the rate limit ONLY on POST /login.
// Other routes are unaffected. On rejection it writes a 429 with a
// Retry-After header and a small HTML body, and audits the rejection
// so operators can alert on bursts.
//
// IP extraction uses r.RemoteAddr with the port stripped. The Mintrud
// Admin MVP runs behind a single reverse proxy in production; if the
// proxy strips X-Forwarded-For, add it as a secondary signal here.
func LoginRateLimitMiddleware(rl *RateLimiter, log *slog.Logger, auditSvc auditRecorder) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Cheap fast-path: only POST /login is rate-limited. This
			// guard means other admin routes never pay the mutex cost.
			if r.URL.Path != "/login" || r.Method != http.MethodPost {
				next.ServeHTTP(w, r)
				return
			}

			ip := clientIP(r)
			allowed, retryAfter := rl.Allow(ip)
			if allowed {
				next.ServeHTTP(w, r)
				return
			}

			// Audit the rejection with the IP as actor. This row is the
			// signal operators alert on for credential-stuffing bursts.
			if auditSvc != nil {
				actor := "rate_limit:" + ip
				ctx := audit.WithActor(r.Context(), actor)
				if err := auditSvc.Record(ctx, audit.RecordInput{
					Action:     "login.rate_limited",
					EntityType: "session",
					Actor:      actor,
					Details: map[string]any{
						"retry_after_seconds": int(retryAfter.Seconds() + 0.5),
					},
				}); err != nil {
					log.Error("audit login.rate_limited", slog.String("err", err.Error()))
				}
			}

			log.Warn("login rate limit exceeded",
				slog.String("ip", ip),
				slog.Duration("retry_after", retryAfter),
			)
			seconds := int(retryAfter.Seconds() + 0.5)
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`<html><body><h1>429 Too Many Requests</h1>` +
				`<p>Too many login attempts. Please wait ` + strconv.Itoa(seconds) + ` seconds and try again.</p>` +
				`</body></html>`))
		})
	}
}

// clientIP extracts the IP from r.RemoteAddr, stripping the port.
// Returns "unknown" when no usable address is present (which is rare;
// chi's httptest always sets it).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// Some test servers pass a bare host without a port.
		if r.RemoteAddr != "" {
			return r.RemoteAddr
		}
		return "unknown"
	}
	return host
}
