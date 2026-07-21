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

For a self-contained prod run, use [docker-compose.prod.yml](docker-compose.prod.yml):

```sh
cp .env.example .env          # then edit — see "Secrets" below
docker compose -f docker-compose.prod.yml up -d
```

### Secrets

All secrets live in a **gitignored `.env`** next to the compose file — never
committed. The committed `docker-compose.yml` is dev-only and carries an
intentionally insecure key; production reads everything from `.env`.

- `FP_ENCRYPTION_KEY` — derives the AES-256 key that encrypts stored OAuth
  tokens / credentials. **Required in prod**; set it to a long random string and
  keep it stable (rotating it makes existing stored credentials undecryptable).
- `FP_ADMIN_PASSPHRASE` — bootstrap admin passphrase, hashed (argon2id) into the
  DB on first run. Remove it from `.env` afterwards.
- `FP_MS_CLIENT_ID` / `FP_MS_CLIENT_SECRET` — Microsoft OAuth app credentials
  (Outlook / To Do / OneDrive). Register the app under a personal/family tenant.
- `FP_BASE_URL` — externally reachable URL; drives absolute links + the OAuth
  redirect URI, so it must match the app registration.

`/login` and `/pair` are rate-limited (10 attempts/min per IP). Outbound feed
fetches (iCal/RSS) refuse loopback, link-local and cloud-metadata addresses;
private LAN ranges stay reachable.
