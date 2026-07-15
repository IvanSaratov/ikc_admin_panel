# Server CLI Refactor Design

Date: 2026-07-15

## Goal

Replace the product-specific `backend/cmd/mintrud-admin` entry point with a
role-based `backend/cmd/server` command, keep the command package small, and use
`github.com/urfave/cli/v3` as the single definition and parsing layer for the
server process CLI and environment variables.

This change is intentionally breaking. It does not preserve old command paths,
flag placement, `MINTRUD_ADMIN_*` variables, or operational artifact names.

## Chosen approach

The application command graph is declared in `backend/cmd/server/main.go` as a
`*cli.Command`. The file owns the public process interface: commands, flags,
defaults, environment sources, help text, and action wiring. It does not own
database preparation, HTTP lifecycle, or application configuration logic.

Executable behavior moves to an importable `backend/internal/runner` package.
That package has no dependency on `urfave/cli`; it accepts typed configuration
and exposes operations used by the CLI actions.

The resulting layout is:

```text
backend/
├── cmd/
│   ├── server/
│   │   ├── main.go
│   │   └── main_test.go
│   └── seed/
└── internal/
    └── runner/
        ├── config.go
        ├── database.go
        ├── logger.go
        ├── serve.go
        └── *_test.go
```

The exact test-file split may follow the production responsibilities, but the
`cmd/server` package must contain only the CLI declaration/wiring and its direct
tests.

## Command graph

There is no default command. Running the executable without a command displays
help and does not start the HTTP server.

```text
<executable>
├── serve
└── db
    ├── status
    ├── migrate
    ├── verify
    └── backup
```

Development commands are:

```bash
go -C backend run ./cmd/server serve
go -C backend run ./cmd/server db status
go -C backend run ./cmd/server db migrate
go -C backend run ./cmd/server db verify
go -C backend run ./cmd/server db backup
```

The root command does not hard-code a product executable name. `urfave/cli`
derives the displayed name from `os.Args[0]`, so release workflows may build
`./cmd/server` under a client-facing MSI executable name without changing the
Go command or subcommands.

## Flags and environment variables

Every server-process configuration variable is represented by an
`urfave/cli/v3` flag whose `Sources` includes the corresponding
`cli.EnvVars(...)` source. A CLI value takes precedence over its environment
source, which takes precedence over the declared default.

Flags live on the narrowest command that uses them. A setting used by both
`serve` and `db` may be declared separately on those commands instead of being
promoted to the root. In particular, `--database` is independently declared on
`serve` and `db`, with the same environment source and default.

| Scope | Flag | Environment variable | Default |
|---|---|---|---|
| root | `--environment` | `IKC_SERVER_ENV` | `dev` |
| root | `--log-level` | `IKC_SERVER_LOG_LEVEL` | `info` |
| root | `--log-format` | `IKC_SERVER_LOG_FORMAT` | environment-dependent: text outside production, JSON in production |
| serve | `--address` | `IKC_SERVER_ADDR` | `:8080` |
| serve | `--database` | `IKC_SERVER_DB` | `data/ikc.db` |
| serve | `--bootstrap-password` | `IKC_SERVER_BOOTSTRAP_PASSWORD` | empty |
| serve | `--frontend` | `IKC_SERVER_FRONTEND` | `embedded` |
| serve | `--session-ttl` | `IKC_SERVER_SESSION_TTL` | `8h` |
| serve | `--cookie-secure` | `IKC_SERVER_COOKIE_SECURE` | production-dependent |
| serve | `--cookie-same-site` | `IKC_SERVER_COOKIE_SAMESITE` | `lax` |
| serve | `--csrf-key` | `IKC_SERVER_CSRF_KEY` | empty; generate an ephemeral key with a warning |
| serve | `--plaintext-csrf` | `IKC_SERVER_PLAINTEXT_CSRF` | `false` |
| serve | `--trusted-origins` | `IKC_SERVER_TRUSTED_ORIGINS` | empty |
| db | `--database` | `IKC_SERVER_DB` | `data/ikc.db` |

All flags, including bootstrap and CSRF secrets, remain visible in generated
help. The documentation continues to recommend environment variables for local
Compose usage. Error messages and logs must never include secret values.

`--cookie-secure` needs an unset state in addition to true and false. When it is
not supplied by either CLI or environment, production environments enable it
and other environments disable it. An explicit CLI or environment value wins.

`--trusted-origins` preserves the current comma-separated environment format
and resolves it into a trimmed list before constructing CSRF middleware.
`--frontend` accepts only `embedded` and `disabled`. Boolean flags use the
boolean syntax accepted by urfave/cli v3; compatibility with the previous
free-form `truthy` parser is not required.

## Configuration flow and package boundaries

