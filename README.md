# Mintrud Admin

Внутренняя админка ИКЦ Эксперт для замены XLSX как источника истины по
Минтруд-процессу.

## Текущий slice

Реализован первый технический slice:

- SQLite schema и goose migrations;
- Go server на `net/http` + `chi`;
- server-rendered UI на `templ`;
- ручные формы для групп программ, работодателей и слушателей;
- базовое назначение слушателя работодателю;
- `action_log` для ручных изменений.

Пока не входят: заявки, протоколы, XLSX, XML/DOCX, Moodle, auth/RBAC.

## Локальный запуск

```bash
make schema-test
make sqlc
make templ
make test
make run
```

По умолчанию приложение слушает `:8080` и создает SQLite DB в
`data/mintrud-admin.db`.

Переопределение:

```bash
MINTRUD_ADMIN_ADDR=:8090 MINTRUD_ADMIN_DB=/tmp/mintrud-admin.db make run
```

## Проверки

```bash
sh tests/run_schema_tests.sh
go test ./...
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate
```

## Запуск в Docker

Один сервис `app`, SQLite лежит в named volume `mintrud_data`. Параметры
читаются из `.env` (шаблон — `.env.example`).

```bash
cp .env.example .env
# отредактируйте MINTRUD_ADMIN_BOOTSTRAP_PASSWORD в .env
# для локального HTTP-деплоя оставьте MINTRUD_ADMIN_PLAINTEXT_CSRF=true
DOCKER_BUILDKIT=1 docker compose up --build
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

После старта приложение доступно на <http://localhost:8080/login> (порт
хоста задаётся в `docker-compose.yml`; в репо `8081`, потому что 8080
часто занят самим Docker Desktop). Данные переживают `docker compose
down` (volume сохраняется); полная очистка — `docker compose down -v`.
Логи: `docker compose logs -f app`. Healthcheck через `docker compose ps`
показывает `healthy` после ~10 секунд.

Multi-stage `Dockerfile`: builder на `golang:1.26.4-alpine` с
sqlc/templ для перегенерации, runtime на `alpine:3.20` с `tini` (PID-1
reaper). Бинарь собирается статически (`CGO_ENABLED=0`), поэтому в
runtime нет зависимостей кроме libc + ca-certificates.
