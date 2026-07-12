# Frontend Transition Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a React/Vite frontend inside the existing Go monorepo while preserving a single Go production runtime and MSI-friendly delivery.

**Architecture:** Go remains the backend, API, auth, storage, document-generation, and production static-file server. React/Vite runs as a separate development frontend with mocked API support, then builds static assets that Go embeds and serves in production mode.

**Tech Stack:** Go `net/http` + chi, SQLite/sqlc, React, Vite, TypeScript, Vitest, React Testing Library, Playwright.

---

## File Structure

- Create `frontend/package.json`: frontend scripts and dependencies.
- Create `frontend/index.html`: Vite HTML entrypoint.
- Create `frontend/vite.config.ts`: React plugin, dev API proxy, Vitest config.
- Create `frontend/tsconfig.json` and `frontend/tsconfig.node.json`: TypeScript config.
- Create `frontend/src/main.tsx`: React app bootstrap.
- Create `frontend/src/app/App.tsx`: application shell and temporary route selection.
- Create `frontend/src/api/client.ts`: fetch wrapper for `/api`.
- Create `frontend/src/api/mockProtocolWorkflow.ts`: mock protocol workflow data for UI-first work.
- Create `frontend/src/features/protocol-workflow/ProtocolWorkflowPage.tsx`: first proving-slice UI.
- Create `frontend/src/features/protocol-workflow/ProtocolWorkflowPage.test.tsx`: component tests.
- Create `frontend/src/ui/ToastProvider.tsx`: local toast component for first slice.
- Create `frontend/tests/protocol-workflow.spec.ts`: Playwright smoke test.
- Create `frontend/playwright.config.ts`: Playwright local server config.
- Modify `backend/app/router.go`: register API routes and optional frontend routes through config.
- Create `backend/app/frontend.go`: embedded frontend serving.
- Create `backend/app/api_routes.go`: first `/api/session` and protocol workflow API route wiring.
- Modify `cmd/mintrud-admin/main.go`: load frontend mode from env.
- Modify `Dockerfile`: add Node build stage before Go build, copy `frontend/dist` into Go build context.
- Modify `docker-compose.yml`: keep current app service, no Node runtime service.
- Create `.github/workflows/ci.yml`: Go tests, frontend tests/build, production-like e2e.

## Task 1: Scaffold React/Vite App

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/index.html`
- Create: `frontend/vite.config.ts`
- Create: `frontend/tsconfig.json`
- Create: `frontend/tsconfig.node.json`
- Create: `frontend/src/main.tsx`
- Create: `frontend/src/app/App.tsx`

- [ ] **Step 1: Create package manifest**

Create `frontend/package.json`:

```json
{
  "name": "mintrud-admin-frontend",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite --host 127.0.0.1",
    "build": "tsc -b && vite build",
    "preview": "vite preview --host 127.0.0.1",
    "test": "vitest run",
    "test:watch": "vitest",
    "e2e": "playwright test"
  },
  "dependencies": {
    "@vitejs/plugin-react": "latest",
    "vite": "latest",
    "typescript": "latest",
    "react": "latest",
    "react-dom": "latest",
    "lucide-react": "latest"
  },
  "devDependencies": {
    "@playwright/test": "latest",
    "@testing-library/jest-dom": "latest",
    "@testing-library/react": "latest",
    "@testing-library/user-event": "latest",
    "@types/react": "latest",
    "@types/react-dom": "latest",
    "jsdom": "latest",
    "vitest": "latest"
  }
}
```

- [ ] **Step 2: Install dependencies**

Run:

```bash
npm --prefix frontend install
```

Expected: `frontend/package-lock.json` is created and `npm` exits with code `0`.

- [ ] **Step 3: Create Vite entrypoint**

Create `frontend/index.html`:

```html
<!doctype html>
<html lang="ru">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Mintrud Admin</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 4: Create Vite config**

Create `frontend/vite.config.ts`:

```ts
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:8080"
    }
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"]
  }
});
```

- [ ] **Step 5: Create TypeScript configs**

