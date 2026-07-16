# Import HTTP API Design

Дата: 2026-07-16
Статус: согласован для подготовки implementation plan

## 1. Контекст

В проекте уже реализованы:

- persistence-модель импортов и staging;
- промышленный preflight/parser профиля `legacy_registry`;
- безопасное временное файловое хранилище;
- атомарный `EnqueueLegacy` с SHA-256, idempotency, FIFO и лимитом очереди.

Следующий этап открывает эти возможности через HTTP. Worker ещё не запущен,
поэтому созданные задания остаются в `queued`; это осознанная граница этапа.

## 2. Цели и границы

Этап добавляет минимальный законченный API-срез:

- `POST /api/imports/legacy` для upload и durable enqueue;
- `GET /api/imports` для очереди и истории;
- `GET /api/imports/{id}` для состояния одного задания;
- `GET /api/csrf` для получения masked token после login;
- единый `application/problem+json` для защищённого import API;
- API-аутентификацию без HTML redirect и актуальную проверку роли через БД;
- CSRF-защиту upload endpoint;
- trace ID для безопасного сопоставления ответа и серверного лога.

В этап не входят:

- background worker, staging строк и canonical writes;
- retry и cancel;
- list, patch или apply строк импорта;
- frontend;
- новая миграция БД.

## 3. Компоненты

### 3.1 Общий API package

Новый `backend/api` владеет транспортными примитивами, не зависящими от
импортов:

- `Problem` и безопасная JSON-сериализация;
- генерация trace ID как 32 lowercase hex символов из 16 случайных байт;
- middleware, записывающий trace ID в context и `X-Trace-ID` ответа;
- helpers для получения trace ID и записи problem response.

`Problem` содержит `status`, стабильный `code`, русскоязычный безопасный
`detail`, `trace_id` и, когда применимо, `existing_import_id`. Stack trace,
nested error, SQL/filesystem path и request body не сериализуются.

### 3.2 API auth и роли

`backend/admin` получает отдельный middleware для JSON API. Он не переиспользует
HTML-семантику `RequireAuth`, потому что отсутствие сессии должно вернуть `401`,
а не `303` на `/login`.

Алгоритм каждого защищённого import API-запроса:

1. Получить `user_id` из session.
2. Загрузить пользователя через `admin.Store.GetByID`.
3. Проверить `status = active`.
4. Опубликовать минимальную identity `{id, login, role}` в context.
5. Проверить разрешённую роль endpoint.

Удалённый, неизвестный или disabled пользователь получает `401`. Ошибка БД
получает `503`. Недостаточная роль получает `403`. Это обеспечивает немедленное
применение disable/role change и не полагается на устаревшую роль в cookie.

Права:

- upload: только `admin`;
- list/detail: `admin` и `operator`.

### 3.3 Imports HTTP handler

Handler зависит от узких интерфейсов enqueue/read services, а не от concrete
SQLite или filesystem типов. Существующий `EnqueueLegacy` остаётся transport-
agnostic.

Handler отвечает за:

- HTTP content type и multipart contract;
- чтение `Idempotency-Key` и actor из auth context;
- вызов enqueue/read service;
- DTO, status codes, `Location` и problem responses;
- cursor/limit validation.

### 3.4 Read service и SQL

Read service возвращает только безопасную проекцию импорта. SQL list/detail
сразу вычисляет `queue_position`; handler не выполняет отдельный запрос для
каждой строки.

Проекция не содержит:

- `source_sha256`;
- `idempotency_key`;
- `temp_file_token` и expiry/path;
- workbook rows или header maps;
- внутренние причины ошибок.

Она содержит ID, profile, очищенное имя файла, uploaded actor, status, phase,
queue position, progress counters, безопасные error code/detail и lifecycle
timestamps.

## 4. Маршруты и middleware

### 4.1 Upload

```http
POST /api/imports/legacy
Content-Type: multipart/form-data; boundary=...
Idempotency-Key: <optional>
X-CSRF-Token: ...
```

Middleware выполняются в порядке:

