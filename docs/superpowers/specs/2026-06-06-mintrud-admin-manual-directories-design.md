# Дизайн ручных справочников Mintrud Admin

## Цель

Собрать первый рабочий vertical slice Mintrud Admin как server-rendered
операторский интерфейс на Go для ручного управления базовыми справочниками:

- группы программ;
- программы;
- работодатели;
- слушатели;
- связи слушателей с работодателями.

В этот slice намеренно не входят импорт клиентских XLSX, прием заявок, workflow
протоколов, генерация XML/DOCX, синхронизация с Moodle, auth/RBAC и полноценный
UI журнала аудита.

## Зафиксированный Контекст

Стек зафиксирован в `docs/tech-stack.md`:

- Go-first modular monolith;
- `net/http` с `chi`;
- `templ` для server-rendered UI;
- HTMX для точечной интерактивности, без SPA;
- Tabler поверх Bootstrap для операторского UI;
- SQLite как хранилище MVP;
- `database/sql` с `sqlc`;
- миграции через `goose`;
- сначала `modernc.org/sqlite`, fallback на `mattn/go-sqlite3` при необходимости.

В текущем репозитории уже есть первая SQLite-схема и SQL contract tests.
Baseline-команда:

```bash
sh tests/run_schema_tests.sh
```

На момент этого дизайна скрипт проходит.

## Scope Первого Slice

Первый slice - ручной CRUD/forms для базовых данных, которые нужны до реализации
заявок и протоколов. Он должен проверить архитектуру приложения, storage path,
UI shell, flow валидации и обработку DB constraints.

### Входит

- Application scaffold с одним executable: `cmd/mintrud-admin`.
- Embedded migrations и DB bootstrap.
- `sqlc` queries для таблиц первого slice.
- Tabler shell с верхнеуровневой навигацией.
- Список/создание/редактирование/деактивация групп программ.
- Список/создание/редактирование/деактивация программ.
- Список/создание/редактирование работодателей.
- Список/создание/редактирование слушателей.
- Детальная страница слушателя со связями с работодателями.
- Добавление/редактирование/деактивация связи слушатель-работодатель.
- Field-level validation и маппинг duplicate constraints.
- Базовая таблица `action_log` и записи в нее при ручных изменениях.
- Developer runbook и тесты для storage/services/handlers.

### Не Входит

- Hard delete реальных записей.
- Клиентские заявки и import staging.
- Создание и нумерация протоколов.
- Генерация XML, DOCX, XLSX.
- Вызовы Moodle API.
- Login, sessions, CSRF, RBAC.
- Установка production Windows service.
- Обновления wiki.

Формы должны быть устроены так, чтобы позже можно было добавить скрытые CSRF
поля без переписывания всех шаблонов.

## Решения По Схеме До CRUD

Перед реализацией реального CRUD слушателей нужны две правки схемы:

1. `workers.email` должен стать `TEXT NOT NULL`.
   Дублирующиеся email остаются допустимыми.
2. Добавить минимальную таблицу `action_log`, потому что tech stack и roadmap
   требуют audit logging для ручных изменений данных.

Первый slice не исправляет эти более поздние расхождения workflow/import:

- naming и status vocabulary для `request_rows` / `request_training_items`;
- отсутствие `imports` / `import_rows` source metadata;
- поддержку суффиксов протоколов `/1`, `/2`, `/3`;
- status vocabulary протоколов.

Это нужно почистить до реализации импорта заявок или workflow протоколов, пока
migration debt остается дешевым.

## Архитектура

Используем modular monolith с feature-local handlers и templates. Shared UI
components остаются маленькими. SQL-доступ генерируется через `sqlc`; feature
services оборачивают generated queries там, где нужны нормализация, валидация и
error mapping.

Плановая структура:

```text
cmd/mintrud-admin/main.go

backend/app/
  router.go
  server.go
  middleware.go

backend/storage/
  db.go
  migrate.go
  tx.go
  sqlite_errors.go
  queries/
  db/

backend/ui/
  layouts/
  components/

backend/programs/
  handler.go
  service.go
  views/

backend/employers/
  handler.go
  service.go
  views/

backend/people/
  handler.go
  service.go
  views/

backend/audit/
  service.go

web/static/
```

