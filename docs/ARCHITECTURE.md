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
Phone (admin)  ─►  Admin UI (templ + HTMX)  ┐
TV (kiosk)     ─►  Kiosk UI (templ + SSE)   │  HTTP (chi): sessions, auth, CSRF, locale,
                                            │             login/pair rate-limit
                                            ▼
        Domain: DataSource / Widget / View / Layout / Playlist
        Widget Registry (types, accepted source types)
        DataSource Registry (types · credential kind · config/credential schema · resource picker)
        Render registry (web)
        Broker: cache + last-good + OAuth token refresh + per-source health   ◄── goroutines
                (refresh cadence: on-show + configurable interval, TTL-capped)
        Connectors: iCal / RSS / text / MS Graph / Bring / Open-Meteo / YouTube
                (shared HTTP client with an SSRF guard on the dialer)
        Crypto (AES-GCM at rest, credentials/tokens) · scs sessions
        SQLite (modernc, pure-Go) + sqlc + goose
```

## Data model

- `data_sources` — a reusable, **typed** connection: `type` (ical / rss / text / bring /
  ms_graph / onedrive / ms_todo / video / …), `config_json` (non-secret config),
  `secret_ciphertext` (credentials/tokens, AES-GCM encrypted), `oauth_status`, health
  columns (`access_expiry` / `last_sync` / `last_error` / `health`), and
  `refresh_interval_secs` (0 = use the global default; see *Data freshness*). The
  **credential type** is a property of the data-source *type* (see below), not per-row.
- `widget_sources` — M:N link between a widget and its data sources, carrying the
  **per-link** `resource` / `filter` / `color` (so one source is reused with different
  slices/looks).
- `widgets` — a named, configured instance referencing 0..n data sources.
- `widget_cache` — the broker's last-good normalized data per widget (`data_json`,
  `expires_at`, `status`, `error_msg`). Wiped on a cache-schema-version change (below).
- `views` — a screen with a **layout** (`layout_json`) + `render_mode` (grid /
  random_single) + `advance_mode` (timer / on_end) + rotation fields. Parked views are
  addressable but not rotated.
- `playlists` / `playlist_items` — an ordered list of views with a default interval and
  a **per-item interval override** (`dwell_seconds`, 0 = default); a playlist can also
  carry a corner **PiP** video (`pip_widget_id` + `pip_config_json`).
- `placements` — a widget placed in a legacy grid view (per-placement overrides).
- `kiosk_devices` — paired displays (token hash + last-seen + assigned playlist).
- `settings`, `sessions` (scs).

### Settings scopes (where does a setting live?)

There are exactly **three** places a setting can live. When adding one, pick by
this rule — it keeps the admin UI unambiguous:

1. **Data source** (`data_sources.config_json` / `secret_ciphertext`) — the
   *connection*: how to reach the data and authenticate. Reusable by many widgets
   **unchanged**. Examples: iCal/RSS URL, text lines, OAuth account/token, Bring
   login. Ask: *"Is this about reaching/authenticating the data, identical for
   every widget that uses it?"*
2. **Widget** (`widgets.config_json`) — *presentation & behaviour* of that widget
   instance, independent of which source feeds it. Examples: calendar mode,
   countdown date/precision, ticker order, to-do scope/hide-undated, hide-title,
   title size/align, video mute/loop. Ask: *"Does this change how the widget
   looks/behaves regardless of the source?"*
3. **Relation** (`widget_sources` row: `resource`, `filter`, `color`) — per-pairing
   choices: which *slice* of the source this widget uses, and how it looks **here**.
   Examples: which calendar / list / album / folder (`resource`, incl. the
   `__all__` sentinel), per-source `filter`, per-source `color`. Ask: *"Would this
   differ for another widget using the same source?"*

Consequence (enforced): the **resource picker lives only on the relation**
(`widget_sources.resource`), never on the data source. A data-source type may
declare a `ResourceKind` (so the relation shows a live picker), but data sources
do not store a chosen resource. Providers read the resource **only** from
`SourceInput.Resource`; an empty value means the provider's sensible default
(e.g. primary calendar, default To Do list, drive root).

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
4. **Resource picker** (optional) — a data-source type may declare a `ResourceKind`.
   The picker then lives on the **widget↔source relation** (the widget's edit page),
   calling the provider's API to list selectable resources (which Outlook calendar,
   which To-Do list, which OneDrive **album**, which Bring list). The choice is saved
   per relation in `widget_sources.resource` — never on the data source itself.

A widget type declares `AcceptsSources` (the data-source type ids it can use); the
admin only offers compatible sources. This keeps "add a provider" a self-contained
change: register a `datasource.Type` + a connector, with credential handling and
form generation provided by the framework.

## Data freshness (the broker)

The kiosk read path **never** calls external services — it renders whatever is in
`widget_cache`. Freshness comes from two mechanisms:

- **On-show refresh** — every time a widget is rendered, the server kicks a
  background refresh for it (`bgRefresh`), so a displayed widget is always being
  updated for the next tick. It's throttled per-widget (~10s) so rapid re-renders
  (the 30s in-view refresh, quick next/prev) can't stampede a slow source. The very
  first paint of a never-fetched widget does a bounded (2s) synchronous fetch so it
  isn't blank.
- **Background cadence** — the broker's loop refreshes any expired widget. The cache
  TTL for a **data-backed** widget is driven off the configured **refresh interval**,
  not the provider's own TTL: a global default (`settings.refresh_interval_secs`,
  default **15 min**, set in *Settings*) with an optional **per-data-source override**
  (`data_sources.refresh_interval_secs`; the broker uses the smallest non-zero interval
  among a widget's sources). Purely computed widgets (clock, countdown) keep their own
  short TTL.

On a fetch error the broker keeps the last-good value and marks it stale; it records
per-source **health** (OAuth sources during token refresh; non-OAuth sources from the
fetch outcome).

**Cache-schema versioning.** `widget_cache` stores normalized JSON whose shape can
change between builds. A `cacheSchemaVersion` constant is compared to a stored setting
at startup; on a mismatch the cache is cleared (and re-fetched) so an old build's rows
are never mis-decoded. Bump it whenever a widget's cached `Data` struct changes.

## Theming

A theme is a named bag of CSS custom properties (design tokens). Widgets render
against the variables, so reskinning is swapping token values. Cascade:
per-view theme → global default → hard default. Night mode overrides a few tokens.

## Auth

Single passphrase (argon2id) for admin, long-lived scs session (90 days). Kiosk
pairs once and stores a permanent device token (only its SHA-256 hash is
persisted). Tailscale-only; no public ingress.

## Security

- **Secrets** — credentials and OAuth tokens are AES-256-GCM encrypted at rest
  (`internal/crypto`, key derived from `FP_ENCRYPTION_KEY`). Secrets are never
  committed: the dev `docker-compose.yml` carries an intentionally insecure key,
  while `docker-compose.prod.yml` reads everything from a gitignored `.env`.
- **Brute-force** — `POST /login` and `POST /pair` are rate-limited by a per-IP
  token bucket (10 attempts/min → `429`; `internal/server/ratelimit.go`).
- **SSRF** — user-supplied feed URLs (iCal/RSS) are fetched through a shared HTTP
  client whose dialer `Control` hook inspects the **resolved** IP and refuses
  loopback, link-local (incl. the `169.254.169.254` cloud-metadata endpoint),
  unspecified and multicast targets. Private LAN ranges stay allowed on purpose —
  this runs on a home LAN and users legitimately point widgets at other devices.
- **CSRF** — admin mutations require a CSRF token (session-backed middleware).

## Kiosk runtime

**Server-rendered only** (templ + HTMX + SSE) — no SPA, no PWA. The kiosk opens
an SSE stream (`/kiosk/stream`); the server pushes events and the browser reacts:

- `navigate` — advance to a view; the client fetches the view fragment
  (`/kiosk/view/{id}`) and swaps it into `#stage`. The fragment is `web.KioskBody`
  (view **+** corner health badge), so the badge persists across swaps.