Create `frontend/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["DOM", "DOM.Iterable", "ES2022"],
    "allowJs": false,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "allowSyntheticDefaultImports": true,
    "strict": true,
    "forceConsistentCasingInFileNames": true,
    "module": "ESNext",
    "moduleResolution": "Node",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "types": ["vitest/globals", "@testing-library/jest-dom"]
  },
  "include": ["src"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

Create `frontend/tsconfig.node.json`:

```json
{
  "compilerOptions": {
    "composite": true,
    "module": "ESNext",
    "moduleResolution": "Node",
    "allowSyntheticDefaultImports": true
  },
  "include": ["vite.config.ts", "playwright.config.ts"]
}
```

- [ ] **Step 6: Create initial React app**

Create `frontend/src/main.tsx`:

```tsx
import React from "react";
import ReactDOM from "react-dom/client";
import { App } from "./app/App";
import "./styles.css";

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
```

Create `frontend/src/app/App.tsx`:

```tsx
export function App() {
  return (
    <main className="app-shell">
      <aside className="app-nav">
        <strong>Mintrud Admin</strong>
        <a href="/protocols">Протоколы</a>
        <a href="/requests">Заявки</a>
        <a href="/workers">Слушатели</a>
      </aside>
      <section className="app-content">
        <h1>Protocol workflow</h1>
        <p>React frontend is ready for the first workflow slice.</p>
      </section>
    </main>
  );
}
```

- [ ] **Step 7: Verify frontend build**

Run:

```bash
npm --prefix frontend run build
```

Expected: command exits `0` and creates `frontend/dist/index.html`.

- [ ] **Step 8: Commit scaffold**

Run:

```bash
git add frontend/package.json frontend/package-lock.json frontend/index.html frontend/vite.config.ts frontend/tsconfig.json frontend/tsconfig.node.json frontend/src/main.tsx frontend/src/app/App.tsx
git commit -m "feat: scaffold react frontend"
```

## Task 2: Add First Protocol Workflow UI on Mocks

**Files:**
- Create: `frontend/src/api/mockProtocolWorkflow.ts`
- Create: `frontend/src/features/protocol-workflow/ProtocolWorkflowPage.tsx`
- Create: `frontend/src/features/protocol-workflow/ProtocolWorkflowPage.test.tsx`
- Create: `frontend/src/test/setup.ts`
- Create: `frontend/src/styles.css`
- Modify: `frontend/src/app/App.tsx`

- [ ] **Step 1: Create mock workflow data**

Create `frontend/src/api/mockProtocolWorkflow.ts`:

```ts
export type WorkflowStageState = "done" | "active" | "blocked" | "pending";

export interface WorkflowStage {
  id: string;
  label: string;
  state: WorkflowStageState;
  reason?: string;
}

export interface ProtocolWorkflow {
  protocolId: number;
  number: string;
  employer: string;
  stages: WorkflowStage[];
}

export const mockProtocolWorkflow: ProtocolWorkflow = {
  protocolId: 1,
  number: "2605А15",
  employer: "Тестовый работодатель",
  stages: [
    { id: "participants", label: "Участники", state: "done" },
    { id: "fix", label: "Фиксация", state: "done" },
    { id: "xml", label: "XML", state: "active" },
    {
      id: "registry",
      label: "Реестровые номера",
      state: "blocked",
      reason: "Заполните номера Минтруда для всех активных участников"
    },
    { id: "docx", label: "DOCX", state: "pending" },
    { id: "closed", label: "Закрытие", state: "pending" }
  ]
};
```

- [ ] **Step 2: Write failing component test**

Create `frontend/src/test/setup.ts`:

```ts
import "@testing-library/jest-dom/vitest";
```

Create `frontend/src/features/protocol-workflow/ProtocolWorkflowPage.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { mockProtocolWorkflow } from "../../api/mockProtocolWorkflow";
import { ProtocolWorkflowPage } from "./ProtocolWorkflowPage";

