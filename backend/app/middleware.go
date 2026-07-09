package app

import (
	"net"
	"net/http"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/sirupsen/logrus"
)

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

func requestLogger(log logrus.FieldLogger) func(http.Handler) http.Handler {
	if log == nil {
		log = logrus.StandardLogger()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}

			next.ServeHTTP(rec, r)

			fields := logrus.Fields{
				"method":      r.Method,
				"path":        requestPath(r),
				"status":      rec.statusCode(),
				"duration_ms": time.Since(start).Milliseconds(),
				"remote_ip":   remoteIP(r),
			}
			if actor := audit.ActorFromContext(r.Context()); actor != "" {
				fields["actor"] = actor
			}
			log.WithFields(fields).Info("request")
		})
	}
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
