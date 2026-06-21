# План реализации Core MVP Mintrud Admin

> **Зафиксированные параметры (входные):**
> - **Scope:** Core MVP — закрыть slice справочников + auth/CSRF + XLSX import заявок + protocols workflow + генерация XML/DOCX + audit UI. **Не входит:** Moodle sync, Windows MSI, background jobs queue.
> - **Legacy:** код `mintrud_generator` доступен локально; подключается через `replace`-директиву в `go.mod` или копированием пакетов.
> - **Auth стек:** `github.com/alexedwards/scs/v2` (sessions) + `github.com/gorilla/csrf`.
> - **Приёмка (E2E):** справочники → XLSX import заявки → training_records → protocol → XML для Минтруда + DOCX-протокол. Автоматизированный e2e тест зелёный.
>
> **Как пользоваться планом агентам:** каждый slice (F1-F3, D1-D4, E1-E2) — самостоятельная задача. Перед стартом прочти свой slice целиком + секцию «Контракты». Не нарушай контракты без согласования с team-lead. Каждый slice заканчивается приёмочными тестами и закрытием пункта в «Приёмке MVP».

---

## 0. Общие принципы для всех агентов

### 0.1 Definition of Done для любого slice

- Все обязательные тесты в секции «Tests» написаны и зелёные.
- `make schema-test && make test && make sqlc && make templ` проходят на macOS.
- Если slice создаёт новые формы — каждая защищена CSRF (после F3) и пишет в `action_log`.
- Handler'ы тонкие; бизнес-логика в services; SQL через `sqlc`.
- `templ generate` выполнен, `_templ.go` закоммичен.
- Чекбоксы в изначальном плане `2026-06-06-mintrud-admin-manual-directories.md` отмечены там, где slice их закрывает.

### 0.2 Контракты (interface freeze)

Эти контракты **обязательны** — другие slices опираются на них. Менять можно только с явным approval.

```go
// backend/audit/service.go
package audit

type Service struct { /* ... */ }

func NewService(q *storagedb.Queries) *Service

// Record пишет запись в action_log. entityID может быть nil.
func (s *Service) Record(ctx context.Context, in RecordInput) error

type RecordInput struct {
    Action     string                  // "create"|"update"|"deactivate"|"login"|"logout"|"import"|...
    EntityType string                  // "program_group"|"program"|"employer"|"worker"|...
    EntityID   sql.NullInt64
    Actor      string                  // "system"|"import"|"operator_unidentified"|<login post-auth>
    Details    map[string]any          // сериализуется в JSON
}

// backend/admin/service.go
package admin

type Service struct { /* ... */ }
func NewService(q *storagedb.Queries) *Service

func (s *Service) Authenticate(ctx context.Context, login, password string) (User, error)
func (s *Service) LoginHandler(w http.ResponseWriter, r *http.Request)  // GET форма + POST логика
func (s *Service) LogoutHandler(w http.ResponseWriter, r *http.Request)

func RequireAuth(s *scs.SessionManager, log *slog.Logger) func(http.Handler) http.Handler
func LoadCSRF() (func(http.Handler) http.Handler, error)

// backend/requests/service.go
package requests

type Service struct { /* ... */ }
func NewService(q *storagedb.Queries, audit *audit.Service) *Service

func (s *Service) CreateRequest(ctx, in CreateRequestInput) (ClientRequest, error)
func (s *Service) ImportRows(ctx, requestID int64, rows []ParsedRow) (ImportResult, error)
func (s *Service) ApplyRow(ctx, requestRowID int64) (ApplyResult, error) // возвращает созданные IDs

// backend/protocols/service.go
package protocols

func (s *Service) CreateProtocol(ctx, in CreateProtocolInput) (Protocol, error)
func (s *Service) AddParticipant(ctx, protocolID, trainingRecordID int64) error
func (s *Service) Fix(ctx, protocolID int64, in FixInput) (Protocol, error) // присваивает номер
func (s *Service) Transition(ctx, protocolID int64, to ProtocolStatus) error

// backend/documents/service.go
package documents

func GenerateXML(ctx, q *storagedb.Queries, protocolID int64) ([]byte, *GenerationRun, error)
func GenerateDOCX(ctx, q *storagedb.Queries, protocolID int64) ([]byte, *GenerationRun, error)
```

### 0.3 DAG зависимостей и параллельность

```
Фаза 0 (старт немедленно):
  F1 slice-completion            — agent #1
  F2 schema-cleanup              — agent #2
  R  documents-research (spike)  — agent #3 (только investigate legacy)

Фаза 1 (после F1+F2):
  F3 auth-foundation             — agent #1 или #2
  D4 audit-ui                    — agent #3 (только после F1)

Фаза 2 (после F1+F2+F3):
  D1 requests-slice              — agent #1
  D2 protocols-slice             — agent #2
  D3 documents-slice             — agent #3 (после R + D2)

Фаза 3 (после D1+D2+D3):
  E1 e2e-integration-tests       — agent #1+#2 (совместно)

Фаза 4:
  E2 verification-runbook        — любой агент
```

Параллельные группы:
- **Группа A (после F3):** D1, D2, D4 — независимы по файлам, но **только один агент** редактирует `backend/app/router.go` одновременно. Координировать через git-ветки и rebase.
- **D3** стартует с research, но реализация ждёт D2 (нужна `Protocol` модель).

### 0.4 Branch / commit policy

- Каждый slice — отдельная git-ветка `slice/<id>-<name>` от `main`.
- Один commit на task внутри slice — легко review/revert.
- Rebase на `main` перед merge.
- Merge только через PR; CI = `make schema-test && make test && make sqlc && make templ` + отсутствие diff в `_templ.go` после regenerate.

