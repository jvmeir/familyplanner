// YouTube embed for the video widget, via the IFrame Player API so we get the
// ENDED event (which drives a screen's "advance on end" mode). fpSetupVideos()
// is called after each view swap; on an advance-on-end screen it plays each
// video once and, when all have finished, advances the playlist.
(function () {
  var stage = document.getElementById("stage");
  if (!stage) return;

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

  var players = [];
  function destroyAll() {
    players.forEach(function (p) { try { p.destroy(); } catch (e) {} });
    players = [];
  }

  window.fpSetupVideos = function () {
    destroyAll();
    var els = stage.querySelectorAll(".w-yt[data-video-id]");
    if (!els.length) return;
    loadAPI();
    var view = stage.querySelector(".view");
    var onEnd = !!(view && view.dataset.advanceOnEnd === "1");
    var total = els.length, ended = 0;

    var build = function () {
      els.forEach(function (el) {
        var id = el.dataset.videoId;
        if (!id) return;
        var mute = el.dataset.mute === "1";
        var loop = el.dataset.loop === "1" && !onEnd; // on_end plays once so it can end
        var holder = document.createElement("div");
        el.innerHTML = "";
        el.appendChild(holder);
        var pv = { autoplay: 1, controls: 0, rel: 0, playsinline: 1, modestbranding: 1, mute: mute ? 1 : 0 };
        if (loop) { pv.loop = 1; pv.playlist = id; }
        players.push(new YT.Player(holder, {
          width: "100%", height: "100%", videoId: id, playerVars: pv,
          events: {
            onReady: function (e) { if (mute) e.target.mute(); try { e.target.playVideo(); } catch (x) {} },
            onStateChange: function (e) {
              if (e.data === YT.PlayerState.ENDED && !loop) {
                ended++;
                if (onEnd && ended >= total && window.fpCtl) window.fpCtl("next");
              }
            }
          }
        }));
      });
    };
    if (apiReady) build(); else queue.push(build);
  };

  window.fpSetupVideos(); // handle a video present in the initial render
})();