- `dwell` — the countdown/progress duration for the current view.
- `refresh` — periodic (30s) in-view data refresh + ticker/config/name sync.
- `config` / `names` — kiosk scale, ticker speed, date format, transition; view-name map.
- `chime` — voice-clock beat (see below).
- `version` — the server's `bootID`; changes each start, so kiosks **auto-reload**
  after a redeploy.
- `cmd` — a UI-only command pushed by the admin remote (mute / PiP), acted on client-side.

**Rotation** (`internal/rotation`) is per-device: a playlist resolves to items
(view + interval), and a `Manager` tracks each connected device so admin/keyboard
commands (next/prev/pause/resume/goto, and the client-only mute/PiP `cmd`s) reach
its SSE loop. Manual next/prev reset the timer but do **not** pause — only an
explicit pause freezes a screen.

**Picture-in-picture (PiP)** — a playlist can carry a corner YouTube video
(`pip_widget_id` + `pip_config_json`: corner, size, hide/show interval, muted) that
keeps playing while views rotate (`yt.js`, YouTube IFrame API). It can dock left/right
(reflowing the main content) or float in a corner. Controls: keyboard (Shift+arrows),
the footer buttons, and the admin device remote (🔇 mute, 📺 show/hide, ⏪/⏩ prev/next),
the last routed over the `cmd` SSE event.

**Resilience** — the kiosk is just a fullscreen browser, so it self-heals two ways:
an independent inline **watchdog** in the shell (runs even if `kiosk.js` throws)
reloads the page if the SSE heartbeat is silent >150s; and a **nightly reload** at
04:00 local clears long-uptime cruft. Combined with the `version` auto-reload, a bad
deploy or a frozen page recovers without a manual refresh.

