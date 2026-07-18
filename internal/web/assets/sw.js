// Family Planner service worker — offline resilience for the kiosk + admin.
//
// Strategy:
//   - Precache the public app shell (CSS, WASM client, runtime, icons, manifests).
//   - Static assets + navigations: cache-first with background refresh (SWR), so
//     the kiosk boots and renders even if the server is unreachable.
//   - GET /api/*: network-first, falling back to the last-good cached response —
//     this is the "show last-known data when offline" behaviour.
//   - SSE (/kiosk/stream) and non-GET (control POSTs): never cached; pass through.
//
// Bump CACHE to invalidate everything on a breaking change.
const CACHE = 'fp-v1';

// Auth-free assets safe to precache at install time. Authenticated HTML pages
// (/spa, /admin, …) are cached on first successful visit by the fetch handler,
// so we never cache a login redirect.
const SHELL = [
  '/static/app.css',
  '/static/wasm_exec.js',
  '/static/app.wasm',
  '/static/icon-192.png',
  '/static/icon-512.png',
  '/manifest.webmanifest',
  '/manifest.kiosk.webmanifest',
];

self.addEventListener('install', (e) => {
  e.waitUntil(
    caches.open(CACHE)
      .then((c) => c.addAll(SHELL))
      .then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim())
  );
});

function putInCache(req, res) {
  if (res && res.status === 200 && res.type !== 'opaqueredirect') {
    const copy = res.clone();
    caches.open(CACHE).then((c) => c.put(req, copy));
  }
  return res;
}

self.addEventListener('fetch', (e) => {
  const req = e.request;
  if (req.method !== 'GET') return; // control POSTs etc. -> straight to network

  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return; // cross-origin (iframes, photos) untouched
  if (url.pathname === '/kiosk/stream') return;     // SSE stream: never cache

  if (url.pathname.startsWith('/api/')) {
    // Network-first with last-good fallback.
    e.respondWith(
      fetch(req).then((res) => putInCache(req, res)).catch(() => caches.match(req))
    );
    return;
  }

  // Shell / static / navigations: serve cache immediately, refresh in background.
  e.respondWith(
    caches.match(req).then((cached) => {
      const network = fetch(req).then((res) => putInCache(req, res)).catch(() => cached);
      return cached || network;
    })
  );
});
