// YouTube embeds via the IFrame Player API. Two entry points:
//   1. window.fpSetupVideos() — (re)initialize the on-screen video widgets inside
//      #stage after a view swap; on an "advance on end" screen it advances the
//      playlist after one full pass.
//   2. window.fpVideosIn(container, {mute, onAllEnded}) — initialize every video
//      widget inside an arbitrary container (used by the corner PiP in kiosk.js).
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
  // pass instead of looping (used to advance an "advance on end" screen/corner).
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
          ctrl.next();
        },
      },
    });
    return ctrl;
  }

  // Initialize every .w-yt in a container. opts.mute overrides the widget's own
  // data-mute (the PiP forces muted); opts.onAllEnded advances after one pass.
  window.fpVideosIn = function (container, opts) {
    opts = opts || {};
    var players = [];
    whenReady(function () {
      container.querySelectorAll(".w-yt[data-video-ids]").forEach(function (el) {
        var mute = opts.mute != null ? opts.mute : el.dataset.mute === "1";
        var c = makePlayer(el, ids(el), { mute: mute }, opts.onAllEnded || null);
        if (c) players.push(c);
      });
    });
    return players;
  };

  // On-screen video widgets inside #stage, recreated on each view swap.
  var stagePlayers = [];
  window.fpSetupVideos = function () {
    if (!stage) return;
    stagePlayers.forEach(function (c) { try { c.player.destroy(); } catch (e) {} });
    var view = stage.querySelector(".view");
    var onEnd = !!(view && view.dataset.advanceOnEnd === "1");
    stagePlayers = window.fpVideosIn(stage, {
      onAllEnded: onEnd ? function () { if (window.fpCtl) fpCtl("next"); } : null,
    });
  };

  if (stage) window.fpSetupVideos(); // initial on-screen video
})();