```text
CLI arguments / IKC_SERVER_*
             │
             ▼
      urfave/cli flags
             │
             ▼
       runner.Config
       ├── logging
       ├── database
       ├── HTTP/frontend
       ├── bootstrap
       ├── session
       └── CSRF
             │
             ▼
       runner operations
       ├── Serve
       └── RunDatabase
```

`cmd/server/main.go` maps parsed flag values into typed runner configuration.
The runner validates relationships that cannot be expressed by one flag alone,
such as requiring an absolute database path in production and deriving the
cookie security default from the environment.

The `admin` package stops reading process environment variables. Its session
and CSRF constructors accept typed configuration. Environment constants and
environment-loading helpers are removed. The `app` package receives resolved
session and CSRF dependencies/configuration instead of loading them internally.

The seed command is not converted into another server subcommand and does not
share the server CLI contract. It remains separate developer tooling and keeps
its direct logging configuration, renamed to `IKC_SEED_ENV`,
`IKC_SEED_LOG_LEVEL`, and `IKC_SEED_LOG_FORMAT`.

## Runtime and error behavior

`urfave/cli` owns argument parsing, generated help, unknown-command errors, and
flag-source precedence. Runner actions return operational and validation errors
to the command. The process reports them without exposing secrets and exits
nonzero.

Logger creation happens after CLI/environment parsing so logging flags control
the runtime logger. The existing Zap global replacement, standard-library log
redirection, and logger synchronization behavior are retained around command
execution.

The HTTP server retains the current lifecycle guarantees:

- prepare and migrate the database before opening the HTTP listener;
- ensure the bootstrap administrator before serving requests;
- handle interrupt and SIGTERM;
- gracefully shut down admitted requests before releasing database ownership;
- force-close after the existing timeout if graceful shutdown fails.

The SQLite maintenance semantics remain unchanged. Only their CLI declaration,
configuration input, and package location change.

## Technical rename scope

Process and application artifacts are renamed consistently:

- Go command: `backend/cmd/server`;
- local/container binary: `server` and `/app/server`;
- local Docker image: `ikc-server:local`;
- Compose project: `ikc-server`;
- named volume: `ikc_data`;
- container user/group: `ikc`;
- default database: `data/ikc.db`;
- session cookie: `ikc_session`;
- frontend document title: `ИКЦ Эксперт`;
- frontend package: `ikc-expert-frontend`.

README, `infra/README.md`, Dockerfile, Compose, `.env.example`, tests, and
`AGENTS.md` are updated to the new command and names. A filled local
`infra/.env` is not edited, because it may contain secrets; operators must
rename its keys using the updated `.env.example`.

Domain language that actually denotes the Ministry of Labour workflow remains
unchanged, including database fields such as `mintrud_registry_number`, feature
fields such as `requires_mintrud_test`, and user-facing workflow text.

The Go module/repository import path remains unchanged because renaming the
remote repository is outside this task.

## Testing

Implementation follows test-driven development. Tests cover:

- the command graph and the absence of an implicit default action;
- flag scope, including rejection of a serve-only flag by `db` and vice versa;
- CLI-over-environment-over-default precedence;
- every new `IKC_SERVER_*` source and absence of old environment compatibility;
- production database-path validation;
- duration, SameSite, boolean, frontend, trusted-origin, and CSRF validation;
- logger configuration and error redaction;
- existing database status, migration, verification, backup, locking, and file
  identity scenarios after moving them into `internal/runner`;
- bootstrap behavior and graceful HTTP shutdown;
- buildability of `./cmd/server` under an arbitrary output filename;
- removal of obsolete `mintrud-admin` technical references while retaining
  legitimate domain terminology.

The proportional final verification is the backend test suite, frontend tests
affected by package/title renames, schema tests, frontend production build, and
a direct server-binary build.

## References

- urfave/cli v3 getting started: <https://cli.urfave.org/v3/getting-started/>
- urfave/cli v3 value sources: <https://cli.urfave.org/v3/examples/flags/value-sources/>
- urfave/cli v3 API: <https://pkg.go.dev/github.com/urfave/cli/v3>

## Acceptance criteria

The design is complete when all of the following are true:

1. `backend/cmd/mintrud-admin` no longer exists and `backend/cmd/server` contains
   only the CLI declaration/wiring and direct tests.
2. Server startup requires the explicit `serve` subcommand.
3. All server configuration is declared through visible urfave/cli v3 flags
   with `IKC_SERVER_*` environment sources at the narrowest applicable command.
4. No runtime `MINTRUD_ADMIN_*` compatibility aliases remain.
5. `admin`, `app`, and `internal/runner` do not read server configuration from
   the process environment.
6. Database, HTTP lifecycle, and security behavior remains covered and passing.
7. The source command can be packaged under an arbitrary binary filename.
8. Operational documentation and developer tooling describe only the new
   command, variables, ports, and artifact names.
