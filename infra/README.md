# Local Docker environment

This directory contains the Docker stack used by developers. It is not part of
the client delivery: release artifacts are built through GitHub Actions and
packaged as MSI separately.

## First run

Run all Compose commands from this directory:

```bash
cd infra
cp .env.example .env
# Set MINTRUD_ADMIN_BOOTSTRAP_PASSWORD in .env.
# Keep MINTRUD_ADMIN_PLAINTEXT_CSRF=true for local plain HTTP.
DOCKER_BUILDKIT=1 docker compose up --build -d
```

The application is available at <http://localhost:8081/login>. Compose maps
host port `8081` to container port `8080`.

## Useful commands

```bash
docker compose ps
docker compose logs --tail=80 app
curl -sS -o /dev/null -w '%{http_code}\n' http://localhost:8081/login
docker compose down
```

`docker compose down` stops the application and keeps the `mintrud_data`
SQLite volume. Use `docker compose down -v` only when the local database must
be erased.

## Layout

- `compose.yaml` defines the local application service and persistent volume.
- `Dockerfile` builds the React frontend and Go backend into one runtime image.
- `Dockerfile.dockerignore` filters the repository-root build context.
- `.env.example` is the tracked local configuration template; `.env` is local
  and must never be committed.

The build context intentionally remains the repository root (`..`) because the
image needs both `backend/` and `frontend/`. Docker automatically applies
`Dockerfile.dockerignore` located next to `Dockerfile`.
