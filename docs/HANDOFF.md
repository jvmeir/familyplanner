# Family Planner — Handoff / Continue-Here

A self-hosted **family planner**: a read-only **kiosk** for a living-room TV plus a phone-based **admin** to configure it. Dutch by default, i18n-ready. Built in **Go**. This doc is the single place to pick the project back up on a new machine.

> Status (2026-06): **M0–M3 built, green, and running on the dev VM.** Kiosk + admin + layouts + playlists + devices + a full widget/datasource framework with OAuth. Not yet on a production host.

---

## 1. Quick start (local dev)

Prereqs: **Go 1.26+**, and the codegen CLIs:

```sh
go install github.com/a-h/templ/cmd/templ@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
# optional: Task runner (https://taskfile.dev) and air (live reload)
```

Then:

```sh
cp .env.example .env        # set FP_ADMIN_PASSPHRASE, FP_ENCRYPTION_KEY, FP_MS_CLIENT_ID/SECRET…
task gen                    # templ generate + sqlc generate (generated code IS committed)
task run                    # http://localhost:8080  (admin /admin, kiosk /kiosk)
task test:norace            # tests (use `task test` where a C compiler is available for -race)
```

First run seeds a demo view + default playlist and (if `FP_ADMIN_PASSPHRASE` set) the admin passphrase.

**No Task?** equivalents: `templ generate && sqlc generate`, `go run ./cmd/server`, `go test ./...`.

> Note: `go test -race` needs a C compiler (cgo). On a bare Windows box without gcc, use `go test` (no `-race`); CI runs `-race` on Linux.

---

## 2. Architecture (see also `docs/ARCHITECTURE.md`)

Three reusable layers: **DataSource** (typed, credentialed connection) → **Widget** (configured instance, links 0..n data sources) → **Placement** in a **View**'s recursive **layout**; **Playlists** rotate views; **Devices** (kiosks) get an assigned playlist.

- **Kiosk is dumb & resilient**: renders from a server-side cache, never calls external APIs directly; shows last-good data on failure. **Read-only** for now (no check-off) until touch/voice.
- **Broker** (`internal/broker`): goroutine refreshes each widget's `widget_cache` on its TTL; refreshes/persists OAuth tokens; hands widgets a fresh access token.
- **Registries**: `widget.Registry` (widget types; each declares config `Schema`, `AcceptsSources`, a `Provider`, a `Decode`) and `datasource.Registry` (data-source types; each declares config/credential `Schema`, a `CredentialKind`, and an optional resource picker). Render formatters live in `internal/web` (keyed by type) to avoid templ↔widget cycles.

### Package map
```
cmd/server            main (config, db, registries, server, broker)
internal/config       FP_* env config (incl. app OAuth creds)
internal/db           store, migrations (goose), sqlc queries+generated (dbgen)
internal/crypto       AES-256-GCM seal/open (credentials at rest)
internal/auth         argon2id passphrase, device tokens
internal/i18n         go-i18n; locales/active.nl.json (Dutch default)
internal/theme        design-token themes + cascade
internal/layout       recursive split-tree model + ops (SplitLeaf/Remove/SetWidget/SetWeight)
internal/rotation     per-device playlist playback (State + Manager)
internal/oauth        x/oauth2 provider configs (ms_graph, onedrive, ms_todo, google_photos) + token refresh
internal/broker       cache refresh + OAuth token refresh
internal/widget       widget types + connectors (countdown, clock, calendar/iCal+Graph,
                      weather/Open-Meteo, quote, web, shopping/Bring, photos/OneDrive+GooglePhotos,
                      todolist/MS To Do). Connectors have overridable base URLs + mocked tests.
internal/server       chi routes, auth/session/CSRF mw, SSE kiosk, admin CRUD, OAuth flow
internal/web           templ views + render formatters + assets (app.css, htmx.min.js, kiosk.js, layouteditor.js)
```

---

## 3. Widgets & data sources

**Widgets:** countdown, clock, calendar (Agenda — modes agenda/dagen-lijst/dagen-tabel/week/maand; per-source filter; RRULE expansion), weather (Open-Meteo, no key), quote, web (iframe), shopping (Bring), photos (single/random/by-date), todolist (MS To Do).

**Data-source types & credential kinds:**

