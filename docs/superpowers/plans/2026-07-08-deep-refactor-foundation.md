# Deep Refactor Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first safe foundation slice for the deep refactor: current docs, `logrus` as the runtime logger, HTTP request logging, and a cleaner `backend/app` wiring layout.

**Architecture:** Keep the existing modular monolith and preserve all routes, auth, CSRF, audit, import, protocol, and document behavior. Introduce a small platform logging package that returns configured `*logrus.Logger`, adapt current `slog` call sites to `logrus`, then split app wiring from route registration without changing middleware order.

**Tech Stack:** Go 1.26.4, `net/http`, `chi`, `templ`, `sqlc`, `goose`, SQLite, `github.com/sirupsen/logrus`.

---

## Scope

This plan implements the foundation only. It deliberately does not move shared `FieldErrors`, `parseInt64Param`, CRUD-domain helpers, or request/protocol/document use cases. Those are separate follow-up plans after this slice is green.

## Files

- Create: `backend/platform/logging/logging.go` - logrus construction, formatter/level/env parsing.
- Create: `backend/platform/logging/logging_test.go` - focused tests for logging config.
- Create: `backend/app/container.go` - app dependency construction for router wiring.
- Create: `backend/app/middleware.go` - actor propagation and request logging middleware.
- Create: `backend/app/routes.go` - route registration grouped by domain.
- Create: `docs/architecture.md` - current architecture map and operating rules.
- Modify: `README.md` - update implemented slice and logging/docs references.
- Modify: `go.mod`, `go.sum` - add `github.com/sirupsen/logrus`.
- Modify: `cmd/mintrud-admin/main.go` - use centralized logrus logger.
- Modify: `cmd/seed/main.go` - replace stdlib `log` calls with logrus.
- Modify: `backend/app/server.go` - accept `*logrus.Logger`.
- Modify: `backend/app/router.go` - shrink to compatibility wrapper or delete after moving code.
- Modify: `backend/app/router_test.go` - add request logging coverage and pass test logger.
- Modify: `backend/admin/handler.go` - replace `*slog.Logger` with `logrus.FieldLogger`.
- Modify: `backend/admin/middleware.go` - replace `*slog.Logger` with `logrus.FieldLogger`.
- Modify: `backend/admin/ratelimit.go` - replace `*slog.Logger` with `logrus.FieldLogger`.
- Modify: `backend/admin/csrf.go` - replace package-level `slog.Warn` with package-level logrus.
- Modify: `backend/admin/handler_test.go` - replace test `slog` logger with discard logrus logger.
- Modify: `backend/admin/frozen_handlers_test.go` - replace test `slog` logger with discard logrus logger.
- Modify: `backend/admin/ratelimit_test.go` - replace `slog.Default()` with discard logrus logger.
- Modify: `backend/requests/handler.go` - replace `*slog.Logger` with `logrus.FieldLogger`.
- Modify: `backend/requests/handler_test.go` - replace test logger.
- Modify: `backend/documents/service.go` - replace `*slog.Logger` with `logrus.FieldLogger`.
- Modify: `backend/documents/adapter_legacy.go` - call logrus legacy adapter.
- Modify: `backend/documents/legacy/logadapter.go` - replace slog adapter with logrus adapter.
- Modify: `backend/documents/testenv_test.go` - keep document service test logger nil or pass discard logrus logger.

## Task 1: Update Docs To Match Reality

**Files:**
- Modify: `README.md`
- Create: `docs/architecture.md`

- [ ] **Step 1: Update README current slice**

Replace the `README.md` section beginning at `## Текущий slice` with:

```markdown
## Текущее состояние

Приложение уже включает рабочий vertical slice внутренней админки:

- SQLite schema и goose migrations;
- Go server на `net/http` + `chi`;
- server-rendered UI на `templ`;
- auth baseline: login, sessions, CSRF, bootstrap admin, login rate limit;
- ручные формы для групп программ, программ, работодателей и слушателей;
- назначение слушателей работодателям;
- импорт XLSX-заявок в staging rows и применение строк;
- протоколы: создание, фиксация номера, участники и переходы состояния;
- генерация XML/DOCX через изолированный legacy adapter;
- `action_log` для ручных, auth, import, protocol и document events.

Пока не входят: Moodle integration, полная RBAC-модель, production installer/MSI
и расширенная наблюдаемость за пределами application logs.
```

- [ ] **Step 2: Add architecture document**

Create `docs/architecture.md` with:

