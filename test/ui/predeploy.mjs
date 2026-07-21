// Pre-deploy UI pass: register a kiosk, subscribe to the default playlist, cycle
// through every screen, and flag UI inconsistencies (widget/text overflow,
// horizontal overflow, JS errors). Screenshots each screen for visual review.
// Run before every deploy:  FP_UI_BASE=http://localhost:PORT node predeploy.mjs
import { chromium } from 'playwright-core';

const BASE = process.env.FP_UI_BASE || 'http://localhost:8099';
const CHANNEL = process.env.FP_UI_CHANNEL || 'chrome';
const PASS = process.env.FP_UI_PASSPHRASE || 'secret';

const browser = await chromium.launch({ channel: CHANNEL, headless: true });
const ctx = await browser.newContext({ viewport: { width: 3440, height: 1440 } });
const page = await ctx.newPage();
const errs = [];
page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
page.on('console', (m) => { if (m.type() === 'error') errs.push('console: ' + m.text()); });

await page.goto(BASE + '/pair', { waitUntil: 'domcontentloaded' });
await page.fill('input[name="passphrase"]', PASS);
await Promise.all([page.waitForNavigation(), page.click('button[type="submit"]')]);
await page.goto(BASE + '/kiosk', { waitUntil: 'load' });
await page.waitForSelector('.view .widget', { timeout: 15000 });

const issues = [];
const seen = new Set();
const curId = () => page.evaluate(() => document.getElementById('stage')?.dataset.viewId || '');

async function check(id) {
  await page.waitForTimeout(500);
  const r = await page.evaluate(() => {
    const bad = [];
    document.querySelectorAll('.view .widget').forEach((w, i) => {
      if (w.scrollHeight > w.clientHeight + 2 || w.scrollWidth > w.clientWidth + 2) {
        bad.push({ i, overflowY: w.scrollHeight - w.clientHeight, overflowX: w.scrollWidth - w.clientWidth });
      }
    });
    const de = document.documentElement;
    return { bad, hOverflow: de.scrollWidth > de.clientWidth + 2 };
  });
  if (r.bad.length) issues.push(`view ${id}: ${r.bad.length} widget(s) clipped ${JSON.stringify(r.bad)}`);
  if (r.hOverflow) issues.push(`view ${id}: horizontal overflow`);
  await page.screenshot({ path: 'predeploy-' + id + '.png' });
}

let id = await curId();
let guard = 0;
while (id && !seen.has(id) && guard++ < 16) {
  seen.add(id);
  await check(id);
  await page.click('.kfooter-controls button:last-child'); // ⏭ next
  await page
    .waitForFunction((prev) => (document.getElementById('stage')?.dataset.viewId || '') !== prev, id, { timeout: 8000 })
    .catch(() => {});
  id = await curId();
}

console.log('screens checked:', [...seen].join(', ') || '(none)');
console.log('js errors:', errs.length ? errs : 'none');
console.log('UI issues:', issues.length ? issues : 'none');
if (issues.length || errs.length) process.exitCode = 1;
await browser.close();
