# Архитектура Mintrud Admin

## Общая форма

Приложение остаётся Go-first modular monolith. HTTP слой принимает request,
handler разбирает HTTP-детали, service выполняет workflow, `sqlc` queries
работают с SQLite, а `audit.Service` записывает действие оператора в
`action_log`.

## Карта пакетов

- `backend/app`: сборка сервера, зависимостей, middleware и маршрутов.
- `backend/admin`: вход, сессии, CSRF, bootstrap admin, login rate limit.
- `backend/audit`: запись и просмотр журнала действий.
- `backend/programs`: группы программ и программы.
- `backend/employers`: работодатели.
- `backend/people`: слушатели и связи слушатель-работодатель.
- `backend/requests`: XLSX upload, staging rows, применение строк.
- `backend/protocols`: протоколы, участники, номера и переходы состояния.
- `backend/documents`: генерация XML/DOCX и изоляция legacy generator.
- `backend/storage`: SQLite, миграции, транзакции и generated sqlc code.
- `backend/platform`: инфраструктурные helpers без бизнес-логики.

## Направление зависимостей

Доменные пакеты могут импортировать `storage`, `audit`, `platform` и свои
`views`. Общие пакеты не импортируют домены. Legacy generator доступен только
через `backend/documents`; остальные пакеты не импортируют
`backend/documents/legacy`.

## Логирование

Runtime logging стандартизирован на `github.com/sirupsen/logrus`. Логировать
можно технические поля: method, path, status, duration, actor, ids и безопасные
hash values. Нельзя логировать значения `.env`, пароли, tokens, содержимое XLSX,
сырые ФИО, СНИЛС, email и исходные имена файлов.

## Проверки

После обычного refactor-этапа:

```bash
make test
make schema-test
```

Если менялись `*.templ`:

```bash
make templ
make test
```

Если менялись SQL queries или migrations:

```bash
make sqlc
make schema-test
make test
```