```markdown
# Архитектура Mintrud Admin

## Общая форма

Приложение остаётся Go-first modular monolith. HTTP слой принимает request,
handler разбирает HTTP-детали, service выполняет workflow, `sqlc` queries
работают с SQLite, а `audit.Service` записывает действие оператора в
`action_log`.

## Карта пакетов

- `backend/app`: сборка сервера, зависимостей, middleware и маршрутов.
- `backend/admin`: вход, сессии, CSRF, bootstrap admin, login rate limit.
- `backend/audit`: запись и просмотр журнала действий.
- `backend/programs`: группы программ и программы.
- `backend/employers`: работодатели.
- `backend/people`: слушатели и связи слушатель-работодатель.
- `backend/requests`: XLSX upload, staging rows, применение строк.
- `backend/protocols`: протоколы, участники, номера и переходы состояния.
- `backend/documents`: генерация XML/DOCX и изоляция legacy generator.
- `backend/storage`: SQLite, миграции, транзакции и generated sqlc code.
- `backend/platform`: инфраструктурные helpers без бизнес-логики.

## Направление зависимостей

Доменные пакеты могут импортировать `storage`, `audit`, `platform` и свои
`views`. Общие пакеты не импортируют домены. Legacy generator доступен только
через `backend/documents`; остальные пакеты не импортируют
`backend/documents/legacy`.

## Логирование

Runtime logging стандартизирован на `github.com/sirupsen/logrus`. Логировать
можно технические поля: method, path, status, duration, actor, ids и безопасные
hash values. Нельзя логировать значения `.env`, пароли, tokens, содержимое XLSX,
сырые ФИО, СНИЛС, email и исходные имена файлов.

## Проверки

После обычного refactor-этапа:

```bash
make test
make schema-test
```

Если менялись `*.templ`:

```bash
make templ
make test
```

Если менялись SQL queries или migrations:

```bash
make sqlc
make schema-test
make test
```
```

- [ ] **Step 3: Review docs**

Run:

```bash
rg -n "Пока не входят|slog|log/slog|localhost:8080" README.md docs/architecture.md
```

Expected:

- no `slog` or `log/slog` matches;
- `localhost:8080` may appear only when explicitly describing the container port, not as the compose URL;
- `Пока не входят` should only list Moodle/RBAC/installer/observability gaps.

- [ ] **Step 4: Commit docs**

Run:

```bash
git add README.md docs/architecture.md
git commit -m "docs: update architecture and current scope"
```

Expected: commit succeeds with only documentation files.

## Task 2: Add Central Logrus Construction

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `backend/platform/logging/logging.go`
- Create: `backend/platform/logging/logging_test.go`

- [ ] **Step 1: Add dependency**

Run:

```bash
GOCACHE=/private/tmp/ikc-go-build go get github.com/sirupsen/logrus@v1.9.3
```

Expected: `go.mod` contains `github.com/sirupsen/logrus v1.9.3`.

- [ ] **Step 2: Write logging tests**

Create `backend/platform/logging/logging_test.go`:

```go
package logging

import (
	"bytes"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNewDefaultsToInfoText(t *testing.T) {
	var out bytes.Buffer
	logger, err := New(Config{Output: &out})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if logger.Level != logrus.InfoLevel {
		t.Fatalf("level = %v, want info", logger.Level)
	}
	if _, ok := logger.Formatter.(*logrus.TextFormatter); !ok {
		t.Fatalf("formatter = %T, want TextFormatter", logger.Formatter)
	}
}

func TestNewUsesJSONInProd(t *testing.T) {
	logger, err := New(Config{Env: "prod"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := logger.Formatter.(*logrus.JSONFormatter); !ok {
		t.Fatalf("formatter = %T, want JSONFormatter", logger.Formatter)
	}
}

func TestNewAcceptsExplicitLevelAndFormat(t *testing.T) {
	logger, err := New(Config{Level: "debug", Format: "json"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if logger.Level != logrus.DebugLevel {
		t.Fatalf("level = %v, want debug", logger.Level)
	}
	if _, ok := logger.Formatter.(*logrus.JSONFormatter); !ok {
		t.Fatalf("formatter = %T, want JSONFormatter", logger.Formatter)
	}
}

func TestNewRejectsInvalidLevel(t *testing.T) {
	if _, err := New(Config{Level: "verbose"}); err == nil {
		t.Fatalf("New invalid level: got nil error")
	}
}

func TestNewRejectsInvalidFormat(t *testing.T) {
	if _, err := New(Config{Format: "xml"}); err == nil {
		t.Fatalf("New invalid format: got nil error")
	}
}
```

- [ ] **Step 3: Run test and verify it fails**

Run:

```bash
GOCACHE=/private/tmp/ikc-go-build go test ./backend/platform/logging
```

Expected: FAIL because `Config` and `New` are undefined.

- [ ] **Step 4: Implement logging package**

Create `backend/platform/logging/logging.go`:

