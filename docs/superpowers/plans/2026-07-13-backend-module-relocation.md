# Backend Module Relocation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move all Go-owned source and build inputs into a self-contained `backend/` module without changing the existing package architecture or application behavior.

**Architecture:** Keep the current packages directly under `backend/`, relocate the two commands and embedded migrations beside them, and change the module path to the import prefix the code already uses. Keep full-stack orchestration at the repository root and update only path-sensitive integration points: frontend asset discovery, sqlc, schema tests, CI, Docker, and developer documentation.

**Tech Stack:** Go 1.26.4, sqlc 1.30.0, SQLite/goose, React/Vite, GitHub Actions, Docker Compose

---

## File Map

**Move without restructuring:**

- `go.mod` → `backend/go.mod`
- `go.sum` → `backend/go.sum`
- `sqlc.yaml` → `backend/sqlc.yaml`
- `cmd/mintrud-admin/` → `backend/cmd/mintrud-admin/`
- `cmd/seed/` → `backend/cmd/seed/`
- `migrations/` → `backend/migrations/`

**Modify:**

- `backend/cmd/mintrud-admin/main.go` and `main_test.go` — support frontend assets from repository-root and backend-module working directories.
- `backend/storage/migrate.go` — import relocated migrations.
- `backend/sqlc.yaml` — use backend-relative paths.
- `tests/schema_smoke.sql` — use repository-relative migration paths.
- `.github/workflows/ci.yml` — execute Go tooling in `backend/`.
- `Dockerfile` — build the module from `/src/backend`.
- `README.md` — document nested-module commands.

**Deliberately unchanged:** existing package boundaries under `backend/`, `.dockerignore`, `docker-compose.yml`, migration SQL contents, HTTP routes, environment variable names, and frontend source.

### Task 1: Record the pre-move baseline

**Files:**

- Test: `tests/run_schema_tests.sh`
- Test: all packages selected by root `go.mod`
- Test: `frontend/`

- [ ] **Step 1: Verify schema and Go tests**

Run:

```bash
sh tests/run_schema_tests.sh
go test ./...
```

Expected: all schema cases and Go packages pass. Record any pre-existing failure before moving files.

- [ ] **Step 2: Verify sqlc reproducibility**

Run:

```bash
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
git diff --exit-code -- backend/storage/db
```

Expected: generation succeeds and generated code has no diff.

- [ ] **Step 3: Verify frontend tests and build**

Run:

```bash
npm test --prefix frontend
npm run build --prefix frontend
```

Expected: tests pass and Vite creates `frontend/dist`.

### Task 2: Support both frontend working directories

**Files:**

- Modify: `cmd/mintrud-admin/main_test.go`
- Modify: `cmd/mintrud-admin/main.go`

- [ ] **Step 1: Add failing tests for asset lookup**

Add `os` and `path/filepath` to the existing test imports, then add:

