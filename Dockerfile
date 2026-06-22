# --- Builder ---------------------------------------------------------------
# Pinned to the same Go version declared in go.mod so the toolchain in the
# image cannot drift from what developers use locally. alpine keeps the
# builder small; git is required for `go mod download` to fetch modules.
FROM golang:1.26.4-alpine AS builder

# sqlc and templ versions must match the Makefile pins. We install them
# with `go install` so the binary lands in GOBIN (default $GOPATH/bin)
# and is on PATH for the subsequent generate steps. The runtime image
# does not need either tool — the generated sources are baked into the
# binary.
ARG SQLC_VERSION=v1.30.0
ARG TEMPL_VERSION=v0.3.1020
RUN apk add --no-cache git ca-certificates \
    && go install github.com/sqlc-dev/sqlc/cmd/sqlc@${SQLC_VERSION} \
    && go install github.com/a-h/templ/cmd/templ@${TEMPL_VERSION}

WORKDIR /src

# Copy module manifests first and download deps as a separate layer.
# Subsequent source edits don't bust the module cache.
COPY go.mod go.sum ./
RUN go mod download

# Now copy the rest of the source and run the two code generators before
# building. sqlc reads migrations/*.sql + backend/storage/queries/*.sql
# and writes backend/storage/db/*.go; templ reads *.templ files and
# writes *_templ.go next to them. Both must be re-run on every build so
# a stale commit cannot ship out-of-sync generated code.
COPY . .
RUN sqlc generate \
    && templ generate \
    && CGO_ENABLED=0 GOOS=linux go build \
        -ldflags="-s -w" \
        -trimpath \
        -o /out/mintrud-admin \
        ./cmd/mintrud-admin

# --- Runtime ---------------------------------------------------------------
# alpine carries busybox + wget (used for HEALTHCHECK) and a normal user
# database. The Go binary is statically linked (CGO_ENABLED=0 above), so
# we don't need glibc — only the dynamic loader, which alpine provides.
# Image size is ~15 MB on top of the binary.
FROM alpine:3.20 AS runtime

# ca-certificates for any outbound HTTPS calls (currently none, but
# cheap insurance). wget is pre-installed on alpine for HEALTHCHECK.
# tini is the standard PID-1 reaper: it forwards SIGTERM to the Go
# process so container stop drains in-flight requests instead of
# killing mid-handler. The Go binary handles SIGTERM itself, so
# tini's job is mostly signal-forwarding + zombie reaping.
RUN apk add --no-cache ca-certificates tini \
    && addgroup -S mintrud && adduser -S -G mintrud mintrud

WORKDIR /app

# SQLite lives at /app/data/mintrud-admin.db. The named volume mounts
# /app/data, so the file persists across `docker compose down` /
# `up` cycles. /app itself is owned by root; the data subdirectory
# is created writable by mintrud:mintrud at startup (mkdir -m 0755).
COPY --from=builder --chown=mintrud:mintrud /out/mintrud-admin /app/mintrud-admin
RUN mkdir -p /app/data && chown -R mintrud:mintrud /app

USER mintrud:mintrud

# Listen on 8080 (default for the app). Exposed for documentation and
# for compose's port mapping; docker-compose.yml still publishes it
# explicitly via `ports:`.
EXPOSE 8080

# /login is the only always-public route, so a HEAD-equivalent spider
# is a strong "app is up + DB migrated + handlers wired" probe without
# needing a dedicated /healthz endpoint. wget exits 0 on HTTP 2xx/3xx
# and non-zero otherwise, which is exactly what HEALTHCHECK wants.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q --spider http://127.0.0.1:8080/login || exit 1

ENTRYPOINT ["/sbin/tini", "--", "/app/mintrud-admin"]