```go
package logging

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// Config описывает runtime-настройки logger. Пустые значения безопасны:
// локальная разработка получает text/info, production получает JSON/info.
type Config struct {
	Env    string
	Level  string
	Format string
	Output io.Writer
}

// New создаёт изолированный logrus.Logger для приложения.
func New(cfg Config) (*logrus.Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	formatter, err := formatter(cfg.Env, cfg.Format)
	if err != nil {
		return nil, err
	}
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	logger := logrus.New()
	logger.SetOutput(out)
	logger.SetLevel(level)
	logger.SetFormatter(formatter)
	return logger, nil
}

func parseLevel(raw string) (logrus.Level, error) {
	if strings.TrimSpace(raw) == "" {
		return logrus.InfoLevel, nil
	}
	level, err := logrus.ParseLevel(strings.ToLower(strings.TrimSpace(raw)))
	if err != nil {
		return logrus.InfoLevel, fmt.Errorf("parse log level %q: %w", raw, err)
	}
	return level, nil
}

func formatter(env string, format string) (logrus.Formatter, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		switch strings.ToLower(strings.TrimSpace(env)) {
		case "prod", "production":
			format = "json"
		default:
			format = "text"
		}
	}

	switch format {
	case "json":
		return &logrus.JSONFormatter{}, nil
	case "text":
		return &logrus.TextFormatter{
			DisableColors: true,
			FullTimestamp: true,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported log format %q", format)
	}
}
```

- [ ] **Step 5: Run logging tests**

Run:

```bash
GOCACHE=/private/tmp/ikc-go-build go test ./backend/platform/logging
```

Expected: PASS.

- [ ] **Step 6: Commit logging package**

Run:

```bash
git add go.mod go.sum backend/platform/logging/logging.go backend/platform/logging/logging_test.go
git commit -m "infra: add logrus logging foundation"
```

Expected: commit succeeds.

## Task 3: Convert Runtime Logger Types To Logrus

**Files:**
- Modify: `backend/admin/handler.go`
- Modify: `backend/admin/middleware.go`
- Modify: `backend/admin/ratelimit.go`
- Modify: `backend/admin/csrf.go`
- Modify: `backend/requests/handler.go`
- Modify: `backend/documents/service.go`
- Modify: `backend/documents/legacy/logadapter.go`
- Modify: `backend/documents/adapter_legacy.go`
- Modify: affected tests in `backend/admin`, `backend/requests`, `backend/documents`

- [ ] **Step 1: Convert admin middleware signature**

In `backend/admin/middleware.go`, replace `log/slog` with logrus:

```go
import (
	"context"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/sirupsen/logrus"
)
```

Change the constructor:

```go
func RequireAuth(sm *scs.SessionManager, log logrus.FieldLogger) func(http.Handler) http.Handler {
	if log == nil {
		log = logrus.StandardLogger()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			rawID := sm.GetInt64(ctx, SessionKeyUserID)
			if rawID == 0 {
				redirectToLogin(w, r)
				return
			}

			login := sm.GetString(ctx, SessionKeyUserLogin)
			if login == "" {
				log.Warn("auth: session has user_id but no user_login; redirecting to login")
				_ = sm.Destroy(ctx)
				redirectToLogin(w, r)
				return
			}

			ctx = context.WithValue(ctx, ctxKeyUserLogin, login)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

- [ ] **Step 2: Convert login rate limiter logging**

In `backend/admin/ratelimit.go`, replace `log/slog` import with:

```go
"github.com/sirupsen/logrus"
```

Change the middleware signature and logging calls:

```go
func LoginRateLimitMiddleware(rl *RateLimiter, log logrus.FieldLogger, auditSvc auditRecorder) func(http.Handler) http.Handler {
	if log == nil {
		log = logrus.StandardLogger()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
					log.WithError(err).Error("audit login.rate_limited")
				}
			}

			log.WithFields(logrus.Fields{
				"ip":          ip,
				"retry_after": retryAfter.String(),
			}).Warn("login rate limit exceeded")

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
```

- [ ] **Step 3: Convert admin handler logging**

In `backend/admin/handler.go`, replace `log/slog` import with:

```go
"github.com/sirupsen/logrus"
```

Change the field and constructor:

```go
type Handler struct {
	service  *Service
	audit    *audit.Service
	sessions *scs.SessionManager
	log      logrus.FieldLogger
}