```go
func TestFrontendAssetsDir_PrefersRepositoryRootLayout(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "frontend", "dist")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "index.html"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	if got, want := frontendAssetsDir(), filepath.Join("frontend", "dist"); got != want {
		t.Fatalf("frontendAssetsDir() = %q, want %q", got, want)
	}
}

func TestFrontendAssetsDir_FallsBackFromBackendModule(t *testing.T) {
	root := t.TempDir()
	backendDir := filepath.Join(root, "backend")
	assets := filepath.Join(root, "frontend", "dist")
	if err := os.MkdirAll(backendDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "index.html"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(backendDir)

	if got, want := frontendAssetsDir(), filepath.Join("..", "frontend", "dist"); got != want {
		t.Fatalf("frontendAssetsDir() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Verify the new tests fail**

Run: `go test ./cmd/mintrud-admin`

Expected: build failure because `frontendAssetsDir` is undefined.

- [ ] **Step 3: Implement working-directory-aware asset lookup**

Use the helper in `frontendConfigFromEnv`:

```go
return app.FrontendConfig{
	Mode:   app.FrontendEmbedded,
	Assets: os.DirFS(frontendAssetsDir()),
}
```

Add below that function:

```go
func frontendAssetsDir() string {
	fromRepositoryRoot := filepath.Join("frontend", "dist")
	if _, err := os.Stat(filepath.Join(fromRepositoryRoot, "index.html")); err == nil {
		return fromRepositoryRoot
	}
	return filepath.Join("..", "frontend", "dist")
}
```

- [ ] **Step 4: Verify and commit the path preparation**

Run:

```bash
gofmt -w cmd/mintrud-admin/main.go cmd/mintrud-admin/main_test.go
go test ./cmd/mintrud-admin
git add cmd/mintrud-admin/main.go cmd/mintrud-admin/main_test.go
git commit -m "refactor: support backend working directory"
```

Expected: focused tests pass and the commit contains only the asset-path preparation.

### Task 3: Relocate the Go module, commands, and migrations

**Files:**

- Move: `go.mod`, `go.sum`, `sqlc.yaml`, `cmd/`, and `migrations/` into `backend/`
- Modify: `backend/go.mod`
- Modify: `backend/sqlc.yaml`
- Modify: `backend/storage/migrate.go`

- [ ] **Step 1: Move backend-owned files without restructuring packages**

Run:

```bash
mv go.mod backend/go.mod
mv go.sum backend/go.sum
mv sqlc.yaml backend/sqlc.yaml
mv cmd backend/cmd
mv migrations backend/migrations
```

Expected: all five root paths disappear and their contents appear under `backend/`.

- [ ] **Step 2: Set the nested module path**

Change the first line of `backend/go.mod` to:

```go
module github.com/IvanSaratov/ikc_admin_panel/backend
```

- [ ] **Step 3: Update the migration import**

In `backend/storage/migrate.go`, use:

```go
import (
	"database/sql"
	"fmt"

	"github.com/IvanSaratov/ikc_admin_panel/backend/migrations"
	"github.com/pressly/goose/v3"
)
```

Keep the rest of the file unchanged.

- [ ] **Step 4: Make sqlc paths backend-relative**

Set `backend/sqlc.yaml` to:

```yaml
version: "2"
sql:
  - engine: sqlite
    schema:
      - migrations/001_initial_schema.sql
      - migrations/002_schema_cleanup.sql
      - migrations/003_status_and_actor_relax.sql
      - migrations/004_auth_schema.sql
    queries: storage/queries
    gen:
      go:
        package: db
        out: storage/db
        emit_json_tags: true
        emit_prepared_queries: false
        emit_interface: false
        emit_exact_table_names: false
```

- [ ] **Step 5: Verify Go and sqlc from the new module root**

Run:

```bash
gofmt -w backend/storage/migrate.go backend/cmd/mintrud-admin/main.go backend/cmd/mintrud-admin/main_test.go
go -C backend mod tidy
go -C backend test ./...
go -C backend run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
git diff --exit-code -- backend/storage/db
```

Expected: all backend packages pass and sqlc creates no generated-code diff.

### Task 4: Update schema tests, CI, and Docker

**Files:**

- Modify: `tests/schema_smoke.sql`
- Modify: `.github/workflows/ci.yml`
- Modify: `Dockerfile`

- [ ] **Step 1: Point schema tests at relocated migrations**

Replace the first four `.read` directives in `tests/schema_smoke.sql` with:

```sql
.read backend/migrations/001_initial_schema.sql
.read backend/migrations/002_schema_cleanup.sql
.read backend/migrations/003_status_and_actor_relax.sql
.read backend/migrations/004_auth_schema.sql
```

Run: `sh tests/run_schema_tests.sh`

Expected: every schema and constraint case reports `PASS`.

- [ ] **Step 2: Run CI Go tooling inside backend**

Update the relevant `.github/workflows/ci.yml` section to:

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version-file: backend/go.mod

      - uses: actions/setup-node@v4
        with:
          node-version: 24
          cache: npm
          cache-dependency-path: frontend/package-lock.json

      - run: sh tests/run_schema_tests.sh
      - run: go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
        working-directory: backend
      - run: go test ./...
        working-directory: backend
```

Keep all frontend steps unchanged.

- [ ] **Step 3: Build the nested module from the Docker root context**

Replace the Go builder workdir/copy/build block in `Dockerfile` with:

```dockerfile
WORKDIR /src/backend

# Copy backend module manifests first and download deps as a separate layer.
# Subsequent source edits don't bust the module cache.
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Keep the repository root as build context because the final image also needs
# the separately-built frontend. Generate and compile from the backend module.
COPY . /src
RUN sqlc generate \
    && CGO_ENABLED=0 GOOS=linux go build \
        -ldflags="-s -w" \
        -trimpath \
        -o /out/mintrud-admin \
        ./cmd/mintrud-admin
```

Replace the Go-version comment above the builder with:

