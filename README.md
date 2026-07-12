# Mintrud Admin

Внутренняя админка ИКЦ Эксперт для замены XLSX как источника истины по
Минтруд-процессу.

## Текущее состояние

Приложение уже включает рабочий vertical slice внутренней админки:

- SQLite schema и goose migrations;
- Go server на `net/http` + `chi`;
- React SPA, собранный Vite и обслуживаемый Go-сервером;
- auth baseline: login, sessions, CSRF, bootstrap admin, login rate limit;
- ручные формы для групп программ, программ, работодателей и слушателей;
- назначение слушателей работодателям;
- импорт XLSX-заявок в staging rows и применение строк;
- протоколы: создание, фиксация номера, участники и переходы состояния;
- генерация XML/DOCX через изолированный legacy adapter;
- `action_log` для ручных, auth, import, protocol и document events.

Пока не входят: Moodle integration, полная RBAC-модель, production installer/MSI
и расширенная наблюдаемость за пределами application logs.

## Локальный запуск

Прямой `go run` не загружает корневой `.env`, поэтому перед запуском
задайте обязательные переменные в текущей shell-сессии:

```bash
export MINTRUD_ADMIN_BOOTSTRAP_PASSWORD='local-dev-only'
export MINTRUD_ADMIN_PLAINTEXT_CSRF=true
```

```bash
sh tests/run_schema_tests.sh
go -C backend run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
go -C backend test ./...
npm ci --prefix frontend
npm test --prefix frontend
npm run build --prefix frontend
go -C backend run ./cmd/mintrud-admin
```

`backend/` — самостоятельный Go-модуль: в нём находятся `go.mod`, команды,
миграции и конфигурация sqlc. Go-команды из корня репозитория запускаются через
`go -C backend`. Docker по-прежнему использует корень репозитория как build
context, потому что образ включает и backend, и собранный frontend.

Версия sqlc (`v1.30.0`) pinned в этой команде и в `Dockerfile` — держите их
синхронно. sqlc работает с `backend/sqlc.yaml` и миграциями из
`backend/migrations/`. React SPA собирается из `frontend/` через Vite.

По умолчанию приложение слушает `:8080` и создает SQLite DB в
`data/mintrud-admin.db` относительно `backend/`, то есть в
`backend/data/mintrud-admin.db` от корня репозитория.

Переопределение (при условии, что обе переменные из `export` выше остаются
заданы в текущей shell-сессии):

```bash
MINTRUD_ADMIN_ADDR=:8090 MINTRUD_ADMIN_DB=/tmp/mintrud-admin.db go -C backend run ./cmd/mintrud-admin
```

## Запуск в Docker

Один сервис `app`, SQLite лежит в named volume `mintrud_data`. Параметры
читаются из `.env` (шаблон — `.env.example`).

```bash
cp .env.example .env
# отредактируйте MINTRUD_ADMIN_BOOTSTRAP_PASSWORD в .env
# для локального HTTP-деплоя оставьте MINTRUD_ADMIN_PLAINTEXT_CSRF=true
DOCKER_BUILDKIT=1 docker compose up --build -d
```

`DOCKER_BUILDKIT=1` нужен из-за бага Docker Desktop 29.x: legacy-builder
не может сделать manifest HEAD для `golang:*` базовых образов (возвращает
EOF). BuildKit работает штатно. В Docker Desktop 30+ баг починят — флаг
можно будет убрать.

`MINTRUD_ADMIN_PLAINTEXT_CSRF=true` нужен потому что gorilla/csrf v1.7+
отвергает HTTP Referer-заголовки как downgrade-атаку и без этого флага
POST-формы получают 403. За reverse proxy с TLS-терминацией флаг
ставить нельзя — он отключает HTTPS-downgrade защиту; в этом случае
прокси передаёт `https://`-схему в Go-процесс и middleware работает
штатно.

После старта приложение доступно на <http://localhost:8081/login>. Порт
хоста задаётся в `docker-compose.yml`; контейнер слушает `8080`, наружу
в репо опубликован `8081`, потому что `8080` часто занят самим Docker
Desktop.

Полезные команды:

```bash
docker compose ps
docker compose logs -f app
curl -sS -o /dev/null -w '%{http_code}\n' http://localhost:8081/login
docker compose down
docker compose down -v # полная очистка SQLite volume
```

Данные переживают `docker compose down` (volume сохраняется). Healthcheck
через `docker compose ps` показывает `healthy` после ~10 секунд.

Multi-stage `Dockerfile`: frontend-builder на `node:24-alpine` собирает React
SPA, Go builder на `golang:1.26.4-alpine` кэширует зависимости по `backend/go.mod`
и `backend/go.sum`, запускает sqlc внутри вложенного Go-модуля,
используя `backend/sqlc.yaml` и `backend/migrations/`, и компилирует сервер.
Runtime на `alpine:3.20` с
`tini` (PID-1 reaper). Бинарь собирается статически
(`CGO_ENABLED=0`), поэтому в runtime нет зависимостей кроме libc +
ca-certificates.

## Публикация репозитория

Перед открытой публикацией проверьте, что в git не попали локальные
секреты и реальные персональные данные:

```bash
git status --short --ignored
git ls-files -z | xargs -0 rg -l --hidden --no-ignore -i \
  '(password|secret|token|api[_-]?key|private[_-]?key|ИНН|СНИЛС|паспорт|email|@)'
```

`.env` и локальные SQLite-файлы игнорируются через `.gitignore` и
`.dockerignore`; коммитить нужно только `.env.example`. Fixture/seed-данные
должны быть явно синтетическими: используйте домены `.example` или
`example.test`, тестовые ФИО и ненастоящие организации.