func NewHandler(service *Service, auditSvc *audit.Service, sessions *scs.SessionManager, log logrus.FieldLogger) *Handler {
	if log == nil {
		log = logrus.StandardLogger()
	}
	return &Handler{service: service, audit: auditSvc, sessions: sessions, log: log}
}
```

Replace calls of the form:

```go
h.log.Error("render login form", slog.String("err", err.Error()))
```

with:

```go
h.log.WithError(err).Error("render login form")
```

Apply the same replacement for audit login failure, renew session token, audit login success, destroy session, audit logout, and render login form error.

- [ ] **Step 4: Convert CSRF warning**

In `backend/admin/csrf.go`, replace `log/slog` with:

```go
"github.com/sirupsen/logrus"
```

Replace the missing-key warning with:

```go
logrus.Warn(
	"MINTRUD_ADMIN_CSRF_KEY is unset; generated an ephemeral per-process CSRF key. " +
		"Tokens will be invalidated on every restart. Set MINTRUD_ADMIN_CSRF_KEY to a stable 32-byte hex value.",
)
```

- [ ] **Step 5: Convert requests handler logging**

In `backend/requests/handler.go`, replace `log/slog` import with:

```go
"github.com/sirupsen/logrus"
```

Change the field and constructor:

```go
log     logrus.FieldLogger
```

```go
func NewHandler(queries *storagedb.Queries, auditSvc *audit.Service, log logrus.FieldLogger) *Handler {
	if log == nil {
		log = logrus.StandardLogger()
	}
	svc := NewService(queries, auditSvc)
	return &Handler{
		service: svc,
		queries: queries,
		audit:   auditSvc,
		log:     log,
	}
}
```

Use `WithError` and `WithFields`:

```go
h.log.WithError(err).Error("list requests")
h.log.WithError(err).Error("list employers for new request")
h.log.WithFields(logrus.Fields{
	"content_type_hash": sha256Hex([]byte(header.Header.Get("Content-Type"))),
	"filename_hash":     sha256Hex([]byte(header.Filename)),
}).Warn("upload with unusual content-type")
h.log.WithError(err).Warn("upload rejected")
h.log.WithFields(logrus.Fields{"request_id": requestID, "row_id": rowID}).WithError(err).Warn("apply row")
h.log.WithFields(logrus.Fields{"request_id": requestID, "row_id": rowID}).WithError(err).Warn("skip row")
h.log.WithError(err).Error("lookup request row for authz")
```

- [ ] **Step 6: Convert documents service logging**

In `backend/documents/service.go`, replace `log/slog` import with:

```go
"github.com/sirupsen/logrus"
```

Change the field and constructor:

```go
log     logrus.FieldLogger
```

```go
func NewService(db *sql.DB, queries *storagedb.Queries, auditSvc *audit.Service, log logrus.FieldLogger) *Service {
	if log == nil {
		log = logrus.StandardLogger()
	}
	return &Service{
		db:      db,
		queries: queries,
		audit:   auditSvc,
		log:     log,
		now:     func() time.Time { return time.Now().UTC() },
	}
}
```

Replace:

```go
s.log.Error("create generation_runs row", "protocol_id", protocolID, "type", docType, "err", err)
```

with:

```go
s.log.WithFields(logrus.Fields{
	"protocol_id": protocolID,
	"type":        docType,
}).WithError(err).Error("create generation_runs row")
```

- [ ] **Step 7: Replace legacy slog adapter with logrus adapter**

Replace `backend/documents/legacy/logadapter.go` with:

```go
package legacy

import "github.com/sirupsen/logrus"

