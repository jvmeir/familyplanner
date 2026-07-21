// Kiosk runtime: a static header clock, SSE-driven view swaps, the footer view
// label, and playback controls.
(function () {
  const stage = document.getElementById("stage");
  if (!stage) return;

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

  async function loadView(id) {
    if (!id) return;
    try {
      const r = await fetch("/kiosk/view/" + id, { headers: { Accept: "text/html" } });
      if (r.ok) stage.innerHTML = await r.text();
    } catch (e) {
      // keep last-good content on any error
    }
  }

  const es = new EventSource("/kiosk/stream");
  window.fpES = es; // shared so voiceclock.js can listen for "chime" without a 2nd stream
  es.addEventListener("navigate", function (e) {
    currentViewID = e.data;
    updateViewLabel(currentViewID);
    loadView(currentViewID);
  });
  es.addEventListener("refresh", function () {
    loadView(currentViewID);
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