Не кладем CRUD первого slice в общий пакет `admin`. Модуль `admin` резервируется
для auth, sessions, shell-level permissions и последующих операторских concerns.
Пакеты внутри `backend/` считаются внутренними для приложения и не являются
public API, даже без Go-механики `internal`.

## Storage

Оставляем `migrations/001_initial_schema.sql` source of truth и при подключении
goose добавляем только marker `-- +goose Up`. Секцию `Down` пока не добавляем,
потому что текущие plain `sqlite3` schema tests выполнили бы и ее.

Storage bootstrap должен задавать поведение SQLite connection в application
code, а не только в migration SQL:

- foreign keys on;
- WAL для file databases;
- busy timeout;
- консервативные настройки connection pool.

Для Go tests по умолчанию используем temp file databases вместо `:memory:`,
потому что pooled SQLite connections и WAL ведут себя иначе для in-memory DB.

## UI Design

UI - сдержанный операторский интерфейс. Desktop и laptop - основной сценарий.
Mobile должен оставаться usable через card/list variants там, где таблицы
становятся слишком широкими.

Верхнеуровневая навигация:

- Programs;
- Employers;
- Workers.

Группы программ живут внутри раздела Programs. Связи слушатель-работодатель
живут в контекстах Worker detail и Employer detail; это не primary nav item.

Сначала используем full-page list и form flows. HTMX используем только там, где
он дает понятную пользу:

- list filters/search позже;
- inline validation позже;
- status/deactivate actions позже.

Первая реализация может оставить обычные POST/redirect flows, если так slice
получится меньше и проще для проверки.

## Валидация

Валидация явная, в feature services:

- нормализовать ИНН перед employer insert/update;
- нормализовать СНИЛС перед worker insert/update;
- требовать email слушателя;
- требовать положительные `default_hours` программы;
- enforce одну active связь worker-employer на пару worker/employer;
- маппить DB uniqueness и foreign-key failures в field-level form errors.

Дублирующиеся email допустимы.

## Тестирование

Baseline verification:

```bash
sh tests/run_schema_tests.sh
go test ./...
sqlc generate
templ generate
```

Storage tests должны покрывать:

- миграцию с пустой temp file DB;
- включенные foreign keys на app connection;
- включенный WAL для file DB;
- CRUD для first-slice entities;
- uniqueness для нормализованных ИНН и СНИЛС;
- uniqueness для program group/code;
- partial uniqueness для active worker-employer pair.

Handler tests должны покрывать:

- list pages возвращают 200;
- invalid forms возвращают field errors;
- успешный POST делает redirect;
- duplicate INN/SNILS маппится в form errors.

Manual browser smoke должен покрывать:

- открыть app shell;
- создать/отредактировать группу программ;
- создать/отредактировать программу;
- создать/отредактировать работодателя;
- создать/отредактировать слушателя с email;
- назначить слушателя работодателю;
- деактивировать назначение.

## Порядок Реализации

1. Поправить схему под `workers.email NOT NULL` и `action_log`.
2. Добавить Go module и tool configuration.
3. Подключить migrations и storage bootstrap.
4. Добавить `sqlc` queries для first-slice entities.
5. Добавить Tabler/templ shell.
6. Добавить раздел Programs.
7. Добавить раздел Employers.
8. Добавить раздел Workers со связями с работодателями.
9. Добавить tests и runbook.
10. Запустить local dev server и проверить browser smoke.

## Открытые Риски

- `modernc.org/sqlite` нужно проверить на macOS и Windows build paths.
- `templ` и HTMX conventions должны оставаться простыми; избегаем custom
  component sprawl.
- Текущую схему protocol/import нужно пересмотреть до того, как эти slices
  начнут от нее зависеть.
- Auth/CSRF is out of this slice, but form structure should not block it.