// FieldLogger — минимальная поверхность logger, которую использует legacy
// generator. Она совпадает с нужной частью logrus и не даёт legacy-коду
// диктовать остальную logging-архитектуру приложения.
type FieldLogger interface {
	Infof(format string, args ...any)
	Debugf(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
	Info(args ...any)
	Debug(args ...any)
	Warn(args ...any)
	Error(args ...any)
}

type logrusAdapter struct {
	log logrus.FieldLogger
}

func newLogrusAdapter(log logrus.FieldLogger) FieldLogger {
	if log == nil {
		log = logrus.StandardLogger()
	}
	return &logrusAdapter{log: log}
}

// NewLogrusAdapter возвращает logger для legacy generator.
func NewLogrusAdapter() FieldLogger {
	return newLogrusAdapter(nil)
}

func (a *logrusAdapter) Infof(format string, args ...any)  { a.log.Infof(format, args...) }
func (a *logrusAdapter) Debugf(format string, args ...any) { a.log.Debugf(format, args...) }
func (a *logrusAdapter) Warnf(format string, args ...any)  { a.log.Warnf(format, args...) }
func (a *logrusAdapter) Errorf(format string, args ...any) { a.log.Errorf(format, args...) }
func (a *logrusAdapter) Info(args ...any)                  { a.log.Info(args...) }
func (a *logrusAdapter) Debug(args ...any)                 { a.log.Debug(args...) }
func (a *logrusAdapter) Warn(args ...any)                  { a.log.Warn(args...) }
func (a *logrusAdapter) Error(args ...any)                 { a.log.Error(args...) }
```

- [ ] **Step 8: Update adapter call**

In `backend/documents/adapter_legacy.go`, replace:

```go
return legacy.CreateDocx(set, string(template), timeType, legacy.NewSlogAdapter())
```

with:

```go
return legacy.CreateDocx(set, string(template), timeType, legacy.NewLogrusAdapter())
```

- [ ] **Step 9: Update tests to use discard logrus logger**

Where tests currently use:

```go
logger := slog.New(slog.NewTextHandler(io.Discard, nil))
```

replace with:

```go
logger := logrus.New()
logger.SetOutput(io.Discard)
```

Add import:

```go
"github.com/sirupsen/logrus"
```

Remove `log/slog` imports from tests.

- [ ] **Step 10: Verify no slog remains in runtime code**

Run:

```bash
rg -n "log/slog|slog\\." backend cmd
```

Expected: no matches.

- [ ] **Step 11: Run focused package tests**

Run:

```bash
GOCACHE=/private/tmp/ikc-go-build go test ./backend/admin ./backend/requests ./backend/documents
```

Expected: PASS.

- [ ] **Step 12: Commit logger conversion**

Run:

```bash
git add backend/admin backend/requests backend/documents
git commit -m "refactor: convert runtime logging to logrus"
```

Expected: commit succeeds.

## Task 4: Wire Logrus Through Server And Commands

**Files:**
- Modify: `backend/app/server.go`
- Modify: `cmd/mintrud-admin/main.go`
- Modify: `cmd/seed/main.go`
- Modify: `cmd/mintrud-admin/main_test.go` - update only if compile errors show the server startup signature changed test-visible behavior.

- [ ] **Step 1: Update server signature**

In `backend/app/server.go`, replace logger import/use with:

```go
import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/sirupsen/logrus"
)
```

Change `NewServer` signature:

```go
func NewServer(addr string, database *sql.DB, log logrus.FieldLogger) (*Server, error) {
	if log == nil {
		log = logrus.StandardLogger()
	}
```

Remove the `_ *scs.SessionManager` unused import guard.

- [ ] **Step 2: Configure logger in main**

In `cmd/mintrud-admin/main.go`, remove stdlib `log` import and add:

```go
	"github.com/IvanSaratov/ikc_admin_panel/backend/platform/logging"
```

At the top of `run`, construct logger:

```go
	logger, err := logging.New(logging.Config{
		Env:    os.Getenv("MINTRUD_ADMIN_ENV"),
		Level:  os.Getenv("MINTRUD_ADMIN_LOG_LEVEL"),
		Format: os.Getenv("MINTRUD_ADMIN_LOG_FORMAT"),
	})
	if err != nil {
		return err
	}
```

Pass it to the server:

```go
server, err := app.NewServer(addr, database, logger)
```

Replace startup log:

```go
logger.WithField("addr", addr).Info("mintrud admin listening")
```

Replace `main` fatal path:

```go
func main() {
	if err := run(); err != nil {
		logger, logErr := logging.New(logging.Config{})
		if logErr != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		logger.WithError(err).Fatal("mintrud admin stopped")
	}
}
```

- [ ] **Step 3: Convert seed command**

In `cmd/seed/main.go`, replace stdlib `log` with:

```go
log "github.com/sirupsen/logrus"
```

Keep existing `log.Fatalf` and `log.Printf` calls working through the logrus package alias. This is acceptable for the seed CLI because it is developer tooling and the plan standardizes the dependency without changing seed output semantics.

- [ ] **Step 4: Run command package tests**

Run:

```bash
GOCACHE=/private/tmp/ikc-go-build go test ./cmd/mintrud-admin ./cmd/seed ./backend/app
```

Expected: PASS.

- [ ] **Step 5: Commit command wiring**

Run:

```bash
git add backend/app/server.go cmd/mintrud-admin/main.go cmd/seed/main.go cmd/mintrud-admin/main_test.go
git commit -m "refactor: wire logrus through server startup"
```

Expected: commit succeeds.

## Task 5: Add Request Logging Middleware

**Files:**
- Create: `backend/app/middleware.go`
- Modify: `backend/app/router_test.go`

- [ ] **Step 1: Add request logging test**

Append this test to `backend/app/router_test.go`:

```go
func TestRequestLoggingMiddlewareWritesSafeFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})

	router, _ := newTestRouterWithDBAndLog(t, logger)
	cookies := testLoginPOST(t, router)

	rec := authedGET(t, router, "/programs", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	logOutput := buf.String()
	for _, want := range []string{`"method":"GET"`, `"path":"/programs"`, `"status":200`, `"actor":"admin"`} {
		if !strings.Contains(logOutput, want) {
			t.Fatalf("log output missing %s: %s", want, logOutput)
		}
	}
	if strings.Contains(logOutput, "test-password") || strings.Contains(logOutput, "csrf_token") {
		t.Fatalf("log output contains sensitive value: %s", logOutput)
	}
}
```

Add imports to `backend/app/router_test.go`:

```go
	"bytes"

	"github.com/sirupsen/logrus"
