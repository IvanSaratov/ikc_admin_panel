# Frontend Shell Design

Date: 2026-07-12
Project: IKC Expert Mintrud Admin
Status: approved for implementation planning

## Context

The project is moving from a Go `templ` server-rendered admin UI to a separate
React frontend. The current backend still contains old UI fragments in
`backend/*/views`, `backend/ui`, server-rendered protected routes, Tabler/HTMX
assets, and templ generation in CI/Docker. The new `frontend/` already contains
a minimal Vite/React scaffold and a mocked protocol workflow screen.

The product source of truth is the `work-ikc-expert` wiki domain, especially the
Mintrud Admin roadmap. The admin supports the operator workflow around client
XLSX requests, import review, registries, protocols, XML/DOCX generation,
Moodle enrollment, and audit.

Current design decision: build the full frontend site skeleton now, with typed
mocks and polished UI states. API integration will be added later section by
section.

## Goals

- Remove all UI rendering responsibility from the backend.
- Build a complete React SPA shell covering all current and planned admin
  sections.
- Use typed mock data/services so the frontend can be developed before the API
  is complete.
- Make the UI visually polished, dense enough for operator work, and animated
  where motion clarifies state.
- Keep the backend as auth/session/API/static-assets infrastructure.
- Preserve test coverage across frontend routes, mocked workflows, and backend
  behavior after UI removal.

## Non-Goals

- Implement production API integration for every screen in this phase.
- Add a separate Mintrud API section. This was explicitly removed from the
  navigation.
- Implement RBAC, external notifications, or advanced analytics logic now.
  These sections appear as polished placeholders or partial mock screens.
- Reuse the old Tabler/HTMX/templ UI.

## Chosen Approach

Use a "full shell plus mocked pages of varying depth" approach:

- MVP sections get realistic mocked layouts with tables, filters, detail
  drawers/pages, status badges, loading states, empty states, and error states.
- Post-MVP sections get polished "in development" screens in the same design
  system.
- The mock layer exposes the same kind of typed operations that the future API
  client will expose, so replacing mocks with HTTP calls does not require
  rewriting page components.

This balances product completeness with implementation risk. It avoids spending
too much time on post-MVP business logic while still making the application feel
whole.

## Information Architecture

The app uses a persistent left sidebar with collapsible groups. The group that
contains the current route is expanded automatically. User-opened group state is
remembered locally.

Top-level standalone route:

- `Рабочий стол`

Sidebar groups:

- `Операции`
  - `Заявки`
  - `Импорт`
  - `Протоколы`
  - `Документы`
  - `Moodle`
- `Реестр`
  - `Слушатели`
  - `Работодатели`
  - `Программы`
- `Контроль`
  - `Журнал`
  - `Аналитика`
- `Администрирование`
  - `Пользователи и роли`
  - `Уведомления`
  - `Настройки`

No `Минтруд API` route is included.

## Route Map

Public routes:

- `/login` - React login screen.

Authenticated app routes:

- `/` - redirects to `/dashboard`.
- `/dashboard` - operator dashboard.
- `/requests` - client requests list.
- `/requests/:requestId` - request detail.
- `/imports` - import runs and staging queue.
- `/imports/:importId` - import review detail.
- `/protocols` - protocol list.
- `/protocols/:protocolId` - protocol workflow detail.
- `/documents` - generation runs for XML/DOCX/XLSX.
- `/moodle` - Moodle accounts/enrollment/credentials mock area.
- `/workers` - worker registry.
- `/workers/:workerId` - worker detail.
- `/employers` - employer registry.
- `/employers/:employerId` - employer detail.
- `/programs` - program groups and programs.
- `/audit` - action log.
- `/analytics` - post-MVP placeholder.
- `/users` - post-MVP users/roles placeholder.
- `/notifications` - post-MVP notifications placeholder.
- `/settings` - partial mock settings area.

## Page Depth

### Рабочий стол

Operational overview with:

- counts for requests in review, import conflicts, blocked protocols, pending
  documents, and Moodle work;
- "requires attention" queue;
- recent activity feed;
- quick actions for new request, import, protocol, document run;
- mock/API mode indicator.

### Заявки

Mocked request list and details:

- filters by status, employer, received date, attention reason;
- dense table with request status, employer, row counts, conflict counts, and
  next action;
- detail page or drawer with request metadata, attached file summary, related
  import rows, and generated protocol candidates.

### Импорт

Mocked import workflow:

