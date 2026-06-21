# Выбор техстека Mintrud Admin

## Решение

Для Mintrud Admin выбираем **Go-first modular monolith**.

Базовый стек:

- язык и runtime: **Go**;
- HTTP: стандартный `net/http` + router `chi`;
- UI rendering: `templ`;
- интерактивность: HTMX, без SPA;
- UI kit: Tabler поверх Bootstrap;
- БД MVP: SQLite на локальном диске Windows Server;
- SQL-доступ: `database/sql` + `sqlc`;
- миграции: `goose`;
- SQLite driver: сначала проверить `modernc.org/sqlite`, fallback - `mattn/go-sqlite3`;
- sessions: `alexedwards/scs`;
- CSRF: выбрать между `gorilla/csrf` и `nosurf` во время auth-spike;
- validation: явная доменная валидация, `go-playground/validator` только для простых form/DTO checks;
- background jobs: in-process jobs на MVP, без внешней очереди.

## Почему Go

Проект не стартует с нуля. Существующий `mintrud_generator` уже написан на Go и
содержит самые рискованные части:

- генерация XML для Минтруда;
- генерация DOCX-протоколов;
- чтение и формирование XLSX;
- Moodle REST-интеграция;
- Windows/MSI/WiX-задел.

Риск заново переписать документную генерацию и интеграции выше, чем риск того,
что в Go придется аккуратнее собрать CRUD, forms, validation, auth, audit и
template patterns.

## Почему не большой Go-framework

Gin, Echo и Fiber не выбираем как базу, потому что приложение не является
API-first backend для SPA. Нам нужна внутренняя server-rendered админка:

- страницы и формы;
- сессии;
- CSRF;
- upload/download файлов;
- audit log;
- HTMX partial updates;
- строгая структура workflow.

`chi` остается близко к стандартному `net/http`, не диктует архитектуру и дает
достаточно middleware/routing возможностей.

## UI-решение

UI строится как server-rendered интерфейс:

- full page navigation для крупных разделов;
- HTMX для фильтров, modal forms, staging-review actions, inline validation и
  обновления статусов;
- Tabler/Bootstrap для layout, sidebar, tables, forms, badges, modals, empty
  states и responsive behavior;
- без SPA и без кастомной CSS-системы с нуля.

Desktop/laptop остается основным рабочим сценарием. Phone/tablet должны
поддерживаться адаптивно, но не через принудительное ужатие широких таблиц.
Для телефона нужны card/list variants для ключевых экранов.

## Архитектурная форма

Приложение строим как modular monolith с явными внутренними границами:

- `admin` - auth, sessions, operator shell, permissions;
- `people` - слушатели и связи слушателей с работодателями;
- `employers` - работодатели и канонические названия;
- `programs` - группы программ и программы;
- `requests` - заявки, строки импорта, staging-review;
- `protocols` - lifecycle протоколов и участники;
- `documents` - XML, DOCX, XLSX, ZIP generation;
- `moodle` - Moodle adapter и sync logic;
- `audit` - action log и security-relevant events;
- `storage` - DB, migrations, filesystem paths.

Handlers должны быть тонкими. Workflow живет в application services. SQL не
размазываем по handlers: используем `sqlc` queries и небольшие repository
wrappers там, где это реально упрощает код.

Moodle не должен становиться центром доменной модели. Все вызовы Moodle идут
через adapter layer, чтобы в будущем можно было заменить Moodle внутренними LMS
модулями.

## База данных

SQLite допустим для MVP при условиях:

- файл БД лежит на локальном диске Windows Server, не на network share;
- включаем WAL;
- write transactions короткие;
- импорты и генерация сериализованы или явно защищены от конфликтов;
- backup/restore описаны в install/runbook;
- переход на PostgreSQL пересматривается до LMS-scope, тяжелых background jobs,
  нескольких серверов или регулярного write contention.

Схему и SQL пишем без лишней SQLite-специфики. Миграции остаются source of
truth.

## Документы и интеграции

Первый технический spike должен доказать, что старый Go-код можно вынести или
переиспользовать для:

- XML generation;
- DOCX protocol generation;
- XLSX import/export;
- Moodle REST API calls.

Для XML/DOCX/XLSX нужны golden tests. DOCX и XLSX проверяем не бинарным
сравнением, а через нормализованное содержимое распакованных OpenXML-файлов,
где бинарный output нестабилен.

## Security baseline

Security foundation включаем с MVP-скелета:

- login;
- session timeout;
- CSRF protection;
- SameSite cookies;
- secure cookies для production;
- audit log для import/export/protocol changes/document generation/Moodle sync;
- file upload limits и проверки типа файла;
- least-privilege Windows service account в deployment docs.

Полная ролевая модель может быть post-MVP, но код не должен мешать добавить
RBAC позже.

## Deployment

Первый delivery может быть ручной установкой по инструкции и скриптам:

- создать `%ProgramData%\MintrudAdmin`;
- положить executable;
- создать config/data/logs/exports/templates directories;
- применить миграции;
- установить Windows Service или другой согласованный способ запуска;
- выполнить health check.

MSI остается отдельным обязательным milestone после стабилизации service layout,
config paths, migrations, upgrade, backup и rollback.

Release pipeline должен проверяться на Windows: install, upgrade, uninstall,
service restart, firewall/port, filesystem permissions, path handling.
Разработка на macOS подходит для основного dev-loop.

## Spike gate

Перед полноценной реализацией делаем короткий spike.

Критерии успеха:

1. Старую generator-логику можно переиспользовать как packages или internal
   service без переноса старого UI/server кода.
2. `templ + HTMX + Tabler` дают чистый vertical slice: list, form, modal action,
   validation errors, responsive phone layout.
3. `goose + sqlc + SQLite` работают на macOS и в Windows build.
4. Выбран SQLite driver:
   - `modernc.org/sqlite`, если pure-Go сборки надежны;
   - `mattn/go-sqlite3`, если pure-Go driver не проходит compatibility,
     correctness или performance expectations.
5. Первые golden tests подтверждают стабильный XML/DOCX/XLSX output на реальных
   fixtures.

Если spike проваливается по generator reuse или maintainability Go UI, fallback
- ASP.NET Core / .NET LTS. В этом fallback старый Go generator сначала
оборачивается как worker/binary через стабильный контракт, а не переписывается
сразу.

## Альтернативы

### ASP.NET Core / C#

Сильный fallback: строгая типизация, хорошая Windows Service/MSI история,
Razor Pages/MVC, OpenXML tooling. Не выбран первым, потому что придется
переносить или оборачивать старую Go-генерацию.

### Django

Быстрый CRUD/forms/admin, но хуже подходит под Windows/MSI и слабее по строгой
типизации. Для этого проекта менее прагматичен из-за существующего Go-кода.

### Kotlin / Java Spring Boot

Технически сильный enterprise-вариант, но добавляет больше moving parts
без достаточного выигрыша над Go или .NET для текущего проекта.
