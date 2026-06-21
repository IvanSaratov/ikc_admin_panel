GO_RUN_ENV := GOMODCACHE=/private/tmp/ikc-go-mod GOCACHE=/private/tmp/ikc-go-build

.PHONY: schema-test test sqlc templ run

schema-test:
	sh tests/run_schema_tests.sh

test:
	$(GO_RUN_ENV) go test ./...

sqlc:
	$(GO_RUN_ENV) go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate

templ:
	$(GO_RUN_ENV) go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate

run:
	$(GO_RUN_ENV) go run ./cmd/mintrud-admin