- upload/drop zone as visual mock;
- import runs list;
- staging rows table with statuses: new, duplicate, conflict, requires_review,
  invalid, imported, skipped;
- row-level actions: apply, skip, resolve conflict, choose canonical employer
  name, repair missing fields;
- conflict/detail drawer for row review;
- import progress and validation states.

### Протоколы

Mocked protocol workflow:

- list with number, group, employer, period, participants, XML/DOCX status,
  blocked reason, and next action;
- protocol detail with timeline: participants, fixation, XML, registry numbers,
  DOCX, close;
- participants table;
- gate panel showing concrete blocking reasons;
- generation history;
- manual status actions as mock commands.

### Документы

Mocked generation-run workspace:

- XML/DOCX/XLSX runs;
- status, generated time, related protocol/request, file name, mock download
  action;
- failure states and rebuild-needed states.

### Moodle

Mocked Moodle operations:

- enrollment queue;
- Moodle account status;
- generated credentials file state;
- warnings for manual review when identity matching is partial;
- "integration not connected" state that still allows UI testing.

### Реестр Sections

`Слушатели`, `Работодатели`, and `Программы` use shared registry patterns:

- searchable/filterable tables;
- detail drawer or page;
- status badges;
- related records;
- empty/loading/error states;
- mock create/edit actions that update local mock state where practical.

Programs include program groups, default hours, active/inactive state, and
optional Moodle course mapping.

### Контроль Sections

`Журнал` is a realistic mocked audit/action log with filters and structured
detail display.

`Аналитика` is a polished in-development screen showing planned metric groups
without pretending real analytics exist.

### Администрирование Sections

`Пользователи и роли` and `Уведомления` are polished post-MVP placeholders.

`Настройки` is a partial mock section for:

- template downloads;
- backup status;
- frontend mock/API mode;
- system information;
- local UI preferences.

## Visual Direction

Chosen direction: Quiet operations.

Characteristics:

- light main workspace;
- dark compact sidebar;
- restrained neutral palette with clear status colors;
- 8px or smaller radii for cards and controls;
- dense tables and scanning-friendly typography;
- icons from `lucide-react`;
- no Bootstrap/Material visual language;
- no decorative gradients, orbs, or bokeh backgrounds;
- no marketing-style hero sections.

The design should feel like a serious internal operations tool, not a landing
page. It can still be visually sophisticated through spacing, hierarchy,
motion, state design, and high-quality component composition.

## Animation Direction

Use Motion for React animations. The animation system should make state changes
clear without distracting operators.

Allowed animation patterns:

- route/page transitions;
- collapsible sidebar group transitions;
- detail drawer enter/exit;
- status badge changes;
- row updates after mock actions;
- import progress;
- protocol timeline active/blocked transitions;
- skeleton/loading transitions;
- empty-state entry transitions.

Motion rules:

- default durations: 120-240ms;
- use restrained easing;
- avoid looping animations in production UI except subtle progress/loading
  indicators;
- wrap the app in `MotionConfig` with `reducedMotion="user"`;
- avoid layout shifts by giving fixed-format UI elements stable dimensions.

The browser demo shown during design used looped CSS animations only to
communicate the motion language. The implementation should trigger animations
from real route/state changes.

## Frontend Architecture

Use React, TypeScript, and Vite.

Recommended libraries:

- `react-router` for nested SPA routing and layout routes;
- `motion` for animation;
- `@tanstack/react-table` for dense data tables;
- `lucide-react` for icons.

Do not adopt a heavy component framework for the first implementation. Build a
small local design system with:

- `AppShell`;
- `Sidebar`;
- `TopBar`;
- `PageHeader`;
- `Toolbar`;
- `DataTable`;
- `StatusBadge`;
- `MetricTile`;
- `DetailDrawer`;
- `EmptyState`;
- `LoadingState`;
- `ErrorState`;
- `PlaceholderPage`;
- form controls and segmented filters.

Suggested source layout:

```text
frontend/src/
  app/
    App.tsx
    router.tsx
    routes.ts
    shell/
  api/
    client.ts
    mock/
  components/
    data-table/
    feedback/
    layout/
    navigation/
    status/
  features/
    auth/
    dashboard/
    requests/
    imports/
    protocols/
    documents/
    moodle/
    workers/
    employers/
    programs/
    audit/
    analytics/
    users/
    notifications/
    settings/
  styles/
    tokens.css
    globals.css
```

## Mock Data Strategy

