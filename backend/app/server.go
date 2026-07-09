package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/alexedwards/scs/v2"
	"github.com/sirupsen/logrus"
)

type Server struct {
	httpServer *http.Server
}

// NewServer builds the HTTP server.
//
// All auth wiring (session manager, CSRF middleware) is created here so
// the caller (main.go) doesn't have to repeat env-handling logic. If
// any of the underlying constructors fails (e.g. bad CSRF key), the
// error is returned at startup rather than at first-request.
func NewServer(addr string, database *sql.DB, log logrus.FieldLogger) (*Server, error) {
	if log == nil {
		log = logrus.StandardLogger()
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
	})

	return &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: handler,
		},
	}, nil
}

func (s *Server) ListenAndServe() error {
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}
	return nil
}

// _ unused import guard so scs is referenced even if we stop using it
// directly in this file in the future.
var _ *scs.SessionManager