`trace → request logging → API auth → admin role → CSRF → handler`

`Idempotency-Key` необязателен на backend boundary, но frontend обязан
генерировать его для каждой пользовательской попытки upload и повторно
использовать при сетевом retry.

Multipart содержит ровно одну часть `file` с непустым filename. Обычные form
fields, второй файл и произвольные дополнительные parts запрещены. Handler не
буферизует workbook целиком и не создаёт вторую временную копию.

Для новой загрузки multipart reader оборачивает file part так, чтобы достижение
EOF одновременно проверяло отсутствие следующей части. Любая лишняя часть
становится read error до commit; `FileStore` удаляет частичный файл. При
idempotent reuse service может вернуть существующее задание без чтения file
body; payload повтора намеренно не сравнивается с первоначальным запросом.

Размер всего HTTP body ограничен через `http.MaxBytesReader` значением file
limit плюс 1 MiB на boundary и headers. Независимый лимит `FileStore` остаётся
окончательной проверкой именно file bytes.

Успешный новый ответ:

```json
{
  "id": 42,
  "status": "queued",
  "phase": null,
  "queue_position": 2,
  "reused": false,
  "status_url": "/api/imports/42"
}
```

Новый import возвращает `202 Accepted` и
`Location: /api/imports/{id}`. Idempotent reuse возвращает `202`, пока import
активен, и `200 OK`, если import terminal.

### 4.2 List

```http
GET /api/imports?cursor=<opaque>&limit=50
```

Сортировка стабильна: `id DESC`. Cursor — base64url без padding от строки
`v1:<last_id>` последнего возвращённого элемента. `limit` по умолчанию 50,
максимум 200.
Некорректные, нулевые и отрицательные значения получают `400`.

Service запрашивает `limit + 1` строку. Ответ возвращает `items` и
`next_cursor`; cursor отсутствует, если следующей страницы нет.

### 4.3 Detail

```http
GET /api/imports/{id}
```

ID обязан быть положительным decimal integer. Отсутствующий import получает
`404`. Ответ использует ту же полную безопасную проекцию, что list item, чтобы
UI не поддерживал два несовместимых формата состояния.

## 5. HTTP errors

| Ситуация | HTTP | code |
|---|---:|---|
| Некорректный multipart, cursor, ID или idempotency key | 400 | `invalid_input` |
| Нет действующей session | 401 | `unauthorized` |
| Недостаточная роль или CSRF failure | 403 | `forbidden` / `csrf_failed` |
| Import не найден | 404 | `import_not_found` |
| Повтор SHA | 409 | `duplicate_file` |
| Idempotency key другого профиля | 409 | `idempotency_conflict` |
| Upload превышает лимит | 413 | `file_too_large` |
| File part не является XLSX | 415 | `not_xlsx` |
| Структура workbook не поддерживается | 422 | `unsupported_workbook` |
| Лимит active queue достигнут | 429 | `queue_full` |
| Неожиданная ошибка | 500 | `internal_error` |
| БД или временное хранилище недоступны | 503 | `storage_unavailable` |

Оба варианта `409` содержат `existing_import_id`. `429` получает
`Retry-After: 5`; это подсказка UI повторить запрос состояния, а не автоматически
повторно загружать body.

Import subrouter задаёт JSON NotFound/MethodNotAllowed handlers, чтобы ошибка
пути или метода внутри `/api/imports` не возвращала HTML.

## 6. CSRF

Public login API остаётся без CSRF, поскольку до login нет authenticated
session. Mutating import upload проходит через существующую gorilla/csrf
защиту.

После login frontend вызывает защищённый `GET /api/csrf`. Endpoint проходит
API auth и safe GET через gorilla/csrf, устанавливает HttpOnly CSRF cookie и
возвращает `{"csrf_token":"<masked token>"}`. Frontend передаёт token в
`X-CSRF-Token` при upload. Отдельный endpoint необходим, потому что HttpOnly
cookie намеренно недоступна JavaScript.

