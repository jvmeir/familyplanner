// Kiosk runtime: a static header clock, SSE-driven view swaps, the footer view
// label, and playback controls.
(function () {
  const stage = document.getElementById("stage");
  if (!stage) return;

  // Mark the document as a kiosk so the CSS auto-scaling applies reliably
  // (independent of :has() support in the kiosk browser).
  document.documentElement.classList.add("kiosk");

  // ---- clock (client-side; date format pushed via the config event) ----
  const dateEl = document.getElementById("kdate");
  const timeEl = document.getElementById("ktime");
  const fmtTime = new Intl.DateTimeFormat("nl-BE", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  let dateFmt = "long";
  let fmtDate = new Intl.DateTimeFormat("nl-BE", dateOpts(dateFmt));
  function dateOpts(f) {
    if (f === "short") return { day: "2-digit", month: "2-digit", year: "numeric" };
    return { weekday: "long", day: "numeric", month: "long", year: "numeric" }; // long
  }
  function tick() {
    const now = new Date();
    if (dateEl) dateEl.textContent = dateFmt === "none" ? "" : fmtDate.format(now);
    if (timeEl) timeEl.textContent = fmtTime.format(now);
    tickCountdowns();
  }
  tick();
  setInterval(tick, 1000);

  // Live "days, hours, minutes, seconds" countdown widgets (Precision=dhms).
  function tickCountdowns() {
    document.querySelectorAll(".countdown-live[data-target]").forEach(function (el) {
      const t = parseInt(el.dataset.target, 10);
      if (!isNaN(t)) el.textContent = fmtCountdown(t);
    });
  }
  function fmtCountdown(target) {
    let s = target - Math.floor(Date.now() / 1000);
    if (s < 0) s = 0;
    const d = Math.floor(s / 86400); s %= 86400;
    const h = Math.floor(s / 3600); s %= 3600;
    const m = Math.floor(s / 60), sec = s % 60;
    const p = (n) => (n < 10 ? "0" : "") + n;
    return d + "d " + p(h) + "u " + p(m) + "m " + p(sec) + "s";
  }

  // ---- footer view label (name looked up from the id->name map) ----
  const viewEl = document.getElementById("kview");
  let viewNames = {};
  try { viewNames = JSON.parse((viewEl && viewEl.dataset.names) || "{}"); } catch (e) {}
  function updateViewLabel(id) {
    if (viewEl) viewEl.textContent = viewNames[id] || "";
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
        setupCellScroll();
      }
    } catch (e) {
      // keep last-good content on any error
    }
  }

  // Auto-scroll any calendar cell/agenda that overflows: creep top→bottom→top so
  // days with many events show them all without shrinking the text.
  var scrollTimers = [];
  function setupCellScroll() {
    scrollTimers.forEach(clearInterval);
    scrollTimers = [];
    stage.querySelectorAll(".cell-scroll").forEach(function (el) {
      if (el.scrollHeight - el.clientHeight < 4) return; // fits: nothing to scroll
      var dir = 1, pause = 24; // start paused at the top
      var timer = setInterval(function () {
        if (pause > 0) { pause--; return; }
        var max = el.scrollHeight - el.clientHeight;
        el.scrollTop += dir;
        if (el.scrollTop >= max) { el.scrollTop = max; dir = -1; pause = 24; }
        else if (el.scrollTop <= 0) { el.scrollTop = 0; dir = 1; pause = 24; }
      }, 70);
      scrollTimers.push(timer);
    });
  }
  setupCellScroll();

  const es = new EventSource("/kiosk/stream");
  window.fpES = es; // shared so voiceclock.js can listen for "chime" without a 2nd stream
  es.addEventListener("navigate", function (e) {
    var changed = e.data !== currentViewID;
    currentViewID = e.data;
    stage.dataset.viewId = e.data; // keep the DOM in sync with the active view
    updateViewLabel(currentViewID);
    loadView(currentViewID, changed);
  });
  es.addEventListener("refresh", function () {
    loadView(currentViewID, false);
    loadTicker();
  });
  // Fresh id->name map (covers screens added/renamed after page load).
  es.addEventListener("names", function (e) {
    try { viewNames = JSON.parse(e.data); } catch (_) {}
    updateViewLabel(currentViewID);
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

  // Time-to-next-screen: the server sends the dwell (0 = paused) with each
  // navigate; drive the full-width progress bar under the bar.
  var progEl = document.getElementById("kprogress");
  es.addEventListener("dwell", function (e) {
    if (!progEl) return;
    var secs = parseInt(e.data, 10) || 0;
    progEl.style.transition = "none";
    progEl.style.width = "0%";
    void progEl.offsetWidth; // reflow so the reset applies before animating
    if (secs > 0) {
      progEl.style.transition = "width " + secs + "s linear";
      progEl.style.width = "100%";
    }
  });

  // Live kiosk config (pushed on connect + each refresh): UI scale, ticker
  // scroll speed, and the banner date format.
  es.addEventListener("config", function (e) {
    var c = {};
    try { c = JSON.parse(e.data); } catch (_) {}
    if (c.scale) document.documentElement.style.setProperty("--kiosk-scale", c.scale);
    if (c.tickerSecs) document.documentElement.style.setProperty("--kticker-secs", c.tickerSecs + "s");
    if (c.dateFmt) { dateFmt = c.dateFmt; fmtDate = new Intl.DateTimeFormat("nl-BE", dateOpts(dateFmt)); tick(); }
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