```

Add helper:

```go
func newTestRouterWithDBAndLog(t *testing.T, logger logrus.FieldLogger) (http.Handler, *sql.DB) {
	t.Helper()

	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "mintrud-test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	if err := seedAdminUser(t, db); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	sessions := admin.NewSessionManager(admin.SessionConfig{
		TTL:      8 * time.Hour,
		SameSite: 0,
		Secure:   false,
	})
	csrfKey := []byte("0123456789abcdef0123456789abcdef")
	csrfMW := csrf.Protect(csrfKey,
		csrf.Secure(false),
		csrf.HttpOnly(true),
		csrf.FieldName("csrf_token"),
		csrf.RequestHeader("X-CSRF-Token"),
		csrf.CookieName("csrf_token"),
		csrf.Path("/"),
	)

	return app.NewRouter(app.Deps{
		Database: db,
		Sessions: sessions,
		CSRF:     csrfMW,
		Log:      logger,
	}), db
}
```

Then change existing `newTestRouterWithDB` to:

```go
func newTestRouterWithDB(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return newTestRouterWithDBAndLog(t, logger)
}
```

Add `io` import if it is not already present.

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
GOCACHE=/private/tmp/ikc-go-build go test ./backend/app -run TestRequestLoggingMiddlewareWritesSafeFields -count=1
```

Expected: FAIL because no request logging middleware writes those fields.

- [ ] **Step 3: Implement middleware**

Create `backend/app/middleware.go`:

```go
package app

import (
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/sirupsen/logrus"
)

// withActor переносит login из auth context в audit context, чтобы сервисы
// записывали action_log от имени текущего оператора.
func withActor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if login := admin.UserLoginFromContext(r.Context()); login != "" {
			r = r.WithContext(audit.WithActor(r.Context(), login))
		}
		next.ServeHTTP(w, r)
	})
}

// requestLogger пишет безопасный access-log без query string, form body,
// cookies, CSRF/session tokens и персональных данных.
func requestLogger(log logrus.FieldLogger) func(http.Handler) http.Handler {
	if log == nil {
		log = logrus.StandardLogger()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			fields := logrus.Fields{
				"method":      r.Method,
				"path":        r.URL.Path,
				"status":      rec.status,
				"duration_ms": time.Since(started).Milliseconds(),
				"remote_ip":   remoteIP(r.RemoteAddr),
			}
			if actor := audit.ActorFromContext(r.Context()); actor != "" {
				fields["actor"] = actor
			}
			log.WithFields(fields).Info("http request")
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	if remoteAddr == "" {
		return "unknown"
	}
	if net.ParseIP(remoteAddr) != nil {
		return remoteAddr
	}
	if _, err := strconv.Atoi(remoteAddr); err == nil {
		return "unknown"
	}
	return remoteAddr
}
```

- [ ] **Step 4: Wire middleware into router**

In `backend/app/router.go` before it is split, wrap public login endpoints:

```go
router.With(requestLogger(deps.Log)).Get("/login", adminHandler.GetLogin)
if deps.LoginRate != nil {
	router.With(requestLogger(deps.Log), admin.LoginRateLimitMiddleware(deps.LoginRate, deps.Log, auditSvc)).
		Post("/login", adminHandler.PostLogin)
} else {
	router.With(requestLogger(deps.Log)).Post("/login", adminHandler.PostLogin)
}
```

Replace the existing public login block rather than duplicating routes.

Inside the protected group, add request logging after `withActor`:

```go
r.Use(authMiddleware)
r.Use(withActor)
r.Use(requestLogger(deps.Log))
```

This order matters: `RequireAuth` publishes the user login, `withActor` copies it into the audit context, and `requestLogger` then sees `actor` before the handler runs.

- [ ] **Step 5: Run focused middleware test**

Run:

```bash
GOCACHE=/private/tmp/ikc-go-build go test ./backend/app -run TestRequestLoggingMiddlewareWritesSafeFields -count=1
```

Expected: PASS.

- [ ] **Step 6: Run app tests**

Run:

```bash
GOCACHE=/private/tmp/ikc-go-build go test ./backend/app
```

Expected: PASS.

- [ ] **Step 7: Commit request logging**

Run:

```bash
git add backend/app/middleware.go backend/app/router.go backend/app/router_test.go
git commit -m "feat: add safe request logging middleware"
```

Expected: commit succeeds.

## Task 6: Split App Wiring From Route Registration

**Files:**
- Create: `backend/app/container.go`
- Create: `backend/app/routes.go`
- Modify: `backend/app/router.go`
- Modify: `backend/app/middleware.go`
- Modify: `backend/app/server.go` if needed after signature updates.