CSRF failure handler определяет API request по пути `/api/` и возвращает
безопасный `application/problem+json`; legacy form routes сохраняют текущую
HTML/text семантику. Проверка API auth и роли выполняется раньше CSRF, поэтому
отсутствие session стабильно возвращает `401`.

## 7. Runtime wiring и файлы

В production upload root равен `<database-directory>/imports`. Он передаётся в
`app.ServerConfig`; `NewServer` создаёт `LocalFileStore` и import service до
начала listen. Ошибка создания или hardening директории прекращает startup, а
не оставляет endpoint, который всегда отвечает `503`.

SQLite backup копирует только database file, поэтому соседняя import directory
не включается в backup. Новая environment variable в этом этапе не нужна.

Ожидаемые новые файлы:

- `backend/api/problem.go`, `trace.go` и тесты;
- `backend/admin/api_middleware.go` и тесты;
- `backend/imports/http_handler.go`, `read_service.go` и тесты.

Ожидаемые изменённые файлы:

- `backend/admin/csrf.go` и тесты;
- `backend/storage/queries/imports.sql` и generated sqlc;
- `backend/app/container.go`, `api_routes.go`, `server.go` и тесты;
- `backend/internal/runner/serve.go` и тесты.

Точный implementation plan может разделить DTO/helper tests на дополнительные
файлы внутри этих пакетов. Миграции, frontend и industrial XLSX не изменяются.

## 8. Logging и PII

Request log содержит trace ID, method, path без query, status, duration и actor.
Он не содержит multipart filename, headers, query cursor, idempotency key,
SHA-256, filesystem path или тело ответа.

Problem responses не сериализуют nested errors. Тестовые workbooks используют
только синтетические имена и домены `.example`/`example.test`. Промышленный файл
из `temp` не открывается HTTP-тестами и не попадает в fixtures или git.

## 9. Testing

### API primitives

- уникальность и безопасный формат trace ID;
- сохранение/возврат входящего валидного trace ID не допускается: сервер всегда
  создаёт собственный ID;
- content type и shape `Problem`;
- writer не включает nested error.

### Auth и роли

- session отсутствует;
- user отсутствует или disabled;
- DB unavailable;
- operator forbidden для upload;
- operator/admin разрешены для read;
- actor и identity опубликованы в context;
- auth выполняется до CSRF.

### Upload handler

- корректная синтетическая книга создаёт import и возвращает `202`/Location;
- terminal idempotent reuse возвращает `200` без чтения body;
- отсутствие файла, пустой filename, неправильное part name;
- второй file, дополнительное field, повреждённый multipart;
- HTTP body/file limit;
- invalid XLSX и unsupported workbook;
- duplicate SHA, idempotency conflict и queue full;
- DB/filesystem failures;
- полная table-driven проверка domain error → HTTP problem mapping;
- responses/logs не содержат password, row data, SHA, token или nested error.

### Read API

- empty/list pages, default/max limit и next cursor;
- invalid cursor/limit;
- stable pagination без пропусков и повторов;
- queued positions, processing/terminal position 0;
- detail и `404`;
- одна SQL query на страницу без N+1;
- projection не содержит закрытых persistence fields.

### Integration и regression

- реальные session, API auth, role и CSRF через app router;
- startup создаёт directory `0700`, filesystem failure останавливает server;
- login/session API и legacy form routes сохраняют прежнее поведение;
- `go test -count=1 ./...`;
- `go test -race -count=1 ./api ./admin ./imports ./app`;
- schema suite остаётся зелёным;
- sqlc regeneration стабильна, `git diff --check` не находит ошибок.

## 10. Критерии готовности

- Admin загружает одну поддерживаемую книгу и получает durable import resource.
- Operator не может загружать, но читает list/detail.
- UI может показать очередь и polling состояния без дополнительных backend
  запросов на каждую строку списка.
- Все согласованные HTTP codes и problems покрыты тестами.
- Ни один ответ или log не раскрывает secret, PII или internal path/error.
- Worker, retry, cancel и rows API остаются явно отложенными следующими этапами.
