# Family Planner

A self-hosted family planner: a **kiosk view** for a living-room TV plus a phone-based **admin** to configure it. Dutch by default, i18n-ready. Built in Go.

> Status: **M0** — first vertical slice (passphrase login, kiosk device pairing, a live countdown + clock on a themed grid over SSE, Dutch UI, demo seed, Docker/CI). See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Stack

Go · chi · templ · SSE · modernc SQLite · sqlc · goose · scs · argon2id · go-i18n

## Local development

Prerequisites: Go 1.26+, and the codegen tools:

```sh
go install github.com/a-h/templ/cmd/templ@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

Then (using [Task](https://taskfile.dev)):

```sh
cp .env.example .env      # adjust FP_ADMIN_PASSPHRASE etc.
task gen                  # generate templ + sqlc code
task run                  # http://localhost:8080  (admin at /admin, kiosk at /kiosk)
task test:norace          # tests (use `task test` where a C compiler is available)
```

First run seeds a demo view and, if `FP_ADMIN_PASSPHRASE` is set, the admin passphrase.

## Deploy

CI builds a Docker image and pushes it to `ghcr.io/jvmeir/familyplanner`. On the
server, a pull-based updater (Watchtower) swaps in new images. The app is exposed
only over Tailscale.
