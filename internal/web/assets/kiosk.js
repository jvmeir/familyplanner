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
  let transitionOn = true; // slide animation on screen change (toggled via config)
  let lastDwellSecs = 30;  // most recent dwell (for the on_end no-video fallback)
  updateViewLabel(currentViewID);

  async function loadView(id, animate) {
    if (!id) return;
    try {
      const r = await fetch("/kiosk/view/" + id, { headers: { Accept: "text/html" } });
      if (r.ok) {
        stage.innerHTML = await r.text();
        // Slide the new view in on a real navigation (not the periodic refresh),
        // so data ticks don't cause a distracting re-animation every 30s.
        if (animate && transitionOn) {
          const v = stage.querySelector(".view");
          if (v) v.classList.add("kslide");
        }
        tickCountdowns(); // set dhms text now so it doesn't flash the day-only fallback
        setupCellScroll();
        if (window.fpSetupVideos) window.fpSetupVideos();
        scheduleEndFallback();
      }
    } catch (e) {
      // keep last-good content on any error
    }
  }

  // On an "advance-on-end" screen the server suspends its timer, so the client
  // must advance. Videos advance on their ENDED event (yt.js); if the rendered
  // widget isn't a video (e.g. a random-single screen showed a calendar), fall
  // back to advancing after the normal dwell so rotation never gets stuck.
  var endFallback = null;
  function scheduleEndFallback() {
    if (endFallback) { clearTimeout(endFallback); endFallback = null; }
    const v = stage.querySelector(".view");
    const onEnd = !!(v && v.dataset.advanceOnEnd === "1");
    if (onEnd && !stage.querySelector(".w-yt")) {
      const secs = lastDwellSecs > 0 ? lastDwellSecs : 30;
      endFallback = setTimeout(function () { if (window.fpCtl) fpCtl("next"); }, secs * 1000);
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
  scheduleEndFallback(); // cover the initial (inline-rendered) screen

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
    // Don't reload the view while a video is playing — reloading destroys the
    // player (restarting it) and, on a random-single screen, re-randomizes the
    // widget. The video plays to its end undisturbed; only the ticker refreshes.
    if (!stage.querySelector(".w-yt")) {
      loadView(currentViewID, false);
    }
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
    var secs = parseInt(e.data, 10) || 0;
    lastDwellSecs = secs;
    if (!progEl) return;
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
    if ("transition" in c) transitionOn = !!c.transition;
  });

  // UI-only commands pushed by the admin remote (mute / PiP), acted on here.
  es.addEventListener("cmd", function (e) {
    switch (e.data) {
      case "mute":
      case "unmute": if (window.fpPip) fpPip.mute(); break;
      case "pip-toggle": if (window.fpPip) fpPip.toggle(); break;
      case "pip-next": if (window.fpPip) fpPip.next(); break;
      case "pip-prev": if (window.fpPip) fpPip.prev(); break;
    }
  });

  // ---- controls (also reachable from a phone remote later) ----
  window.fpCtl = function (cmd) {
    fetch("/kiosk/control/" + cmd, { method: "POST" }).catch(function () {});
  };
  window.fpGoto = function (id) {
    if (!id) return;
    fetch("/kiosk/control/goto?view=" + encodeURIComponent(id), { method: "POST" }).catch(function () {});
  };

  // Keyboard remote (e.g. a presentation clicker or a keyboard on the kiosk):
  // ←/→ previous/next screen, ↑ pause/resume, ↓ mute/unmute the voice clock.
  // Plain arrows drive the playlist / voice clock; Shift+arrows drive the corner
  // PiP video. Shift+arrow is only text-selection in a browser (nothing on a
  // kiosk) and isn't an OS global shortcut, so there's no clash.
  var kbPaused = false;
  document.addEventListener("keydown", function (e) {
    if (e.key === "i" || e.key === "I") {
      var sc = document.getElementById("kshortcuts");
      if (sc) sc.hidden = !sc.hidden;
      e.preventDefault();
      return;
    }
    if (e.shiftKey) {
      switch (e.key) {
        case "ArrowUp": if (window.fpPip) fpPip.playPause(); break;   // pause/resume PiP
        case "ArrowDown": if (window.fpPip) fpPip.toggle(); break;     // show/hide PiP
        case "ArrowRight": if (window.fpPip) fpPip.next(); break;      // next PiP video
        case "ArrowLeft": if (window.fpPip) fpPip.prev(); break;       // previous PiP video
        default: return;
      }
      e.preventDefault();
      return;
    }
    switch (e.key) {
      case "ArrowLeft": fpCtl("prev"); break;
      case "ArrowRight": fpCtl("next"); break;
      case "ArrowUp": kbPaused = !kbPaused; fpCtl(kbPaused ? "pause" : "resume"); break;
      case "ArrowDown": if (window.fpSnooze) fpSnooze(); break;
      default: return;
    }
    e.preventDefault();
  });
})();
