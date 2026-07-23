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
  // Nightly sanity reload: a 24/7 kiosk browser slowly accumulates cruft (SSE
  // reconnects, long-lived YouTube iframes, timers). A single full reload at a
  // dead hour (04:00 local) keeps it fresh without ever interrupting daytime
  // viewing. Updates already reload via the "version" event; this covers the
  // long-uptime-between-updates case. Guarded so it fires once per night.
  // NOTE: declared before tick() runs — tick() reads these, so they must not be
  // in the temporal dead zone when the first tick() fires synchronously below.
  const RELOAD_HOUR = 4;
  let reloadedToday = false;
  function maybeNightlyReload(now) {
    if (now.getHours() === RELOAD_HOUR) {
      if (!reloadedToday) {
        reloadedToday = true;
        location.reload();
      }
    } else {
      reloadedToday = false; // re-arm once we leave the reload hour
    }
  }

  function tick() {
    const now = new Date();
    if (dateEl) dateEl.textContent = dateFmt === "none" ? "" : fmtDate.format(now);
    if (timeEl) timeEl.textContent = fmtTime.format(now);
    tickCountdowns();
    maybeNightlyReload(now);
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

  // ---- photo slideshow: cycle a .w-slideshow <img> through its album,
  // shuffled and no-repeat, every data-photo-secs. Self-clears when removed
  // from the DOM (view swap) so timers don't leak. Used on the main stage AND
  // inside the corner PiP. (function declaration — hoisted, no TDZ.)
  function fpShuffle(a) {
    for (var i = a.length - 1; i > 0; i--) {
      var j = (i * 2654435761 + fpShuffleSeed++) % (i + 1); // deterministic-ish, avoids Math.random ban concerns
      var t = a[i]; a[i] = a[j]; a[j] = t;
    }
    return a;
  }
  var fpShuffleSeed = 1;
  function startSlideshows(root) {
    if (!root) return;
    root.querySelectorAll(".w-slideshow").forEach(function (img) {
      if (img.__fpSlide) return; // already cycling
      var urls = [], caps = [];
      try { urls = JSON.parse(img.dataset.photoUrls || "[]"); } catch (e) {}
      try { caps = JSON.parse(img.dataset.photoCaptions || "[]"); } catch (e) {}
      if (urls.length < 2) return;
      var secs = parseInt(img.dataset.photoSecs, 10) || 8;
      var cap = img.parentElement && img.parentElement.querySelector(".w-photo-cap");
      // Shuffle INDICES so the caption stays paired with its photo.
      var idxs = urls.map(function (_, i) { return i; });
      var order = fpShuffle(idxs.slice()), pos = 0;
      img.__fpSlide = setInterval(function () {
        if (!document.body.contains(img)) { clearInterval(img.__fpSlide); img.__fpSlide = null; return; }
        pos++;
        if (pos >= order.length) { order = fpShuffle(idxs.slice()); pos = 0; }
        var i = order[pos];
        img.src = urls[i];
        if (cap) cap.textContent = caps[i] || "";
      }, Math.max(2, secs) * 1000);
    });
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
        setupCellScroll(stage, lastDwellSecs);
        startSlideshows(stage);
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

  // Auto-scroll any overflowing cell (calendar/agenda/list) so all content is
  // shown without shrinking the text. Time-based: one full top→bottom→top trip
  // spans the view's dwell (with brief holds at each end), so everything is
  // reachable before the rotation advances. dwellSecs<=0 (paused) falls back to
  // a steady 30s loop. Per-element rAF, cancelled + restarted on each call.
  function setupCellScroll(root, dwellSecs) {
    if (!root) return;
    var cycleMs = (dwellSecs > 0 ? dwellSecs : 30) * 1000;
    root.querySelectorAll(".cell-scroll").forEach(function (el) {
      if (el.__fpScrollRAF) { cancelAnimationFrame(el.__fpScrollRAF); el.__fpScrollRAF = 0; }
      el.scrollTop = 0;
      if (el.scrollHeight - el.clientHeight < 4) return; // fits: nothing to scroll
      var start = null;
      function frame(t) {
        if (!document.body.contains(el)) { el.__fpScrollRAF = 0; return; } // swapped out
        if (start === null) start = t;
        var over = el.scrollHeight - el.clientHeight;
        if (over >= 4) {
          var phase = ((t - start) % cycleMs) / cycleMs; // 0..1 over one dwell
          var pos;
          if (phase < 0.12) pos = 0;                              // hold at top
          else if (phase < 0.5) pos = over * (phase - 0.12) / 0.38; // scroll down
          else if (phase < 0.62) pos = over;                      // hold at bottom
          else pos = over * (1 - (phase - 0.62) / 0.38);          // scroll back up
          el.scrollTop = pos;
        }
        el.__fpScrollRAF = requestAnimationFrame(frame);
      }
      el.__fpScrollRAF = requestAnimationFrame(frame);
    });
  }
  setupCellScroll(stage, lastDwellSecs);
  startSlideshows(stage); // cover the initial (inline-rendered) screen
  scheduleEndFallback();

  const es = new EventSource("/kiosk/stream");
  window.fpES = es; // shared so voiceclock.js can listen for "chime" without a 2nd stream
  var beat = function () { window.__fpBeat = Date.now(); }; // watchdog heartbeat
  es.addEventListener("open", beat);
  es.addEventListener("navigate", function (e) {
    beat();
    var changed = e.data !== currentViewID;
    currentViewID = e.data;
    stage.dataset.viewId = e.data; // keep the DOM in sync with the active view
    updateViewLabel(currentViewID);
    loadView(currentViewID, changed);
  });
  es.addEventListener("refresh", function () {
    beat();
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
    // Re-pace the auto-scroll to this view's actual dwell (the dwell event
    // arrives just after navigate, so loadView used the previous value).
    setupCellScroll(stage, lastDwellSecs);
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

  // ---- corner PiP: an independent playlist rotated client-side into .kpip.
  // The server pushes the item list (viewID + dwell + onEnd) on the "pip" event;
  // we fetch each view "bare" and swap it in, advancing on the dwell timer or,
  // for a video view marked onEnd, when the video finishes.
  var kpip = document.querySelector(".kpip");
  var kpipBody = null, kpipProg = null; // set up once in pipStart
  var pipItems = [], pipIdx = 0, pipTimer = null, pipPlayers = [], pipHidden = false;
  function pipStopPlayers() {
    pipPlayers.forEach(function (c) { try { c.player.destroy(); } catch (e) {} });
    pipPlayers = [];
  }
  // Animate the corner progress bar over `secs` (mirrors the main .kprogress);
  // secs <= 0 leaves it empty (e.g. a video view that advances on end).
  function pipProgress(secs) {
    if (!kpipProg) return;
    kpipProg.style.transition = "none";
    kpipProg.style.width = "0%";
    void kpipProg.offsetWidth;
    if (secs > 0) {
      kpipProg.style.transition = "width " + secs + "s linear";
      kpipProg.style.width = "100%";
    }
  }
  function pipRotate(i) {
    if (!kpipBody || !pipItems.length) return;
    pipIdx = (i % pipItems.length + pipItems.length) % pipItems.length;
    var item = pipItems[pipIdx];
    clearTimeout(pipTimer);
    pipStopPlayers();
    fetch("/kiosk/view/" + item.id + "?bare=1", { headers: { Accept: "text/html" } })
      .then(function (r) { return r.ok ? r.text() : Promise.reject(); })
      .then(function (html) {
        kpipBody.innerHTML = html;
        startSlideshows(kpipBody);
        setupCellScroll(kpipBody, item.dwell);
        var hasVideo = !!kpipBody.querySelector(".w-yt");
        var onEnd = item.onEnd && hasVideo ? function () { pipNext(); } : null;
        pipPlayers = window.fpVideosIn ? window.fpVideosIn(kpipBody, { mute: true, onAllEnded: onEnd }) : [];
        if (onEnd) { pipProgress(0); } else { pipProgress(Math.max(3, item.dwell)); pipTimer = setTimeout(pipNext, Math.max(3, item.dwell) * 1000); }
      })
      .catch(function () { pipTimer = setTimeout(pipNext, 10000); }); // retry-ish on error
  }
  function pipNext() { pipRotate(pipIdx + 1); }
  function pipPrev() { pipRotate(pipIdx - 1); }
  function pipStart(items) {
    pipItems = items || [];
    clearTimeout(pipTimer);
    pipStopPlayers();
    if (!kpip) return;
    if (!pipItems.length) { kpip.style.display = "none"; kpip.innerHTML = ""; kpipBody = null; return; }
    // Build the persistent body + progress skeleton once (rotation only swaps
    // the body, so the progress bar survives).
    if (!kpipBody) {
      kpip.innerHTML = '<div class="kpip-body"></div><div class="kpip-prog"><i></i></div>';
      kpipBody = kpip.querySelector(".kpip-body");
      kpipProg = kpip.querySelector(".kpip-prog > i");
    }
    kpip.style.display = pipHidden ? "none" : "";
    pipRotate(0);
  }
  window.fpPip = {
    toggle: function () { if (!kpip) return; pipHidden = !pipHidden; kpip.style.display = pipHidden ? "none" : ""; },
    next: pipNext,
    prev: pipPrev,
    mute: function () { pipPlayers.forEach(function (c) { try { c.player.isMuted() ? c.player.unMute() : c.player.mute(); } catch (e) {} }); },
    playPause: function () { pipPlayers.forEach(function (c) { try { c.playPause(); } catch (e) {} }); },
  };
  es.addEventListener("pip", function (e) {
    var items = [];
    try { items = JSON.parse(e.data); } catch (_) {}
    // Only restart if the payload actually changed (avoid resetting the corner
    // rotation on every 30s refresh tick).
    if (JSON.stringify(items) === JSON.stringify(pipItems)) return;
    pipStart(items);
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