### 0.5 Риски и предположения (team-lead notes)

- **R1.** Точный путь к `mintrud_generator` неизвестен — первый шаг slice R = research.
- **R2.** Tabler/HTMX сейчас через CDN. Для офлайн-установки нужен embed — отложено в post-MVP. В runbook отметить.
- **R3.** SQLite WAL + serialized writes: на MVP хватит, но это hard limit. e2e тест должен проверять рестарт (close DB → open DB).
- **R4.** DOCX/XLSX output нестабилен бинарно. Golden tests — через нормализацию OpenXML содержимого, **не** `bytes.Equal`.

---

## F1. Slice Completion — закрыть ручные справочники

**Goal:** закрыть план `2026-06-06-mintrud-admin-manual-directories.md` Tasks 6-9 полностью — edit/deactivate/detail pages + выделенный `backend/audit/service.go`, который реально пишется из handlers.

**Dependencies:** нет. Можно стартовать немедленно.

**Parallelizable with:** F2, R.

**Files to create:**
- `backend/audit/service.go`
- `backend/audit/service_test.go`

**Files to modify:**
- `backend/programs/service.go`, `backend/programs/handler.go`, `backend/programs/views/views.templ`
- `backend/employers/service.go`, `backend/employers/handler.go`, `backend/employers/views/views.templ`
- `backend/people/service.go`, `backend/people/handler.go`, `backend/people/views/views.templ`
- `backend/storage/queries/action_log.sql`
- `backend/storage/queries/programs.sql`, `employers.sql`, `workers.sql`, `worker_employers.sql`
- `backend/app/router.go`

**Implementation steps:**

1. **Audit service (TDD).** Написать тесты на `audit.Service.Record`. Реализовать:
   - JSON-сериализация `Details`.
   - INSERT в `action_log` через sqlc.
   - `Actor` по умолчанию `"operator_unidentified"` (после F3 будет реальный login).
2. **sqlc queries:** добавить `UpdateGroup`, `DeactivateGroup`, `UpdateProgram`, `DeactivateProgram`, `UpdateEmployer`, `UpdateWorker`, `UpdateAssignment`, `DeactivateAssignment`, `GetWorkerByID`, `GetEmployerByID`, `GetGroupByID`, `GetProgramByID`.
3. **Programs:** формы и handlers `GET /programs/groups/{id}/edit`, `POST /programs/groups/{id}/edit`, `POST /programs/groups/{id}/deactivate`, аналогично для programs.
4. **Employers:** `GET /employers/{id}`, `GET /employers/{id}/edit`, `POST /employers/{id}`, `POST /employers/{id}/deactivate`. INN нормализуется в `Update` так же как в `Create`.
5. **People:** `GET /workers/{id}` detail со списком assignments + employer cards. `GET /workers/{id}/edit`, `POST /workers/{id}`, `POST /workers/assignments/{id}/deactivate`.
6. **Audit integration:** во всех mutating handlers вызвать `audit.Record` после успешного commit. Пример для CreateGroup: `Action="create", EntityType="program_group", EntityID=<new id>`.
7. **Routes:** зарегистрировать все новые роуты в `router.go`.
8. **Verify:** `make templ && make test` зелёный. Открыть `make run`, вручную пройти create/edit/deactivate для каждого справочника.

**Required tests:**

- `backend/audit/service_test.go`:
  - `TestRecord_PersistsAllFields`
  - `TestRecord_NilEntityID_Allowed`
  - `TestRecord_DetailsJSONSerialized`
- `backend/programs/service_test.go` (дополнить):
  - `TestUpdateGroup_NormalizesFields`
  - `TestDeactivateGroup_DoesNotHardDelete`
  - `TestUpdateProgram_RejectsInvalidGroup`
  - `TestDeactivateProgram_Idempotent`
- `backend/employers/service_test.go`:
  - `TestUpdateEmployer_RenormalizesINN`
  - `TestUpdateEmployer_DuplicateINN_MapsToFieldError`
- `backend/people/service_test.go`:
  - `TestUpdateWorker_RenormalizesSNILS`
  - `TestDeactivateAssignment_AllowsReassignLater`
  - `TestGetWorkerDetail_IncludesAssignments`
- `backend/app/router_test.go`:
  - `TestEdit_GET_ReturnsForm_200`
  - `TestEdit_POST_UpdatesRecord_Redirects`
  - `TestDeactivate_POST_ChangesStatus_Redirects`
  - `TestDetail_GET_Returns200_WithChildren`
- Integration (service+audit+DB):
  - `TestMutation_AlwaysWritesAudit`

**Acceptance criteria:**

- План `2026-06-06-mintrud-admin-manual-directories.md` Tasks 6-9 — все чекбоксы отмечены.
- В БД после CRUD-сессии в `action_log` есть по одной+ записи на каждый create/update/deactivate.
- README smoke-чеклист (design spec lines 234-242) проходит вручную.

---

## F2. Schema Cleanup — закрыть migration debt

**Goal:** выполнить cleanup из `docs/superpowers/specs/2026-06-06-mintrud-admin-manual-directories-design.md:86-94` до того, как slices D1/D2 начнут от схемы зависеть.

**Dependencies:** нет. Можно стартовать немедленно.

**Parallelizable with:** F1, R.

**Files to create:**
- `migrations/002_schema_cleanup.sql`

**Files to modify:**
- `tests/schema_smoke.sql`
- `tests/schema_constraints.sql`
- `backend/storage/db/models.go` (через `make sqlc` после миграции)

**Implementation steps:**

