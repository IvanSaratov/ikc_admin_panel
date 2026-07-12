# Frontend Shell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan stage-by-stage. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the complete mocked React frontend shell for Mintrud Admin, move login into the SPA, and remove all old Go `templ` UI.

**Architecture:** React/Vite owns all UI routes through a typed route map, local design system, mocked services, and client-side routing. Go owns sessions, JSON auth endpoints, domain services, document generation, static SPA serving, and future JSON APIs. Existing server-rendered handlers are converted or removed only after the mocked frontend shell is in place.

**Tech Stack:** React, TypeScript, Vite, React Router, Motion, TanStack Table, lucide-react, Vitest, Testing Library, Playwright, Go `net/http` + chi.

---

## Scope Check

The approved design covers a broad but single cohesive transition: full frontend shell plus backend UI removal. It is not split into separate specs because the navigation, mocked pages, auth boundary, and old UI cleanup are tightly coupled. Implementation is split into user-reviewed stages. Do not commit individual tasks. Commit only after completing a stage, presenting the result to the user, and receiving explicit approval.

## Stage-Gated Execution Protocol

This plan is executed by stage, not by individual task commit.

Rules for every stage:

1. Start the stage by reading the task list and confirming the stage goal.
2. Execute the tasks in order with TDD as written.
3. Keep changes uncommitted while the stage is in progress.
4. Run the stage verification commands before showing the result.
5. Show the user:
   - what changed;
   - what routes/screens are available;
   - screenshots or a local URL when the stage has visible UI;
   - exact verification commands and results;
   - `git status --short`.
6. Ask: `Approve Stage N commit?`
7. If the user requests changes, make them, rerun verification, and show the stage again.
8. Commit only after explicit approval.

If a stage is too large in practice, stop before expanding scope, explain the split, and ask the user to approve a smaller stage boundary. Do not invent additional commits without approval.

## Stage Breakdown

### Stage 1: Frontend Foundation

Tasks: 1-5.

Goal: install frontend libraries, create route metadata, design tokens, typed mocks, app shell, shared feedback/status/table/drawer components, and stub routes.

Verification before showing:

```bash
npm --prefix frontend test
npm --prefix frontend run build
```

Show the user:

- `/dashboard` stub shell;
- collapsible sidebar groups;
- top bar;
- route stubs for all planned sections.

Commit after approval:

```bash
git add frontend/package.json frontend/package-lock.json frontend/src
git commit -m "feat(frontend): add shell foundation"
```

### Stage 2: Mocked Application Pages and Motion

Tasks: 6-10.

Goal: replace stubs with mocked dashboard, registries, operations workflow, audit/settings, in-development sections, responsive behavior, and Motion animations.

Verification before showing:

```bash
npm --prefix frontend test
npm --prefix frontend run build
```

Show the user:

- dashboard;
- requests/import/protocol mock workflow;
- registries;
- audit/settings;
- in-development sections;
- animation examples in the real app, not the temporary companion demo.

Commit after approval:

```bash
git add frontend/src
git commit -m "feat(frontend): add mocked admin pages"
```

### Stage 3: Frontend E2E Coverage

Task: 11.

Goal: replace the old Playwright smoke with route coverage and workflow coverage for the full mocked frontend.

Verification before showing:

```bash
npm --prefix frontend test
npm --prefix frontend run build
npm --prefix frontend run e2e
```

Show the user:

- Playwright route coverage summary;
- workflow test summary;
- any screenshots/traces only if failures occurred.

Commit after approval:

```bash
git add frontend/tests
git commit -m "test(frontend): cover shell routes and mock workflow"
```

### Stage 4: SPA Login and JSON Auth Boundary

Task: 12.

Goal: add backend JSON session/login/logout endpoints and connect the React login route to them.

Verification before showing:

```bash
go test ./backend/admin ./backend/app
npm --prefix frontend test
npm --prefix frontend run build
```

Show the user:

- `/login` React screen;
- successful mock/manual login flow if a local backend is running;
- backend auth API test results.

Commit after approval:

```bash
git add backend/admin backend/app frontend/src/features/auth frontend/src/api/client.ts frontend/src/app/router.tsx frontend/src/styles/globals.css
git commit -m "feat: add SPA login and JSON auth API"
```

### Stage 5: Backend UI Removal and Build Cleanup

Tasks: 13-14.

Goal: remove old `templ` UI, server-rendered protected pages, Tabler/HTMX UI assets, templ generation, and documentation references.

Verification before showing:

```bash
sh tests/run_schema_tests.sh
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
go test ./...
npm --prefix frontend test
npm --prefix frontend run build
```

Show the user:

- removed backend UI paths;
- updated CI/Docker/README;
- confirmation that SPA fallback serves frontend routes;
- `go.mod` no longer depends on `github.com/a-h/templ`.

Commit after approval:

```bash
git add backend web go.mod go.sum .github/workflows/ci.yml Dockerfile README.md
git commit -m "refactor: remove backend-rendered UI"
```

### Stage 6: Final Public-Git Hygiene and Docker Smoke

Task: 15.

Goal: run the final public-git hygiene audit and Docker compose smoke test.

Verification before showing:

```bash
git status --short --ignored
git ls-files -z | xargs -0 rg -l --hidden --no-ignore -i '(password|secret|token|api[_-]?key|private[_-]?key|BEGIN .*PRIVATE KEY|ИНН|СНИЛС|паспорт|email|@)'
rg -l --hidden --no-ignore -i --glob '!.env' --glob '!.env.*' --glob '!*.db' --glob '!*.sqlite' --glob '!*.sqlite3' '(password|secret|token|api[_-]?key|private[_-]?key|BEGIN .*PRIVATE KEY|ИНН|СНИЛС|паспорт|email|@)' .
DOCKER_BUILDKIT=1 docker compose up --build -d
docker compose ps
curl -sS -o /dev/null -w '%{http_code}\n' http://localhost:8081/login
docker compose logs --tail=80 app
docker compose down
```

Show the user:

- hygiene findings summarized by path/category, never secret values;
- Docker health status;
- `/login` HTTP status from `http://localhost:8081/login`.

Commit after approval only if fixes were needed:

```bash
git add <changed-files>
git commit -m "chore: finish frontend shell verification"
```

If no files changed, report that no final commit is needed.

## File Structure Map

Create or replace frontend app infrastructure:

- Create `frontend/src/app/routes.ts` - route and navigation metadata.
- Create `frontend/src/app/router.tsx` - `createBrowserRouter` configuration.
- Replace `frontend/src/app/App.tsx` - router provider and motion provider.
- Modify `frontend/src/main.tsx` - keep root render, import new global styles.
- Create `frontend/src/app/shell/AppShell.tsx` - authenticated layout.
- Create `frontend/src/app/shell/AppShell.test.tsx` - shell routing/nav tests.
- Create `frontend/src/app/shell/Sidebar.tsx` - collapsible sidebar.
- Create `frontend/src/app/shell/TopBar.tsx` - search/user/mock status top bar.

Create local design system:

- Create `frontend/src/styles/tokens.css` - CSS custom properties.
- Create `frontend/src/styles/globals.css` - global reset and app layout styles.
- Remove or replace `frontend/src/styles.css`.
- Create `frontend/src/components/layout/PageHeader.tsx`.
- Create `frontend/src/components/layout/Toolbar.tsx`.
- Create `frontend/src/components/status/StatusBadge.tsx`.
- Create `frontend/src/components/data-table/DataTable.tsx`.
- Create `frontend/src/components/feedback/EmptyState.tsx`.
- Create `frontend/src/components/feedback/LoadingState.tsx`.
- Create `frontend/src/components/feedback/ErrorState.tsx`.
- Create `frontend/src/components/feedback/InDevelopmentPage.tsx`.
- Create `frontend/src/components/drawer/DetailDrawer.tsx`.
- Create focused tests next to each non-trivial component.

Create typed mocks:

- Create `frontend/src/api/mock/types.ts`.
- Create `frontend/src/api/mock/fixtures.ts`.
- Create `frontend/src/api/mock/mockStore.ts`.
- Create `frontend/src/api/mock/services.ts`.
- Modify `frontend/src/api/client.ts` to export typed service functions backed by mocks for now.
- Remove or fold `frontend/src/api/mockProtocolWorkflow.ts` into the new mock layer.

Create feature routes:

- Create `frontend/src/features/auth/LoginPage.tsx`.
- Create `frontend/src/features/auth/LoginPage.test.tsx`.
- Create `frontend/src/features/dashboard/DashboardPage.tsx`.
- Create `frontend/src/features/requests/RequestsPage.tsx`.
- Create `frontend/src/features/requests/RequestDetailPage.tsx`.
- Create `frontend/src/features/imports/ImportsPage.tsx`.
- Create `frontend/src/features/imports/ImportDetailPage.tsx`.
- Create `frontend/src/features/protocols/ProtocolsPage.tsx`.
- Create or replace `frontend/src/features/protocol-workflow/ProtocolWorkflowPage.tsx` with `frontend/src/features/protocols/ProtocolDetailPage.tsx`.
- Create `frontend/src/features/documents/DocumentsPage.tsx`.
- Create `frontend/src/features/moodle/MoodlePage.tsx`.
- Create `frontend/src/features/workers/WorkersPage.tsx`.
- Create `frontend/src/features/workers/WorkerDetailPage.tsx`.
- Create `frontend/src/features/employers/EmployersPage.tsx`.
- Create `frontend/src/features/employers/EmployerDetailPage.tsx`.
- Create `frontend/src/features/programs/ProgramsPage.tsx`.
- Create `frontend/src/features/audit/AuditPage.tsx`.
- Create `frontend/src/features/analytics/AnalyticsPage.tsx`.
- Create `frontend/src/features/users/UsersPage.tsx`.
- Create `frontend/src/features/notifications/NotificationsPage.tsx`.
- Create `frontend/src/features/settings/SettingsPage.tsx`.

Create or update e2e tests:

- Replace `frontend/tests/protocol-workflow.spec.ts` with broader route coverage.
- Create `frontend/tests/app-shell.spec.ts`.
- Create `frontend/tests/mock-workflow.spec.ts`.

Backend auth/API and cleanup:

- Create `backend/admin/api_handler.go` - JSON login/logout/session handlers.
- Create `backend/admin/api_handler_test.go` - JSON auth handler tests.
- Modify `backend/app/api_routes.go` - register auth/session JSON endpoints.
- Modify `backend/app/routes.go` - stop registering server-rendered protected UI routes after equivalent API behavior is in place.
- Modify `backend/app/router_test.go` - replace old HTML shell assertions with JSON/session/SPAs tests.
- Remove `backend/*/views`, `backend/ui`, `web/static/app.css`.
- Modify handlers that import `*/views`; either convert to API handlers or remove route wiring.
- Modify `go.mod` and `go.sum` to remove `github.com/a-h/templ`.
- Modify `.github/workflows/ci.yml` to remove `templ generate`.
- Modify `Dockerfile` to remove templ installation/generation.
- Modify `README.md` to document React frontend and no templ generation.

## Task 1: Frontend Dependencies and Route Metadata

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/package-lock.json`
- Create: `frontend/src/app/routes.ts`
- Create: `frontend/src/app/routes.test.ts`

- [ ] **Step 1: Write route metadata tests**

Create `frontend/src/app/routes.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { navGroups, routeIds, routesByPath } from "./routes";