**Hardware & non-goals** — the kiosk is a fullscreen Chromium/Edge on the old
Surface Go wired to a monitor, pointed at `/kiosk`. No native kiosk app / no
kiosk-agent, so night-dim/burn-in are in-browser CSS only and there's no
power/CEC control. Brief SSE drops are tolerated (last view stays, client
reconnects); there's no offline caching (would need HTTPS + a service worker).

> Note: an API + Go→WASM **SPA** kiosk and a **PWA** (service worker / offline /
> installable) layer were both prototyped and then removed. The project stays
> plain server-rendered.

## Health monitoring

The broker records each data source's health (`data_sources.access_expiry` /
`last_sync` / `last_error` / `health`). For **OAuth** sources it's set during token
refresh (`oauth.ClassifyError` detects a dead refresh token via `invalid_grant`);
for **non-OAuth** sources (iCal/RSS/text/video/Bring) it's set from the widget's
fetch outcome. The pure `internal/health` package aggregates the signals —
refresh-token dead (red), access expired, failed sync, stale data (amber) — into a
severity-ranked summary. It's read-only and never calls external services. Shown as
a subtle corner badge on every kiosk screen (hidden when healthy) and a status pill
on the admin data-sources page.

## Deployment

Multi-stage Docker → distroless static (~15 MB, CGO off). CI (`build.yml`) runs
generate + `git diff` staleness check + vet + race tests, and only then builds &
pushes the image to GHCR. `docker-compose.prod.yml` pulls that image and takes all
secrets from a gitignored `.env` (a named `/data` volume holds SQLite + cached
files); Watchtower pulls new images on the server. Tailscale Serve terminates HTTPS
on the `.ts.net` name so OAuth redirects work without public exposure. `.gitattributes`
normalizes line endings (LF) so committed generated code stays stable across OSes.

Dev-VM redeploy (LAN) uses a tarball-over-SCP + a remote `fp-deploy.sh` that
**clean-extracts** over the app dir while preserving `data/` and `.env`, then
`docker compose up -d --build`. On startup goose runs pending migrations and the
cache-schema check clears `widget_cache` if the shape changed.

## Milestones

- **M0** — skeleton + login + kiosk pairing + live countdown/clock grid + Dutch
  i18n + themes + demo seed + Docker/CI. ✅
- ~~**M0.5** — kiosk-agent~~ — **dropped.** The kiosk is just a fullscreen
  browser; no supervisor binary.
- **M1** — domain CRUD, **recursive split/merge layout editor**, schema-driven
  widget forms, rotation engine, HTMX. ✅
- **M2** — iCal / Open-Meteo / countdown connectors + broker caching. ✅
- **M3** — OAuth: MS Graph (Outlook/To Do) + OneDrive; Bring; per-link resource
  pickers. ✅
- **Health** — OAuth token/expiry + sync monitoring (now incl. non-OAuth sources),
  kiosk badge + admin pill. ✅
- **Voice clock** — global quarter/half/hour chimes + hourly Dutch announcement
  (NMBS-style, attention chime + configurable rate/auto-lead), server-synced via
  SSE, quiet hours, admin toggle. ✅
- **M4** — corner **PiP** video (dock/interval) + video widget; **weather forecast**
  (hi/lo, N-day, address geocoding); photos **album slideshow** (no-repeat); RSS
  ticker; agenda today-highlight + full-period window; configurable refresh cadence;
  per-item playlist intervals; kiosk **self-healing** (watchdog + nightly reload);
  **security** hardening (SSRF guard, login/pair rate-limit, prod compose). ✅
- **M5** — further voice (spoken commands) + kiosk write-back later.

**Removed (do not reintroduce without a reason):** the *quote of the day* and *web
page* widgets, and the *Google Photos* data source (Library API readonly was
restricted; OneDrive is the photo source). An API+SPA Go→WASM kiosk and a PWA layer
were also prototyped and reverted.

## Global kiosk behaviours (voice clock)

Some behaviours are kiosk-wide, not view content. The **voice clock** is the
first: the SSE loop fires a `chime` event on each quarter-hour boundary (gated by
an `enabled` flag + quiet hours in `settings`), so all screens chime together; the
browser (`voiceclock.js`, loaded by the kiosk shell) plays the configured
Web-Audio sound per beat (separate sounds for :15/:45, :30 and the hour) and, on
the hour, can play an attention chime then speak the Dutch time via
`SpeechSynthesis` (nl-BE) at a configurable rate. The server fires the timer
slightly early so a marking pip / the spoken readout lands on the exact boundary
(`Chime.Lead()` auto-estimates the lead). The pure timing/phrasing/quiet-hour logic
lives in `internal/voiceclock`. Audio needs a user gesture (or Chromium
`--autoplay-policy=no-user-gesture-required`) to start.

*(Explored and reverted: an API+SPA Go→WASM kiosk and a PWA offline layer.)*