1. **Изучить текущую схему** `migrations/001_initial_schema.sql` и design spec, понять каждую проблему из списка debt.
2. **`imports` + `import_rows` tables:** добавить. Колонки:
   - `imports(id, source_type, source_file_name, source_sha256, uploaded_by_actor, received_at, status, created_at, updated_at)`.
   - `import_rows(id, import_id, row_number, raw_data, created_at)` FK к `imports`.
3. **FK fix:** `client_requests.source_import_id` теперь ссылается на `imports(id)` (с `ON DELETE SET NULL`).
4. **Request rows vocabulary:** оставить enum как есть, но **задокументировать** в комментарии в схеме, какой status что значит. Если выявлены реально ненужные статусы — убрать.
5. **Protocols suffix:** добавить `protocols.protocol_suffix TEXT` (например `'1'`, `'2'`, `'3'` или NULL). Unique index `ux_protocols_group_year_seq_suffix` на `(program_group_id, sequence_year, annual_sequence_number, protocol_suffix)`. Поле `protocol_number` остаётся вычисляемым на app-уровне (например `2026-03/001/2`).
6. **Status vocabulary протоколов:** если есть лишние/недостающие статусы — привести в соответствие с реальным workflow (`draft → fixed → xml_uploaded → registry_entered → generated → completed`, плюс `cancelled`). Закомментировать в схеме.
7. **Goose `-- +goose Up`** маркер в начале файла 002. **Down секцию пока не добавлять** (см. design: plain `sqlite3` schema tests не должны ничего дропать).
8. **Schema tests:** добавить constraint snippets:
   - INSERT в `client_requests` с `source_import_id` не из `imports` → FK violation.
   - Два протокола с одинаковой тройкой `(group, year, seq)` и одним `suffix` → unique violation.
   - Два протокола с одинаковой тройкой но **разными** suffix → OK.
9. **Smoke data:** обновить под новые таблицы.
10. **Regenerate sqlc:** `make sqlc`.

**Required tests:**

- `tests/run_schema_tests.sh` — зелёный.
- `tests/schema_constraints.sql`:
  - `test_imports_fk_rejects_orphan_source_import_id`
  - `test_protocols_unique_group_year_seq_suffix`
  - `test_protocols_different_suffix_allowed`
- `tests/schema_smoke.sql`:
  - Smoke row для `imports` + `import_rows`.
  - Smoke protocol с `protocol_suffix='2'`.
- `backend/storage/db_test.go`:
  - `TestMigrate_002_AppliesCleanOnEmptyDB`
  - `TestMigrate_002_RunsAfter001`

**Acceptance criteria:**

- Schema debt из `mintrud-open-schema-debt` (пункты 1-4) закрыт.
- `make schema-test && make sqlc` зелёный.
- Новые поля/таблицы попадают в `backend/storage/db/models.go`.

---

## R. Documents Research Spike (investigate legacy)

**Goal:** до старта D3 выяснить, какие пакеты `mintrud_generator` переиспользовать для XML/DOCX/XLSX, как их подключить, какие у них публичные API. Без кода — только исследование и отчёт.

**Dependencies:** нет. Можно стартовать немедленно.

**Parallelizable with:** F1, F2.

**Files to create:**
- `docs/superpowers/notes/2026-06-21-legacy-mintrud-generator-audit.md`

**Implementation steps:**

1. Найти локальный путь к `mintrud_generator`. Если неизвестен — спросить team-lead (не угадывать).
2. Составить inventory:
   - Структура пакетов.
   - Какие пакеты отвечают за: XML generation, DOCX generation, XLSX read/write, Moodle REST.
   - Текущие публичные API (function signatures).
   - Зависимости (внешние Go-библиотеки, CGO, платформо-специфичный код).
   - Покрытие тестами / есть ли golden fixtures.
3. Оценить переиспользование для каждой из 4 частей:
   - **XML:** можно ли вызвать `generator.GenerateXML(...) (bytes, error)` как есть?
   - **DOCX:** какой templating-движок? `bytes.Equal` невозможен — какие нормализации нужны?
   - **XLSX:** какие библиотеки? Совместимы ли с текущим `go.mod`?
   - **Moodle:** (out of Core MVP scope, но зафиксировать в отчёте для будущего slice).
4. Зафиксировать **способы подключения**:
   - Вариант A: `go.mod` `replace github.com/.../mintrud_generator => ../mintrud_generator` + import нужных packages.
   - Вариант B: копирование релевантных пакетов в `backend/documents/legacy/`.
   - Рекомендация: что выбрать и почему.
5. Зафиксировать **тестовые фикстуры**: какие сэмплы XLSX/DOCX/XML есть в legacy для golden tests.
6. Отчёт сохранить в `docs/superpowers/notes/`.

**Required tests:**

- Это research-задача; обязательных тестов нет.
- Отчёт должен содержать **конкретные package paths** и **function signatures** — по нему D3 будет работать без дополнительных исследований.

**Acceptance criteria:**

- Отчёт сохранён.
- В отчёте есть явная рекомендация подключения (A или B) с обоснованием.
- D3 может стартовать без повторного research.

---

## F3. Auth Foundation — sessions + CSRF + login

**Goal:** реализовать security baseline из `docs/tech-stack.md:122-135` на scs + gorilla/csrf.

**Dependencies:** F1 (нужен `audit.Service` для login/logout events), F2 (новые таблицы `users`).

**Parallelizable with:** D4 (после F1), R (после старта).

**Files to create:**
- `migrations/003_auth_schema.sql`
- `backend/admin/store.go`
- `backend/admin/service.go`
- `backend/admin/middleware.go`
- `backend/admin/session.go`
- `backend/admin/handler.go`
- `backend/admin/views/login.templ`
- `backend/admin/service_test.go`
- `backend/admin/middleware_test.go`
- `backend/admin/handler_test.go`

