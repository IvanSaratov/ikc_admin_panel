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
