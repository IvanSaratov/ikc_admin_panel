package app

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/api"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"go.uber.org/zap"
)

type requestLogStateKey struct{}

type requestLogState struct {
	actor string
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status != 0 {
		return
	}
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(body)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

func requestLogger(log *zap.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = zap.NewNop()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			state := &requestLogState{actor: audit.ActorFromContext(r.Context())}
			ctx := context.WithValue(r.Context(), requestLogStateKey{}, state)

			next.ServeHTTP(rec, r.WithContext(ctx))

			fields := []zap.Field{
				zap.String("method", r.Method),
				zap.String("path", requestPath(r)),
				zap.Int("status", rec.statusCode()),
				zap.Int64("duration_ms", time.Since(start).Milliseconds()),
				zap.String("remote_ip", remoteIP(r)),
			}
			if traceID := api.TraceID(r.Context()); traceID != "" {
				fields = append(fields, zap.String("trace_id", traceID))
			}
			if state.actor != "" {
				fields = append(fields, zap.String("actor", state.actor))
			}
			log.Info("request", fields...)
		})
	}
}

// captureRequestLogActor passes an identity established by inner auth
// middleware back to the outer request logger without exposing it in headers.
func captureRequestLogActor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if state, ok := r.Context().Value(requestLogStateKey{}).(*requestLogState); ok {
			state.actor = audit.ActorFromContext(r.Context())
		}
		next.ServeHTTP(w, r)
	})
}

// withActor переносит логин из auth-контекста в audit-контекст для аудита и логов.
func withActor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if login := admin.UserLoginFromContext(r.Context()); login != "" {
			r = r.WithContext(audit.WithActor(r.Context(), login))
		}
		next.ServeHTTP(w, r)
	})
}

func requestPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return r.URL.Path
}

func remoteIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
