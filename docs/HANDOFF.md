# Family Planner — Handoff / Continue-Here

A self-hosted **family planner**: a read-only **kiosk** for a living-room TV plus a phone-based **admin** to configure it. Dutch by default, i18n-ready. Built in **Go**. This doc is the single place to pick the project back up on a new machine.

> Status (2026-07): **M0–M4 + health + voice clock built, green, deployed to the dev VM, and pushed to a public GitHub repo.** Kiosk + admin + recursive layouts + playlists + devices + a full widget/datasource framework with OAuth + per-source health, a global voice clock, a **per-device corner PiP playlist** (a second playlist rotated in a corner), weather forecasts, a configurable refresh cadence, and a self-healing kiosk runtime with security hardening (SSRF guard, login/pair rate-limit). Not yet on a production host.

## 0. Final architecture at a glance (read this first)

- **Rendering is server-side only** — templ (typed HTML) + HTMX + SSE. The server renders every page and pushes view swaps; the browser just swaps HTML fragments. An API+SPA (Go→WASM) variant and a PWA (service worker/offline/installable) layer were both prototyped this cycle and then **removed** — the project deliberately stays plain server-rendered. Do not reintroduce them without a reason.
- **The kiosk is just a browser** (Chromium/Edge, fullscreen) on the old Surface Go wired to a big monitor, pointed at `/kiosk`. There is **no native kiosk app and no kiosk-agent** (the earlier M0.5 cage+Chromium supervisor is dropped). Consequence: night-dim/burn-in are in-browser CSS only; no automatic backlight/CEC power control.
- **Health monitoring** is live: the broker records each OAuth source's token health, and a subtle corner **badge** on every kiosk screen (+ an admin status pill) surfaces inactive auth / failed syncs / stale data. See §9.
- **No offline** over plain LAN http (a service worker would need HTTPS/localhost, and there's no PWA anymore). Brief SSE drops are tolerated (the page holds its last view and reconnects). True offline would require the PWA back + the Tailscale HTTPS URL.

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
internal/oauth        x/oauth2 provider configs (ms_graph, onedrive, ms_todo) + token refresh
internal/broker       cache refresh (on-show + configurable interval) + OAuth token refresh + source health
internal/voiceclock   quarter/half/hour chime + Dutch hourly announcement timing/phrasing (pure)
internal/widget       widget types + connectors (countdown, clock, calendar/iCal+Graph,
                      weather/Open-Meteo + geocoding, shopping/Bring, photos/OneDrive albums,
                      todolist/MS To Do, ticker/RSS, video/YouTube-embed). Shared HTTP client has an
                      SSRF guard; connectors have overridable base URLs + mocked tests.
internal/server       chi routes, auth/session/CSRF mw + login/pair rate-limit, SSE kiosk
                      (rotation + PiP + cmd), admin CRUD, OAuth flow, cache-schema invalidation
internal/web           templ views + render formatters + assets (app.css, htmx.min.js, kiosk.js,
                      voiceclock.js, yt.js, layouteditor.js, kiosk-preview.js)
```

---

## 3. Widgets & data sources

**Widgets:** countdown, clock, calendar (modes agenda/dagen-lijst/dagen-tabel/week/maand; per-source filter; RRULE expansion; today-highlight; agenda spans the whole current week + configured look-ahead), weather (Open-Meteo, no key — current conditions **+ N-day forecast** with hi/lo and a condition emoji; location by **place name** (geocoded) or lat/lon), shopping (Bring, category grouping, nl localization), photos (OneDrive **album slideshow**, client-side, no-repeat, seconds-per-photo, optional **date/place caption** — date from `photo.takenDateTime`, place reverse-geocoded from the photo's GPS), todolist (MS To Do, due labels, hide-undated), ticker (RSS), video (one YouTube URL in the widget; compose several as playlist views).

**Data-source types & credential kinds:**

| type | credential | resource picker |
|---|---|---|
| `ical` | none (url / webcal://) | — |
| `rss` | none (feed url) | — |
| `text` | none (inline lines) | — |
| `bring` | basic (email/password, encrypted) | which list |
| `ms_graph` (Outlook calendar) | oauth2 | which calendar |
| `onedrive` (photos) | oauth2 | which **album** |
| `ms_todo` | oauth2 | which list |

(The video widget takes a YouTube URL directly — it is **not** a data source.)

**Resource selection is per widget→source link** (`widget_sources.resource`), with a live picker on the widget's edit page — so one data source (e.g. one Bring account, one Outlook connection) is reused across widgets that each show a different list/calendar/album. Each data source also has a **refresh interval** field (0 = use the global default at `/admin/settings`).

---

## 4. OAuth model (important)

- **App client id/secret are app-level config** — one Microsoft app covers Outlook calendar + To Do + OneDrive: `FP_MS_CLIENT_ID`, `FP_MS_CLIENT_SECRET`. (Google was removed with the Google Photos source.)
- Creating an OAuth datasource = **interactive sign-in** (`/admin/datasources/{id}/oauth/start` → `…/oauth/callback`); only the user's **token** is stored (encrypted). Broker auto-refreshes + persists rotations.
- **Microsoft app registration** already exists (tenant `jeancloud365.onmicrosoft.com`, app "FamilyPlanner", audience = work + personal MS accounts, delegated scopes `Calendars.Read`/`Tasks.Read`/`Files.Read`/`offline_access`). The **client_id** is in your local notes/`.env`; the **client secret** is not stored in this repo — rotate/retrieve via:
  ```sh
  az ad app credential reset --id <FP_MS_CLIENT_ID>
  ```
- **Redirect URI** currently registered: `http://localhost:8080/admin/datasources/oauth/callback`. So do OAuth connects **locally**. For the kiosk/Tailscale, add the `https://<host>.ts.net/...callback` redirect (`az ad app update`) and set `FP_BASE_URL` accordingly.

---

## 5. Config (env vars, all `FP_*`)

See `.env.example`. Key ones: `FP_ENV`, `FP_ADDR`, `FP_BASE_URL`, `FP_DATA_DIR`, `FP_ENCRYPTION_KEY` (derives the AES key; **required in prod**, keep stable), `FP_ADMIN_PASSPHRASE` (bootstrap), `FP_LOCALE`, `FP_TIMEZONE` (Europe/Brussels), `FP_SESSION_DAYS`, and `FP_MS_CLIENT_ID` / `FP_MS_CLIENT_SECRET`. Runtime tunables (global refresh interval, kiosk scale, ticker widget/speed, banner date, transition, theme, voice clock) live in the DB `settings`, edited at `/admin/settings`.

---

## 6. Deploy

- **Dockerfile** = multi-stage → distroless static (~15 MB, CGO off). Dev: `docker-compose.yml` (inline insecure key, bind mount). **Prod: `docker-compose.prod.yml`** pulls the GHCR image and reads all secrets from a gitignored `.env` (named `/data` volume). See README → *Secrets*.
- **CI** (`.github/workflows/build.yml`): on push/PR runs templ+sqlc generate, `git diff` staleness check, vet, `-race` tests; on `main` builds + pushes `ghcr.io/jvmeir/familyplanner:{latest,sha}`. Pull-based redeploy (Watchtower) intended for the production host (Hetzner, Tailscale-only).
- **Dev VM** (LAN, for running containers): SSH/SMB workflow in local notes (not committed — contains credentials). Pattern: edit locally → `tar` (excluding `.git`/`node_modules`) → SCP to the VM → run **`fp-deploy.sh`** (clean-extracts preserving `data/` + `.env`, prunes docker, `compose up -d --build`). Recursive SCP was flaky against that sshd, hence the tarball. On boot goose migrates and the cache-schema check clears `widget_cache` if a widget's data shape changed.

---

## 7. What's done vs. pending

**Done & deployed:** M0 (skeleton/auth/kiosk/SSE), M1 (CRUD, recursive split/merge layout editor w/ drag-resize, playlists, devices, phone remote, HTMX), M2 (broker+cache, calendar/weather/countdown widgets), M3 (OAuth framework + Outlook calendar + OneDrive photos + MS To Do; Bring shopping; per-link resource pickers; app-level OAuth creds), **health monitoring** (§9), **voice clock** (quarter/half/hour + Dutch hourly announcement), and **M4**: **per-device PiP playlist** (a second playlist rotated in a corner; dockable) + single-URL video/ticker widgets; **weather forecast** (hi/lo, N-day, address geocoding); photos **client-side album slideshow** (optional date/place captions); agenda **today-highlight + full-week window**; **configurable refresh cadence** (global + per-source) with on-show refresh; **per-item playlist intervals**; kiosk **self-healing** (SSE-heartbeat watchdog + nightly reload + version auto-reload); **security** (SSRF guard, login/pair rate-limit, prod compose, cache-schema invalidation). Pushed to `github.com/jvmeir/familyplanner`; deployed to the dev VM.

**Removed (do not reintroduce without a reason):** the *quote of the day* and *web page* widgets; the *Google Photos* data source (Library API readonly restricted Mar 2025 — OneDrive is the photo source, so `FP_GOOGLE_*` creds are gone); the *video* data source type (a video widget now carries its YouTube URL directly — compose several clips as playlist views); OneDrive **folder** browsing (albums only, for a predictable slideshow); the old playlist-level PiP (`pip_widget_id` — superseded by the per-device PiP playlist); a yt-dlp downloader (YouTube bot-check — embeds are used instead). Also an API+SPA Go→WASM kiosk and a PWA layer were prototyped and reverted — the kiosk stays plain server-rendered.

**Corner PiP model:** a device has a **primary** playlist (`kiosk_devices.playlist_id`, main stage) and an optional **PiP** playlist (`pip_playlist_id` + `pip_config_json`: corner/size/muted). The SSE loop pushes the PiP playlist's items on a `pip` event; the client rotates them into `.kpip` (fetching `/kiosk/view/{id}?bare=1`), advancing on dwell or on video-end for advance-on-end video views. Assigned on both the pairing page and the admin Devices page. Migrations: `00012` (per-source `refresh_interval_secs`), `00013` (`kiosk_devices.pip_playlist_id` + `pip_config_json`).

**Pending / next:**
- **CI/GHCR**: wire the build workflow to actually push images (Watchtower redeploy) + the production Hetzner deploy.
- **Tailnet HTTPS** (+ `FP_BASE_URL`) so OAuth reconnect works on the wall.
- Kiosk write-back interactions (deferred — kiosk read-only) + spoken voice *commands* (M5).
- Health: no periodic all-source sweep yet (only sources touched by an active widget, plus never-connected via `oauth_status`, are checked).

## 8. Gotchas
- **Generated code is committed** (templ `*_templ.go`, sqlc `dbgen`); CI fails if stale — run `task gen` after editing `.templ`/`.sql` and commit.
- **`go build` does not lint JS.** The kiosk JS is embedded via `embed.FS`, so a syntax/temporal-dead-zone error in `kiosk.js`/`yt.js` compiles fine but can freeze the whole kiosk IIFE at runtime (clock + SSE + rotation all dead). **Smoke-test the kiosk in a real browser after touching JS** (load `/kiosk`, confirm the clock advances, no console errors). The inline **watchdog** now auto-recovers a frozen page in ~2.5 min, but don't rely on it.
- **Bump `cacheSchemaVersion`** (`internal/server/server.go`) whenever a widget's cached `Data` struct changes shape — startup clears `widget_cache` on a mismatch so old rows aren't mis-decoded (and the new data shows immediately after deploy).
- **SSRF guard blocks loopback in prod.** The shared widget HTTP client refuses loopback/link-local; tests that hit `httptest` servers (127.0.0.1) need the `allowLoopback` exemption (set in the widget package's `TestMain`). If a widget must reach `localhost` in prod, it won't — by design.
- **`go test -race`** needs gcc (use plain `go test` locally on Windows).
- **OAuth only works where the redirect resolves** (localhost now) — connect locally, not against the LAN VM.
- **Container DNS**: if external widgets time out in a container, add `dns: [1.1.1.1, 8.8.8.8]` to the compose service.
- **Kiosk is read-only**; widgets display only (no check-off yet).
- **Deploy** uses `fp-deploy.sh` (see §6): it clean-extracts (a plain `tar` over the tree leaves stale files → duplicate-symbol build fails) while **preserving `data/` + `.env`** via `find -delete` (the harness sandbox blocks the `rm` token). Redeploys bump `bootID`, so connected kiosks auto-reload (a PiP/video briefly restarts — expected).
- **Secrets live in `/root/familyplanner/.env`** (gitignored, never committed): `FP_MS_CLIENT_ID` / `FP_MS_CLIENT_SECRET` / `FP_ENCRYPTION_KEY`. Preserve this file across deploys.
- **VM disk fills up** from repeated image builds → `no space left on device`. Reclaim with `docker builder prune -f` + `docker image prune -f` (safe: dangling only).

## 9. Health monitoring
The broker records each data source's health (migration `00007` adds `access_expiry` / `last_sync` / `last_error` / `health` to `data_sources`). For **OAuth** sources it's set during token refresh (`oauth.ClassifyError` maps `invalid_grant` → "refresh token dead → reconnect"); for **non-OAuth** sources (iCal/RSS/text/Bring) it's set from the widget's fetch outcome (ok / error). The pure `internal/health` package aggregates four signals — **refresh-token dead** (red), **access expired**, **failed sync**, **stale data** (>1h) (amber) — ranked by severity. It's read-only (never calls external APIs; reads the state the broker already stored).

Surfaced as a **subtle corner badge** on every kiosk screen (hidden when healthy; rendered via `web.KioskBody` so it persists across SSE view swaps and refreshes each tick) and a **status pill** on the admin Gegevensbronnen page. Note: health is recorded for sources touched by an active widget, plus never-connected sources (via `oauth_status`); there's no periodic all-source sweep yet.