- [ ] **Step 1: Create container**

Create `backend/app/container.go`:

```go
package app

import (
	"database/sql"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/documents"
	"github.com/IvanSaratov/ikc_admin_panel/backend/employers"
	"github.com/IvanSaratov/ikc_admin_panel/backend/people"
	"github.com/IvanSaratov/ikc_admin_panel/backend/programs"
	"github.com/IvanSaratov/ikc_admin_panel/backend/protocols"
	"github.com/IvanSaratov/ikc_admin_panel/backend/requests"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/sirupsen/logrus"
)

// container держит зависимости, созданные один раз при сборке router.
// Это не DI framework: структура нужна только чтобы routes.go был картой URL,
// а не местом создания сервисов.
type container struct {
	db       *sql.DB
	queries  *storagedb.Queries
	auditSvc *audit.Service
	log      logrus.FieldLogger

	adminHandler    *admin.Handler
	programHandler  *programs.Handler
	employerHandler *employers.Handler
	peopleHandler   *people.Handler
	auditHandler    *audit.Handler
	protocolHandler *protocols.Handler
	requestHandler  *requests.Handler
	documentHandler *documents.Handler
}

func newContainer(deps Deps) *container {
	queries := storagedb.New(deps.Database)
	auditSvc := audit.NewService(queries)

	documentSvc := documents.NewService(deps.Database, queries, auditSvc, deps.Log)
	documents.SetDefaultService(documentSvc)

	adminSvc := admin.NewService(queries)
	adminHandler := admin.NewHandler(adminSvc, auditSvc, deps.Sessions, deps.Log)
	admin.SetDefaultHandler(adminHandler)

	requestHandler := requests.NewHandler(queries, auditSvc, deps.Log)
	requestHandler.Service().SetDB(deps.Database)

	return &container{
		db:              deps.Database,
		queries:         queries,
		auditSvc:        auditSvc,
		log:             deps.Log,
		adminHandler:    adminHandler,
		programHandler:  programs.NewHandler(queries, auditSvc),
		employerHandler: employers.NewHandler(queries, auditSvc),
		peopleHandler:   people.NewHandler(queries, auditSvc),
		auditHandler:    audit.NewHandler(queries),
		protocolHandler: protocols.NewHandler(queries, deps.Database, auditSvc),
		requestHandler:  requestHandler,
		documentHandler: documents.NewHandler(queries, auditSvc, documentSvc),
	}
}
```

- [ ] **Step 2: Create route registration**

Create `backend/app/routes.go`:

```go
package app

import (
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/go-chi/chi/v5"
)

func registerRoutes(router chi.Router, deps Deps, c *container) {
	authMiddleware := admin.RequireAuth(deps.Sessions, deps.Log)

	router.With(requestLogger(deps.Log)).Get("/login", c.adminHandler.GetLogin)
	if deps.LoginRate != nil {
		router.With(requestLogger(deps.Log), admin.LoginRateLimitMiddleware(deps.LoginRate, deps.Log, c.auditSvc)).
			Post("/login", c.adminHandler.PostLogin)
	} else {
		router.With(requestLogger(deps.Log)).Post("/login", c.adminHandler.PostLogin)
	}

	router.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Use(withActor)
		r.Use(requestLogger(deps.Log))

		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "/programs", http.StatusSeeOther)
		})
		r.Post("/logout", c.adminHandler.PostLogout)

		r.Get("/audit", c.auditHandler.List)

		r.Get("/programs", c.programHandler.List)
		r.Post("/programs/groups", c.programHandler.CreateGroup)
		r.Get("/programs/groups/{id}/edit", c.programHandler.EditGroup)
		r.Post("/programs/groups/{id}/edit", c.programHandler.EditGroup)
		r.Post("/programs/groups/{id}/deactivate", c.programHandler.DeactivateGroup)
		r.Post("/programs", c.programHandler.CreateProgram)
		r.Get("/programs/{id}/edit", c.programHandler.EditProgram)
		r.Post("/programs/{id}/edit", c.programHandler.EditProgram)
		r.Post("/programs/{id}/deactivate", c.programHandler.DeactivateProgram)

		r.Get("/employers", c.employerHandler.List)
		r.Post("/employers", c.employerHandler.Create)
		r.Get("/employers/{id}", c.employerHandler.Detail)
		r.Get("/employers/{id}/edit", c.employerHandler.Edit)
		r.Post("/employers/{id}", c.employerHandler.Edit)
		r.Post("/employers/{id}/deactivate", c.employerHandler.Deactivate)

		r.Get("/workers", c.peopleHandler.List)
		r.Post("/workers", c.peopleHandler.CreateWorker)
		r.Get("/workers/{id}", c.peopleHandler.Detail)
		r.Get("/workers/{id}/edit", c.peopleHandler.Edit)
		r.Post("/workers/{id}", c.peopleHandler.Edit)
		r.Post("/workers/assignments", c.peopleHandler.AssignEmployer)
		r.Post("/workers/assignments/{id}/deactivate", c.peopleHandler.DeactivateAssignment)

		r.Get("/protocols", c.protocolHandler.List)
		r.Post("/protocols", c.protocolHandler.Create)
		r.Get("/protocols/{id}", c.protocolHandler.Detail)
		r.Get("/protocols/{id}/fix", c.protocolHandler.Fix)
		r.Post("/protocols/{id}/fix", c.protocolHandler.Fix)
		r.Post("/protocols/{id}/participants", c.protocolHandler.AddParticipant)
		r.Post("/protocols/{id}/participants/{pid}", c.protocolHandler.RemoveParticipant)
		r.Post("/protocols/{id}/transition", c.protocolHandler.Transition)

		r.Post("/protocols/{id}/generate", c.documentHandler.Generate)
		r.Get("/protocols/{id}/download", c.documentHandler.Download)

		r.Get("/requests", c.requestHandler.List)
		r.Get("/requests/new", c.requestHandler.NewRequestForm)
		r.Post("/requests/new", c.requestHandler.Upload)
		r.Get("/requests/{id}", c.requestHandler.Detail)
		r.Post("/requests/{id}/rows/{rowID}/apply", c.requestHandler.ApplyRow)
		r.Post("/requests/{id}/rows/{rowID}/skip", c.requestHandler.SkipRow)
	})
}
```