describe("ProtocolWorkflowPage", () => {
  it("renders the protocol number and blocked reason", () => {
    render(<ProtocolWorkflowPage workflow={mockProtocolWorkflow} />);

    expect(screen.getByRole("heading", { name: /2605А15/i })).toBeInTheDocument();
    expect(screen.getByText(/Заполните номера Минтруда/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

Run:

```bash
npm --prefix frontend test -- ProtocolWorkflowPage.test.tsx
```

Expected: FAIL because `ProtocolWorkflowPage` does not exist.

- [ ] **Step 4: Implement workflow component**

Create `frontend/src/features/protocol-workflow/ProtocolWorkflowPage.tsx`:

```tsx
import type { ProtocolWorkflow, WorkflowStage } from "../../api/mockProtocolWorkflow";

interface Props {
  workflow: ProtocolWorkflow;
}

export function ProtocolWorkflowPage({ workflow }: Props) {
  return (
    <div className="workflow-page">
      <header className="workflow-header">
        <div>
          <p className="eyebrow">Протокол</p>
          <h1>{workflow.number}</h1>
          <p>{workflow.employer}</p>
        </div>
        <button className="primary-button" type="button">
          Обновить статус
        </button>
      </header>

      <section className="pipeline" aria-label="Этапы протокола">
        {workflow.stages.map((stage) => (
          <StageCard key={stage.id} stage={stage} />
        ))}
      </section>
    </div>
  );
}

function StageCard({ stage }: { stage: WorkflowStage }) {
  return (
    <article className={`stage-card stage-card-${stage.state}`}>
      <span className="stage-state">{stage.state}</span>
      <h2>{stage.label}</h2>
      {stage.reason ? <p className="stage-reason">{stage.reason}</p> : null}
    </article>
  );
}
```

- [ ] **Step 5: Wire component into app**

Replace `frontend/src/app/App.tsx` with:

```tsx
import { mockProtocolWorkflow } from "../api/mockProtocolWorkflow";
import { ProtocolWorkflowPage } from "../features/protocol-workflow/ProtocolWorkflowPage";

export function App() {
  return (
    <main className="app-shell">
      <aside className="app-nav">
        <strong>Mintrud Admin</strong>
        <a href="/protocols">Протоколы</a>
        <a href="/requests">Заявки</a>
        <a href="/workers">Слушатели</a>
      </aside>
      <section className="app-content">
        <ProtocolWorkflowPage workflow={mockProtocolWorkflow} />
      </section>
    </main>
  );
}
```

Create `frontend/src/styles.css`:

```css
body {
  margin: 0;
  font-family: Inter, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  background: #f6f7f9;
  color: #15171a;
  letter-spacing: 0;
}

.app-shell {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 240px 1fr;
}

.app-nav {
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  background: #111827;
  color: white;
}

.app-nav a {
  color: #d1d5db;
  text-decoration: none;
}

.app-content {
  padding: 32px;
}

.workflow-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 24px;
}

.eyebrow {
  margin: 0 0 4px;
  color: #667085;
  font-size: 13px;
}

.workflow-header h1 {
  margin: 0;
  font-size: 32px;
}

.primary-button {
  border: 0;
  border-radius: 8px;
  padding: 10px 14px;
  background: #2563eb;
  color: white;
  font-weight: 600;
}

.pipeline {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}

.stage-card {
  min-height: 136px;
  border: 1px solid #d0d5dd;
  border-radius: 8px;
  padding: 16px;
  background: white;
}

.stage-card h2 {
  margin: 12px 0 8px;
  font-size: 18px;
}

.stage-state {
  font-size: 12px;
  text-transform: uppercase;
  color: #475467;
}

.stage-card-done {
  border-color: #16a34a;
}

.stage-card-active {
  border-color: #2563eb;
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.12);
}

.stage-card-blocked {
  border-color: #dc2626;
}

.stage-reason {
  margin: 0;
  color: #b42318;
  font-size: 14px;
}
```

- [ ] **Step 6: Verify tests pass**

Run:

```bash
npm --prefix frontend test -- ProtocolWorkflowPage.test.tsx
```

Expected: PASS.

- [ ] **Step 7: Verify build passes**

Run:

```bash
npm --prefix frontend run build
```

Expected: PASS and `frontend/dist` is created.

- [ ] **Step 8: Commit mock workflow UI**

Run:

```bash
git add frontend/src
git commit -m "feat: add protocol workflow mock UI"
```

## Task 3: Add Go Frontend Serving Mode

**Files:**
- Create: `backend/app/frontend.go`
- Modify: `backend/app/router.go`
- Modify: `backend/app/server.go`
- Modify: `cmd/mintrud-admin/main.go`

- [ ] **Step 1: Write router test for disabled frontend mode**

Add a test to `backend/app/router_test.go`:

```go
func TestFrontendDisabled_DoesNotServeSPA(t *testing.T) {
	router := newTestRouterWithFrontendMode(t, app.FrontendDisabled)

	req := httptest.NewRequest(http.MethodGet, "/protocols/1/workflow", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Fatalf("disabled frontend served SPA HTML")
	}
}

func newTestRouterWithFrontendMode(t *testing.T, mode app.FrontendMode) http.Handler {
	t.Helper()
	router, db := newTestRouterWithDBAndLog(t, nil)
	_ = router

	sessions := admin.NewSessionManager(admin.SessionConfig{
		TTL:      8 * time.Hour,
		SameSite: 0,
		Secure:   false,
	})
	csrfMW := csrf.Protect([]byte("0123456789abcdef0123456789abcdef"),
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
		Frontend: app.FrontendConfig{Mode: mode},
	})
}
```

- [ ] **Step 2: Run test to verify it fails to compile**

Run:

```bash
go test ./backend/app -run TestFrontendDisabled_DoesNotServeSPA
```

Expected: FAIL because `FrontendConfig` and `FrontendDisabled` are undefined.

- [ ] **Step 3: Add frontend config and embedded server**

Before creating `backend/app/frontend.go`, copy the existing Vite build into the Go package embed location:

```bash
rm -rf backend/app/frontend/dist
mkdir -p backend/app/frontend/dist
cp -R frontend/dist/. backend/app/frontend/dist/
```

Create `backend/app/frontend.go`:

```go
package app

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

type FrontendMode string

const (
	FrontendEmbedded FrontendMode = "embedded"
	FrontendDisabled FrontendMode = "disabled"
)

type FrontendConfig struct {
	Mode FrontendMode
}

//go:embed all:frontend/dist
var frontendEmbed embed.FS

func registerFrontendRoutes(router interface {
	Handle(pattern string, h http.Handler)
	Get(pattern string, h http.HandlerFunc)
}, cfg FrontendConfig) {
	if cfg.Mode == FrontendDisabled {
		return
	}

	frontend, err := fs.Sub(frontendEmbed, "frontend/dist")
	if err != nil {
		panic(err)
	}

	router.Handle("/assets/*", http.FileServer(http.FS(frontend)))
	router.Get("/*", spaHandler(frontend))
}

func spaHandler(frontend fs.FS) http.HandlerFunc {
	index, err := fs.ReadFile(frontend, "index.html")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(frontend))

	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			writeIndex(w, index)
			return
		}
		if _, err := fs.Stat(frontend, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		writeIndex(w, index)
	}
}

func writeIndex(w http.ResponseWriter, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(index)
}
```

- [ ] **Step 4: Add frontend config to deps**

Modify `backend/app/router.go`:

```go
type Deps struct {
	Database  *sql.DB
	Sessions  *scs.SessionManager
	CSRF      func(http.Handler) http.Handler
	LoginRate *admin.RateLimiter
	Log       logrus.FieldLogger
	Frontend  FrontendConfig
}
```

At the end of `NewRouter`, after `registerRoutes(router, deps, newContainer(deps))`, add:

```go
registerFrontendRoutes(router, deps.Frontend)
```

- [ ] **Step 5: Load frontend mode in main**

Add to `cmd/mintrud-admin/main.go`:

```go
func frontendModeFromEnv() app.FrontendMode {
	switch env("MINTRUD_ADMIN_FRONTEND", string(app.FrontendEmbedded)) {
	case string(app.FrontendDisabled):
		return app.FrontendDisabled
	default:
		return app.FrontendEmbedded
	}
}
```

Thread this value through `app.NewServer` by adding a frontend config parameter to `app.NewServer` and passing it into `NewRouter`.

- [ ] **Step 6: Verify Go tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit frontend server mode**

Run:

```bash
git add backend/app cmd/mintrud-admin
git commit -m "feat: serve embedded frontend assets"
```

## Task 4: Add Initial JSON API for Workflow

**Files:**
- Create: `backend/app/api_routes.go`
- Create: `backend/protocols/api_handler.go`
- Create: `backend/protocols/api_handler_test.go`
- Modify: `backend/app/routes.go`

- [ ] **Step 1: Write API handler test**

Create `backend/protocols/api_handler_test.go`:

```go
package protocols

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkflowAPI_ReturnsJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteWorkflowJSON(w, WorkflowResponse{
			ProtocolID: 1,
			Number: "2605А15",
			Employer: "Тестовый работодатель",
			Stages: []WorkflowStageResponse{
				{ID: "xml", Label: "XML", State: "active"},
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/protocols/1/workflow", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), `"number":"2605А15"`) {
		t.Fatalf("response body = %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./backend/protocols -run TestWorkflowAPI_ReturnsJSON
```

Expected: FAIL because response types and `WriteWorkflowJSON` do not exist.

- [ ] **Step 3: Add JSON response helper**

Create `backend/protocols/api_handler.go`:

```go
package protocols

import (
	"encoding/json"
	"net/http"
)

type WorkflowResponse struct {
	ProtocolID int64                   `json:"protocolId"`
	Number     string                  `json:"number"`
	Employer   string                  `json:"employer"`
	Stages     []WorkflowStageResponse `json:"stages"`
}

type WorkflowStageResponse struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

func WriteWorkflowJSON(w http.ResponseWriter, response WorkflowResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(response)
}
```

- [ ] **Step 4: Verify API helper test**

Run:

```bash
go test ./backend/protocols -run TestWorkflowAPI_ReturnsJSON
```

Expected: PASS.

- [ ] **Step 5: Register API route group**

Create `backend/app/api_routes.go`:

```go
package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func registerAPIRoutes(router chi.Router, deps Deps, c *container) {
	router.Route("/api", func(r chi.Router) {
		r.Get("/session", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"authenticated":true}`))
		})
	})
}
```

Call `registerAPIRoutes(router, deps, newContainer(deps))` from `NewRouter` before the legacy HTML routes. Reuse the same container instance instead of constructing it twice.

- [ ] **Step 6: Verify all Go tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit initial API route**

Run:

```bash
git add backend/app/api_routes.go backend/app/router.go backend/protocols/api_handler.go backend/protocols/api_handler_test.go
git commit -m "feat: add initial workflow api contract"
```

## Task 5: Wire Frontend API Client

**Files:**
- Create: `frontend/src/api/client.ts`
- Modify: `frontend/src/app/App.tsx`
- Modify: `frontend/src/features/protocol-workflow/ProtocolWorkflowPage.test.tsx`

- [ ] **Step 1: Create API client**

Create `frontend/src/api/client.ts`:

```ts
import type { ProtocolWorkflow } from "./mockProtocolWorkflow";