**Files to modify:**
- `go.mod` (добавить `github.com/alexedwards/scs/v2`, `github.com/gorilla/csrf`, `golang.org/x/crypto/bcrypt`)
- `backend/app/server.go` (init session manager, csrf, mount middleware)
- `backend/app/router.go` (login/logout routes, protected group)
- `backend/ui/layouts/shell.templ` (current operator, logout link, CSRF helper)
- Все forms в `backend/{programs,employers,people}/views/*.templ` (добавить CSRF field)
- `backend/audit/service.go` — принимать `Actor` из ctx (после auth это реальный login)

**Implementation steps:**

1. **Schema:** таблица `users(id, login UNIQUE, password_hash, role CHECK IN ('operator','admin'), status CHECK IN ('active','disabled'), created_at, updated_at)`. Не делать отдельную `sessions` таблицу — scs cookie store на MVP.
2. **Seed admin user:** при старте приложение проверяет, есть ли пользователь `admin`. Если нет — берёт пароль из env `MINTRUD_ADMIN_BOOTSTRAP_PASSWORD` (bcrypt-hashed перед записью) и создаёт запись. **Только env**: никаких autogenerate/log одноразовых паролей, никаких дефолтов. Если env не задан, а пользователя `admin` нет — приложение отказывается стартовать с понятной ошибкой. Если пользователь уже есть — env игнорируется (не даёт сменить пароль). Сменить пароль можно отдельным `cmd/mintrud-admin/reset-admin` или manually через SQL/bcrypt.
3. **`admin.Store`:** `GetUserByLogin(ctx, login) (User, error)` через sqlc.
4. **`admin.Service`:**
   - `Authenticate` использует `bcrypt.CompareHashAndPassword`.
   - `LoginHandler` GET — отдаёт форму; POST — `Authenticate` + `sessionManager.Put(ctx, "user_id", user.ID)` + redirect.
   - `LogoutHandler` — `sessionManager.Destroy` + redirect.
5. **Session manager:** `scs.New()` с cookie-based store. **Все настройки — через env:**
   - `MINTRUD_ADMIN_SESSION_TTL` — duration (Go time.Duration формат, например `8h`, `30m`), default `8h`.
   - `MINTRUD_ADMIN_COOKIE_SAMESITE` — `lax` (default) | `strict` | `none`.
   - `MINTRUD_ADMIN_COOKIE_SECURE` — `true` если `MINTRUD_ADMIN_ENV=prod`, иначе `false` (для dev). Можно переопределить явно.
   - `Cookie.HttpOnly=true` (всегда).
   Никаких хардкодов внутри `session.go` — все значения через config struct, который строится из env в `main.go`.
6. **CSRF:** `csrf.Protect(authKey, csrf.Secure(true/false), csrf.RequestHeader("X-CSRF-Token"))`. `authKey` из env (32 байта), если нет — генерировать per-process с лог-варнингом. Добавить helper `csrf.TemplateField(r)` для templ — `{{.csrfField}}` эквивалент.
7. **`RequireAuth` middleware:** проверяет `sessionManager.Get(ctx, "user_id")`; если нет — redirect `/login?next=...`.
8. **Audit integration:** login success/failure, logout — в `action_log` с actor = login.
9. **Router structure:**
   ```go
   r := chi.NewRouter()
   r.Use(csrfMiddleware)
   r.Get("/login", admin.LoginGet)
   r.Post("/login", admin.LoginPost)
   r.With(admin.RequireAuth).Group(func(r chi.Router) {
       // все текущие роуты сюда
       r.Get("/logout", admin.Logout)
   })
   ```
10. **Forms:** во все `<form method=post>` добавить `@csrfField(r)` (templ helper из `backend/ui/components`).
11. **Shell:** показать текущий `user.Login` + кнопку Logout.
12. **`Actor` propagation:** middleware кладёт login в ctx, `audit.Record` читает оттуда, если `Actor` пуст.

**Required tests:**

- `backend/admin/service_test.go`:
  - `TestAuthenticate_ValidCredentials`
  - `TestAuthenticate_WrongPassword_Errors`
  - `TestAuthenticate_DisabledUser_Errors`
- `backend/admin/middleware_test.go`:
  - `TestRequireAuth_NoSession_RedirectsToLogin`
  - `TestRequireAuth_WithSession_CallsNext`
- `backend/admin/handler_test.go`:
  - `TestLogin_GET_RendersForm`
  - `TestLogin_POST_Success_Redirects`
  - `TestLogin_POST_InvalidCredentials_RerendersForm`
  - `TestLogout_GET_DestroysSession_Redirects`
- Integration:
  - `TestProtectedRoute_WithoutSession_302`
  - `TestProtectedRoute_WithSession_200`
  - `TestPOST_WithoutCSRFToken_403`
  - `TestPOST_WithCSRFToken_200`

**Acceptance criteria:**

- Security baseline (tech-stack.md lines 122-135) реализован.
- Все формы защищены CSRF.
- Login/logout audited.
- `backend/app/middleware.go` создан (его не было в дизайне slice, но обязателен для auth).

---

## D1. Requests Slice — XLSX import staging

**Goal:** оператор загружает XLSX-заявку, система парсит строки в staging, оператор ревьюит и применяет, создавая `workers`, `worker_employers`, `training_records`.

**Dependencies:** F1 (audit), F2 (`imports`/`import_rows` schema), F3 (auth + CSRF на формах).

**Parallelizable with:** D2, D4 (но координировать `router.go`).

