# Backend Module Relocation Design

## Goal

Consolidate all Go-owned source and build inputs under `backend/` and make
`backend/` a self-contained Go module. Preserve the current package layout and
application behavior so that later architecture work starts from a clean
backend boundary rather than being mixed into the relocation.

## Scope

The relocation includes:

- moving `cmd/` to `backend/cmd/`;
- moving `migrations/` to `backend/migrations/`;
- moving `go.mod`, `go.sum`, and `sqlc.yaml` to `backend/`;
- keeping the existing packages directly under `backend/` with their current
  names and internal structure;
- updating paths in Go imports, sqlc configuration, Docker build steps,
  repository documentation, CI configuration, and helper scripts where they
  refer to the old locations;
- preserving the Docker Compose entry point at
  `http://localhost:8081/login`.

The relocation explicitly excludes:

- introducing an `internal/` directory;
- splitting packages into handler, service, repository, or model layers;
- renaming existing application packages;
- changing HTTP routes, database schema, business rules, environment variable
  names, or runtime behavior;
- adding a root `go.work` file unless verification demonstrates a concrete
  tool requirement. No such requirement is expected for the initial design.

## Target Layout

```text
repository/
├── backend/
│   ├── go.mod
│   ├── go.sum
│   ├── sqlc.yaml
│   ├── cmd/
│   │   ├── mintrud-admin/
│   │   └── seed/
│   ├── migrations/
│   ├── admin/
│   ├── app/
│   ├── audit/
│   ├── documents/
│   ├── employers/
│   ├── people/
│   ├── platform/
│   ├── programs/
│   ├── protocols/
│   ├── requests/
│   └── storage/
├── frontend/
├── Dockerfile
├── docker-compose.yml
└── README.md
```

Root-level files that orchestrate the full-stack repository remain at the
root. This includes Docker Compose configuration, the Dockerfile, shared
environment templates, repository-level documentation, GitHub configuration,
and tests that exercise the assembled full-stack application.

## Module and Import Paths

The relocated module declares:

```go
module github.com/IvanSaratov/ikc_admin_panel/backend
```

Existing application imports already use this prefix, for example
`github.com/IvanSaratov/ikc_admin_panel/backend/storage`, so most imports remain
unchanged. The migration package is the expected exception: its import changes
from `github.com/IvanSaratov/ikc_admin_panel/migrations` to
`github.com/IvanSaratov/ikc_admin_panel/backend/migrations`.

No packages outside `backend/` currently import the Go module. After the move,
the root is a full-stack repository boundary rather than a Go module boundary.

## Build and Developer Workflows

Canonical Go commands run against the nested module:

```bash
go -C backend test ./...
go -C backend run ./cmd/mintrud-admin
go -C backend run ./cmd/seed <database-path>
```

Developers working inside `backend/` can use the shorter equivalents:

```bash
go test ./...
go run ./cmd/mintrud-admin
go run ./cmd/seed <database-path>
```

sqlc runs with `backend/` as its working directory. Its schema, query, and
generated output paths are therefore relative to that directory.

The Docker build context remains the repository root because the image build
needs both `frontend/` and `backend/`. The Go builder copies the backend module
manifests for dependency caching, copies the repository sources, generates sqlc
code from the backend configuration, and builds `backend/cmd/mintrud-admin`.
Runtime paths and the Compose port mapping remain unchanged.

## Dependency and Runtime Behavior

This change is a filesystem and build-configuration migration. The application
composition, HTTP router, session and CSRF configuration, SQLite behavior,
embedded migrations, frontend asset serving, signal handling, and seed fixture
semantics remain unchanged.

The migration package remains embedded in the server binary. Moving its files
does not change migration ordering or contents. Generated sqlc sources remain
committed under `backend/storage/db` and must be regenerated from the relocated
configuration to confirm path correctness.

## Verification

The migration is complete only when all of the following hold:

1. No stale references to root `cmd/`, root `migrations/`, root `go.mod`, or
   root `sqlc.yaml` remain in tracked configuration and documentation.
2. sqlc generation succeeds from the relocated module and produces no
   unexpected generated-code diff.
3. `go -C backend test ./...` passes.
4. The frontend test and production build commands still pass, and the Docker
   build continues to assemble both components.
5. The Docker Compose image builds and the app becomes healthy at
   `http://localhost:8081/login`.
6. The root contains no Go module manifest or root-level `cmd/` and
   `migrations/` directories after the move.

## Migration Safety

The work will be staged so structural moves, path corrections, generation,
tests, and Docker verification can be reviewed independently. File moves will
preserve history where Git can detect renames. Existing unrelated worktree
changes, including deletion of earlier Superpowers documents, are outside this
scope and must not be restored or included in commits for this migration.
