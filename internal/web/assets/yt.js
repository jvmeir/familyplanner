// YouTube embeds via the IFrame Player API. Used two ways:
//   1. On-screen video widgets inside #stage — recreated on each view swap; on an
//      "advance on end" screen they advance the playlist after one pass.
//   2. A persistent corner PiP (.kpip in the kiosk shell) that keeps playing while
//      screens rotate, cycling its video list with an optional hide interval.
(function () {
  var stage = document.getElementById("stage");

  var apiReady = false, queue = [];
  function loadAPI() {
    if (window.YT && window.YT.Player) { apiReady = true; return; }
    if (!document.getElementById("yt-api")) {
      var tag = document.createElement("script");
      tag.id = "yt-api";
      tag.src = "https://www.youtube.com/iframe_api";
      document.head.appendChild(tag);
    }
    var prev = window.onYouTubeIframeAPIReady;
    window.onYouTubeIframeAPIReady = function () {
      if (prev) { try { prev(); } catch (e) {} }
      apiReady = true;
      queue.forEach(function (f) { f(); });
      queue = [];
    };
  }
  function whenReady(f) { if (apiReady) f(); else { loadAPI(); queue.push(f); } }

  function ids(el) { try { return JSON.parse(el.dataset.videoIds || "[]"); } catch (e) { return []; } }

  // makePlayer plays a list on el, cycling, and returns a small controller
  // (player + next/prev/playPause). onAllEnded (optional) is called after one full
  // pass instead of looping (used to advance an "advance on end" screen).
  // interval > 0 hides el between videos (corner PiP).
  function makePlayer(el, list, opts, onAllEnded) {
    if (!list.length) return null;
    var holder = document.createElement("div");
    el.innerHTML = "";
    el.appendChild(holder);
    var ctrl = { el: el, i: 0, player: null };
    function load(idx) { ctrl.i = (idx % list.length + list.length) % list.length; try { ctrl.player.loadVideoById(list[ctrl.i]); } catch (x) {} }
    ctrl.next = function () { load(ctrl.i + 1); };
    ctrl.prev = function () { load(ctrl.i - 1); };
    ctrl.playPause = function () {
      try { ctrl.player.getPlayerState() === YT.PlayerState.PLAYING ? ctrl.player.pauseVideo() : ctrl.player.playVideo(); } catch (x) {}
    };
    var pv = { autoplay: 1, controls: 0, rel: 0, playsinline: 1, modestbranding: 1, mute: opts.mute ? 1 : 0 };
    ctrl.player = new YT.Player(holder, {
      width: "100%", height: "100%", videoId: list[0], playerVars: pv,
      events: {
        onReady: function (e) { if (opts.mute) e.target.mute(); try { e.target.playVideo(); } catch (x) {} },
        onStateChange: function (e) {
          if (e.data !== YT.PlayerState.ENDED) return;
          if (ctrl.i + 1 >= list.length && onAllEnded) { onAllEnded(); return; }
          if (opts.interval > 0) {
            el.style.visibility = "hidden";
            try { ctrl.player.pauseVideo(); } catch (x) {} // idle while hidden (bandwidth)
            setTimeout(function () { el.style.visibility = "visible"; ctrl.next(); }, opts.interval * 1000);
          } else {
            ctrl.next();
          }
        },
      },
    });
    return ctrl;
  }

  // ---- on-screen widgets (inside #stage) ----
  var stagePlayers = [];
  window.fpSetupVideos = function () {
    if (!stage) return;
    stagePlayers.forEach(function (c) { try { c.player.destroy(); } catch (e) {} });
    stagePlayers = [];
    var els = stage.querySelectorAll(".w-yt[data-video-ids]");
    if (!els.length) return;
    var view = stage.querySelector(".view");
    var onEnd = !!(view && view.dataset.advanceOnEnd === "1");
    whenReady(function () {
      els.forEach(function (el) {
        var done = onEnd ? function () { if (window.fpCtl) fpCtl("next"); } : null;
        var c = makePlayer(el, ids(el), { mute: el.dataset.mute === "1", interval: 0 }, done);
        if (c) stagePlayers.push(c);
      });
    });
  };

  // ---- persistent corner PiP (in the kiosk shell) ----
  var pip = null;      // the PiP controller
  var pipEl = null;    // the .kpip container
  function setupPip() {
    pipEl = document.querySelector(".kpip");
    var el = pipEl && pipEl.querySelector(".w-yt");
    if (!el || pip) return; // set up once; survives view swaps
    var list = ids(el);
    if (!list.length) return;
    whenReady(function () {
      pip = makePlayer(el, list, {
        mute: el.dataset.mute !== "0", // PiP defaults muted
        interval: parseInt(el.dataset.interval, 10) || 0,
      }, null);
    });
  }

  // PiP remote (Shift+arrows in kiosk.js): pause/resume, show/hide, next/prev.
  window.fpPip = {
    playPause: function () { if (pip) pip.playPause(); },
    next: function () { if (pip) pip.next(); },
    prev: function () { if (pip) pip.prev(); },
    toggle: function () {
      if (!pipEl) return;
      var hidden = pipEl.style.display === "none";
      pipEl.style.display = hidden ? "" : "none";
      if (pip) { try { hidden ? pip.player.playVideo() : pip.player.pauseVideo(); } catch (x) {} }
    },
  };

  if (stage) window.fpSetupVideos(); // initial on-screen video
  setupPip();
})();
