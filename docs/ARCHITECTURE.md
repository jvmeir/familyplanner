# Family Planner — Architecture

## Goals

- **Kiosk is dumb and resilient.** It renders pre-fetched, normalized data from
  our server, never calling Outlook/Google directly. If a source is down, it
  shows last-good data.
- **Reusable layers.** Credentialed **DataSources** → configured **Widgets** →
  placed in a **View**'s layout. Define once, reuse everywhere.
- **Extending = registering.** A new widget or data-source type is a small
  self-contained package + one registry call. No core changes.
- **Tailscale-only**, single passphrase, long sessions. Dutch by default, i18n-ready.

## Components

```
Phone (admin)  ─►  Admin UI (templ)         ┐
TV (kiosk)     ─►  Kiosk UI (templ + SSE)   │  HTTP (chi): sessions, auth, locale
                                            ▼
        Domain: DataSource / Widget / View / Layout
        Widget Registry (types, accepted source types)
        DataSource Registry (types · credential kind · config/credential schema · resource picker)
        Render registry (web)
        Broker: polling + cache + last-good + OAuth token refresh   ◄── goroutines
        Connectors: iCal / MS Graph / Bring / Open-Meteo
        Crypto (AES-GCM at rest, credentials/tokens) · scs sessions
        SQLite (modernc, pure-Go) + sqlc + goose
```

## Data model

- `data_sources` — a reusable, **typed** connection: `type` (ical / bring / ms_graph / …),
  `config_json` (non-secret config), `secret_ciphertext` (credentials/tokens, AES-GCM
  encrypted), `oauth_status`. The **credential type** is a property of the data-source
  *type* (see below), not stored per-row.
- `widget_sources` — M:N link between a widget and its data sources, carrying a
  **per-link filter** (so the same source can be reused with different filters).
- `widgets` — a named, configured instance referencing 0..n data sources.
- `views` — a screen with a **layout** + rotation fields (`in_rotation`,
  `rotation_order`, `dwell_seconds`). Parked views are addressable but not rotated.
- `placements` — a widget placed in a view (with per-placement display overrides).
- `kiosk_devices` — paired displays (token hash + last-seen).
- `settings`, `sessions` (scs).

### View layout (evolving)

- **M0:** a fixed `cols × rows` CSS grid; placements carry `col/row/col_span/row_span`.
- **M1 (planned):** a **recursive split layout** — every view starts as a single
  pane (1×1); a pane can be split horizontally/vertically into weighted children,
  recursively, and panes can be merged back. Stored as a layout tree
  (`views.layout_json`) whose leaves reference widgets; rendered with nested flex
  containers. This replaces fixed grid coordinates with a tiling model.

## Widget extensibility

Each widget type registers a `widget.Type` with: a config schema (drives the
admin form — M1), accepted data-source types, a `Provider` (server-side
fetch+normalize), and a render keyed by type (in the `web` package, to avoid
templ↔widget import cycles). The registry boundary is plugin-ready: it can later
be backed by WASM (Extism) or go-plugin without touching the rest of the app.

## DataSource extensibility & credential types

Data sources mirror the widget registry: each **data-source type** registers a
`datasource.Type` declaring four things —

1. **Config schema** — non-secret fields (e.g. iCal `url`, MS `client_id`). Drives
   the admin form (same schema engine as widgets). Stored in `config_json`.
2. **Credential type** (`CredentialKind`) — a first-class element of every data
   source, one of:
   - `none` — no auth (e.g. iCal feed URL, Open-Meteo).
   - `basic` — username/email + password (e.g. Bring). Credential fields stored
     **encrypted** in `secret_ciphertext`.
   - `oauth2` — authorization-code flow (e.g. Microsoft Graph). The user supplies
     their app's `client_id`/`client_secret`; the **connect flow**
     (`/admin/datasources/{id}/oauth/start` → `…/oauth/callback`) obtains and
     stores the access/refresh tokens (encrypted). The broker refreshes tokens on
     use and persists rotations; widgets only ever receive a fresh access token.
