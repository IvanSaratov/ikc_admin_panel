# Frontend Transition Design

## Decision

Use a single repository with a separate `frontend/` application built with React, Vite, and TypeScript. Keep Go as the only production runtime. The customer-facing package remains a compiled Windows service/MSI with no Go SDK, Node.js, npm, or Vite dev server installed on the customer server.

Production serving model:

```text
GET /api/*      Go JSON API
GET /assets/*   embedded Vite build assets
GET /*          embedded frontend/dist/index.html for SPA routes
```

Development serving model:

```text
Go backend       http://localhost:8080      /api/*
Vite frontend    http://localhost:5173      React app, HMR, proxy /api to Go
```

The current `templ` UI is treated as a temporary migration layer. New operator UI work should target the React frontend. Existing Go services, storage, migrations, document generation, auth, audit, and import logic stay in Go.

## Rationale

The product is an operator-facing admin panel for document, request, protocol, XML, DOCX, XLSX, and Moodle workflows. The central protocol process is not just CRUD: it has a multi-step workflow from client request review through participant selection, protocol fixation, XML generation, manual upload, registry number entry, DOCX generation, and manual closure.

Server-rendered `templ` plus HTMX can handle forms, tables, validation, partial updates, and simple notifications. It becomes awkward for the target UI requirements: component-level UI tests, mock-first UI development, reusable interactive components, animated workflow screens, drag-and-drop pipeline behavior, toast/action-center notifications, and long-running asynchronous operation states.

React/Vite adds frontend complexity, but it keeps that complexity isolated in `frontend/`. Vite builds static HTML/CSS/JS, so production delivery still remains simple.

## Repository Shape

Target layout:

```text
backend/                 Go domain services, handlers, API, storage
cmd/mintrud-admin/        Go binary entrypoint
frontend/                React/Vite/TypeScript application
frontend/src/            UI components, pages, API client, mocks
frontend/tests/          Playwright end-to-end tests
migrations/              SQLite migrations
tests/                   Existing schema checks
web/                     Legacy static files during migration only
docker-compose.yml       Production-like local integration stack
```

## Backend Direction

Go remains responsible for:

- SQLite migrations and storage.
- Authentication, sessions, CSRF/session security, and rate limiting.
- Request/import/protocol/business workflow services.
- DOCX, XML, XLSX, Moodle integration, and background job execution.
- Audit log.
- JSON API under `/api/*`.
- Static frontend serving in production.

The router should support an explicit frontend mode:

- `embedded`: serve built React assets from `go:embed`.
- `disabled`: serve only API routes, used with Vite in development and in API-focused tests.

Default production behavior should be `embedded`.

## Frontend Direction

React/Vite frontend responsibilities:

- Application shell: navigation, account menu, page layout.
- Reusable UI primitives: buttons, forms, dialogs, toasts, status badges, empty states, loading states.
- Operator pages: dashboard/status list, requests, import review, protocols, protocol workflow, participants, employers, workers, programs, audit.
- API client and typed DTOs.
- Mock API mode for UI-first development.
- Unit/component tests for UI logic.
- Playwright tests for critical flows.

The first proving slice should be the protocol workflow screen because it exercises the hardest UI needs:

1. Stepper or pipeline visualization.
2. Participant selection and readiness checks.
3. Blocked-stage reasons.
4. XML/DOCX actions.
5. Toast notifications.
6. Long-running job state.

## Testing Strategy

Keep tests layered rather than duplicating every assertion everywhere:

- Go tests: business logic, storage, API handlers, auth, imports, documents.
- Vitest/component tests: React components, UI state, validation, toasts, workflow rendering, disabled/enabled actions.
- Frontend build check: TypeScript and Vite production bundle.
- Playwright E2E: a small number of production-like user flows through Docker Compose or the compiled Go server.

Recommended standard commands:

```bash
go test ./...
npm --prefix frontend test
npm --prefix frontend run build
npm --prefix frontend run e2e
```

## Windows Delivery

The customer receives a built Windows service/MSI. The installer should include:

- `mintrud-admin.exe`.
- Configuration template or configured file.
- SQLite data directory under a stable Windows application data location.
- Service registration.
- Firewall rule only if the deployment needs direct inbound access.
- Embedded frontend assets already inside the binary, or static assets installed next to the binary if embedding is intentionally disabled.

The customer must not run `npm install`, `npm run build`, Vite, or Go commands.

## Migration Policy

Do not rewrite everything at once. Use a vertical-slice transition:

1. Add frontend toolchain and mock-first protocol workflow UI.
2. Add Go embedded static serving and API-only dev mode.
3. Add initial `/api/session` and `/api/protocols/{id}/workflow` endpoints.
4. Wire the protocol workflow screen to real API data.
5. Add production-like Playwright coverage through the Go-served app.
6. Migrate remaining pages incrementally, leaving existing `templ` pages available until replaced.

## Non-Goals

- Do not split into two repositories at this stage.
- Do not require Node.js on the customer server.
- Do not introduce Next.js or a Node production runtime.
- Do not introduce a queue/broker service for MVP notifications.
- Do not migrate the database away from SQLite as part of the frontend transition.

## Open Decisions

- Exact UI component library choice: custom components first, or a library such as Radix/shadcn-style primitives.
- Exact API DTO generation strategy: handwritten TypeScript types first, OpenAPI generation only if API drift becomes painful.
- Whether to keep legacy `templ` routes permanently under a debug/admin path or remove them after React parity.
