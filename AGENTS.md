# Agent Notes

## Docker run

Use the compose stack as the canonical app run:

```bash
cp .env.example .env
# Edit .env locally and set MINTRUD_ADMIN_BOOTSTRAP_PASSWORD.
# Keep MINTRUD_ADMIN_PLAINTEXT_CSRF=true for local plain HTTP.
DOCKER_BUILDKIT=1 docker compose up --build -d
```

The app is available at <http://localhost:8081/login>. Compose maps host
`8081` to container `8080`; do not document `localhost:8080` as the compose
URL unless the port mapping changes.

Useful checks:

```bash
docker compose ps
docker compose logs --tail=80 app
curl -sS -o /dev/null -w '%{http_code}\n' http://localhost:8081/login
```

`docker compose down` stops the app and keeps the `mintrud_data` SQLite
volume. `docker compose down -v` wipes the local database.

## Secrets and public git hygiene

Never print `.env` values in chat or logs. It is safe to print only variable
names when auditing local configuration.

Before saying the repo is ready for public git, check both tracked files and
untracked files that could be added:

```bash
git status --short --ignored
git ls-files -z | xargs -0 rg -l --hidden --no-ignore -i \
  '(password|secret|token|api[_-]?key|private[_-]?key|BEGIN .*PRIVATE KEY|ИНН|СНИЛС|паспорт|email|@)'
rg -l --hidden --no-ignore -i \
  --glob '!.env' --glob '!.env.*' --glob '!*.db' --glob '!*.sqlite' --glob '!*.sqlite3' \
  '(password|secret|token|api[_-]?key|private[_-]?key|BEGIN .*PRIVATE KEY|ИНН|СНИЛС|паспорт|email|@)' .
```

Expected false positives include test passwords, CSRF token code, generated
sqlc models, and synthetic fixture data. Inspect matching files locally and
summarize paths/categories, not secret values. Treat realistic employee records,
real corporate email domains, production credentials, local DB files, and
filled `.env` files as blockers for public publication.

Fixture and seed data must be obviously synthetic: use reserved domains such
as `.example` / `example.test`, test names, fake organizations, and no real
customer or employee records.