export async function getProtocolWorkflow(protocolId: number): Promise<ProtocolWorkflow> {
  const response = await fetch(`/api/protocols/${protocolId}/workflow`, {
    headers: { Accept: "application/json" }
  });
  if (!response.ok) {
    throw new Error(`Failed to load protocol workflow: ${response.status}`);
  }
  return response.json() as Promise<ProtocolWorkflow>;
}
```

- [ ] **Step 2: Add loading and error state test**

Extend `frontend/src/features/protocol-workflow/ProtocolWorkflowPage.test.tsx` with:

```tsx
it("renders loading text while workflow is unavailable", () => {
  render(<ProtocolWorkflowPage workflow={undefined} />);

  expect(screen.getByText(/Загрузка протокола/i)).toBeInTheDocument();
});
```

- [ ] **Step 3: Run test to verify it fails**

Run:

```bash
npm --prefix frontend test -- ProtocolWorkflowPage.test.tsx
```

Expected: FAIL because `workflow` is required.

- [ ] **Step 4: Make workflow prop optional**

Update `ProtocolWorkflowPage.tsx` props:

```tsx
interface Props {
  workflow?: ProtocolWorkflow;
}

export function ProtocolWorkflowPage({ workflow }: Props) {
  if (!workflow) {
    return <p>Загрузка протокола...</p>;
  }

  return (
    <div className="workflow-page">
      {/* keep the existing rendered workflow markup here */}
    </div>
  );
}
```

- [ ] **Step 5: Verify frontend tests**

Run:

```bash
npm --prefix frontend test
```

Expected: PASS.

- [ ] **Step 6: Commit API client wiring**

Run:

```bash
git add frontend/src/api/client.ts frontend/src/features/protocol-workflow/ProtocolWorkflowPage.tsx frontend/src/features/protocol-workflow/ProtocolWorkflowPage.test.tsx
git commit -m "feat: add frontend workflow api client"
```

## Task 6: Add Playwright Smoke Test

**Files:**
- Create: `frontend/playwright.config.ts`
- Create: `frontend/tests/protocol-workflow.spec.ts`
- Modify: `frontend/package.json`

- [ ] **Step 1: Create Playwright config**

Create `frontend/playwright.config.ts`:

```ts
import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  timeout: 30_000,
  use: {
    baseURL: "http://127.0.0.1:5173",
    trace: "on-first-retry"
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] }
    }
  ],
  webServer: {
    command: "npm run dev",
    url: "http://127.0.0.1:5173",
    reuseExistingServer: !process.env.CI,
    timeout: 120_000
  }
});
```

- [ ] **Step 2: Create smoke test**

Create `frontend/tests/protocol-workflow.spec.ts`:

```ts
import { expect, test } from "@playwright/test";

