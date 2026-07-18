# Synthetic UI tests

Playwright-driven browser tests for the kiosk **SPA** (Go→WASM client) and its
**PWA** behaviour (service worker + offline rendering).

## Run

```sh
task test:ui        # builds the server (with the WASM client) and runs the suite
```

or manually:

```sh
task build          # produces bin/familyplanner with app.wasm embedded
cd test/ui
npm install
node run.mjs
```

`run.mjs` starts a fresh server on a throwaway SQLite DB (port 8099), waits for
`/health`, runs `spa.test.mjs` (Node's built-in test runner), then stops the
server and propagates the exit code.

## Requirements

- **Chrome or Edge installed.** Playwright launches it by channel; override with
  `FP_UI_CHANNEL=msedge`. No browser download is needed (uses `playwright-core`).
- The service-worker / offline tests need a **secure context**, which `localhost`
  satisfies — so they only pass against `http://localhost`, not a plain-LAN IP.

## Env overrides

| var | default | meaning |
|---|---|---|
| `FP_UI_PORT` | `8099` | server port |
| `FP_UI_CHANNEL` | `chrome` | Playwright browser channel (`chrome`/`msedge`) |
| `FP_UI_PASSPHRASE` | `secret` | admin/pair passphrase |
| `FP_UI_BIN` | `../../bin/familyplanner[.exe]` | server binary to launch |
| `FP_UI_BASE` | `http://localhost:8099` | (set by `run.mjs`) base URL the tests hit |
