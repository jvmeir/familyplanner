// Orchestrator for the synthetic UI tests: start a fresh server on a throwaway
// SQLite DB, wait for health, run the Playwright suite against it, then tear the
// server down and propagate the test exit code.
//
// Assumes `bin/familyplanner` was built with the WASM client embedded
// (`task build`, which the `test:ui` task depends on). Chrome or Edge must be
// installed (Playwright launches it by channel; override with FP_UI_CHANNEL).
import { spawn } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import { existsSync, mkdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const PORT = process.env.FP_UI_PORT || '8099';
const BASE = `http://localhost:${PORT}`;
const isWin = process.platform === 'win32';

const binCandidates = [
  process.env.FP_UI_BIN,
  join('..', '..', 'bin', isWin ? 'familyplanner.exe' : 'familyplanner'),
  join('..', '..', 'bin', 'familyplanner'),
].filter(Boolean);
const BIN = binCandidates.find((p) => existsSync(p));
if (!BIN) {
  console.error('server binary not found; run `task build` first. Looked at:', binCandidates);
  process.exit(2);
}

const DATA = join(tmpdir(), 'fp-ui-data');
rmSync(DATA, { recursive: true, force: true });
mkdirSync(DATA, { recursive: true });

const srv = spawn(BIN, [], {
  env: {
    ...process.env,
    FP_ADDR: ':' + PORT,
    FP_DATA_DIR: DATA,
    FP_ADMIN_PASSPHRASE: process.env.FP_UI_PASSPHRASE || 'secret',
    FP_ENCRYPTION_KEY: 'ui-test-key-0123456789',
  },
  stdio: 'inherit',
});

function shutdown(code) {
  try { srv.kill(); } catch {}
  process.exit(code);
}

let up = false;
for (let i = 0; i < 60; i++) {
  try {
    const r = await fetch(BASE + '/health');
    if (r.ok) { up = true; break; }
  } catch {}
  await sleep(200);
}
if (!up) {
  console.error('server did not become healthy at', BASE);
  shutdown(1);
}

const tests = spawn(process.execPath, ['--test', 'kiosk.test.mjs'], {
  env: { ...process.env, FP_UI_BASE: BASE },
  stdio: 'inherit',
});
tests.on('exit', (code) => shutdown(code ?? 1));