test("protocol workflow mock screen is visible", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByRole("heading", { name: /2605А15/i })).toBeVisible();
  await expect(page.getByText(/Реестровые номера/i)).toBeVisible();
  await expect(page.getByText(/Заполните номера Минтруда/i)).toBeVisible();
});
```

- [ ] **Step 3: Install Playwright browsers**

Run:

```bash
npm --prefix frontend exec playwright install chromium
```

Expected: Chromium browser dependency installs successfully.

- [ ] **Step 4: Run Playwright**

Run:

```bash
npm --prefix frontend run e2e
```

Expected: PASS.

- [ ] **Step 5: Commit Playwright setup**

Run:

```bash
git add frontend/playwright.config.ts frontend/tests/protocol-workflow.spec.ts frontend/package.json frontend/package-lock.json
git commit -m "test: add frontend playwright smoke test"
```

## Task 7: Update Docker Build for Frontend Assets

**Files:**
- Modify: `Dockerfile`
- Modify: `.dockerignore`

- [ ] **Step 1: Add Node frontend build stage**

Modify `Dockerfile` before the Go builder stage:

```dockerfile
FROM node:24-alpine AS frontend-builder

WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build
```

- [ ] **Step 2: Copy frontend dist into Go embed location**

In the Go builder stage, after `COPY . .`, add:

```dockerfile
COPY --from=frontend-builder /src/frontend/dist ./backend/app/frontend/dist
```

This matches the valid Go embed path from Task 3.

- [ ] **Step 3: Verify Docker build**

Run:

```bash
DOCKER_BUILDKIT=1 docker compose build app
```

Expected: build exits `0`.

- [ ] **Step 4: Verify app starts**

Run:

```bash
DOCKER_BUILDKIT=1 docker compose up --build -d
docker compose ps
curl -sS -o /dev/null -w '%{http_code}\n' http://localhost:8081/login
```

Expected: service is healthy and curl prints `200` or `302`.

- [ ] **Step 5: Commit Docker changes**

Run:

```bash
git add Dockerfile .dockerignore
git commit -m "build: include frontend assets in docker image"
```

## Task 8: Add CI Workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Create CI workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - uses: actions/setup-node@v4
        with:
          node-version: 24
          cache: npm
          cache-dependency-path: frontend/package-lock.json

      - run: sh tests/run_schema_tests.sh
      - run: go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
      - run: go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate
      - run: go test ./...

      - run: npm ci
        working-directory: frontend
      - run: npm test
        working-directory: frontend
      - run: npm run build
        working-directory: frontend
      - run: npx playwright install --with-deps chromium
        working-directory: frontend
      - run: npm run e2e
        working-directory: frontend
```

- [ ] **Step 2: Verify workflow syntax locally if actionlint is available**

Run:

```bash
command -v actionlint && actionlint .github/workflows/ci.yml || true
```

Expected: no syntax errors if `actionlint` is installed.

- [ ] **Step 3: Commit CI workflow**

Run:

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add go and frontend checks"
```

## Final Verification

- [ ] Run backend tests:

```bash
go test ./...
```

Expected: PASS.

- [ ] Run frontend unit tests:

```bash
npm --prefix frontend test
```

Expected: PASS.

- [ ] Run frontend build:

```bash
npm --prefix frontend run build
```

Expected: PASS.

- [ ] Run Playwright:

```bash
npm --prefix frontend run e2e
```

Expected: PASS.

- [ ] Run Docker production-like smoke:

```bash
DOCKER_BUILDKIT=1 docker compose up --build -d
curl -sS -o /dev/null -w '%{http_code}\n' http://localhost:8081/login
```

Expected: service starts and curl prints `200` or `302`.
