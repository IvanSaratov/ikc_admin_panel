package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
)

type Server struct {
	httpServer *http.Server
}

func NewServer(addr string, database *sql.DB) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: NewRouter(database),
		},
	}
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