**Files to create:**
- `backend/requests/service.go`
- `backend/requests/handler.go`
- `backend/requests/parser.go`
- `backend/requests/normalizer.go`
- `backend/requests/state.go` (state machine для request_rows)
- `backend/requests/views/{list,detail,row_partial}.templ`
- `backend/requests/service_test.go`
- `backend/requests/handler_test.go`
- `backend/requests/parser_test.go`
- `backend/requests/normalizer_test.go`
- `tests/fixtures/requests/sample_valid.xlsx`
- `tests/fixtures/requests/sample_with_duplicate.xlsx`
- `tests/fixtures/requests/sample_invalid_email.xlsx`

**Files to modify:**
- `backend/storage/queries/client_requests.sql`
- `backend/storage/queries/request_rows.sql`
- `backend/storage/queries/request_training_items.sql`
- `backend/storage/queries/imports.sql`
- `backend/app/router.go` (`/requests/*` группа)
- `backend/ui/layouts/shell.templ` (пункт Requests)
- `backend/ui/components/components.templ` (status badge helpers)

**Implementation steps:**

1. **State machine** (`state.go`): `pending → parsed → {applied|skipped|invalid}`; для items: `pending → {valid|invalid|duplicate|conflict} → applied|skipped`. Все переходы — явная функция `CanTransition(from, to) bool`.
2. **Parser** (`parser.go`): чтение XLSX через `github.com/xuri/excelize/v2` (если legacy использует другой — взять оттуда). На выходе `[]ParsedRow{RowNumber, RawFullName, RawSNILS, RawEmail, RawPosition, RawPrograms[]}`.
3. **Normalizer** (`normalizer.go`):
   - ФИО → split на last/first/middle, trim.
   - СНИЛС → формат `XXX-XXX-XXX YY`, normalized digits-only.
   - Email → lowercase, trim, RFC-проверка через `net/mail`.
   - Program codes → uppercase, trim.
4. **CreateRequest flow:**
   - `CreateRequest(ctx, employerID, receivedDate, notes)` → INSERT `client_requests`.
   - `ImportRows` кладёт XLSX в `imports` + `import_rows` (raw) + создаёт `request_rows` после normalize.
   - Параллельно создаёт `request_training_items` для каждой программы в строке.
5. **ApplyRow** (`service.go`):
   - Найти `worker` по `snils_normalized`. Если нет — создать.
   - Найти `worker_employer` active. Если нет — создать.
   - Создать `training_records` на каждый `request_training_item`.
   - Atomic transaction. Audit на каждый created entity.
   - Дубликат (тот же worker + program + активная training_record) → status `duplicate`, требует resolution.
6. **UI flows:**
   - List requests с фильтрами по status.
   - Detail request: таблица rows, каждая со своим status badge + кнопкой apply / skip / edit.
   - HTMX-эндпоинты для inline apply (возвращают partial).
7. **Routing:** `/requests`, `/requests/new`, `/requests/{id}`, `/requests/{id}/rows/{rowID}/apply` (POST), `.../skip`.
8. **Audit:** import, parse, apply, skip.

**Required tests:**

- `parser_test.go`:
  - `TestParse_ValidXLSX_GoldenFixture` (читает `sample_valid.xlsx`, сравнивает с ожидаемым `ParsedRow[]`).
  - `TestParse_EmptyFile_Errors`
  - `TestParse_MissingRequiredSheet_Errors`
- `normalizer_test.go`:
  - `TestNormalizeSNILS_Formats_And_DigitOnlyNormalized`
  - `TestNormalizeEmail_Lowercases`
  - `TestNormalizeEmail_InvalidFormat_Errors`
  - `TestSplitFullName_TwoParts_OK_ThreeParts_OK`
- `service_test.go`:
  - `TestCreateRequest_Persists`
  - `TestImportRows_CreatesRequestRowsAndItems`
  - `TestApplyRow_NewWorker_NewAssignment_NewTrainingRecord`
  - `TestApplyRow_ExistingWorker_NewAssignment`
  - `TestApplyRow_DuplicateTrainingRecord_MarksDuplicate`
  - `TestApplyRow_InvalidEmail_MarksInvalid`
- `handler_test.go`:
  - `TestUpload_POST_CreatesRequest_Redirects`
  - `TestList_GET_RendersRequests`
  - `TestDetail_GET_RendersRows`
  - `TestApply_POST_TransitionsStatus`
- Integration:
  - `TestE2E_UploadParseApply_AllEntitiesCreated` (без protocol — это E1).

**Acceptance criteria:**

- Дизайн `requests` slice (tech-stack.md) — пройден.
- e2e мини-тест: upload XLSX → parse → apply → в БД есть `worker` + `worker_employer` + `training_records` + `action_log` entries.

---

## D2. Protocols Slice — lifecycle + нумерация

**Goal:** оператор создаёт протокол по `program_group`, добавляет участников из `training_records`, фиксирует даты, получает номер с суффиксом, проводит через весь lifecycle.

**Dependencies:** F1, F2 (`protocol_suffix` schema), F3.

**Parallelizable with:** D1, D4 (координировать `router.go`).

**Files to create:**
- `backend/protocols/service.go`
- `backend/protocols/handler.go`
- `backend/protocols/numbering.go`
- `backend/protocols/state.go`
- `backend/protocols/views/{list,detail,fix_form,participant_partial}.templ`
- `backend/protocols/service_test.go`
- `backend/protocols/handler_test.go`
- `backend/protocols/numbering_test.go`
- `backend/protocols/state_test.go`

**Files to modify:**
- `backend/storage/queries/protocols.sql`
- `backend/storage/queries/protocol_participants.sql`
- `backend/app/router.go` (`/protocols/*` группа)
- `backend/ui/layouts/shell.templ`