- [ ] **Step 3: Shrink router.go**

Replace `backend/app/router.go` with:

```go
package app

import (
	"database/sql"
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
)

type Deps struct {
	Database  *sql.DB
	Sessions  *scs.SessionManager
	CSRF      func(http.Handler) http.Handler
	LoginRate *admin.RateLimiter
	Log       logrus.FieldLogger
}

// NewRouter собирает HTTP router. Порядок middleware сохраняет auth baseline:
// session load/save, CSRF, request logging, затем protected routes через auth.
func NewRouter(deps Deps) http.Handler {
	if deps.Log == nil {
		deps.Log = logrus.StandardLogger()
	}
	if deps.Sessions == nil {
		deps.Sessions = scs.New()
	}
	if deps.CSRF == nil {
		deps.CSRF = func(next http.Handler) http.Handler { return next }
	}

	router := chi.NewRouter()
	router.Use(deps.Sessions.LoadAndSave)
	router.Use(deps.CSRF)

	registerRoutes(router, deps, newContainer(deps))
	return router
}
```

- [ ] **Step 4: Run app tests**

Run:

```bash
GOCACHE=/private/tmp/ikc-go-build go test ./backend/app
```

Expected: PASS.

- [ ] **Step 5: Run router route smoke search**

Run:

```bash
rg -n "r\\.(Get|Post)|router\\.(Get|Post)" backend/app
```

Expected: route registrations appear in `backend/app/routes.go`; `backend/app/router.go` only contains middleware setup.

- [ ] **Step 6: Commit app split**

Run:

```bash
git add backend/app
git commit -m "refactor: split app wiring and routes"
```

Expected: commit succeeds.

## Task 7: Full Verification

**Files:**
- No planned source edits unless verification exposes a compile or test failure.

- [ ] **Step 1: Check for old logging**

Run:

```bash
rg -n "log/slog|slog\\." backend cmd
```

Expected: no matches.

- [ ] **Step 2: Run Go tests**

Run:

```bash
make test
```

Expected: PASS.

- [ ] **Step 3: Run schema tests**

Run:

```bash
make schema-test
```

Expected: PASS.

- [ ] **Step 4: Check git status**

Run:

```bash
git status --short
```

Expected: clean working tree.

- [ ] **Step 5: Final commit only if fixes were needed**

If Step 2 or Step 3 required a fix, inspect the changed files:

```bash
git status --short
```

Then stage the exact files shown by `git status --short` and commit them. For
example, if the only fix was in `backend/app/middleware.go`, run:

```bash
git add backend/app/middleware.go
git commit -m "test: keep refactor foundation checks green"
```

Expected: no commit is created when verification passed without changes.

## Self-Review

Spec coverage:

- Documentation update is covered by Task 1.
- `logrus` runtime logging is covered by Tasks 2-4.
- Request-level logging is covered by Task 5.
- App wiring split is covered by Task 6.
- Verification gates are covered by Task 7.

Deferred to later plans:

- Shared `httpx`/`domain` helpers.
- CRUD domain cleanup for `programs`, `employers`, `people`.
- Workflow cleanup for `requests`, `protocols`, `documents`.
- Final public-git hygiene scan.