describe("app routes", () => {
  it("contains the approved grouped navigation without Mintrud API", () => {
    expect(routeIds).toContain("dashboard");
    expect(navGroups.map((group) => group.label)).toEqual([
      "Операции",
      "Реестр",
      "Контроль",
      "Администрирование"
    ]);
    expect(navGroups.flatMap((group) => group.items.map((item) => item.label))).toEqual([
      "Заявки",
      "Импорт",
      "Протоколы",
      "Документы",
      "Moodle",
      "Слушатели",
      "Работодатели",
      "Программы",
      "Журнал",
      "Аналитика",
      "Пользователи и роли",
      "Уведомления",
      "Настройки"
    ]);
    expect(Object.values(routesByPath).some((route) => route.label.includes("Минтруд API"))).toBe(false);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
npm --prefix frontend test -- src/app/routes.test.ts
```

Expected: FAIL because `frontend/src/app/routes.ts` does not exist.

- [ ] **Step 3: Install frontend dependencies**

Run:

```bash
npm --prefix frontend install react-router motion @tanstack/react-table
```

Expected: `frontend/package.json` and `frontend/package-lock.json` update. `lucide-react` is already present and remains.

- [ ] **Step 4: Add route metadata**

Create `frontend/src/app/routes.ts`:

```ts
import {
  BarChart3,
  Bell,
  BookOpen,
  BriefcaseBusiness,
  ClipboardList,
  Database,
  FileArchive,
  FileText,
  GraduationCap,
  History,
  Home,
  Settings,
  ShieldCheck,
  UploadCloud,
  Users
} from "lucide-react";
import type { ComponentType } from "react";

export type RouteId =
  | "dashboard"
  | "requests"
  | "requestDetail"
  | "imports"
  | "importDetail"
  | "protocols"
  | "protocolDetail"
  | "documents"
  | "moodle"
  | "workers"
  | "workerDetail"
  | "employers"
  | "employerDetail"
  | "programs"
  | "audit"
  | "analytics"
  | "users"
  | "notifications"
  | "settings";

export interface AppRoute {
  id: RouteId;
  path: string;
  label: string;
  description: string;
  icon: ComponentType<{ className?: string; "aria-hidden"?: boolean }>;
  group?: "operations" | "registry" | "control" | "admin";
  inDevelopment?: boolean;
}

export const appRoutes: AppRoute[] = [
  {
    id: "dashboard",
    path: "/dashboard",
    label: "Рабочий стол",
    description: "Операционный обзор заявок, протоколов и очередей внимания.",
    icon: Home
  },
  {
    id: "requests",
    path: "/requests",
    label: "Заявки",
    description: "Клиентские заявки и их текущий статус.",
    icon: ClipboardList,
    group: "operations"
  },
  {
    id: "requestDetail",
    path: "/requests/:requestId",
    label: "Карточка заявки",
    description: "Детали заявки и связанные строки импорта.",
    icon: ClipboardList,
    group: "operations"
  },
  {
    id: "imports",
    path: "/imports",
    label: "Импорт",
    description: "XLSX-загрузки, staging rows, дубли и конфликты.",
    icon: UploadCloud,
    group: "operations"
  },
  {
    id: "importDetail",
    path: "/imports/:importId",
    label: "Разбор импорта",
    description: "Проверка строк, конфликтов и решений оператора.",
    icon: UploadCloud,
    group: "operations"
  },
  {
    id: "protocols",
    path: "/protocols",
    label: "Протоколы",
    description: "Протоколы, статусы XML/DOCX и gate-причины.",
    icon: FileText,
    group: "operations"
  },
  {
    id: "protocolDetail",
    path: "/protocols/:protocolId",
    label: "Карточка протокола",
    description: "Workflow протокола, участники и документы.",
    icon: FileText,
    group: "operations"
  },
  {
    id: "documents",
    path: "/documents",
    label: "Документы",
    description: "История генерации XML, DOCX и XLSX.",
    icon: FileArchive,
    group: "operations"
  },
  {
    id: "moodle",
    path: "/moodle",
    label: "Moodle",
    description: "Зачисления, аккаунты и файл учетных данных.",
    icon: GraduationCap,
    group: "operations"
  },
  {
    id: "workers",
    path: "/workers",
    label: "Слушатели",
    description: "Реестр физических лиц и их обучений.",
    icon: Users,
    group: "registry"
  },
  {
    id: "workerDetail",
    path: "/workers/:workerId",
    label: "Карточка слушателя",
    description: "Данные слушателя, работодатели и обучения.",
    icon: Users,
    group: "registry"
  },
  {
    id: "employers",
    path: "/employers",
    label: "Работодатели",
    description: "Организации, ИНН, заявки и слушатели.",
    icon: BriefcaseBusiness,
    group: "registry"
  },
  {
    id: "employerDetail",
    path: "/employers/:employerId",
    label: "Карточка работодателя",
    description: "Компания, связанные заявки и слушатели.",
    icon: BriefcaseBusiness,
    group: "registry"
  },
  {
    id: "programs",
    path: "/programs",
    label: "Программы",
    description: "Группы программ, часы и Moodle mapping.",
    icon: BookOpen,
    group: "registry"
  },
  {
    id: "audit",
    path: "/audit",
    label: "Журнал",
    description: "Действия оператора и системные события.",
    icon: History,
    group: "control"
  },
  {
    id: "analytics",
    path: "/analytics",
    label: "Аналитика",
    description: "Будущие управленческие метрики.",
    icon: BarChart3,
    group: "control",
    inDevelopment: true
  },
  {
    id: "users",
    path: "/users",
    label: "Пользователи и роли",
    description: "Будущая RBAC-модель.",
    icon: ShieldCheck,
    group: "admin",
    inDevelopment: true
  },
  {
    id: "notifications",
    path: "/notifications",
    label: "Уведомления",
    description: "Будущие email и Telegram уведомления.",
    icon: Bell,
    group: "admin",
    inDevelopment: true
  },
  {
    id: "settings",
    path: "/settings",
    label: "Настройки",
    description: "Шаблоны, backup, режим моков и системная информация.",
    icon: Settings,
    group: "admin"
  }
];

export const routeIds = appRoutes.map((route) => route.id);

export const routesByPath = Object.fromEntries(
  appRoutes.map((route) => [route.path, route])
) as Record<string, AppRoute>;

export const navGroups = [
  {
    id: "operations",
    label: "Операции",
    items: appRoutes.filter((route) => route.group === "operations" && !route.path.includes(":"))
  },
  {
    id: "registry",
    label: "Реестр",
    items: appRoutes.filter((route) => route.group === "registry" && !route.path.includes(":"))
  },
  {
    id: "control",
    label: "Контроль",
    items: appRoutes.filter((route) => route.group === "control")
  },
  {
    id: "admin",
    label: "Администрирование",
    items: appRoutes.filter((route) => route.group === "admin")
  }
] as const;

export const dashboardRoute = appRoutes.find((route) => route.id === "dashboard")!;
```

- [ ] **Step 5: Run test to verify it passes**

Run:

```bash
npm --prefix frontend test -- src/app/routes.test.ts
```

Expected: PASS.

- [ ] **Step 6: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 2: Design Tokens and Feedback Components

**Files:**
- Create: `frontend/src/styles/tokens.css`
- Create: `frontend/src/styles/globals.css`
- Modify: `frontend/src/main.tsx`
- Delete: `frontend/src/styles.css` after imports are switched
- Create: `frontend/src/components/feedback/EmptyState.tsx`
- Create: `frontend/src/components/feedback/LoadingState.tsx`
- Create: `frontend/src/components/feedback/ErrorState.tsx`
- Create: `frontend/src/components/feedback/InDevelopmentPage.tsx`
- Create: `frontend/src/components/feedback/feedback.test.tsx`

- [ ] **Step 1: Write feedback component tests**

Create `frontend/src/components/feedback/feedback.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EmptyState } from "./EmptyState";
import { ErrorState } from "./ErrorState";
import { InDevelopmentPage } from "./InDevelopmentPage";
import { LoadingState } from "./LoadingState";

describe("feedback components", () => {
  it("renders empty state with action", () => {
    render(<EmptyState title="Нет заявок" description="Загрузите XLSX" actionLabel="Новая заявка" />);

    expect(screen.getByRole("heading", { name: "Нет заявок" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Новая заявка" })).toBeInTheDocument();
  });

  it("renders loading status accessibly", () => {
    render(<LoadingState label="Загрузка заявок" />);

    expect(screen.getByRole("status", { name: "Загрузка заявок" })).toBeInTheDocument();
  });

  it("renders error state with retry", () => {
    render(<ErrorState title="Ошибка импорта" description="Файл не прочитан" actionLabel="Повторить" />);

    expect(screen.getByRole("alert")).toHaveTextContent("Ошибка импорта");
    expect(screen.getByRole("button", { name: "Повторить" })).toBeInTheDocument();
  });

  it("renders in-development page with planned capabilities", () => {
    render(
      <InDevelopmentPage
        title="Аналитика"
        description="Раздел готовится"
        planned={["Динамика заявок", "Статусы протоколов"]}
      />
    );

    expect(screen.getByRole("heading", { name: "Аналитика" })).toBeInTheDocument();
    expect(screen.getByText("Динамика заявок")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
npm --prefix frontend test -- src/components/feedback/feedback.test.tsx
```

Expected: FAIL because feedback components do not exist.

- [ ] **Step 3: Add design tokens**

Create `frontend/src/styles/tokens.css`:

```css
:root {
  color-scheme: light;
  --color-bg: #f4f6f8;
  --color-surface: #ffffff;
  --color-surface-muted: #f8fafc;
  --color-sidebar: #17202a;
  --color-sidebar-muted: #223044;
  --color-text: #17202a;
  --color-text-muted: #64748b;
  --color-line: #d7dde5;
  --color-line-soft: #eef2f6;
  --color-primary: #2563eb;
  --color-primary-weak: #eff6ff;
  --color-success: #15803d;
  --color-success-weak: #dcfce7;
  --color-warning: #b45309;
  --color-warning-weak: #fef3c7;
  --color-danger: #b91c1c;
  --color-danger-weak: #fee2e2;
  --radius-sm: 6px;
  --radius-md: 8px;
  --shadow-panel: 0 1px 2px rgba(16, 24, 40, 0.04);
  --shadow-drawer: 0 16px 40px rgba(16, 24, 40, 0.12);
  --space-1: 4px;
  --space-2: 8px;
  --space-3: 12px;
  --space-4: 16px;
  --space-5: 20px;
  --space-6: 24px;
  --font-sans: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}
```

Create `frontend/src/styles/globals.css`:

```css
@import "./tokens.css";

* {
  box-sizing: border-box;
}

body {
  margin: 0;
  min-width: 320px;
  min-height: 100vh;
  font-family: var(--font-sans);
  background: var(--color-bg);
  color: var(--color-text);
  letter-spacing: 0;
}

button,
input,
select,
textarea {
  font: inherit;
}

button {
  cursor: pointer;
}

a {
  color: inherit;
}

:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
```

Modify `frontend/src/main.tsx` to import `./styles/globals.css` instead of `./styles.css`.

- [ ] **Step 4: Add feedback components**

Create `frontend/src/components/feedback/EmptyState.tsx`:

```tsx
interface EmptyStateProps {
  title: string;
  description: string;
  actionLabel?: string;
  onAction?: () => void;
}

export function EmptyState({ title, description, actionLabel, onAction }: EmptyStateProps) {
  return (
    <section className="empty-state">
      <h2>{title}</h2>
      <p>{description}</p>
      {actionLabel ? (
        <button className="button button-primary" type="button" onClick={onAction}>
          {actionLabel}
        </button>
      ) : null}
    </section>
  );
}
```

Create `frontend/src/components/feedback/LoadingState.tsx`:

```tsx
interface LoadingStateProps {
  label: string;
}

export function LoadingState({ label }: LoadingStateProps) {
  return (
    <div className="loading-state" role="status" aria-label={label}>
      <span className="loading-dot" aria-hidden="true" />
      <span>{label}</span>
    </div>
  );
}
```

Create `frontend/src/components/feedback/ErrorState.tsx`:

```tsx
interface ErrorStateProps {
  title: string;
  description: string;
  actionLabel?: string;
  onAction?: () => void;
}

export function ErrorState({ title, description, actionLabel, onAction }: ErrorStateProps) {
  return (
    <section className="error-state" role="alert">
      <h2>{title}</h2>
      <p>{description}</p>
      {actionLabel ? (
        <button className="button button-secondary" type="button" onClick={onAction}>
          {actionLabel}
        </button>
      ) : null}
    </section>
  );
}
```

Create `frontend/src/components/feedback/InDevelopmentPage.tsx`:

```tsx
interface InDevelopmentPageProps {
  title: string;
  description: string;
  planned: string[];
}

export function InDevelopmentPage({ title, description, planned }: InDevelopmentPageProps) {
  return (
    <section className="in-dev-page">
      <p className="eyebrow">Сейчас в разработке</p>
      <h1>{title}</h1>
      <p>{description}</p>
      <div className="in-dev-panel">
        <h2>Планируемые возможности</h2>
        <ul>
          {planned.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      </div>
    </section>
  );
}
```

Append component styles to `frontend/src/styles/globals.css`:

```css
.button {
  min-height: 36px;
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  padding: 8px 12px;
  font-weight: 700;
}

.button-primary {
  background: var(--color-primary);
  color: white;
}

.button-secondary {
  background: var(--color-surface);
  border-color: var(--color-line);
  color: var(--color-text);
}

.empty-state,
.error-state,
.in-dev-page {
  display: grid;
  gap: var(--space-3);
  max-width: 720px;
}

.empty-state,
.error-state {
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  padding: var(--space-6);
  box-shadow: var(--shadow-panel);
}

.error-state {
  border-color: var(--color-danger-weak);
}

.loading-state {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--color-text-muted);
}

.loading-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: var(--color-primary);
}

.eyebrow {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
}

.in-dev-panel {
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  padding: var(--space-5);
}
```

- [ ] **Step 5: Run tests and build**

```bash
npm --prefix frontend test -- src/components/feedback/feedback.test.tsx
npm --prefix frontend run build
```

Expected: both PASS.

- [ ] **Step 6: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 3: Mock Domain Model and Services

**Files:**
- Create: `frontend/src/api/mock/types.ts`
- Create: `frontend/src/api/mock/fixtures.ts`
- Create: `frontend/src/api/mock/mockStore.ts`
- Create: `frontend/src/api/mock/services.ts`
- Create: `frontend/src/api/mock/services.test.ts`
- Modify: `frontend/src/api/client.ts`

- [ ] **Step 1: Write mock service tests**

Create `frontend/src/api/mock/services.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import {
  getProtocolWorkflow,
  listAuditEvents,
  listEmployers,
  listImportRows,
  listPrograms,
  listRequests,
  listWorkers,
  resolveImportRow
} from "./services";

describe("mock services", () => {
  it("uses obviously synthetic fixture data", async () => {
    const workers = await listWorkers();
    const employers = await listEmployers();

    expect(workers.every((worker) => worker.email.endsWith(".example") || worker.email.endsWith("example.test"))).toBe(true);
    expect(employers.every((employer) => employer.name.includes("Тест") || employer.name.includes("Пример"))).toBe(true);
  });

  it("connects request, import rows, and protocol workflow", async () => {
    const requests = await listRequests();
    const rows = await listImportRows("import-1");
    const workflow = await getProtocolWorkflow("protocol-2605-a-15");

    expect(requests[0].id).toBe("request-1");
    expect(rows.some((row) => row.status === "conflict")).toBe(true);
    expect(workflow.stages.map((stage) => stage.id)).toEqual([
      "participants",
      "fix",
      "xml",
      "registry",
      "docx",
      "closed"
    ]);
  });

  it("updates import row status in the mock store", async () => {
    await resolveImportRow("row-2", "skipped");
    const rows = await listImportRows("import-1");

    expect(rows.find((row) => row.id === "row-2")?.status).toBe("skipped");
  });

  it("exposes programs and audit events", async () => {
    await expect(listPrograms()).resolves.toHaveLength(5);
    await expect(listAuditEvents()).resolves.toEqual(
      expect.arrayContaining([expect.objectContaining({ action: "import.row.conflict" })])
    );
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
npm --prefix frontend test -- src/api/mock/services.test.ts
```

Expected: FAIL because mock service files do not exist.

- [ ] **Step 3: Add mock types**

Create `frontend/src/api/mock/types.ts` with these exported types:

```ts
export type AttentionLevel = "normal" | "warning" | "danger";
export type RequestStatus = "review" | "ready" | "completed" | "cancelled";
export type ImportRowStatus = "new" | "duplicate" | "conflict" | "requires_review" | "invalid" | "imported" | "skipped";
export type ProtocolStageState = "done" | "active" | "blocked" | "pending";
export type GenerationStatus = "success" | "failed" | "needs_rebuild" | "running";
export type MoodleStatus = "not_started" | "queued" | "enrolled" | "review_required" | "failed";

export interface ClientRequest {
  id: string;
  employerId: string;
  employerName: string;
  receivedDate: string;
  status: RequestStatus;
  rowsTotal: number;
  rowsNeedReview: number;
  nextAction: string;
  attention: AttentionLevel;
}

export interface ImportRun {
  id: string;
  requestId: string;
  fileName: string;
  uploadedAt: string;
  rowsTotal: number;
  status: "review" | "completed";
}

export interface ImportRow {
  id: string;
  importId: string;
  rowNumber: number;
  fullName: string;
  snils: string;
  email: string;
  employerName: string;
  position: string;
  programs: string[];
  status: ImportRowStatus;
  reason: string;
}

export interface ProtocolStage {
  id: "participants" | "fix" | "xml" | "registry" | "docx" | "closed";
  label: string;
  state: ProtocolStageState;
  reason?: string;
}

export interface ProtocolWorkflow {
  id: string;
  number: string;
  employerName: string;
  programGroup: string;
  period: string;
  participants: number;
  stages: ProtocolStage[];
}

export interface Worker {
  id: string;
  fullName: string;
  snils: string;
  email: string;
  employerName: string;
  position: string;
  activeTrainings: number;
}

export interface Employer {
  id: string;
  name: string;
  inn: string;
  status: "active" | "inactive";
  requests: number;
  workers: number;
}

export interface Program {
  id: string;
  groupCode: string;
  code: string;
  name: string;
  defaultHours: number;
  status: "active" | "inactive";
  moodleCourseId?: string;
}

export interface GenerationRun {
  id: string;
  type: "xml" | "docx" | "xlsx" | "moodle_credentials";
  status: GenerationStatus;
  relatedEntity: string;
  fileName: string;
  generatedAt: string;
}

export interface MoodleAccount {
  id: string;
  workerName: string;
  employerName: string;
  email: string;
  status: MoodleStatus;
  course: string;
}

export interface AuditEvent {
  id: string;
  at: string;
  actor: string;
  action: string;
  entity: string;
  details: string;
}
```

- [ ] **Step 4: Add fixtures, store, and services**

Create `frontend/src/api/mock/fixtures.ts` with synthetic arrays for every type. Use these exact IDs where referenced by tests:

```ts
import type {
  AuditEvent,
  ClientRequest,
  Employer,
  GenerationRun,
  ImportRow,
  ImportRun,
  MoodleAccount,
  Program,
  ProtocolWorkflow,
  Worker
} from "./types";

export const requests: ClientRequest[] = [
  {
    id: "request-1",
    employerId: "employer-1",
    employerName: "ООО Тест-Сервис",
    receivedDate: "2026-07-08",
    status: "review",
    rowsTotal: 18,
    rowsNeedReview: 7,
    nextAction: "Разобрать конфликты импорта",
    attention: "danger"
  },
  {
    id: "request-2",
    employerId: "employer-2",
    employerName: "АО Пример-Проект",
    receivedDate: "2026-07-09",
    status: "ready",
    rowsTotal: 11,
    rowsNeedReview: 0,
    nextAction: "Сформировать протокол",
    attention: "normal"
  }
];

export const importRuns: ImportRun[] = [
  {
    id: "import-1",
    requestId: "request-1",
    fileName: "client-request-test-service.xlsx",
    uploadedAt: "2026-07-08 11:24",
    rowsTotal: 18,
    status: "review"
  }
];

export const importRows: ImportRow[] = [
  {
    id: "row-1",
    importId: "import-1",
    rowNumber: 2,
    fullName: "Иванов Иван Иванович",
    snils: "000-000-001 00",
    email: "ivanov@example.test",
    employerName: "ООО Тест-Сервис",
    position: "Инженер",
    programs: ["А-1", "П-1"],
    status: "new",
    reason: "Новая строка готова к применению"
  },
  {
    id: "row-2",
    importId: "import-1",
    rowNumber: 3,
    fullName: "Петров Петр Петрович",
    snils: "000-000-002 00",
    email: "petrov@example.test",
    employerName: "ООО Тест-Сервис",
    position: "Мастер участка",
    programs: ["А-1"],
    status: "conflict",
    reason: "СНИЛС совпал, ФИО отличается от существующей карточки"
  },
  {
    id: "row-3",
    importId: "import-1",
    rowNumber: 4,
    fullName: "Сидорова Анна Сергеевна",
    snils: "000-000-003 00",
    email: "training@example.test",
    employerName: "ООО Тест-Сервис",
    position: "Специалист",
    programs: ["В-1"],
    status: "duplicate",
    reason: "Точная копия ранее импортированной строки"
  }
];

export const protocols: ProtocolWorkflow[] = [
  {
    id: "protocol-2605-a-15",
    number: "2605А15",
    employerName: "ООО Тест-Сервис",
    programGroup: "А",
    period: "2026-07-01 - 2026-07-05",
    participants: 18,
    stages: [
      { id: "participants", label: "Участники", state: "done" },
      { id: "fix", label: "Фиксация", state: "done" },
      { id: "xml", label: "XML", state: "active" },
      { id: "registry", label: "Реестровые номера", state: "blocked", reason: "Заполните номера Минтруда для 3 участников" },
      { id: "docx", label: "DOCX", state: "pending" },
      { id: "closed", label: "Закрытие", state: "pending" }
    ]
  }
];

export const workers: Worker[] = [
  {
    id: "worker-1",
    fullName: "Иванов Иван Иванович",
    snils: "000-000-001 00",
    email: "ivanov@example.test",
    employerName: "ООО Тест-Сервис",
    position: "Инженер",
    activeTrainings: 2
  },
  {
    id: "worker-2",
    fullName: "Петров Петр Петрович",
    snils: "000-000-002 00",
    email: "petrov@example.test",
    employerName: "АО Пример-Проект",
    position: "Мастер участка",
    activeTrainings: 1
  }
];

export const employers: Employer[] = [
  { id: "employer-1", name: "ООО Тест-Сервис", inn: "0000000000", status: "active", requests: 3, workers: 18 },
  { id: "employer-2", name: "АО Пример-Проект", inn: "0000000001", status: "active", requests: 2, workers: 11 }
];

export const programs: Program[] = [
  { id: "program-a-1", groupCode: "А", code: "А-1", name: "Общие вопросы охраны труда", defaultHours: 40, status: "active", moodleCourseId: "course-example-a1" },
  { id: "program-b-1", groupCode: "Б", code: "Б-1", name: "Безопасные методы работ", defaultHours: 16, status: "active" },
  { id: "program-v-1", groupCode: "В", code: "В-1", name: "Работы повышенной опасности", defaultHours: 24, status: "active" },
  { id: "program-p-1", groupCode: "П", code: "П-1", name: "Первая помощь", defaultHours: 16, status: "active" },
  { id: "program-s-1", groupCode: "С", code: "С-1", name: "Средства индивидуальной защиты", defaultHours: 8, status: "active" }
];

export const generationRuns: GenerationRun[] = [
  { id: "run-1", type: "xml", status: "success", relatedEntity: "2605А15", fileName: "2605A15.xml", generatedAt: "2026-07-10 14:20" },
  { id: "run-2", type: "docx", status: "needs_rebuild", relatedEntity: "2605А15", fileName: "2605A15.zip", generatedAt: "2026-07-10 15:12" }
];

export const moodleAccounts: MoodleAccount[] = [
  { id: "moodle-1", workerName: "Иванов Иван Иванович", employerName: "ООО Тест-Сервис", email: "ivanov@example.test", status: "enrolled", course: "А-1" },
  { id: "moodle-2", workerName: "Петров Петр Петрович", employerName: "АО Пример-Проект", email: "petrov@example.test", status: "review_required", course: "Б-1" }
];

export const auditEvents: AuditEvent[] = [
  { id: "audit-1", at: "2026-07-10 15:18", actor: "operator_unidentified", action: "import.row.conflict", entity: "row-2", details: "СНИЛС совпал, ФИО отличается" },
  { id: "audit-2", at: "2026-07-10 15:22", actor: "system", action: "protocol.xml.generated", entity: "2605А15", details: "XML сформирован из mock workflow" }
];
```

Create `frontend/src/api/mock/mockStore.ts`:

```ts
import { auditEvents, employers, generationRuns, importRows, importRuns, moodleAccounts, programs, protocols, requests, workers } from "./fixtures";
import type { ImportRowStatus } from "./types";

const store = {
  requests: structuredClone(requests),
  importRuns: structuredClone(importRuns),
  importRows: structuredClone(importRows),
  protocols: structuredClone(protocols),
  workers: structuredClone(workers),
  employers: structuredClone(employers),
  programs: structuredClone(programs),
  generationRuns: structuredClone(generationRuns),
  moodleAccounts: structuredClone(moodleAccounts),
  auditEvents: structuredClone(auditEvents)
};

export function getMockStore() {
  return store;
}

export function setImportRowStatus(rowId: string, status: ImportRowStatus) {
  const row = store.importRows.find((item) => item.id === rowId);
  if (!row) {
    throw new Error(`Import row ${rowId} not found`);
  }
  row.status = status;
  row.reason = status === "skipped" ? "Оператор пропустил строку в mock workflow" : row.reason;
}
```

Create `frontend/src/api/mock/services.ts`:

```ts
import { getMockStore, setImportRowStatus } from "./mockStore";
import type { ImportRowStatus } from "./types";

const wait = () => new Promise((resolve) => window.setTimeout(resolve, 20));

export async function listRequests() {
  await wait();
  return getMockStore().requests;
}

export async function listImportRuns() {
  await wait();
  return getMockStore().importRuns;
}

export async function listImportRows(importId: string) {
  await wait();
  return getMockStore().importRows.filter((row) => row.importId === importId);
}

export async function resolveImportRow(rowId: string, status: ImportRowStatus) {
  await wait();
  setImportRowStatus(rowId, status);
}

export async function listProtocols() {
  await wait();
  return getMockStore().protocols;
}

export async function getProtocolWorkflow(protocolId: string) {
  await wait();
  const protocol = getMockStore().protocols.find((item) => item.id === protocolId);
  if (!protocol) {
    throw new Error(`Protocol ${protocolId} not found`);
  }
  return protocol;
}

export async function listWorkers() {
  await wait();
  return getMockStore().workers;
}

export async function listEmployers() {
  await wait();
  return getMockStore().employers;
}

export async function listPrograms() {
  await wait();
  return getMockStore().programs;
}

export async function listGenerationRuns() {
  await wait();
  return getMockStore().generationRuns;
}

export async function listMoodleAccounts() {
  await wait();
  return getMockStore().moodleAccounts;
}

export async function listAuditEvents() {
  await wait();
  return getMockStore().auditEvents;
}
```

Modify `frontend/src/api/client.ts`:

```ts
export {
  getProtocolWorkflow,
  listAuditEvents,
  listEmployers,
  listGenerationRuns,
  listImportRows,
  listImportRuns,
  listMoodleAccounts,
  listPrograms,
  listProtocols,
  listRequests,
  listWorkers,
  resolveImportRow
} from "./mock/services";
```

- [ ] **Step 5: Run tests**

```bash
npm --prefix frontend test -- src/api/mock/services.test.ts
```

Expected: PASS.

- [ ] **Step 6: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 4: Router, Motion Provider, and App Shell

**Files:**
- Create: `frontend/src/app/router.tsx`
- Replace: `frontend/src/app/App.tsx`
- Create: `frontend/src/app/shell/AppShell.tsx`
- Create: `frontend/src/app/shell/Sidebar.tsx`
- Create: `frontend/src/app/shell/TopBar.tsx`
- Create: `frontend/src/app/shell/AppShell.test.tsx`
- Create minimal route page files used by router if they do not exist yet

- [ ] **Step 1: Write app shell test**

Create `frontend/src/app/shell/AppShell.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { describe, expect, it } from "vitest";
import { AppShell } from "./AppShell";

function renderShell(path = "/dashboard") {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/dashboard" element={<h1>Рабочий стол</h1>} />
          <Route path="/requests" element={<h1>Заявки</h1>} />
        </Route>
      </Routes>
    </MemoryRouter>
  );
}

describe("AppShell", () => {
  it("renders brand, top bar, dashboard link, and grouped navigation", () => {
    renderShell();

    expect(screen.getByText("ИКЦ Эксперт")).toBeInTheDocument();
    expect(screen.getByRole("searchbox", { name: "Поиск по админке" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Рабочий стол" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Операции/ })).toBeInTheDocument();
  });

  it("toggles a sidebar group and navigates to requests", async () => {
    const user = userEvent.setup();
    renderShell();

    await user.click(screen.getByRole("button", { name: /Операции/ }));
    await user.click(screen.getByRole("link", { name: "Заявки" }));

    expect(screen.getByRole("heading", { name: "Заявки" })).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
npm --prefix frontend test -- src/app/shell/AppShell.test.tsx
```

Expected: FAIL because `AppShell` does not exist.

- [ ] **Step 3: Implement shell components**

Create `frontend/src/app/shell/TopBar.tsx`:

```tsx
import { Search } from "lucide-react";

export function TopBar() {
  return (
    <header className="topbar">
      <label className="topbar-search">
        <Search aria-hidden className="topbar-search-icon" />
        <span className="sr-only">Поиск по админке</span>
        <input aria-label="Поиск по админке" type="search" placeholder="Поиск по заявкам, протоколам, слушателям" />
      </label>
      <div className="topbar-status">
        <span className="mock-pill">Mock data</span>
        <span className="user-pill">operator</span>
      </div>
    </header>
  );
}
```

Create `frontend/src/app/shell/Sidebar.tsx`:

```tsx
import { ChevronDown, ChevronRight } from "lucide-react";
import { useMemo, useState } from "react";
import { NavLink, useLocation } from "react-router";
import { dashboardRoute, navGroups } from "../routes";

function isActiveGroup(pathname: string, paths: string[]) {
  return paths.some((path) => pathname === path || pathname.startsWith(`${path}/`));
}

export function Sidebar() {
  const location = useLocation();
  const activeGroup = useMemo(
    () => navGroups.find((group) => isActiveGroup(location.pathname, group.items.map((item) => item.path)))?.id,
    [location.pathname]
  );
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(navGroups.map((group) => [group.id, group.id === activeGroup]))
  );

  return (
    <aside className="sidebar" aria-label="Основная навигация">
      <div className="sidebar-brand">ИКЦ Эксперт</div>
      <NavLink className="sidebar-link sidebar-link-root" to={dashboardRoute.path}>
        <dashboardRoute.icon aria-hidden className="sidebar-icon" />
        {dashboardRoute.label}
      </NavLink>
      <div className="sidebar-groups">
        {navGroups.map((group) => {
          const isOpen = openGroups[group.id] ?? group.id === activeGroup;
          return (
            <section className="sidebar-group" key={group.id}>
              <button
                className="sidebar-group-button"
                type="button"
                aria-expanded={isOpen}
                onClick={() => setOpenGroups((current) => ({ ...current, [group.id]: !isOpen }))}
              >
                {isOpen ? <ChevronDown aria-hidden className="sidebar-icon" /> : <ChevronRight aria-hidden className="sidebar-icon" />}
                {group.label}
              </button>
              <div className="sidebar-subnav" hidden={!isOpen}>
                {group.items.map((item) => (
                  <NavLink className="sidebar-link" to={item.path} key={item.id}>
                    <item.icon aria-hidden className="sidebar-icon" />
                    {item.label}
                  </NavLink>
                ))}
              </div>
            </section>
          );
        })}
      </div>
    </aside>
  );
}
```

Create `frontend/src/app/shell/AppShell.tsx`:

```tsx
import { Outlet } from "react-router";
import { Sidebar } from "./Sidebar";
import { TopBar } from "./TopBar";

export function AppShell() {
  return (
    <div className="app-shell">
      <Sidebar />
      <div className="app-main">
        <TopBar />
        <main className="app-content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
```

Append shell styles to `frontend/src/styles/globals.css`:

```css
.app-shell {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 256px minmax(0, 1fr);
}

.sidebar {
  background: var(--color-sidebar);
  color: white;
  padding: var(--space-5);
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.sidebar-brand {
  font-weight: 900;
}

.sidebar-groups {
  display: grid;
  gap: var(--space-3);
}

.sidebar-group-button,
.sidebar-link {
  width: 100%;
  min-height: 36px;
  border: 0;
  border-radius: var(--radius-md);
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 8px 10px;
  background: transparent;
  color: #d8e2ea;
  text-decoration: none;
}

.sidebar-link.active,
.sidebar-link:hover,
.sidebar-group-button:hover {
  background: var(--color-sidebar-muted);
  color: white;
}

.sidebar-subnav {
  display: grid;
  gap: var(--space-1);
  padding-top: var(--space-1);
}

.sidebar-subnav .sidebar-link {
  padding-left: 28px;
}

.sidebar-icon {
  width: 16px;
  height: 16px;
  flex: 0 0 auto;
}

.app-main {
  min-width: 0;
  display: grid;
  grid-template-rows: auto 1fr;
}

.topbar {
  min-height: 64px;
  border-bottom: 1px solid var(--color-line);
  background: rgba(255, 255, 255, 0.86);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  padding: 0 var(--space-6);
}

.topbar-search {
  width: min(520px, 100%);
  min-height: 38px;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  padding: 0 10px;
}

.topbar-search input {
  width: 100%;
  border: 0;
  outline: 0;
}

.topbar-search-icon {
  width: 16px;
  height: 16px;
  color: var(--color-text-muted);
}

.topbar-status {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.mock-pill,
.user-pill {
  border: 1px solid var(--color-line);
  border-radius: 999px;
  background: var(--color-surface);
  padding: 5px 9px;
  color: var(--color-text-muted);
  font-size: 12px;
  font-weight: 700;
}

.app-content {
  min-width: 0;
  padding: var(--space-6);
}
```

- [ ] **Step 4: Add router and App**

Create `frontend/src/app/router.tsx` with imports for all route pages. Use temporary `InDevelopmentPage` route elements for feature pages introduced in Tasks 6-9:

```tsx
import { Navigate, createBrowserRouter } from "react-router";
import { InDevelopmentPage } from "../components/feedback/InDevelopmentPage";
import { AppShell } from "./shell/AppShell";

function PageStub({ title }: { title: string }) {
  return <InDevelopmentPage title={title} description="Каркас раздела подключен к навигации." planned={["Макет таблиц", "Фильтры", "Детальная карточка"]} />;
}

export const router = createBrowserRouter([
  {
    path: "/login",
    element: <PageStub title="Вход" />
  },
  {
    element: <AppShell />,
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },
      { path: "/dashboard", element: <PageStub title="Рабочий стол" /> },
      { path: "/requests", element: <PageStub title="Заявки" /> },
      { path: "/requests/:requestId", element: <PageStub title="Карточка заявки" /> },
      { path: "/imports", element: <PageStub title="Импорт" /> },
      { path: "/imports/:importId", element: <PageStub title="Разбор импорта" /> },
      { path: "/protocols", element: <PageStub title="Протоколы" /> },
      { path: "/protocols/:protocolId", element: <PageStub title="Карточка протокола" /> },
      { path: "/documents", element: <PageStub title="Документы" /> },
      { path: "/moodle", element: <PageStub title="Moodle" /> },
      { path: "/workers", element: <PageStub title="Слушатели" /> },
      { path: "/workers/:workerId", element: <PageStub title="Карточка слушателя" /> },
      { path: "/employers", element: <PageStub title="Работодатели" /> },
      { path: "/employers/:employerId", element: <PageStub title="Карточка работодателя" /> },
      { path: "/programs", element: <PageStub title="Программы" /> },
      { path: "/audit", element: <PageStub title="Журнал" /> },
      { path: "/analytics", element: <PageStub title="Аналитика" /> },
      { path: "/users", element: <PageStub title="Пользователи и роли" /> },
      { path: "/notifications", element: <PageStub title="Уведомления" /> },
      { path: "/settings", element: <PageStub title="Настройки" /> }
    ]
  }
]);
```

Replace `frontend/src/app/App.tsx`:

```tsx
import { MotionConfig } from "motion/react";
import { RouterProvider } from "react-router/dom";
import { router } from "./router";

export function App() {
  return (
    <MotionConfig reducedMotion="user">
      <RouterProvider router={router} />
    </MotionConfig>
  );
}
```

- [ ] **Step 5: Run tests and build**

```bash
npm --prefix frontend test -- src/app/shell/AppShell.test.tsx src/app/routes.test.ts
npm --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 6: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 5: Shared Layout, Status, Drawer, and Table Components

**Files:**
- Create: `frontend/src/components/layout/PageHeader.tsx`
- Create: `frontend/src/components/layout/Toolbar.tsx`
- Create: `frontend/src/components/status/StatusBadge.tsx`
- Create: `frontend/src/components/drawer/DetailDrawer.tsx`
- Create: `frontend/src/components/data-table/DataTable.tsx`
- Create: `frontend/src/components/data-table/DataTable.test.tsx`
- Create: `frontend/src/components/status/StatusBadge.test.tsx`

- [ ] **Step 1: Write component tests**

Create `frontend/src/components/status/StatusBadge.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StatusBadge } from "./StatusBadge";

describe("StatusBadge", () => {
  it("renders label and semantic variant class", () => {
    render(<StatusBadge label="blocked" tone="danger" />);

    expect(screen.getByText("blocked")).toHaveClass("status-badge-danger");
  });
});
```

Create `frontend/src/components/data-table/DataTable.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { DataTable } from "./DataTable";

interface Row {
  name: string;
  status: string;
}

const rows: Row[] = [
  { name: "ООО Тест-Сервис", status: "review" },
  { name: "АО Пример-Проект", status: "ready" }
];

describe("DataTable", () => {
  it("renders rows and supports text filtering", async () => {
    const user = userEvent.setup();
    render(
      <DataTable
        ariaLabel="Тестовая таблица"
        data={rows}
        columns={[
          { accessorKey: "name", header: "Название" },
          { accessorKey: "status", header: "Статус" }
        ]}
      />
    );

    expect(screen.getByRole("table", { name: "Тестовая таблица" })).toBeInTheDocument();
    await user.type(screen.getByRole("searchbox", { name: "Фильтр таблицы" }), "Пример");
    expect(screen.queryByText("ООО Тест-Сервис")).not.toBeInTheDocument();
    expect(screen.getByText("АО Пример-Проект")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
npm --prefix frontend test -- src/components/status/StatusBadge.test.tsx src/components/data-table/DataTable.test.tsx
```

Expected: FAIL because components do not exist.

- [ ] **Step 3: Implement shared components**

Create `frontend/src/components/status/StatusBadge.tsx`:

```tsx
export type StatusTone = "neutral" | "success" | "warning" | "danger" | "info";

interface StatusBadgeProps {
  label: string;
  tone?: StatusTone;
}

export function StatusBadge({ label, tone = "neutral" }: StatusBadgeProps) {
  return <span className={`status-badge status-badge-${tone}`}>{label}</span>;
}
```

Create `frontend/src/components/layout/PageHeader.tsx`:

```tsx
import type { ReactNode } from "react";

interface PageHeaderProps {
  eyebrow?: string;
  title: string;
  description?: string;
  actions?: ReactNode;
}

export function PageHeader({ eyebrow, title, description, actions }: PageHeaderProps) {
  return (
    <header className="page-header">
      <div>
        {eyebrow ? <p className="eyebrow">{eyebrow}</p> : null}
        <h1>{title}</h1>
        {description ? <p>{description}</p> : null}
      </div>
      {actions ? <div className="page-header-actions">{actions}</div> : null}
    </header>
  );
}
```

Create `frontend/src/components/layout/Toolbar.tsx`:

```tsx
import type { ReactNode } from "react";

interface ToolbarProps {
  children: ReactNode;
}

export function Toolbar({ children }: ToolbarProps) {
  return <div className="toolbar">{children}</div>;
}
```

Create `frontend/src/components/drawer/DetailDrawer.tsx`:

```tsx
import { X } from "lucide-react";
import type { ReactNode } from "react";

interface DetailDrawerProps {
  title: string;
  open: boolean;
  onClose: () => void;
  children: ReactNode;
}

export function DetailDrawer({ title, open, onClose, children }: DetailDrawerProps) {
  if (!open) {
    return null;
  }

  return (
    <aside className="detail-drawer" aria-label={title}>
      <div className="detail-drawer-header">
        <h2>{title}</h2>
        <button className="icon-button" type="button" aria-label="Закрыть" onClick={onClose}>
          <X aria-hidden />
        </button>
      </div>
      <div className="detail-drawer-body">{children}</div>
    </aside>
  );
}
```

Create `frontend/src/components/data-table/DataTable.tsx`:

```tsx
import {
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState
} from "@tanstack/react-table";
import { useMemo, useState } from "react";

interface DataTableProps<TData extends object> {
  ariaLabel: string;
  data: TData[];
  columns: ColumnDef<TData, unknown>[];
}

export function DataTable<TData extends object>({ ariaLabel, data, columns }: DataTableProps<TData>) {
  const [globalFilter, setGlobalFilter] = useState("");
  const [sorting, setSorting] = useState<SortingState>([]);
  const stableData = useMemo(() => data, [data]);
  const table = useReactTable({
    data: stableData,
    columns,
    state: { globalFilter, sorting },
    onGlobalFilterChange: setGlobalFilter,
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel()
  });

  return (
    <section className="data-table-panel">
      <div className="data-table-tools">
        <input
          aria-label="Фильтр таблицы"
          type="search"
          value={globalFilter}
          onChange={(event) => setGlobalFilter(event.target.value)}
          placeholder="Фильтр"
        />
      </div>
      <div className="data-table-scroll">
        <table className="data-table" aria-label={ariaLabel}>
          <thead>
            {table.getHeaderGroups().map((headerGroup) => (
              <tr key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <th key={header.id}>
                    {header.isPlaceholder ? null : (
                      <button className="table-sort-button" type="button" onClick={header.column.getToggleSortingHandler()}>
                        {flexRender(header.column.columnDef.header, header.getContext())}
                      </button>
                    )}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {table.getRowModel().rows.map((row) => (
              <tr key={row.id}>
                {row.getVisibleCells().map((cell) => (
                  <td key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
```

Append these styles to `frontend/src/styles/globals.css`:

```css
.page-stack {
  display: grid;
  gap: var(--space-5);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--space-4);
}

.page-header h1 {
  margin: 4px 0 0;
  font-size: 28px;
}

.page-header p {
  margin: 6px 0 0;
  color: var(--color-text-muted);
}

.page-header-actions {
  display: flex;
  gap: var(--space-2);
}

.toolbar {
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  padding: var(--space-3);
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  width: max-content;
  border-radius: 999px;
  padding: 3px 8px;
  font-size: 12px;
  font-weight: 800;
}

.status-badge-neutral {
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
}

.status-badge-success {
  background: var(--color-success-weak);
  color: var(--color-success);
}

.status-badge-warning {
  background: var(--color-warning-weak);
  color: var(--color-warning);
}

.status-badge-danger {
  background: var(--color-danger-weak);
  color: var(--color-danger);
}

.status-badge-info {
  background: var(--color-primary-weak);
  color: var(--color-primary);
}

.icon-button {
  width: 36px;
  height: 36px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.detail-drawer {
  position: fixed;
  inset: 0 0 0 auto;
  z-index: 50;
  width: min(520px, 100vw);
  background: var(--color-surface);
  border-left: 1px solid var(--color-line);
  box-shadow: var(--shadow-drawer);
}

.detail-drawer-header {
  min-height: 64px;
  border-bottom: 1px solid var(--color-line);
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-3);
  padding: 0 var(--space-5);
}

.detail-drawer-header h2 {
  margin: 0;
  font-size: 18px;
}

.detail-drawer-body {
  padding: var(--space-5);
}

.data-table-panel {
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  box-shadow: var(--shadow-panel);
}

.data-table-tools {
  border-bottom: 1px solid var(--color-line);
  padding: var(--space-3);
}

.data-table-tools input {
  min-height: 36px;
  width: min(320px, 100%);
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  padding: 0 10px;
}

.data-table-scroll {
  overflow-x: auto;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  min-width: 720px;
}

.data-table th,
.data-table td {
  height: 44px;
  border-bottom: 1px solid var(--color-line-soft);
  padding: 8px 12px;
  text-align: left;
  vertical-align: middle;
  white-space: nowrap;
}

.data-table th {
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
  font-size: 12px;
}

.table-sort-button {
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  font-weight: 800;
}
```

- [ ] **Step 4: Run tests**

```bash
npm --prefix frontend test -- src/components/status/StatusBadge.test.tsx src/components/data-table/DataTable.test.tsx
npm --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 5: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 6: Dashboard and In-Development Route Pages

**Files:**
- Create: `frontend/src/features/dashboard/DashboardPage.tsx`
- Create: `frontend/src/features/dashboard/DashboardPage.test.tsx`
- Create: `frontend/src/features/analytics/AnalyticsPage.tsx`
- Create: `frontend/src/features/users/UsersPage.tsx`
- Create: `frontend/src/features/notifications/NotificationsPage.tsx`
- Modify: `frontend/src/app/router.tsx`

- [ ] **Step 1: Write dashboard and in-development tests**

Create `frontend/src/features/dashboard/DashboardPage.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { DashboardPage } from "./DashboardPage";

describe("DashboardPage", () => {
  it("renders operational metrics and attention queue", async () => {
    render(<DashboardPage />);

    expect(await screen.findByRole("heading", { name: "Рабочий стол" })).toBeInTheDocument();
    expect(screen.getByText("Требуют внимания")).toBeInTheDocument();
    expect(screen.getByText(/ООО Тест-Сервис/)).toBeInTheDocument();
    expect(screen.getByText("Mock data")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
npm --prefix frontend test -- src/features/dashboard/DashboardPage.test.tsx
```

Expected: FAIL because `DashboardPage` does not exist.

- [ ] **Step 3: Implement dashboard**

Create `frontend/src/features/dashboard/DashboardPage.tsx`:

```tsx
import { AlertTriangle, ClipboardList, FileText, UploadCloud } from "lucide-react";
import { useEffect, useState } from "react";
import { listRequests } from "../../api/client";
import type { ClientRequest } from "../../api/mock/types";
import { EmptyState } from "../../components/feedback/EmptyState";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";
import { StatusBadge } from "../../components/status/StatusBadge";

export function DashboardPage() {
  const [requests, setRequests] = useState<ClientRequest[] | null>(null);

  useEffect(() => {
    void listRequests().then(setRequests);
  }, []);

  if (!requests) {
    return <LoadingState label="Загрузка рабочего стола" />;
  }

  const attention = requests.filter((request) => request.attention !== "normal");

  return (
    <div className="page-stack">
      <PageHeader
        eyebrow="Операции"
        title="Рабочий стол"
        description="Очереди, блокировки и быстрые переходы по Минтруд-процессу."
        actions={<span className="mock-pill">Mock data</span>}
      />
      <section className="metric-grid" aria-label="Операционные показатели">
        <Metric icon={<ClipboardList aria-hidden />} label="Заявки в review" value="14" />
        <Metric icon={<UploadCloud aria-hidden />} label="Конфликты импорта" value="7" tone="warning" />
        <Metric icon={<FileText aria-hidden />} label="DOCX blocked" value="3" tone="danger" />
        <Metric icon={<AlertTriangle aria-hidden />} label="Следующее действие" value="XML" />
      </section>
      <section className="panel">
        <div className="panel-header">
          <h2>Требуют внимания</h2>
          <StatusBadge label={`${attention.length} задачи`} tone={attention.length > 0 ? "warning" : "success"} />
        </div>
        {attention.length === 0 ? (
          <EmptyState title="Очередь пуста" description="Нет заявок, требующих решения оператора." />
        ) : (
          <div className="attention-list">
            {attention.map((request) => (
              <article className="attention-item" key={request.id}>
                <div>
                  <strong>{request.employerName}</strong>
                  <p>{request.nextAction}</p>
                </div>
                <StatusBadge label={request.status} tone={request.attention === "danger" ? "danger" : "warning"} />
              </article>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function Metric({ icon, label, value, tone = "info" }: { icon: React.ReactNode; label: string; value: string; tone?: "info" | "warning" | "danger" }) {
  return (
    <article className={`metric-tile metric-tile-${tone}`}>
      <div className="metric-icon">{icon}</div>
      <div>
        <p>{label}</p>
        <strong>{value}</strong>
      </div>
    </article>
  );
}
```

Create `frontend/src/features/analytics/AnalyticsPage.tsx`:

```tsx
import { InDevelopmentPage } from "../../components/feedback/InDevelopmentPage";

export function AnalyticsPage() {
  return (
    <InDevelopmentPage
      title="Аналитика"
      description="Раздел появится после стабилизации MVP workflow."
      planned={["Динамика заявок", "Статусы протоколов", "Ошибки импортов", "Moodle-очередь"]}
    />
  );
}
```

Create `frontend/src/features/users/UsersPage.tsx`:

```tsx
import { InDevelopmentPage } from "../../components/feedback/InDevelopmentPage";

export function UsersPage() {
  return (
    <InDevelopmentPage
      title="Пользователи и роли"
      description="RBAC появится после MVP, когда будет подтвержден состав ролей."
      planned={["Операторы", "Роли доступа", "Персональный audit", "Блокировка пользователей"]}
    />
  );
}
```

Create `frontend/src/features/notifications/NotificationsPage.tsx`:

```tsx
import { InDevelopmentPage } from "../../components/feedback/InDevelopmentPage";

export function NotificationsPage() {
  return (
    <InDevelopmentPage
      title="Уведомления"
      description="Внешние уведомления не входят в MVP и будут подключаться по подтвержденным сценариям."
      planned={["Email об ошибках", "Telegram для критичных событий", "Уведомления о backup", "Ошибки Moodle"]}
    />
  );
}
```

Modify `frontend/src/app/router.tsx` to import and use `DashboardPage`, `AnalyticsPage`, `UsersPage`, and `NotificationsPage`.

- [ ] **Step 4: Run tests**

```bash
npm --prefix frontend test -- src/features/dashboard/DashboardPage.test.tsx
npm --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 5: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 7: Registry Pages

**Files:**
- Create: `frontend/src/features/workers/WorkersPage.tsx`
- Create: `frontend/src/features/workers/WorkerDetailPage.tsx`
- Create: `frontend/src/features/employers/EmployersPage.tsx`
- Create: `frontend/src/features/employers/EmployerDetailPage.tsx`
- Create: `frontend/src/features/programs/ProgramsPage.tsx`
- Create: `frontend/src/features/registry/registry-pages.test.tsx`
- Modify: `frontend/src/app/router.tsx`

- [ ] **Step 1: Write registry tests**

Create `frontend/src/features/registry/registry-pages.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import { EmployersPage } from "../employers/EmployersPage";
import { ProgramsPage } from "../programs/ProgramsPage";
import { WorkersPage } from "../workers/WorkersPage";

describe("registry pages", () => {
  it("renders workers registry", async () => {
    render(<WorkersPage />, { wrapper: MemoryRouter });

    expect(await screen.findByRole("heading", { name: "Слушатели" })).toBeInTheDocument();
    expect(screen.getByText("Иванов Иван Иванович")).toBeInTheDocument();
  });

  it("renders employers registry", async () => {
    render(<EmployersPage />, { wrapper: MemoryRouter });

    expect(await screen.findByRole("heading", { name: "Работодатели" })).toBeInTheDocument();
    expect(screen.getByText("ООО Тест-Сервис")).toBeInTheDocument();
  });

  it("renders programs registry", async () => {
    render(<ProgramsPage />, { wrapper: MemoryRouter });

    expect(await screen.findByRole("heading", { name: "Программы" })).toBeInTheDocument();
    expect(screen.getByText("Общие вопросы охраны труда")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
npm --prefix frontend test -- src/features/registry/registry-pages.test.tsx
```

Expected: FAIL because registry pages do not exist.

- [ ] **Step 3: Implement registry pages**

Create `frontend/src/features/workers/WorkersPage.tsx`:

```tsx
import { Link } from "react-router";
import { useEffect, useState } from "react";
import { listWorkers } from "../../api/client";
import type { Worker } from "../../api/mock/types";
import { DataTable } from "../../components/data-table/DataTable";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";

export function WorkersPage() {
  const [items, setItems] = useState<Worker[] | null>(null);

  useEffect(() => {
    void listWorkers().then(setItems);
  }, []);

  if (!items) {
    return <LoadingState label="Загрузка слушателей" />;
  }

  return (
    <div className="page-stack">
      <PageHeader title="Слушатели" description="Физлица, работодатели и активные обучения." />
      <DataTable
        ariaLabel="Слушатели"
        data={items}
        columns={[
          { accessorKey: "fullName", header: "ФИО", cell: ({ row }) => <Link to={`/workers/${row.original.id}`}>{row.original.fullName}</Link> },
          { accessorKey: "snils", header: "СНИЛС" },
          { accessorKey: "email", header: "Email" },
          { accessorKey: "employerName", header: "Работодатель" },
          { accessorKey: "activeTrainings", header: "Обучения" }
        ]}
      />
    </div>
  );
}
```

Create `frontend/src/features/employers/EmployersPage.tsx`:

```tsx
import { Link } from "react-router";
import { useEffect, useState } from "react";
import { listEmployers } from "../../api/client";
import type { Employer } from "../../api/mock/types";
import { DataTable } from "../../components/data-table/DataTable";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";
import { StatusBadge } from "../../components/status/StatusBadge";

export function EmployersPage() {
  const [items, setItems] = useState<Employer[] | null>(null);

  useEffect(() => {
    void listEmployers().then(setItems);
  }, []);

  if (!items) {
    return <LoadingState label="Загрузка работодателей" />;
  }

  return (
    <div className="page-stack">
      <PageHeader title="Работодатели" description="Компании, ИНН, заявки и связанные слушатели." />
      <DataTable
        ariaLabel="Работодатели"
        data={items}
        columns={[
          { accessorKey: "name", header: "Название", cell: ({ row }) => <Link to={`/employers/${row.original.id}`}>{row.original.name}</Link> },
          { accessorKey: "inn", header: "ИНН" },
          { accessorKey: "status", header: "Статус", cell: ({ row }) => <StatusBadge label={row.original.status} tone={row.original.status === "active" ? "success" : "neutral"} /> },
          { accessorKey: "requests", header: "Заявки" },
          { accessorKey: "workers", header: "Слушатели" }
        ]}
      />
    </div>
  );
}
```

Create `frontend/src/features/programs/ProgramsPage.tsx`:

```tsx
import { useEffect, useState } from "react";
import { listPrograms } from "../../api/client";
import type { Program } from "../../api/mock/types";
import { DataTable } from "../../components/data-table/DataTable";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";
import { StatusBadge } from "../../components/status/StatusBadge";

export function ProgramsPage() {
  const [items, setItems] = useState<Program[] | null>(null);

  useEffect(() => {
    void listPrograms().then(setItems);
  }, []);

  if (!items) {
    return <LoadingState label="Загрузка программ" />;
  }

  return (
    <div className="page-stack">
      <PageHeader title="Программы" description="Группы, часы, статус и Moodle mapping." />
      <DataTable
        ariaLabel="Программы"
        data={items}
        columns={[
          { accessorKey: "groupCode", header: "Группа" },
          { accessorKey: "code", header: "Код" },
          { accessorKey: "name", header: "Название" },
          { accessorKey: "defaultHours", header: "Часы" },
          { accessorKey: "moodleCourseId", header: "Moodle course" },
          { accessorKey: "status", header: "Статус", cell: ({ row }) => <StatusBadge label={row.original.status} tone={row.original.status === "active" ? "success" : "neutral"} /> }
        ]}
      />
    </div>
  );
}
```

Create `WorkerDetailPage.tsx`, `EmployerDetailPage.tsx`, and a shared detail panel pattern. Example for `frontend/src/features/workers/WorkerDetailPage.tsx`:

```tsx
import { useParams } from "react-router";
import { PageHeader } from "../../components/layout/PageHeader";

export function WorkerDetailPage() {
  const { workerId } = useParams();
  return (
    <div className="page-stack">
      <PageHeader title="Карточка слушателя" description={`Mock detail route: ${workerId}`} />
      <section className="panel">
        <h2>Связанные данные</h2>
        <p>В API-этапе здесь появятся обучения, работодатели, Moodle и протоколы слушателя.</p>
      </section>
    </div>
  );
}
```

Use the same explicit structure for `EmployerDetailPage.tsx`, with title `Карточка работодателя` and text `В API-этапе здесь появятся заявки, слушатели и протоколы работодателя.`

Modify `frontend/src/app/router.tsx` to use these pages for `/workers`, `/employers`, `/programs`, and detail routes.

- [ ] **Step 4: Run tests and build**

```bash
npm --prefix frontend test -- src/features/registry/registry-pages.test.tsx
npm --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 5: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 8: Operations Pages for Requests, Imports, Protocols, Documents, and Moodle

**Files:**
- Create: `frontend/src/features/requests/RequestsPage.tsx`
- Create: `frontend/src/features/requests/RequestDetailPage.tsx`
- Create: `frontend/src/features/imports/ImportsPage.tsx`
- Create: `frontend/src/features/imports/ImportDetailPage.tsx`
- Create: `frontend/src/features/protocols/ProtocolsPage.tsx`
- Create: `frontend/src/features/protocols/ProtocolDetailPage.tsx`
- Create: `frontend/src/features/documents/DocumentsPage.tsx`
- Create: `frontend/src/features/moodle/MoodlePage.tsx`
- Create: `frontend/src/features/operations/operations-pages.test.tsx`
- Modify: `frontend/src/app/router.tsx`
- Delete or stop importing: `frontend/src/features/protocol-workflow/ProtocolWorkflowPage.tsx`

- [ ] **Step 1: Write operations tests**

Create `frontend/src/features/operations/operations-pages.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import { ImportDetailPage } from "../imports/ImportDetailPage";
import { MoodlePage } from "../moodle/MoodlePage";
import { ProtocolDetailPage } from "../protocols/ProtocolDetailPage";
import { RequestsPage } from "../requests/RequestsPage";

describe("operations pages", () => {
  it("renders requests queue", async () => {
    render(<RequestsPage />, { wrapper: MemoryRouter });

    expect(await screen.findByRole("heading", { name: "Заявки" })).toBeInTheDocument();
    expect(screen.getByText("ООО Тест-Сервис")).toBeInTheDocument();
  });

  it("allows resolving an import row in mock mode", async () => {
    const user = userEvent.setup();
    render(<ImportDetailPage />, { wrapper: MemoryRouter });

    expect(await screen.findByText("Петров Петр Петрович")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Пропустить row-2" }));
    expect(await screen.findByText("skipped")).toBeInTheDocument();
  });

  it("renders protocol timeline with blocked registry stage", async () => {
    render(<ProtocolDetailPage />, { wrapper: MemoryRouter });

    expect(await screen.findByRole("heading", { name: /2605А15/ })).toBeInTheDocument();
    expect(screen.getByText(/Заполните номера Минтруда/)).toBeInTheDocument();
  });

  it("renders Moodle review queue", async () => {
    render(<MoodlePage />, { wrapper: MemoryRouter });

    expect(await screen.findByRole("heading", { name: "Moodle" })).toBeInTheDocument();
    expect(screen.getByText("review_required")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
npm --prefix frontend test -- src/features/operations/operations-pages.test.tsx
```

Expected: FAIL because operation pages do not exist.

- [ ] **Step 3: Implement operation list pages**

Implement `RequestsPage`, `ImportsPage`, `ProtocolsPage`, `DocumentsPage`, and `MoodlePage` with `PageHeader`, `DataTable`, `StatusBadge`, and mock services. Use route links to details.

Create `RequestsPage`, `ImportsPage`, `ProtocolsPage`, `DocumentsPage`, and `MoodlePage` with the same table component structure used in Task 7. Required columns:

```text
RequestsPage: employerName, receivedDate, status, rowsNeedReview, nextAction, link to /requests/:requestId
ImportsPage: fileName, requestId, status, rowsTotal, link to /imports/:importId
ProtocolsPage: number, employerName, programGroup, period, participants, link to /protocols/:protocolId
DocumentsPage: type, status, relatedEntity, fileName, generatedAt, mock download button
MoodlePage: workerName, employerName, email, course, status
```

Use `StatusBadge` for every status column. Use `LoadingState` while service promises resolve. Use `PageHeader` for each route title and description.

- [ ] **Step 4: Implement details and protocol timeline**

Create `frontend/src/features/imports/ImportDetailPage.tsx` with this behavior:

```tsx
import { useEffect, useState } from "react";
import { listImportRows, resolveImportRow } from "../../api/client";
import type { ImportRow } from "../../api/mock/types";
import { DataTable } from "../../components/data-table/DataTable";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";
import { StatusBadge } from "../../components/status/StatusBadge";

export function ImportDetailPage() {
  const [rows, setRows] = useState<ImportRow[] | null>(null);
  const refresh = () => listImportRows("import-1").then(setRows);

  useEffect(() => {
    void refresh();
  }, []);

  if (!rows) {
    return <LoadingState label="Загрузка строк импорта" />;
  }

  return (
    <div className="page-stack">
      <PageHeader title="Разбор импорта" description="Staging rows, конфликты и решения оператора." />
      <DataTable
        ariaLabel="Строки импорта"
        data={rows}
        columns={[
          { accessorKey: "rowNumber", header: "#" },
          { accessorKey: "fullName", header: "ФИО" },
          { accessorKey: "snils", header: "СНИЛС" },
          { accessorKey: "programs", header: "Программы", cell: ({ row }) => row.original.programs.join(", ") },
          { accessorKey: "status", header: "Статус", cell: ({ row }) => <StatusBadge label={row.original.status} tone={row.original.status === "conflict" ? "danger" : "info"} /> },
          { id: "actions", header: "Действия", cell: ({ row }) => <button className="button button-secondary" type="button" onClick={() => resolveImportRow(row.original.id, "skipped").then(refresh)}>Пропустить {row.original.id}</button> }
        ]}
      />
    </div>
  );
}
```

Create `ProtocolDetailPage` by loading `getProtocolWorkflow("protocol-2605-a-15")`, rendering a `PageHeader` with `workflow.number`, and mapping `workflow.stages` into `.protocol-stage` cards with `StatusBadge` and `stage.reason`.

Create `RequestDetailPage` with `PageHeader title="Карточка заявки"`, a summary panel for `request-1`, and two link panels to `/imports/import-1` and `/protocols/protocol-2605-a-15`.

- [ ] **Step 5: Wire routes and remove old protocol mock page import**

Modify `frontend/src/app/router.tsx` to use operation pages for all operation routes. Stop importing `ProtocolWorkflowPage`. Delete the old protocol workflow test after replacing its coverage with `operations-pages.test.tsx`.

- [ ] **Step 6: Run tests and build**

```bash
npm --prefix frontend test -- src/features/operations/operations-pages.test.tsx
npm --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 7: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 9: Settings and Audit Pages

**Files:**
- Create: `frontend/src/features/audit/AuditPage.tsx`
- Create: `frontend/src/features/settings/SettingsPage.tsx`
- Create: `frontend/src/features/control/control-pages.test.tsx`
- Modify: `frontend/src/app/router.tsx`

- [ ] **Step 1: Write control page tests**

Create `frontend/src/features/control/control-pages.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AuditPage } from "../audit/AuditPage";
import { SettingsPage } from "../settings/SettingsPage";

describe("control pages", () => {
  it("renders audit events", async () => {
    render(<AuditPage />);

    expect(await screen.findByRole("heading", { name: "Журнал" })).toBeInTheDocument();
    expect(screen.getByText("import.row.conflict")).toBeInTheDocument();
  });

  it("renders settings sections", () => {
    render(<SettingsPage />);

    expect(screen.getByRole("heading", { name: "Настройки" })).toBeInTheDocument();
    expect(screen.getByText("Режим данных")).toBeInTheDocument();
    expect(screen.getByText("Backup SQLite")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
npm --prefix frontend test -- src/features/control/control-pages.test.tsx
```

Expected: FAIL because pages do not exist.

- [ ] **Step 3: Implement AuditPage and SettingsPage**

`AuditPage` uses `listAuditEvents()` and `DataTable` columns `at`, `actor`, `action`, `entity`, `details`.

`SettingsPage` uses static panels:

- `Режим данных` with value `Mock data`;
- `Backup SQLite` with value `ежедневно, mock status`;
- `Шаблоны` with buttons for client XLSX template and protocol example;
- `Системная информация` with app version `frontend-shell-mock`.

- [ ] **Step 4: Wire routes and run tests**

Modify `frontend/src/app/router.tsx` to use `AuditPage` and `SettingsPage`.

Run:

```bash
npm --prefix frontend test -- src/features/control/control-pages.test.tsx
npm --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 5: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 10: Motion and Responsive Polish

**Files:**
- Modify: `frontend/src/app/App.tsx`
- Modify: `frontend/src/app/shell/Sidebar.tsx`
- Modify: `frontend/src/app/shell/AppShell.tsx`
- Modify: `frontend/src/components/drawer/DetailDrawer.tsx`
- Modify: `frontend/src/components/status/StatusBadge.tsx`
- Modify: `frontend/src/styles/globals.css`
- Create: `frontend/src/app/motion.test.tsx`

- [ ] **Step 1: Write motion provider test**

Create `frontend/src/app/motion.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { App } from "./App";

describe("App motion/accessibility shell", () => {
  it("renders through MotionConfig and keeps login route accessible", async () => {
    window.history.pushState({}, "", "/login");
    render(<App />);

    expect(await screen.findByRole("heading", { name: /Вход/ })).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test**

```bash
npm --prefix frontend test -- src/app/motion.test.tsx
```

Expected: PASS before and after implementation. If it fails because router state leaks between tests, reset `window.history` in test setup.

- [ ] **Step 3: Add Motion components**

Use imports from `motion/react`:

```tsx
import { AnimatePresence, motion } from "motion/react";
```

Apply `motion.div` to route content in `AppShell`, sidebar subnav containers, `DetailDrawer`, status badge state changes, and protocol stage cards. Keep animation durations between `0.12` and `0.24` seconds. Use `layout` only where dimensions are stable.

Example for sidebar subnav in `Sidebar.tsx`:

```tsx
<AnimatePresence initial={false}>
  {isOpen ? (
    <motion.div
      className="sidebar-subnav"
      initial={{ height: 0, opacity: 0 }}
      animate={{ height: "auto", opacity: 1 }}
      exit={{ height: 0, opacity: 0 }}
      transition={{ duration: 0.18, ease: "easeOut" }}
    >
      ...
    </motion.div>
  ) : null}
</AnimatePresence>
```

- [ ] **Step 4: Add responsive styles**

In `frontend/src/styles/globals.css`, add:

```css
@media (max-width: 860px) {
  .app-shell {
    grid-template-columns: 1fr;
  }

  .sidebar {
    position: sticky;
    top: 0;
    z-index: 20;
    min-height: auto;
  }

  .topbar {
    padding: 0 var(--space-4);
  }

  .app-content {
    padding: var(--space-4);
  }

  .metric-grid {
    grid-template-columns: 1fr;
  }
}

@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
    scroll-behavior: auto !important;
  }
}
```

- [ ] **Step 5: Run frontend verification**

```bash
npm --prefix frontend test
npm --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 6: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 11: Playwright Route and Workflow Coverage

**Files:**
- Delete: `frontend/tests/protocol-workflow.spec.ts`
- Create: `frontend/tests/app-shell.spec.ts`
- Create: `frontend/tests/mock-workflow.spec.ts`

- [ ] **Step 1: Write app shell e2e test**

Create `frontend/tests/app-shell.spec.ts`:

```ts
import { expect, test } from "@playwright/test";

const routes = [
  ["/dashboard", "Рабочий стол"],
  ["/requests", "Заявки"],
  ["/imports", "Импорт"],
  ["/protocols", "Протоколы"],
  ["/documents", "Документы"],
  ["/moodle", "Moodle"],
  ["/workers", "Слушатели"],
  ["/employers", "Работодатели"],
  ["/programs", "Программы"],
  ["/audit", "Журнал"],
  ["/analytics", "Аналитика"],
  ["/users", "Пользователи и роли"],
  ["/notifications", "Уведомления"],
  ["/settings", "Настройки"]
] as const;

test.describe("app shell routes", () => {
  for (const [path, heading] of routes) {
    test(`renders ${path}`, async ({ page }) => {
      await page.goto(path);
      await expect(page.getByRole("heading", { name: heading })).toBeVisible();
    });
  }
});
```

- [ ] **Step 2: Write mock workflow e2e test**

Create `frontend/tests/mock-workflow.spec.ts`:

```ts
import { expect, test } from "@playwright/test";

test("operator can inspect request, import row, and protocol gate", async ({ page }) => {
  await page.goto("/requests");
  await expect(page.getByText("ООО Тест-Сервис")).toBeVisible();

  await page.goto("/imports/import-1");
  await expect(page.getByText("Петров Петр Петрович")).toBeVisible();
  await page.getByRole("button", { name: "Пропустить row-2" }).click();
  await expect(page.getByText("skipped")).toBeVisible();

  await page.goto("/protocols/protocol-2605-a-15");
  await expect(page.getByRole("heading", { name: /2605А15/ })).toBeVisible();
  await expect(page.getByText(/Заполните номера Минтруда/)).toBeVisible();
});
```

- [ ] **Step 3: Remove old e2e test**

```bash
git rm frontend/tests/protocol-workflow.spec.ts
```

- [ ] **Step 4: Run Playwright**

```bash
npm --prefix frontend run e2e
```

Expected: PASS in Chromium. If browsers are missing locally, run `npx --prefix frontend playwright install chromium` and rerun.

- [ ] **Step 5: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 12: JSON Auth API and React Login Page

**Files:**
- Create: `backend/admin/api_handler.go`
- Create: `backend/admin/api_handler_test.go`
- Modify: `backend/app/api_routes.go`
- Modify: `backend/app/container.go`
- Create: `frontend/src/features/auth/LoginPage.tsx`
- Create: `frontend/src/features/auth/LoginPage.test.tsx`
- Modify: `frontend/src/app/router.tsx`
- Modify: `frontend/src/api/client.ts`

- [ ] **Step 1: Write backend JSON auth tests**

Create `backend/admin/api_handler_test.go`:

```go
package admin_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIHandler_Session_Unauthenticated(t *testing.T) {
	t.Parallel()

	_, _, _, mount := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["authenticated"] != false {
		t.Fatalf("authenticated = %v, want false", body["authenticated"])
	}
}

func TestAPIHandler_LoginJSON_Success(t *testing.T) {
	t.Parallel()

	_, sm, _, mount := newTestHandler(t)
	payload := []byte(`{"login":"alice","password":"test-password"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "http://example.com/login")
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var sessionCookieValue string
	for _, c := range rec.Result().Cookies() {
		if c.Name == sm.Cookie.Name {
			sessionCookieValue = c.Value
		}
	}
	if sessionCookieValue == "" {
		t.Fatalf("login did not set session cookie")
	}
}
```

Update `newTestHandler` in `backend/admin/handler_test.go` during implementation so it registers `/api/session`, `/api/login`, and `/api/logout` on the local mux.

- [ ] **Step 2: Run backend auth tests to verify failure**

```bash
go test ./backend/admin -run 'TestAPIHandler' -count=1
```

Expected: FAIL because JSON API handler does not exist or routes are not registered in test fixture.

- [ ] **Step 3: Implement JSON auth API**

Create `backend/admin/api_handler.go`:

```go
package admin

import (
	"encoding/json"
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
)

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type sessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	Login         string `json:"login,omitempty"`
}

func (h *Handler) GetSessionJSON(w http.ResponseWriter, r *http.Request) {
	login := h.sessions.GetString(r.Context(), SessionKeyUserLogin)
	writeJSON(w, http.StatusOK, sessionResponse{
		Authenticated: login != "",
		Login:         login,
	})
}

func (h *Handler) PostLoginJSON(w http.ResponseWriter, r *http.Request) {
	var input loginRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}

	user, err := h.service.Authenticate(r.Context(), input.Login, input.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}
	if err := h.sessions.RenewToken(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session_error"})
		return
	}
	h.sessions.Put(r.Context(), SessionKeyUserID, user.ID)
	h.sessions.Put(r.Context(), SessionKeyUserLogin, user.Login)
	auditCtx := audit.WithActor(r.Context(), user.Login)
	_ = h.audit.Record(auditCtx, audit.RecordInput{Action: "login.success", EntityType: "session", Actor: user.Login})
	writeJSON(w, http.StatusOK, sessionResponse{Authenticated: true, Login: user.Login})
}

func (h *Handler) PostLogoutJSON(w http.ResponseWriter, r *http.Request) {
	_ = h.sessions.Destroy(r.Context())
	writeJSON(w, http.StatusOK, sessionResponse{Authenticated: false})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
```

Modify `backend/app/api_routes.go`:

```go
func registerAPIRoutes(router chi.Router, deps Deps, c *container) {
	router.Route("/api", func(r chi.Router) {
		r.Get("/session", c.adminHandler.GetSessionJSON)
		r.Post("/login", c.adminHandler.PostLoginJSON)
		r.Post("/logout", c.adminHandler.PostLogoutJSON)
	})
}
```

- [ ] **Step 4: Write frontend login test**

Create `frontend/src/features/auth/LoginPage.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LoginPage } from "./LoginPage";

describe("LoginPage", () => {
  it("submits credentials through the provided callback", async () => {
    const user = userEvent.setup();
    const onLogin = vi.fn().mockResolvedValue(undefined);
    render(<LoginPage onLogin={onLogin} />);

    await user.type(screen.getByLabelText("Логин"), "alice");
    await user.type(screen.getByLabelText("Пароль"), "test-password");
    await user.click(screen.getByRole("button", { name: "Войти" }));

    expect(onLogin).toHaveBeenCalledWith({ login: "alice", password: "test-password" });
  });
});
```

- [ ] **Step 5: Implement React login page and client login function**

Create `frontend/src/features/auth/LoginPage.tsx`:

```tsx
import { FormEvent, useState } from "react";

export interface LoginInput {
  login: string;
  password: string;
}

interface LoginPageProps {
  onLogin?: (input: LoginInput) => Promise<void>;
}

export function LoginPage({ onLogin }: LoginPageProps) {
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    try {
      await onLogin?.({ login, password });
    } catch {
      setError("Не удалось войти");
    }
  }

  return (
    <main className="login-page">
      <form className="login-card" onSubmit={submit}>
        <p className="eyebrow">ИКЦ Эксперт</p>
        <h1>Вход</h1>
        <label>
          Логин
          <input value={login} onChange={(event) => setLogin(event.target.value)} autoComplete="username" />
        </label>
        <label>
          Пароль
          <input value={password} onChange={(event) => setPassword(event.target.value)} type="password" autoComplete="current-password" />
        </label>
        {error ? <p role="alert">{error}</p> : null}
        <button className="button button-primary" type="submit">Войти</button>
      </form>
    </main>
  );
}
```

Modify `frontend/src/api/client.ts` to add:

```ts
export async function login(input: { login: string; password: string }) {
  const response = await fetch("/api/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input)
  });
  if (!response.ok) {
    throw new Error("login failed");
  }
  return response.json() as Promise<{ authenticated: boolean; login: string }>;
}
```

Modify `frontend/src/app/router.tsx` to render `LoginPage` on `/login`. Pass `onLogin={login}` and redirect to `/dashboard` after success in a small wrapper component.

- [ ] **Step 6: Run auth tests**

```bash
go test ./backend/admin -run 'TestAPIHandler|TestLogin' -count=1
npm --prefix frontend test -- src/features/auth/LoginPage.test.tsx
```

Expected: PASS.

- [ ] **Step 7: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 13: Backend UI Route Removal and SPA Fallback

**Files:**
- Modify: `backend/app/routes.go`
- Modify: `backend/app/router_test.go`
- Modify or delete server-rendering handlers in `backend/programs`, `backend/employers`, `backend/people`, `backend/protocols`, `backend/requests`, `backend/audit`
- Delete: `backend/*/views`
- Delete: `backend/ui`
- Delete: `web/static/app.css`

- [ ] **Step 1: Write router tests for SPA-owned routes**

Modify `backend/app/router_test.go`: replace `TestProgramsPageReturnsOperatorShell` with a test named `TestFrontendRouteFallsBackToSPAIndex`:

```go
func TestFrontendRouteFallsBackToSPAIndex(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	cookies := testLoginPOST(t, router)

	rec := authedGET(t, router, "/programs", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("body does not contain SPA root: %s", rec.Body.String())
	}
}
```

Modify `newTestRouter` fixture in the same file so it passes `FrontendConfig{Mode: app.FrontendEmbedded, Assets: fstest.MapFS{...}}` with a minimal `index.html` containing `<div id="root"></div>`.

- [ ] **Step 2: Run router test to verify failure**

```bash
go test ./backend/app -run TestFrontendRouteFallsBackToSPAIndex -count=1
```

Expected: FAIL because old server-rendered `/programs` route still intercepts the path.

- [ ] **Step 3: Remove protected HTML route registration**

Modify `backend/app/routes.go`:

- keep public `GET /login` only until SPA serving owns `/login`;
- keep `POST /logout` only if still needed by old tests, otherwise JSON logout handles SPA;
- remove protected GET/POST HTML routes for `/programs`, `/employers`, `/workers`, `/protocols`, `/requests`, `/audit`;
- keep auth middleware for future protected API routes in `api_routes.go`, not for SPA fallback.

The file should no longer register old HTML pages. The SPA fallback in `registerFrontendRoutes` remains registered after API/auth setup.

- [ ] **Step 4: Remove views and unused imports**

Run:

```bash
git rm -r backend/admin/views backend/audit/views backend/employers/views backend/people/views backend/programs/views backend/protocols/views backend/requests/views backend/ui web/static/app.css
```

Then remove `views` imports and HTML render methods from these files if the compiler reports them:

```text
backend/programs/handler.go
backend/employers/handler.go
backend/people/handler.go
backend/protocols/handler.go
backend/requests/handler.go
backend/audit/handler.go
backend/admin/handler.go
```

Keep service constructors and service tests. Do not re-add `templ` imports.

- [ ] **Step 5: Run Go tests and remove remaining UI references**

```bash
go test ./...
```

Expected after removing remaining UI references: PASS. During this step, any failure mentioning `backend/*/views`, `backend/ui`, `templ`, `Render(ctx, w)`, or Tabler/HTMX must be resolved by deleting the UI call path or moving the behavior to a JSON handler.

- [ ] **Step 6: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 14: Remove templ Build Dependencies and Update Docs

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `.github/workflows/ci.yml`
- Modify: `Dockerfile`
- Modify: `README.md`

- [ ] **Step 1: Update CI test expectation**

Modify `.github/workflows/ci.yml` and remove:

```yaml
- run: go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate
```

Keep sqlc generation, Go tests, frontend tests, frontend build, Playwright install, and frontend e2e.

- [ ] **Step 2: Update Dockerfile**

Remove `TEMPL_VERSION` ARG, remove `go install github.com/a-h/templ/cmd/templ@${TEMPL_VERSION}`, and remove `templ generate` from the build command. Keep frontend builder and sqlc generation.

- [ ] **Step 3: Remove Go dependency**

Run:

```bash
go mod tidy
```

Expected: `github.com/a-h/templ` removed from `go.mod` and `go.sum` if no code imports it.

- [ ] **Step 4: Update README**

Replace references to server-rendered UI on `templ` with React SPA frontend. Update local development commands to include:

```bash
npm --prefix frontend ci
npm --prefix frontend run build
sh tests/run_schema_tests.sh
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
go test ./...
go run ./cmd/mintrud-admin
```

Keep Docker compose URL as `http://localhost:8081/login`.

- [ ] **Step 5: Run verification**

```bash
sh tests/run_schema_tests.sh
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
go test ./...
npm --prefix frontend test
npm --prefix frontend run build
npm --prefix frontend run e2e
```

Expected: PASS.

- [ ] **Step 6: Stage gate note**

Do not commit here. Continue to the next task in the current stage. The stage commit command is defined in the Stage Breakdown and must be run only after user approval.

## Task 15: Final Public-Git Hygiene and Docker Smoke

**Files:**
- Modify only if checks reveal a real issue.

- [ ] **Step 1: Run public-git hygiene commands**

Run the AGENTS.md checks without printing secret values:

```bash
git status --short --ignored
git ls-files -z | xargs -0 rg -l --hidden --no-ignore -i '(password|secret|token|api[_-]?key|private[_-]?key|BEGIN .*PRIVATE KEY|ИНН|СНИЛС|паспорт|email|@)'
rg -l --hidden --no-ignore -i --glob '!.env' --glob '!.env.*' --glob '!*.db' --glob '!*.sqlite' --glob '!*.sqlite3' '(password|secret|token|api[_-]?key|private[_-]?key|BEGIN .*PRIVATE KEY|ИНН|СНИЛС|паспорт|email|@)' .
```

Expected: matches are synthetic fixtures, test credentials, CSRF/auth code, generated sqlc models, and expected documentation. Real local `.env` values and DB files must not be printed.

- [ ] **Step 2: Run compose smoke test**

Use the canonical compose stack:

```bash
DOCKER_BUILDKIT=1 docker compose up --build -d
docker compose ps
curl -sS -o /dev/null -w '%{http_code}\n' http://localhost:8081/login
docker compose logs --tail=80 app
```

Expected: compose service is healthy and curl returns `200` for `/login`.

- [ ] **Step 3: Stop compose stack without deleting volume**

```bash
docker compose down
```

Expected: app stops and `mintrud_data` volume remains.

- [ ] **Step 4: Stage gate note**

Do not commit here. Show the final hygiene and Docker smoke results to the user. If fixes were needed, the Stage 6 commit command is defined in the Stage Breakdown and must be run only after user approval. If no files changed, report that no final commit is needed.

## Self-Review Checklist

- Spec coverage:
  - Full site shell: Tasks 1, 4, 6-9.
  - Backend UI removal: Tasks 12-14.
  - React login: Task 12.
  - Collapsible grouped sidebar: Task 4 and Task 10.
  - Quiet operations visual design: Tasks 2, 4, 5, 10.
  - Motion and reduced motion: Task 10.
  - Typed mocks: Task 3.
  - Tables: Task 5 plus feature tasks.
  - Tests: each implementation task starts with tests; e2e in Task 11; final verification in Tasks 14-15.
  - No dedicated Mintrud API section: Task 1 route test.
- Placeholder scan:
  - No `TBD`, `TODO`, or "fill later" instructions are used.
  - In-development pages are an intentional product feature, not missing plan content.
- Type consistency:
  - Route IDs match `routes.ts`.
  - Mock service names match `client.ts` exports and page imports.
  - Protocol ID `protocol-2605-a-15`, import ID `import-1`, row ID `row-2`, and request ID `request-1` are used consistently.