**Implementation steps:**

1. **State machine** (`state.go`): `draft → fixed → xml_uploaded → registry_entered → generated → completed` (+ `cancelled` из любого). Функция `CanTransition(from, to)` — explicit, без implicit fallback.
2. **`CreateProtocol`** — INSERT с status=`draft`, обязательны только `program_group_id`. Остальные поля nullable.
3. **`AddParticipant` / `RemoveParticipant`** — учитывают unique constraint `ux_protocol_participants_active_training_record` (одна active запись на training_record).
4. **`Fix`** (`numbering.go`):
   - Требует заполненные `training_start_date`, `training_end_date`, `protocol_date`.
   - В **одной транзакции**: MAX(annual_sequence_number) для `(program_group_id, sequence_year)` + 1; если `protocol_suffix` указан явно — позволяет делить одно и то же `annual_sequence_number` между суффиксами.
   - Формирует `protocol_number` в формате `<год>-<месяц>/<seq_3_digit>/<suffix>` (или без suffix если NULL).
   - Ставит `fixed_at = now()`, `status = fixed`.
5. **`Transition`** — валидирует `CanTransition`, обновляет status, пишет audit.
6. **UI flows:**
   - List protocols (по group/status filters).
   - Detail protocol: участники, даты, номер, status badge, кнопки переходов.
   - Fix form: только если ещё не fixed.
   - HTMX для add/remove participant (возвращают partial строки таблицы).
7. **Routing:** `/protocols`, `/protocols/new`, `/protocols/{id}`, `/protocols/{id}/fix` (GET/POST), `/protocols/{id}/participants` (POST add), `/protocols/{id}/participants/{pid}` (POST remove), `/protocols/{id}/transition` (POST to=...).
8. **Audit:** create, add/remove participant, fix, every transition.

**Required tests:**

- `numbering_test.go`:
  - `TestNextSequence_FirstInYear_Returns1`
  - `TestNextSequence_SecondInYear_Returns2`
  - `TestFix_WithSuffix_SharesSequenceNumberAcrossSuffixes`
  - `TestFix_DifferentYears_IndependentSequences`
  - `TestFix_DifferentGroups_IndependentSequences`
- `state_test.go`:
  - `TestCanTransition_AllValidPaths`
  - `TestCanTransition_InvalidPathsRejected`
  - `TestTransition_Cancelled_FromAnyState`
- `service_test.go`:
  - `TestCreateProtocol_OnlyGroupID_Required`
  - `TestAddParticipant_TrainingRecordAlreadyActiveInOtherProtocol_Rejected`
  - `TestFix_MissingDates_RejectedWithFieldError`
  - `TestFix_Success_AssignsNumberAndStatus`
  - `TestTransition_FixedToXmlUploaded_OK`
  - `TestTransition_DraftToGenerated_Rejected`
- `handler_test.go`:
  - `TestList_GET_RendersProtocols`
  - `TestDetail_GET_ShowsParticipants`
  - `TestFix_GET_Form_Only_If_Not_Fixed`
  - `TestFix_POST_RedirectsToDetail`
  - `TestAddParticipant_POST_UpdatesList`
  - `TestTransition_POST_ChangesStatus`
- Integration:
  - `TestFullLifecycle_Draft_To_Completed_AllStepsWork`

**Acceptance criteria:**

- Protocols slice полностью функционален.
- Суффиксы `/1`, `/2`, `/3` работают через `protocol_suffix`.
- Schema debt пункт 3 (протоколы) реально используется в коде.

---

## D3. Documents Slice — XML + DOCX generation

**Goal:** генерация XML для Минтруда + DOCX-протоколов из зафиксированного protocol, с golden tests. spike-gate критерий 5.

**Dependencies:** R (research отчёт), D2 (модель Protocol + participants), F1 (audit).

**Parallelizable with:** ничего на фазе реализации — D3 пишется после D2.

**Files to create:**
- `backend/documents/service.go`
- `backend/documents/xml.go`
- `backend/documents/docx.go`
- `backend/documents/normalization.go` (для golden test сравнений)
- `backend/documents/handler.go`
- `backend/documents/views/{generation_partial}.templ`
- `backend/documents/service_test.go`
- `backend/documents/xml_test.go`
- `backend/documents/docx_test.go`
- `tests/fixtures/documents/expected_mintrud.xml` (golden)
- `tests/fixtures/documents/expected_protocol.docx` (golden reference)
- `tests/fixtures/documents/protocol_seed.sql` (фиксированный protocol для golden)

**Files to modify:**
- `go.mod` (подключение legacy как replace или копирование пакетов — решение из отчёта R)
- `backend/app/router.go` (`/protocols/{id}/generate` endpoints)
- `backend/protocols/views/detail.templ` (кнопки generate + download links)

**Implementation steps:**

1. **Прочитать отчёт R** и зафиксировать способ подключения legacy.
2. **Подключить legacy** через `replace`-директиву (рекомендация R) или копирование.
2a. **Adapter layer** (важно): legacy-код почти наверняка ожидает данные в своём собственном формате (например, XLSX rows или Go-структуры из старой кодовой базы). Между нашим `backend/documents/` и legacy API **обязательно** строится adapter:
    - `documents/adapter_legacy.go` с функциями вида `ProtocolToLegacyInput(p Protocol) legacy.GenerateInput`.
    - Наш `documents.GenerateXML(ctx, q, protocolID)` → загружает model → прогоняет через adapter → вызывает legacy → оборачивает ошибки в доменные.
    - Никаких вызовов legacy напрямую из handlers/services вне `backend/documents/`.
    - **Зачем:** если legacy меняется или заменяется (в tech-stack.md явно зафиксирована возможность замены Moodle на внутреннюю LMS — тот же принцип), меняется только adapter. Без adapter layer замена legacy приведёт к cascade-правкам по всему коду.
