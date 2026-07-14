package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/alexedwards/scs/v2"
	"go.uber.org/zap"
)

type Server struct {
	httpServer *http.Server
	handlers   *handlerLifecycle
}

type handlerLifecycle struct {
	next    http.Handler
	mu      sync.Mutex
	closing bool
	active  int
	drained chan struct{}
}

func newHandlerLifecycle(next http.Handler) *handlerLifecycle {
	return &handlerLifecycle{next: next, drained: make(chan struct{})}
}

func (lifecycle *handlerLifecycle) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	lifecycle.mu.Lock()
	if lifecycle.closing {
		lifecycle.mu.Unlock()
		http.Error(writer, "server is shutting down", http.StatusServiceUnavailable)
		return
	}
	lifecycle.active++
	lifecycle.mu.Unlock()

	defer lifecycle.done()
	lifecycle.next.ServeHTTP(writer, request)
}

func (lifecycle *handlerLifecycle) done() {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	lifecycle.active--
	if lifecycle.closing && lifecycle.active == 0 {
		close(lifecycle.drained)
	}
}

func (lifecycle *handlerLifecycle) stop() <-chan struct{} {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if !lifecycle.closing {
		lifecycle.closing = true
		if lifecycle.active == 0 {
			close(lifecycle.drained)
		}
	}
	return lifecycle.drained
}

// NewServer builds the HTTP server.
//
// All auth wiring (session manager, CSRF middleware) is created here so
// the caller (main.go) doesn't have to repeat env-handling logic. If
// any of the underlying constructors fails (e.g. bad CSRF key), the
// error is returned at startup rather than at first-request.
func NewServer(addr string, database *sql.DB, log *zap.Logger, frontend FrontendConfig) (*Server, error) {
	if log == nil {
		log = zap.NewNop()
	}

	sessionCfg, err := admin.LoadSessionConfig()
	if err != nil {
		return nil, fmt.Errorf("load session config: %w", err)
	}
	sessions := admin.NewSessionManager(sessionCfg)

	csrfMW, err := admin.LoadCSRFWithLogger(log)
	if err != nil {
		return nil, fmt.Errorf("load csrf: %w", err)
	}

	// Login rate limit: 10 attempts per IP per 5 minutes (sliding window).
	// Defaults are hard-coded for the MVP — promote to env config when
	// the multi-tenant deployment story lands.
	loginRate := admin.NewRateLimiter(10, 5*time.Minute, nil)

	handler := NewRouter(Deps{
		Database:  database,
		Sessions:  sessions,
		CSRF:      csrfMW,
		LoginRate: loginRate,
		Log:       log,
		Frontend:  frontend,
	})
	handlers := newHandlerLifecycle(handler)

	return &Server{
		httpServer: &http.Server{
			Addr:     addr,
			Handler:  handlers,
			ErrorLog: zap.NewStdLog(log.Named("http")),
		},
		handlers: handlers,
	}, nil
}

func (s *Server) ListenAndServe() error {
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	drained := s.handlers.stop()
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}
	select {
	case <-drained:
	case <-ctx.Done():
		return fmt.Errorf("wait for application handlers: %w", ctx.Err())
	}
	return nil
}

// Close force-closes the HTTP server and waits for every admitted application
// handler to return. The application does not use hijacked connections; adding
// them would require separate lifecycle tracking before database ownership can
// still be released safely. The handler wait deliberately has no timeout: a
// caller must not release database ownership while application code is active.
func (s *Server) Close() error {
	drained := s.handlers.stop()
	err := s.httpServer.Close()
	<-drained
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("force close server: %w", err)
	}
	return nil
}

// _ unused import guard so scs is referenced even if we stop using it
// directly in this file in the future.
var _ *scs.SessionManager