Mocks must be obviously synthetic and safe for public git:

- use `.example` or `example.test` domains;
- use test names and fake organizations;
- do not include realistic employee records, production emails, real INN/SNILS,
  or local credentials.

The mock layer should expose typed service functions, for example:

- `listRequests`;
- `getRequest`;
- `listImportRows`;
- `resolveImportRow`;
- `listProtocols`;
- `getProtocolWorkflow`;
- `listWorkers`;
- `listEmployers`;
- `listPrograms`;
- `listAuditEvents`.

Page components depend on these functions, not on raw fixture imports, so that
API replacement later is localized.

## Backend Boundary

The backend remains a Go service that owns:

- sessions;
- login/logout;
- CSRF or equivalent browser request protection;
- JSON APIs;
- static SPA assets;
- SQLite and domain services;
- document generation;
- import processing;
- audit.

The backend no longer owns:

- HTML layout rendering;
- Tabler/HTMX UI assets;
- `templ` components;
- server-rendered protected pages.

Proposed auth/session API shape:

- `GET /api/session` returns authentication state, user summary, and a CSRF
  token or CSRF bootstrap information usable by the SPA.
- `POST /api/login` accepts JSON credentials and starts a session.
- `POST /api/logout` destroys the session.

Protected JSON APIs should live under `/api/*`. The SPA fallback remains last
in route registration so browser refresh works on nested frontend routes.

## Backend UI Removal Plan

Remove or rewrite these backend UI surfaces during implementation:

- `backend/admin/views`;
- `backend/audit/views`;
- `backend/employers/views`;
- `backend/people/views`;
- `backend/programs/views`;
- `backend/protocols/views`;
- `backend/requests/views`;
- `backend/ui`;
- old `web/static/app.css`;
- imports of `*/views` from handlers;
- server-rendered HTML protected routes in `backend/app/routes.go`;
- `templ` dependency from `go.mod`;
- `templ generate` from CI, Dockerfile, and README.

Handlers that currently render HTML should either be removed until their API
exists or converted into JSON endpoints. Keep domain services and tests where
they are still useful.

## Testing Strategy

Frontend:

- Vitest + Testing Library for shell, sidebar groups, routing, page components,
  table interactions, mock service behavior, empty/loading/error states, and
  placeholders.
- Playwright for:
  - login screen;
  - dashboard rendering;
  - navigation to every route;
  - collapsible sidebar behavior;
  - request/import/protocol mock workflow;
  - post-MVP placeholder pages;
  - responsive layout smoke checks.

Backend:

- Update Go tests after removing templ and server-rendered routes.
- Keep auth/session tests.
- Add or update JSON API tests for login/logout/session.
- Keep storage/domain/document tests that remain relevant.

Verification commands for implementation:

```bash
go test ./...
npm --prefix frontend test
npm --prefix frontend run build
npm --prefix frontend run e2e
```

Docker verification must use the compose URL documented in `AGENTS.md`:
`http://localhost:8081/login`.

## Accessibility and Responsiveness

- Keyboard navigation must work for sidebar, tables, filters, drawers, and
  dialogs.
- Focus states must be visible.
- Reduced motion must be respected.
- Tables should stay usable on smaller screens through horizontal scrolling,
  responsive column priority, or detail cards where appropriate.
- On mobile/tablet width, sidebar becomes a drawer or compact rail.
- Text must not overflow buttons/cards; use stable dimensions and responsive
  constraints.

## Decisions Captured

- Build the whole site skeleton immediately, not one vertical slice.
- Remove all backend UI.
- Include the React login screen in the new frontend.
- Use collapsible grouped sidebar navigation.
- Include post-MVP sections as placeholders.
- Exclude a dedicated Mintrud API section.
- Use Quiet operations as the visual direction.
- Use Motion-style animations for meaningful UI state changes.
- Use React Router and TanStack Table as likely implementation dependencies.

## Implementation Planning Notes

The implementation plan should be split into safe milestones:

1. Add frontend dependencies, routing, shell, design tokens, and core components.
2. Build typed mocks and all route placeholders.
3. Fill MVP route pages with realistic mock tables and workflows.
4. Add animation layer and responsive behavior.
5. Convert backend auth/session to JSON-compatible SPA flow.
6. Remove old templ UI and related build dependencies.
7. Update tests, README, Dockerfile, and CI.

The exact API route conversion should be planned carefully because existing
backend handlers mix HTML form parsing, CSRF fields, redirects, and domain
service calls.
