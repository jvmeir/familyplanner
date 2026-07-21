// Standalone preview: no SSE/rotation, just the same overflow auto-scroll the
// kiosk uses so a busy calendar cell previews the way it will on the wall.
(function () {
  var stage = document.getElementById("stage");
  if (!stage) return;
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
