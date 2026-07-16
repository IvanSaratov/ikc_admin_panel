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
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/imports"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/alexedwards/scs/v2"
	"go.uber.org/zap"
)

type Server struct {
	httpServer *http.Server
	handlers   *handlerLifecycle
}

type ServerConfig struct {
	Addr            string
	Sessions        *scs.SessionManager
	CSRF            func(http.Handler) http.Handler
	Frontend        FrontendConfig
	ImportUploadDir string
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

// NewServer builds the HTTP server from already-resolved dependencies.
func NewServer(config ServerConfig, database *sql.DB, log *zap.Logger) (*Server, error) {
	if log == nil {
		log = zap.NewNop()
	}

	// Login rate limit: 10 attempts per IP per 5 minutes (sliding window).
	// Defaults are hard-coded for the MVP — promote to env config when
	// the multi-tenant deployment story lands.
	loginRate := admin.NewRateLimiter(10, 5*time.Minute, nil)

	var importService *imports.Service
	if database != nil {
		if config.ImportUploadDir == "" {
			return nil, fmt.Errorf("import upload directory is required")
		}
		fileStore, err := imports.NewLocalFileStore(config.ImportUploadDir)
		if err != nil {
			return nil, fmt.Errorf("initialize import upload storage: %w", err)
		}
		queries := storagedb.New(database)
		importService, err = imports.NewService(
			database,
			queries,
			audit.NewService(queries),
			fileStore,
			imports.DefaultConfig(),
		)
		if err != nil {
			return nil, fmt.Errorf("initialize import service: %w", err)
		}
	}

	handler := NewRouter(Deps{
		Database:      database,
		Sessions:      config.Sessions,
		CSRF:          config.CSRF,
		LoginRate:     loginRate,
		Log:           log,
		Frontend:      config.Frontend,
		ImportService: importService,
	})
	handlers := newHandlerLifecycle(handler)

	return &Server{
		httpServer: &http.Server{
			Addr:     config.Addr,
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
