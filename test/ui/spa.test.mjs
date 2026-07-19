// Synthetic browser tests for the kiosk SPA + PWA, using Playwright to drive a
// real Chrome/Edge against a running server (started by run.mjs). Covers:
//   - the WASM client renders the shell + widgets,
//   - the footer "next" control swaps the view through the full SSE loop,
//   - the service worker registers and precaches the shell,
//   - the kiosk still renders offline (PWA resilience).
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { chromium } from 'playwright-core';

const BASE = process.env.FP_UI_BASE || 'http://localhost:8099';
const CHANNEL = process.env.FP_UI_CHANNEL || 'chrome';
const PASSPHRASE = process.env.FP_UI_PASSPHRASE || 'secret';

let browser, ctx, page;

before(async () => {
  browser = await chromium.launch({ channel: CHANNEL, headless: true });
  ctx = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  page = await ctx.newPage();
  // Pair the device (sets the fp_kiosk cookie).
  await page.goto(BASE + '/pair', { waitUntil: 'domcontentloaded' });
  await page.fill('input[name="passphrase"]', PASSPHRASE);
  await Promise.all([page.waitForNavigation(), page.click('button[type="submit"]')]);
});

after(async () => {
  await browser?.close();
});

async function waitForSWControl() {
  await page.evaluate(async () => {
    if (!('serviceWorker' in navigator)) return;
    await navigator.serviceWorker.ready;
    for (let i = 0; i < 60 && !navigator.serviceWorker.controller; i++) {
      await new Promise((r) => setTimeout(r, 100));
    }
  });
}

test('SPA renders the shell and widgets', async () => {
  await page.goto(BASE + '/spa', { waitUntil: 'load' });
  await page.waitForSelector('.view .widget', { timeout: 15000 });
  const s = await page.evaluate(() => ({
    widgets: document.querySelectorAll('.view .widget').length,
    title: document.querySelector('.w-title')?.textContent?.trim(),
    footer: !!document.querySelector('.kfooter'),
    time: document.querySelector('.ktime')?.textContent?.trim() || '',
    jumpOptions: document.querySelectorAll('#kjump option').length,
  }));
  assert.ok(s.widgets >= 1, 'renders at least one widget');
  assert.equal(s.title, 'Kerstmis', 'countdown widget title');
  assert.ok(s.footer, 'footer present');
  assert.match(s.time, /\d{1,2}:\d{2}/, 'live clock rendered');
  assert.ok(s.jumpOptions >= 3, 'jump dropdown populated');
});

test('next control swaps the view via the SSE loop', async () => {
  await page.goto(BASE + '/spa', { waitUntil: 'load' });
  await page.waitForSelector('.kfooter-controls button');
  const labelSel = '.kfooter-label > span:last-child';
  const before = (await page.textContent(labelSel))?.trim();
  await page.click('.kfooter-controls button:nth-child(4)'); // ⏭
  await page.waitForFunction(
    ([sel, prev]) => {
      const el = document.querySelector(sel);
      return el && el.textContent.trim() && el.textContent.trim() !== prev;
    },
    [labelSel, before],
    { timeout: 10000 },
  );
  const after = (await page.textContent(labelSel))?.trim();
  assert.notEqual(after, before, 'view label changed after next');
});

test('service worker registers and precaches the shell', async () => {
  await page.goto(BASE + '/spa', { waitUntil: 'load' });
  await waitForSWControl();
  const sw = await page.evaluate(async () => {
    if (!('serviceWorker' in navigator)) return { supported: false };
    const names = await caches.keys();
    const c = names.length ? await caches.open(names[0]) : null;
    const keys = c ? (await c.keys()).map((r) => new URL(r.url).pathname) : [];
    return {
      supported: true,
      controlled: !!navigator.serviceWorker.controller,
      hasWasm: keys.includes('/static/app.wasm'),
      hasCss: keys.includes('/static/app.css'),
    };
  });
  assert.ok(sw.supported, 'service workers supported');
  assert.ok(sw.controlled, 'service worker controls the page');
  assert.ok(sw.hasWasm, 'app.wasm precached');
  assert.ok(sw.hasCss, 'app.css precached');
});

test('renders offline from cache (PWA resilience)', async () => {
  await page.goto(BASE + '/spa', { waitUntil: 'load' });
  await page.waitForSelector('.view .widget', { timeout: 15000 });
  await waitForSWControl();

  await ctx.setOffline(true);
  try {
    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.waitForSelector('.view .widget', { timeout: 15000 });
    const n = await page.evaluate(() => document.querySelectorAll('.view .widget').length);
    assert.ok(n >= 1, 'kiosk renders while offline');
  } finally {
    await ctx.setOffline(false);
  }
});

// Runs last: it creates an OAuth source, which makes every kiosk screen show the
// health badge from then on.
test('kiosk shows the health badge when an OAuth source needs reconnect', async () => {
  // Admin login (session cookie coexists with the device cookie).
  await page.goto(BASE + '/login', { waitUntil: 'domcontentloaded' });
  await page.fill('input[name="passphrase"]', PASSPHRASE);
  await Promise.all([page.waitForNavigation(), page.click('button[type="submit"]')]);

  // Create an ms_graph data source and never connect it -> reconnect needed.
  await page.goto(BASE + '/admin/datasources', { waitUntil: 'load' });
  await page.fill('input[name="name"]', 'Outlook Test');
  await page.selectOption('select[name="type"]', 'ms_graph');
  await page.waitForTimeout(300); // let the fields HTMX-swap
  await page.click('form[hx-post="/admin/datasources"] button[type="submit"]');
  await page.waitForTimeout(500);

  // The kiosk badge should now be visible, red, and say "opnieuw verbinden".
  await page.goto(BASE + '/spa', { waitUntil: 'load' });
  await page.waitForSelector('.khealth', { state: 'visible', timeout: 15000 });
  const badge = await page.evaluate(() => {
    const el = document.querySelector('.khealth');
    return { cls: el?.className || '', text: (el?.textContent || '').toLowerCase() };
  });
  assert.match(badge.cls, /khealth-error/, 'badge is error-level');
  assert.match(badge.text, /verbinden/, 'badge mentions reconnect');
});