3. **Credential schema** — the secret fields to collect at create time (rendered as
   password inputs), encrypted into `secret_ciphertext`.
4. **Resource picker** (optional) — after authenticating, the configure page
   (`/admin/datasources/{id}/configure`) calls the provider's API to list
   selectable resources (which Outlook calendar, which Bring list, which To-Do
   list); the chosen id is saved into `config_json`.

A widget type declares `AcceptsSources` (the data-source type ids it can use); the
admin only offers compatible sources. This keeps "add a provider" a self-contained
change: register a `datasource.Type` + a connector, with credential handling and
form generation provided by the framework.

## Theming

A theme is a named bag of CSS custom properties (design tokens). Widgets render
against the variables, so reskinning is swapping token values. Cascade:
per-view theme → global default → hard default. Night mode overrides a few tokens.

## Auth

Single passphrase (argon2id) for admin, long-lived scs session (90 days). Kiosk
pairs once and stores a permanent device token (only its SHA-256 hash is
persisted). Tailscale-only; no public ingress.

## Kiosk runtime

**Server-rendered only** (templ + HTMX + SSE) — no SPA, no PWA. The kiosk opens
an SSE stream (`/kiosk/stream`). The server pushes `refresh` (data/tick) and,
with multiple rotation views, `navigate` (advance); the browser re-fetches the
active view fragment (`/kiosk/view/{id}`) and swaps it into `#stage`. The
swapped fragment is `web.KioskBody` = the view **plus** the corner health badge,
so the badge persists across swaps and refreshes with health each tick.

The **kiosk is just a browser** (Chromium/Edge fullscreen) on the Surface Go +
monitor, pointed at `/kiosk`. There is no native kiosk app / no kiosk-agent, so
night-dim/burn-in are in-browser CSS only and there's no automatic power/CEC
control. Pairing is a one-time passphrase entry in the browser (permanent
device cookie). Brief SSE drops are tolerated (the last view stays; the client
reconnects); there is no offline caching (that would need the removed PWA + an
HTTPS origin).

> Note: an API + Go→WASM **SPA** kiosk and a **PWA** (service worker / offline /
> installable) layer were both prototyped and then removed. The project stays
> plain server-rendered.

## Health monitoring

The broker records each data source's auth health (`data_sources.access_expiry`
/ `last_sync` / `last_error` / `health`; `oauth.ClassifyError` detects a dead
refresh token via OAuth `invalid_grant`). The pure `internal/health` package
aggregates four signals — refresh-token dead (red), access expired, failed sync,
stale data (amber) — into a severity-ranked summary. It's read-only and never
calls external services. Shown as a subtle corner badge on every kiosk screen
(hidden when healthy) and a status pill on the admin data-sources page.

## Deployment

Multi-stage Docker → distroless static (~15 MB, CGO off). CI (`build.yml`) runs
generate + `git diff` staleness check + vet + race tests, and only then builds &
pushes the image to GHCR. Watchtower pulls it on the server. Tailscale Serve
terminates HTTPS on the `.ts.net` name so OAuth redirects work without public
exposure. SQLite + cached files live on a mounted `/data` volume.

## Milestones

- **M0** — skeleton + login + kiosk pairing + live countdown/clock grid + Dutch
  i18n + themes + demo seed + Docker/CI. ✅
- ~~**M0.5** — kiosk-agent~~ — **dropped.** The kiosk is just a fullscreen
  browser; no supervisor binary.
- **M1** — domain CRUD, **recursive split/merge layout editor**, schema-driven
  widget forms, rotation engine, HTMX. ✅
- **M2** — iCal / Open-Meteo / countdown / quote connectors + broker caching. ✅
- **M3** — OAuth: MS Graph (Outlook/To Do) + OneDrive; Bring; per-link resource
  pickers. ✅
- **Health** — OAuth token/expiry + sync monitoring, kiosk badge + admin pill. ✅
- **M4** — polish + fun widgets. **M5** — voice (Dutch voice clock, in design).

*(Explored and reverted: an API+SPA Go→WASM kiosk and a PWA offline layer.)*
