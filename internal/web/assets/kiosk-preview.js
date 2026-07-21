// Standalone preview: no SSE/rotation, just the same overflow auto-scroll the
// kiosk uses so a busy calendar cell previews the way it will on the wall.
(function () {
  var stage = document.getElementById("stage");
  if (!stage) return;

  // Live dhms countdown (same as the kiosk).
  function fmtCountdown(target) {
    var s = target - Math.floor(Date.now() / 1000);
    if (s < 0) s = 0;
    var d = Math.floor(s / 86400); s %= 86400;
    var h = Math.floor(s / 3600); s %= 3600;
    var m = Math.floor(s / 60), sec = s % 60;
    var p = function (n) { return (n < 10 ? "0" : "") + n; };
    return d + "d " + p(h) + "u " + p(m) + "m " + p(sec) + "s";
  }
  function tickCountdowns() {
    stage.querySelectorAll(".countdown-live[data-target]").forEach(function (el) {
      var t = parseInt(el.dataset.target, 10);
      if (!isNaN(t)) el.textContent = fmtCountdown(t);
    });
  }
  tickCountdowns();
  setInterval(tickCountdowns, 1000);

  stage.querySelectorAll(".cell-scroll").forEach(function (el) {
    if (el.scrollHeight - el.clientHeight < 4) return;
    var dir = 1, pause = 24;
    setInterval(function () {
      if (pause > 0) { pause--; return; }
      var max = el.scrollHeight - el.clientHeight;
      el.scrollTop += dir;
      if (el.scrollTop >= max) { el.scrollTop = max; dir = -1; pause = 24; }
      else if (el.scrollTop <= 0) { el.scrollTop = 0; dir = 1; pause = 24; }
    }, 70);
  });
})();
