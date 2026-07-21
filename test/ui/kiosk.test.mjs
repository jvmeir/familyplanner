// Synthetic browser tests for the server-rendered kiosk (templ + HTMX + SSE) and
// its PWA behaviour, using Playwright to drive a real Chrome/Edge against a
// running server (started by run.mjs). Covers:
//   - the kiosk page renders the shell + widgets,
//   - the footer "next" control swaps the view through the SSE loop,
//   - the service worker registers and precaches the shell,
//   - the kiosk still renders offline (PWA resilience),
//   - the health badge appears when an OAuth source needs reconnect.
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

test('kiosk renders the shell and widgets', async () => {
  await page.goto(BASE + '/kiosk', { waitUntil: 'load' });
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
  await page.goto(BASE + '/kiosk', { waitUntil: 'load' });
  await page.waitForSelector('#kview');
  // Wait for the SSE stream to set the initial footer view label.
  await page.waitForFunction(() => (document.querySelector('#kview')?.textContent || '').trim() !== '', null, { timeout: 10000 });
  const before = (await page.textContent('#kview'))?.trim();
  await page.click('.kfooter-controls button:nth-child(4)'); // ⏭ next -> fpCtl('next')
  await page.waitForFunction(
    (prev) => (document.querySelector('#kview')?.textContent || '').trim() !== prev,
    before,
    { timeout: 10000 },
  );
  const after = (await page.textContent('#kview'))?.trim();
  assert.notEqual(after, before, 'view label changed after next');
});

test('service worker registers and precaches the shell', async () => {
  await page.goto(BASE + '/kiosk', { waitUntil: 'load' });
  await waitForSWControl();
  const sw = await page.evaluate(async () => {
    if (!('serviceWorker' in navigator)) return { supported: false };
    const names = await caches.keys();
    const c = names.length ? await caches.open(names[0]) : null;
    const keys = c ? (await c.keys()).map((r) => new URL(r.url).pathname) : [];
    return {
      supported: true,
      controlled: !!navigator.serviceWorker.controller,
      hasCss: keys.includes('/static/app.css'),
      hasKioskJs: keys.includes('/static/kiosk.js'),
    };
  });
  assert.ok(sw.supported, 'service workers supported');
  assert.ok(sw.controlled, 'service worker controls the page');
  assert.ok(sw.hasCss, 'app.css precached');
  assert.ok(sw.hasKioskJs, 'kiosk.js precached');
});

test('renders offline from cache (PWA resilience)', async () => {
  await page.goto(BASE + '/kiosk', { waitUntil: 'load' });
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

// Runs last: creating an OAuth source makes the badge appear on the kiosk.
test('kiosk shows the health badge when an OAuth source needs reconnect', async () => {
  await page.goto(BASE + '/login', { waitUntil: 'domcontentloaded' });
  await page.fill('input[name="passphrase"]', PASSPHRASE);
  await Promise.all([page.waitForNavigation(), page.click('button[type="submit"]')]);

  await page.goto(BASE + '/admin/datasources', { waitUntil: 'load' });
  await page.fill('input[name="name"]', 'Outlook Test');
  await page.selectOption('select[name="type"]', 'ms_graph');
  await page.waitForTimeout(300);
  await page.click('form[hx-post="/admin/datasources"] button[type="submit"]');
  await page.waitForTimeout(500);

  await page.goto(BASE + '/kiosk', { waitUntil: 'load' });
  await page.waitForSelector('.khealth', { state: 'visible', timeout: 15000 });
  const badge = await page.evaluate(() => {
    const el = document.querySelector('.khealth');
    return { cls: el?.className || '', text: (el?.textContent || '').toLowerCase() };
  });
  assert.match(badge.cls, /khealth-error/, 'badge is error-level');
  assert.match(badge.text, /verbinden/, 'badge mentions reconnect');
});
