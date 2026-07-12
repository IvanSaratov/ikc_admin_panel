package admin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"go.uber.org/zap"
)

func TestRateLimiter_AllowsRequestsUnderLimit(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(3, time.Minute, nil)
	for i := 0; i < 3; i++ {
		ok, retry := rl.Allow("1.2.3.4")
		if !ok {
			t.Fatalf("attempt %d: expected allow, got deny (retry=%v)", i+1, retry)
		}
		if retry != 0 {
			t.Errorf("attempt %d: retry=%v, want 0 when allowed", i+1, retry)
		}
	}
}

func TestRateLimiter_BlocksAfterLimitExceeded(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(2, time.Minute, nil)
	// Drain the bucket.
	for i := 0; i < 2; i++ {
		if ok, _ := rl.Allow("1.2.3.4"); !ok {
			t.Fatalf("attempt %d: expected allow", i+1)
		}
	}
	ok, retry := rl.Allow("1.2.3.4")
	if ok {
		t.Fatalf("attempt 3: expected deny, got allow")
	}
	if retry <= 0 {
		t.Errorf("retry after deny = %v, want > 0", retry)
	}
	// With 2 tokens per minute, retry after exhaustion must be roughly
	// half a window — allow generous slack for slow CI.
	if retry > 35*time.Second {
		t.Errorf("retry = %v, want <= 35s", retry)
	}
}

func TestRateLimiter_RefillsOverTime(t *testing.T) {
	t.Parallel()

	clock := time.Now()
	rl := NewRateLimiter(2, time.Minute, func() time.Time { return clock })

	// Drain.
	for i := 0; i < 2; i++ {
		if ok, _ := rl.Allow("1.2.3.4"); !ok {
			t.Fatalf("attempt %d: expected allow", i+1)
		}
	}
	// Advance clock by 30s; tokens should refill to 1 (2/min * 30s).
	clock = clock.Add(30 * time.Second)

	ok, _ := rl.Allow("1.2.3.4")
	if !ok {
		t.Errorf("after 30s refill: expected allow (1 token), got deny")
	}
	// Next request must be denied (only 1 token after refill, already spent).
	ok, _ = rl.Allow("1.2.3.4")
	if ok {
		t.Errorf("after spending refilled token: expected deny, got allow")
	}
}

func TestRateLimiter_DifferentKeysIndependent(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(1, time.Minute, nil)
	// First IP exhausts its bucket.
	if ok, _ := rl.Allow("1.2.3.4"); !ok {
		t.Fatalf("first IP first attempt should be allowed")
	}
	if ok, _ := rl.Allow("1.2.3.4"); ok {
		t.Fatalf("first IP second attempt should be denied")
	}
	// Second IP must have its own bucket.
	if ok, _ := rl.Allow("5.6.7.8"); !ok {
		t.Errorf("second IP first attempt should be allowed (independent bucket)")
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(100, time.Minute, nil)
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := rl.Allow("shared-ip"); ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowed != 50 {
		t.Errorf("allowed = %d, want 50 (no race on counter)", allowed)
	}
}

func TestLoginRateLimitMiddleware_AllowsNonLoginPaths(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(1, time.Minute, nil)
	// Drain the bucket for one IP.
	rl.Allow("9.9.9.9")

	called := false
	handler := LoginRateLimitMiddleware(rl, zap.NewNop(), nil)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}),
	)

	for _, path := range []string{"/", "/programs", "/logout", "/login", "/api/session", "/api/logout"} {
		// GET /login must NOT be rate-limited (form render is cheap).
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "9.9.9.9:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !called {
			t.Errorf("%s: handler not called", path)
		}
		if rec.Code == http.StatusTooManyRequests {
			t.Errorf("%s: rate-limited (should only happen on POST /login)", path)
		}
		called = false
	}
}

func TestLoginRateLimitMiddleware_BlocksLoginPosts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		path        string
		contentType string
		body        string
		wantType    string
		wantBody    string
	}{
		{
			name:        "legacy",
			path:        "/login",
			contentType: "application/x-www-form-urlencoded",
			body:        "login=admin&password=wrong",
			wantType:    "text/html",
			wantBody:    "Too Many Requests",
		},
		{
			name:        "api",
			path:        "/api/login",
			contentType: "application/json",
			body:        `{"login":"admin","password":"wrong"}`,
			wantType:    "application/json",
			wantBody:    "rate_limited",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rl := NewRateLimiter(1, time.Minute, nil)
			// Drain.
			rl.Allow("9.9.9.9")

			handler := LoginRateLimitMiddleware(rl, zap.NewNop(), nil)(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Fatalf("handler should not be called when rate-limited")
				}),
			)

			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			req.RemoteAddr = "9.9.9.9:1234"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusTooManyRequests {
				t.Errorf("status = %d, want 429", rec.Code)
			}
			retry := rec.Header().Get("Retry-After")
			if retry == "" {
				t.Errorf("Retry-After header missing")
			} else if n, err := strconv.Atoi(retry); err != nil || n < 1 {
				t.Errorf("Retry-After = %q, want positive integer seconds", retry)
			}
			if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, tc.wantType) {
				t.Errorf("Content-Type = %q, want %q", contentType, tc.wantType)
			}
			body, _ := io.ReadAll(rec.Body)
			if !strings.Contains(string(body), tc.wantBody) {
				t.Errorf("body = %q, want %q", string(body), tc.wantBody)
			}
		})
	}
}

func TestLoginRateLimitMiddleware_AuditsRejection(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(1, time.Minute, nil)
	rl.Allow("9.9.9.9")

	auditCalled := make(chan struct{}, 1)
	wrapper := &auditSpy{onRecord: func(in audit.RecordInput) {
		if in.Action == "login.rate_limited" {
			auditCalled <- struct{}{}
		}
	}}
	handler := LoginRateLimitMiddleware(rl, zap.NewNop(), wrapper)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	)

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	select {
	case <-auditCalled:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("login.rate_limited audit row not written within 2s")
	}
}

func TestClientIP(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		addr   string
		wantIP string
	}{
		{"ipv4 with port", "192.0.2.1:54321", "192.0.2.1"},
		{"ipv6 with port", "[2001:db8::1]:54321", "2001:db8::1"},
		{"bare host fallback", "192.0.2.1", "192.0.2.1"},
		{"empty fallback", "", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.addr
			if got := clientIP(req); got != tc.wantIP {
				t.Errorf("clientIP(%q) = %q, want %q", tc.addr, got, tc.wantIP)
			}
		})
	}
}

// auditSpy satisfies the *audit.Service shape needed by the middleware
// for testing — it doesn't persist anything, just calls onRecord.
type auditSpy struct {
	onRecord func(audit.RecordInput)
}

func (a *auditSpy) Record(_ context.Context, in audit.RecordInput) error {
	if a.onRecord != nil {
		a.onRecord(in)
	}
	return nil
}