| type | credential | resource picker |
|---|---|---|
| `ical` | none (url / webcal://) | — |
| `bring` | basic (email/password, encrypted) | which list |
| `ms_graph` (Outlook calendar) | oauth2 | which calendar |
| `onedrive` (photos) | oauth2 | which folder |
| `ms_todo` | oauth2 | which list |
| `google_photos` | oauth2 | which album *(see gotchas)* |

**Resource selection is per widget→source link** (`widget_sources.resource`), with a live picker on the widget's edit page — so one data source (e.g. one Bring account, one Outlook connection) is reused across widgets that each show a different list/calendar/folder. The datasource-level pick (configure page) acts as a default.

---

## 4. OAuth model (important)

- **App client id/secret are app-level config**, one app per provider:
  - Microsoft (covers Outlook calendar + To Do + OneDrive): `FP_MS_CLIENT_ID`, `FP_MS_CLIENT_SECRET`
  - Google (Google Photos): `FP_GOOGLE_CLIENT_ID`, `FP_GOOGLE_CLIENT_SECRET`
- Creating an OAuth datasource = **interactive sign-in** (`/admin/datasources/{id}/oauth/start` → `…/oauth/callback`); only the user's **token** is stored (encrypted). Broker auto-refreshes + persists rotations.
- **Microsoft app registration** already exists (tenant `jeancloud365.onmicrosoft.com`, app "FamilyPlanner", audience = work + personal MS accounts, delegated scopes `Calendars.Read`/`Tasks.Read`/`Files.Read`/`offline_access`). The **client_id** is in your local notes/`.env`; the **client secret** is not stored in this repo — rotate/retrieve via:
  ```sh
  az ad app credential reset --id <FP_MS_CLIENT_ID>
  ```
- **Redirect URI** currently registered: `http://localhost:8080/admin/datasources/oauth/callback`. So do OAuth connects **locally**. For the kiosk/Tailscale, add the `https://<host>.ts.net/...callback` redirect (`az ad app update`) and set `FP_BASE_URL` accordingly.

---

## 5. Config (env vars, all `FP_*`)

See `.env.example`. Key ones: `FP_ENV`, `FP_ADDR`, `FP_BASE_URL`, `FP_DATA_DIR`, `FP_ENCRYPTION_KEY` (derives the AES key; **required in prod**), `FP_ADMIN_PASSPHRASE` (bootstrap), `FP_LOCALE`, `FP_TIMEZONE` (Europe/Brussels), `FP_SESSION_DAYS`, and the OAuth app creds above.

---

## 6. Deploy

- **Dockerfile** = multi-stage → distroless static (~15 MB, CGO off). `docker-compose.yml` runs it with `/data` volume.
- **CI** (`.github/workflows/build.yml`): on push/PR runs templ+sqlc generate, `git diff` staleness check, vet, `-race` tests; on `main` builds + pushes `ghcr.io/jvmeir/familyplanner:{latest,sha}`. Pull-based redeploy (Watchtower) intended for the production host (Hetzner, Tailscale-only).
- **Dev VM** (LAN, for running containers): details + the SSH/SMB workflow are in local notes (not committed — contains credentials). Pattern: edit locally → copy to the VM → `docker compose up -d --build` over SSH. The deploy reuses the tarball-over-SCP trick (recursive SCP was flaky against that sshd).

---

## 7. What's done vs. pending

**Done & deployed:** M0 (skeleton/auth/kiosk/SSE), M1 (CRUD, recursive split/merge layout editor w/ drag-resize, playlists, devices, phone remote, HTMX), M2 (broker+cache, calendar/weather/quote/web widgets), M3 (OAuth framework + Outlook calendar + OneDrive photos + MS To Do; Bring shopping; per-link resource pickers; app-level OAuth creds).

**Pending / next:**
- **Git/CI**: first push to `github.com/jvmeir/familyplanner` (this commit) → CI/GHCR activates.
- **Tailnet HTTPS redirect** for OAuth on the wall (+ `FP_BASE_URL`); production Hetzner deploy.
- **M0.5 kiosk-agent**: a small Go binary on the Surface (cage + Chromium `--kiosk`, systemd, night dimming/power, heartbeat) — built for linux/amd64.
- **M5 voice**; kiosk write-back interactions (deferred — kiosk read-only).
- Google Photos: the Library API readonly was restricted (Mar 2025); the code exists but OneDrive is the chosen photo source.

## 8. Gotchas
- **Generated code is committed** (templ `*_templ.go`, sqlc `dbgen`); CI fails if stale — run `task gen` after editing `.templ`/`.sql` and commit.
- **`go test -race`** needs gcc (use plain `go test` locally on Windows).
- **OAuth only works where the redirect resolves** (localhost now) — connect locally, not against the LAN VM.
- **Container DNS**: if external widgets time out in a container, add `dns: [1.1.1.1, 8.8.8.8]` to the compose service.
- **Kiosk is read-only**; widgets display only (no check-off yet).