```dockerfile
# Pinned to the same Go version declared in backend/go.mod so the toolchain in
# the image cannot drift from what developers use locally. Alpine keeps the
# builder small; git is required for `go mod download` to fetch modules.
```

The copy/build block above already documents that sqlc reads and writes inside the backend module; no root `migrations/*.sql` or root `go.mod` wording should remain.

- [ ] **Step 4: Scan integration files for stale root assumptions**

Run:

```bash
rg -n 'go-version-file: go\.mod|COPY go\.mod go\.sum|\.read migrations/|WORKDIR /src$' .github Dockerfile tests
```

Expected: no matches.

### Task 5: Update developer documentation

**Files:**

- Modify: `README.md`

- [ ] **Step 1: Replace root Go commands**

The local-run block must read:

```bash
sh tests/run_schema_tests.sh
go -C backend run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
go -C backend test ./...
npm ci --prefix frontend
npm test --prefix frontend
npm run build --prefix frontend
go -C backend run ./cmd/mintrud-admin
```

- [ ] **Step 2: Update the environment override example**

Use:

```bash
MINTRUD_ADMIN_ADDR=:8090 MINTRUD_ADMIN_DB=/tmp/mintrud-admin.db go -C backend run ./cmd/mintrud-admin
```

- [ ] **Step 3: Document the new boundary**

Add this paragraph after the local-run command block:

```markdown
`backend/` — самостоятельный Go-модуль: в нём находятся `go.mod`, команды,
миграции и конфигурация sqlc. Go-команды из корня репозитория запускаются через
`go -C backend`. Docker по-прежнему использует корень репозитория как build
context, потому что образ включает и backend, и собранный frontend.
```

Keep the existing Compose URL `http://localhost:8081/login` and the existing note that sqlc 1.30.0 is pinned in README and Dockerfile.

- [ ] **Step 4: Scan tracked text for stale paths**

Run:

```bash
git grep -n -E 'go (test|run|build) \./|cmd/mintrud-admin|cmd/seed|migrations/|go-version-file: go\.mod|COPY go\.mod go\.sum' -- ':!docs/superpowers/**'
```

Expected: remaining matches either contain `backend/`, are intentionally relative to `backend/` (for example inside `backend/sqlc.yaml`), or are correct comments in relocated files.

### Task 6: Verify and commit the complete relocation

**Files:**

- Verify: `backend/**`, `frontend/**`, `tests/**`, `Dockerfile`, and `docker-compose.yml`

- [ ] **Step 1: Assert the target filesystem boundary**

Run:

```bash
test -f backend/go.mod
test -f backend/go.sum
test -f backend/sqlc.yaml
test -f backend/cmd/mintrud-admin/main.go
test -f backend/cmd/seed/main.go
test -f backend/migrations/embed.go
test ! -e go.mod
test ! -e go.sum
test ! -e sqlc.yaml
test ! -e cmd
test ! -e migrations
test ! -e go.work
```

Expected: all assertions succeed.

- [ ] **Step 2: Run all non-container checks**

Run:

```bash
sh tests/run_schema_tests.sh
go -C backend run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
git diff --exit-code -- backend/storage/db
go -C backend test ./...
npm test --prefix frontend
npm run build --prefix frontend
git diff --check
```

Expected: every command exits 0 and sqlc produces no generated-code diff.

- [ ] **Step 3: Build and start the canonical Compose stack**

Never print `.env`. If `.env` is absent, copy `.env.example` to `.env` and let the user provide `MINTRUD_ADMIN_BOOTSTRAP_PASSWORD` before continuing. Otherwise run:

```bash
DOCKER_BUILDKIT=1 docker compose up --build -d
docker compose ps
docker compose logs --tail=80 app
curl -sS -o /dev/null -w '%{http_code}\n' http://localhost:8081/login
```

Expected: `app` is healthy, logs contain no startup error, and curl returns a 2xx or 3xx status.

- [ ] **Step 4: Inspect the final diff without touching unrelated deletions**

Run:

```bash
git status --short
git diff --check
git diff --stat
```

Expected: the relocation and path updates are present; pre-existing deleted Superpowers documents remain unstaged and are not restored.

- [ ] **Step 5: Commit only the relocation**

```bash
git add -A -- backend cmd migrations go.mod go.sum sqlc.yaml
git add .github/workflows/ci.yml Dockerfile README.md tests/schema_smoke.sql
git commit -m "refactor: consolidate Go module under backend"
```

Expected: the commit contains only the module relocation and integration path fixes, not unrelated deletions under `docs/superpowers/`.
