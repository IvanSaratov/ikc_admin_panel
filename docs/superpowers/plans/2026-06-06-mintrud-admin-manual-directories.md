# План реализации ручных справочников Mintrud Admin

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or execute task-by-task with TDD. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** построить первый Go/templ vertical slice для ручного управления группами программ, программами, работодателями, слушателями и связями слушатель-работодатель.

**Architecture:** Go-first modular monolith. `cmd/mintrud-admin` запускает HTTP server; `backend/app` собирает routes; `backend/storage` отвечает за SQLite/goose/sqlc; feature-пакеты `backend/programs`, `backend/employers`, `backend/people` содержат handlers, services и views.

**Tech Stack:** Go, `net/http`, `chi`, `templ`, HTMX, Tabler/Bootstrap, SQLite, `database/sql`, `sqlc`, `goose`, `modernc.org/sqlite`.

---

## Task 1: Schema Contract

**Files:**
- Modify: `migrations/001_initial_schema.sql`
- Modify: `tests/schema_smoke.sql`
- Modify: `tests/schema_constraints.sql`

- [ ] Write a failing SQL constraint snippet proving `workers.email` rejects `NULL`.
- [ ] Run `sh tests/run_schema_tests.sh`; expected failure until schema is fixed.
- [ ] Change `workers.email` to `TEXT NOT NULL`.
- [ ] Add minimal `action_log` table with `actor`, `action`, `entity_type`, `entity_id`, `details`, `created_at`.
- [ ] Update smoke data to include required worker email.
- [ ] Run `sh tests/run_schema_tests.sh`; expected pass.

## Task 2: Go Module And Tooling

**Files:**
- Create: `go.mod`
- Create: `sqlc.yaml`
- Create: `Makefile`
- Create: `migrations/embed.go`

- [ ] Add module `github.com/IvanSaratov/ikc_admin_panel`.
- [ ] Add dependencies for chi, goose, modernc sqlite, templ runtime.
- [ ] Add `sqlc.yaml` targeting `backend/storage/db`.
- [ ] Add `migrations/embed.go` with `//go:embed *.sql`.
- [ ] Add Make targets for `test`, `schema-test`, `sqlc`, `templ`, and `run`.
- [ ] Run `go mod tidy`; expected success.

## Task 3: Storage Bootstrap

**Files:**
- Create: `backend/storage/db.go`
- Create: `backend/storage/migrate.go`
- Create: `backend/storage/tx.go`
- Create: `backend/storage/sqlite_errors.go`
- Create: `backend/storage/db_test.go`

- [ ] Write failing Go tests for opening a temp file DB, running migrations, FK pragma, WAL pragma, and duplicate SNILS error mapping.
- [ ] Run `go test ./backend/storage`; expected compile/test failure.
- [ ] Implement storage open/migrate/close helpers with file DB pragmas.
- [ ] Implement simple SQLite error mapping helpers.
- [ ] Run `go test ./backend/storage`; expected pass.

## Task 4: sqlc Queries

**Files:**
- Create: `backend/storage/queries/program_groups.sql`
- Create: `backend/storage/queries/programs.sql`
- Create: `backend/storage/queries/employers.sql`
- Create: `backend/storage/queries/workers.sql`
- Create: `backend/storage/queries/worker_employers.sql`
- Generate: `backend/storage/db/*.go`

- [ ] Add CRUD/list/status queries for first-slice tables.
- [ ] Run `sqlc generate` through `go run`; expected generated code.
- [ ] Run `go test ./...`; expected pass or only missing application packages before next task.

## Task 5: Application Scaffold And UI Shell

**Files:**
- Create: `cmd/mintrud-admin/main.go`
- Create: `backend/app/server.go`
- Create: `backend/app/router.go`
- Create: `backend/ui/layouts/base.templ`
- Create: `backend/ui/layouts/shell.templ`
- Create: `backend/ui/components/components.templ`
- Create: `web/static/app.css`

- [ ] Write a failing handler test for `/` returning the shell.
- [ ] Add chi router, static file route, and redirect `/` to `/programs`.
- [ ] Add base/shell templ layouts with Tabler CDN links for this slice.
- [ ] Run `templ generate`.
- [ ] Run `go test ./...`; expected pass.

## Task 6: Programs CRUD

**Files:**
- Create: `backend/programs/service.go`
- Create: `backend/programs/handler.go`
- Create: `backend/programs/views/views.templ`
- Create: `backend/programs/service_test.go`

- [ ] Write failing service tests for required fields, positive hours, duplicate group code, and duplicate program code within group.
- [ ] Implement program group and program service methods.
- [ ] Add list/create/edit/deactivate handlers and templates.
- [ ] Register routes under `/programs`.
- [ ] Run `templ generate` and `go test ./...`; expected pass.

## Task 7: Employers CRUD

**Files:**
- Create: `backend/employers/service.go`
- Create: `backend/employers/handler.go`
- Create: `backend/employers/views/views.templ`
- Create: `backend/employers/service_test.go`

- [ ] Write failing service tests for required canonical name, INN normalization, and duplicate normalized INN.
- [ ] Implement employer service methods.
- [ ] Add list/create/edit/detail handlers and templates.
- [ ] Register routes under `/employers`.
- [ ] Run `templ generate` and `go test ./...`; expected pass.

## Task 8: Workers And Assignments CRUD

**Files:**
- Create: `backend/people/service.go`
- Create: `backend/people/handler.go`
- Create: `backend/people/views/views.templ`
- Create: `backend/people/service_test.go`

- [ ] Write failing service tests for required email, SNILS normalization, duplicate normalized SNILS, and duplicate active worker-employer assignment.
- [ ] Implement worker and worker-employer service methods.
- [ ] Add list/create/edit/detail handlers and templates.
- [ ] Register routes under `/workers`.
- [ ] Run `templ generate` and `go test ./...`; expected pass.

## Task 9: Runbook And Verification

**Files:**
- Create or modify: `README.md`

- [ ] Add local setup, test, and run commands.
- [ ] Run `sh tests/run_schema_tests.sh`.
- [ ] Run `go test ./...`.
- [ ] Run `sqlc generate`.
- [ ] Run `templ generate`.
- [ ] Start local server and smoke check root/list pages.