3. **`GenerateXML(ctx, q, protocolID)`** (`xml.go`):
   - Загрузить protocol + participants + workers + programs + employers.
   - Вызвать legacy XML generation function.
   - Записать `generation_runs` row (type=`xml`, status, file_name, error).
   - Вернуть `([]byte, *GenerationRun, error)`.
4. **`GenerateDOCX(ctx, q, protocolID)`** (`docx.go`):
   - Аналогично, через legacy DOCX templating.
   - Записать `generation_runs`.
5. **Normalization** (`normalization.go`):
   - `NormalizeXML([]byte) []byte` — каноникализация (sort attrs, strip comments, fixed whitespace).
   - `NormalizeDOCX([]byte) ([]NormalizedPart, error)` — распаковать zip, для каждого part (`document.xml`, `header*.xml`, ...) канонализовать XML. Бинарные parts (images) сравнивать по SHA256.
6. **Handler:**
   - `POST /protocols/{id}/generate?type=xml` — генерация, redirect на download.
   - `GET /protocols/{id}/download?run=<id>` — отдаёт файл с правильным Content-Type.
7. **UI:** на detail protocol — кнопки "Generate XML" / "Generate DOCX" (только если `status >= fixed`), список последних generation runs.
8. **Audit:** generate events.

**Required tests (критично, spike-gate):**

- `xml_test.go`:
  - `TestGenerateXML_GoldenFixture` (seed protocol → bytes → NormalizeXML → сравнить с `expected_mintrud.xml`).
  - `TestGenerateXML_FailsOnInvalidProtocol`
  - `TestGenerateXML_RecordsGenerationRun`
- `docx_test.go`:
  - `TestGenerateDOCX_GoldenFixture` (распаковать `expected_protocol.docx`, сравнить через NormalizeDOCX).
  - `TestGenerateDOCX_RecordsGenerationRun`
  - `TestNormalizeDOCX_StableAcrossRuns` (дважды сгенерить — нормализованный output идентичен)
- `service_test.go`:
  - `TestGenerate_StatusNotFixed_Rejected`
  - `TestGenerate_Exception_RecordsFailedRun`
- `handler_test.go`:
  - `TestGenerate_POST_RedirectsToDownload`
  - `TestDownload_GET_ReturnsFile`
- Integration:
  - `TestFullDocFlow_FixedProtocol_GenerateBoth_Downloads`

**Acceptance criteria:**

- Spike-gate критерий 5 (`docs/tech-stack.md:170-172`): golden tests подтверждают стабильный XML/DOCX output на реальных fixtures.
- DOCX/XLSX проверяются через нормализацию, **не** `bytes.Equal`.
- Генерация writes generation_runs.

---

## D4. Audit UI

**Goal:** страница просмотра `action_log` с фильтрами — закрывает «полный audit log» из tech-stack.md.

**Dependencies:** F1 (audit service).

**Parallelizable with:** D1, D2 (после F3, координировать `router.go`).

**Files to create:**
- `backend/audit/handler.go`
- `backend/audit/views/list.templ`
- `backend/audit/handler_test.go`

**Files to modify:**
- `backend/storage/queries/action_log.sql` (фильтры + pagination)
- `backend/app/router.go`
- `backend/ui/layouts/shell.templ`

**Implementation steps:**

1. List query с фильтрами: `actor`, `action`, `entity_type`, `entity_id`, `created_at` range.
2. Pagination через LIMIT/OFFSET (50/page).
3. HTMX для filter apply без full reload.
4. Шаблон: timestamp, actor, action, entity (ссылка на detail если применимо), details (formatted JSON).
5. Audit read-only — не writes в action_log чтение лога.

**Required tests:**

- `handler_test.go`:
  - `TestList_GET_RendersEvents`
  - `TestList_Filters_ByActor`
  - `TestList_Filters_ByEntityType`
  - `TestList_Filters_ByDateRange`
  - `TestList_Pagination_NextPrevLinks`
  - `TestList_EmptyState`

**Acceptance criteria:**

- Страница Audit доступна оператору.
- Фильтры работают.
- Не падает на пустом action_log.

---

## E1. E2E Integration Tests — приёмка Core MVP

**Goal:** автоматизированный e2e тест полного цикла. Это **главный критерий приёмки** MVP.

**Dependencies:** D1, D2, D3, F3 — все domain slices готовы.

**Parallelizable with:** E2 (после E1).

**Files to create:**
- `tests/e2e/full_cycle_test.go`
- `tests/e2e/fixtures/sample_request.xlsx` (или reuse из D1)
- `tests/e2e/helpers.go` (HTTP client с session/CSRF)
- `tests/e2e/e2e_test.go` (test runner + setup)

**Implementation steps:**

1. **Setup helper:** создать temp DB, apply migrations, seed admin user, стартовать `app.NewServer` на случайном порту, вернуть `*http.Client` с cookie jar.
2. **Login helper:** POST `/login`, парсить CSRF token из следующего GET, использовать во всех последующих POST.
3. **`TestFullCycle`:**
   1. Login как admin.
   2. Создать `program_group` "Test Group", создать `program` "Test Program 100h".
   3. Создать `employer` (INN test).
   4. Загрузить `sample_request.xlsx` через `/requests/new`.
   5. Открыть detail, apply все валидные rows.
   6. Assert: `workers`, `worker_employers`, `training_records` существуют.
   7. Создать protocol по program_group.
   8. Добавить участников (training_records из шага 5).
   9. Fix protocol (заполнить даты).
   10. Assert: `protocol_number` присвоен, `status=fixed`.
   11. Generate XML → assert response 200, content-type `application/xml`, content содержит ожидаемые worker names.
   12. Generate DOCX → assert response 200, content-type `application/vnd.openxmlformats-officedocument.wordprocessingml.document`.
   13. Assert: `action_log` содержит entries для каждого ключевого шага.
