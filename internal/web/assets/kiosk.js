// Kiosk runtime: a static header clock, SSE-driven view swaps, the footer view
// label, and playback controls.
(function () {
  const stage = document.getElementById("stage");
  if (!stage) return;

  // Mark the document as a kiosk so the CSS auto-scaling applies reliably
  // (independent of :has() support in the kiosk browser).
  document.documentElement.classList.add("kiosk");

  // ---- static header clock (independent of the server / rotation) ----
  const dateEl = document.getElementById("kdate");
  const timeEl = document.getElementById("ktime");
  const fmtDate = new Intl.DateTimeFormat("nl-BE", { weekday: "long", day: "numeric", month: "long", year: "numeric" });
  const fmtTime = new Intl.DateTimeFormat("nl-BE", { hour: "2-digit", minute: "2-digit" });
  function tick() {
    const now = new Date();
    if (dateEl) dateEl.textContent = fmtDate.format(now);
    if (timeEl) timeEl.textContent = fmtTime.format(now);
  }
  tick();
  setInterval(tick, 1000);

  // ---- footer view label (mirrors the jump dropdown's option text) ----
  const jump = document.getElementById("kjump");
  const viewEl = document.getElementById("kview");
  function updateViewLabel(id) {
    if (jump) {
      jump.value = id;
      const opt = jump.querySelector('option[value="' + id + '"]');
      if (opt && viewEl) viewEl.textContent = opt.textContent;
    }
  }

  // ---- SSE: follow the server's current view ----
  let currentViewID = stage.dataset.viewId || null;
  updateViewLabel(currentViewID);

  async function loadView(id, animate) {
    if (!id) return;
    try {
      const r = await fetch("/kiosk/view/" + id, { headers: { Accept: "text/html" } });
      if (r.ok) {
        stage.innerHTML = await r.text();
        // Slide the new view in on a real navigation (not the periodic refresh),
        // so data ticks don't cause a distracting re-animation every 30s.
        if (animate) {
          const v = stage.querySelector(".view");
          if (v) v.classList.add("kslide");
        }
      }
    } catch (e) {
      // keep last-good content on any error
    }
  }

  const es = new EventSource("/kiosk/stream");
  window.fpES = es; // shared so voiceclock.js can listen for "chime" without a 2nd stream
  es.addEventListener("navigate", function (e) {
    var changed = e.data !== currentViewID;
    currentViewID = e.data;
    updateViewLabel(currentViewID);
    loadView(currentViewID, changed);
  });
  es.addEventListener("refresh", function () {
    loadView(currentViewID, false);
    loadTicker();
  });

  // The ticker lives in the persistent bar (not swapped with the view), so
  // refresh it separately on each tick.
  const tickerEl = document.getElementById("kticker");
  async function loadTicker() {
    if (!tickerEl) return;
    try {
      const r = await fetch("/kiosk/ticker", { headers: { Accept: "text/html" } });
      if (r.ok) tickerEl.innerHTML = await r.text();
    } catch (e) {
      // keep last-good ticker
    }
  }
  // Auto-reload after a redeploy: the server sends its boot id on (re)connect;
  // EventSource reconnects when the container restarts, and a changed id means a
  // new build is live, so reload to pick up new HTML/CSS/JS.
  var bootID = null;
  es.addEventListener("version", function (e) {
    if (bootID === null) bootID = e.data;
    else if (e.data !== bootID) location.reload();
  });

  // Seconds-to-next-screen: the server sends the dwell (0 = paused) with each
  // navigate; count it down locally.
  var cdEl = document.getElementById("kcountdown");
  var cdTimer = null, cdLeft = 0;
  function cdRender() {
    if (cdEl) cdEl.textContent = cdLeft > 0 ? "→ " + cdLeft + "s" : "";
  }
  es.addEventListener("dwell", function (e) {
    cdLeft = parseInt(e.data, 10) || 0;
    if (cdTimer) { clearInterval(cdTimer); cdTimer = null; }
    cdRender();
    if (cdLeft > 0) {
      cdTimer = setInterval(function () {
        cdLeft -= 1;
        if (cdLeft <= 0) { cdLeft = 0; clearInterval(cdTimer); cdTimer = null; }
        cdRender();
      }, 1000);
    }
  });
  // UI scale multiplier (set from admin; applied live on top of viewport scaling).
  es.addEventListener("scale", function (e) {
    var v = parseFloat(e.data);
    if (!isNaN(v)) document.documentElement.style.setProperty("--kiosk-scale", v);
  });

  // ---- controls (also reachable from a phone remote later) ----
  window.fpCtl = function (cmd) {
    fetch("/kiosk/control/" + cmd, { method: "POST" }).catch(function () {});
  };
  window.fpGoto = function (id) {
    if (!id) return;
    fetch("/kiosk/control/goto?view=" + encodeURIComponent(id), { method: "POST" }).catch(function () {});
  };
})();