4. **`TestFullCycle_Restart`:** после полного цикла — закрыть DB, переоткрыть, применить миграции (idempotency), проверить что данные на месте.

**Required tests:**

- `TestFullCycle` — описан выше.
- `TestFullCycle_Restart` — DB close/reopen сохраняет данные.
- `TestFullCycle_AuditLog_AllKeyStepsRecorded`

**Acceptance criteria:**

- Все три теста зелёные.
- Это и есть **критерий приёмки MVP**: «Полный цикл: справочники → XLSX import → training_records → protocol → XML+DOCX».

---

## E2. Verification & Runbook

**Goal:** финальная проверка, runbook, обновлённый README, отметка планов как выполненных.

**Dependencies:** E1.

**Files to create:**
- `docs/runbook.md`

**Files to modify:**
- `README.md`
- `docs/superpowers/plans/2026-06-06-mintrud-admin-manual-directories.md` (отметить чекбоксы)
- `docs/superpowers/plans/2026-06-21-mintrud-admin-core-mvp.md` (отметить status каждого slice)

**Implementation steps:**

1. **`docs/runbook.md`:**
   - Local dev setup (ссылка на Makefile targets).
   - Backup: `sqlite3 data/mintrud-admin.db ".backup data/backup-YYYY-MM-DD.db"`.
   - Restore: остановить сервис, заменить файл, запустить.
   - Bootstrap admin user (env var).
   - Production env vars: `MINTRUD_ADMIN_ADDR`, `MINTRUD_ADMIN_DB`, `MINTRUD_ADMIN_ENV=prod`, `MINTRUD_ADMIN_CSRF_KEY`, `MINTRUD_ADMIN_SESSION_KEY`.
   - Troubleshooting: locked DB, migration failure, forgotten admin password.
2. **README.md:** обновить current slice (Core MVP), добавить ссылку на runbook.
3. **Отметить все чекбоксы** в `2026-06-06-mintrud-admin-manual-directories.md`.
4. **Status markers** в `2026-06-21-mintrud-admin-core-mvp.md`: каждый slice получает `[x]`.
5. **Smoke run на macOS:** `make run` + ручной проход e2e сценария через браузер.
6. **Outstanding risks list** для post-MVP: Moodle, MSI installer, embed Tabler для офлайн, PostgreSQL migration path.

**Required tests:**

- `make schema-test && make test && make sqlc && make templ` — всё зелёное.
- `go test ./tests/e2e/...` — зелёное.
- Manual smoke checklist в runbook — выполнен.

**Acceptance criteria:**

- MVP готов к ручному запуску.
- Команда может развернуть инстанс по runbook.

---

## Приёмка MVP (summary)

Core MVP считается готовым, когда **все** условия выполнены:

- [ ] F1: Slice Completion — все CRUD-операции справочников + audit writes.
- [ ] F2: Schema Cleanup — migration debt закрыт, миграция 002 applied.
- [ ] R: Documents Research — отчёт готов.
- [ ] F3: Auth Foundation — login/sessions/CSRF работают, защищают все формы.
- [ ] D1: Requests Slice — XLSX upload + parse + apply работает end-to-end.
- [ ] D2: Protocols Slice — full lifecycle + numbering с suffix.
- [ ] D3: Documents Slice — XML+DOCX generation, golden tests стабильны.
- [ ] D4: Audit UI — оператор видит историю изменений.
- [ ] E1: E2E Integration Tests — `TestFullCycle` зелёный.
- [ ] E2: Verification & Runbook — README/runbook обновлены, чекбоксы отмечены.

**Главный критерий приёмки:** `go test ./tests/e2e/... -run TestFullCycle` проходит — программа выполняет полный e2e процесс (справочники → XLSX import → training_records → protocol → XML+DOCX).

---

## Распределение по команде (рекомендация team-lead)

| Agent | Фаза 0 | Фаза 1 | Фаза 2 | Фаза 3 |
|-------|--------|--------|--------|--------|
| #1 | F1 | F3 | D1 | E1 |
| #2 | F2 | (free / review) | D2 | E1 |
| #3 | R | D4 | D3 | E2 |
| #4 | (review / lint) | (review) | D1-helper или D4 | E2-helper |

3 параллельных агента на пиковых фазах. Agent #4 — optional, для ревью и помощи.

**Координационный контракт:** только один агент в любой момент времени правит `backend/app/router.go` и `backend/ui/layouts/shell.templ`. Использовать отдельные feature-branches + rebase на main перед merge.

---

## Зафиксированные решения (post user-review)

1. **`mintrud_generator` доступ** — через `gh` CLI (`gh repo clone IvanSaratov/mintrud_generator`) или GitHub MCP. Репозиторий private, авторизован как `IvanSaratov`. Slice R начинает с клонирования во временную директорию и inventory.
2. **Session timeout** — только через env `MINTRUD_ADMIN_SESSION_TTL` (default `8h`).
3. **Bootstrap admin password** — только через env `MINTRUD_ADMIN_BOOTSTRAP_PASSWORD`. Никаких autogenerate или log.
4. **Adapter layer в D3 обязателен** — см. шаг 2a в D3. Это не переговорная позиция: tech-stack.md явно требует изоляции legacy для будущей замены.
