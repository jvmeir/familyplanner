// Analog clock-face widget. Builds an SVG dial (hour/minute/second hands) with
// sunrise/sunset marks, plus a text panel (date · week · moon · sun · daylight)
// and an optional "op deze dag" fact that rotates each time the screen appears.
(function () {
  var clocks = [];       // active clocks (hand refs) for the animation loop
  var animating = false;
  var factCounter = 0;   // advances each time a clock mounts → a new fact per rotation

  function f(n) { return Math.round(n * 100) / 100; }
  function pad(n) { return (n < 10 ? "0" : "") + n; }
  function cap(s) { return s ? s.charAt(0).toUpperCase() + s.slice(1) : s; }
  function hhmm(min) { return pad(Math.floor(min / 60)) + ":" + pad(min % 60); }
  function dur(min) { var h = Math.floor(min / 60), m = min % 60; return (h > 0 ? h + "u " : "") + m + "m"; }
  function isoWeek(dt) {
    var d = new Date(Date.UTC(dt.getFullYear(), dt.getMonth(), dt.getDate()));
    var day = d.getUTCDay() || 7;
    d.setUTCDate(d.getUTCDate() + 4 - day);
    var ys = new Date(Date.UTC(d.getUTCFullYear(), 0, 1));
    return Math.ceil((((d - ys) / 86400000) + 1) / 7);
  }

  window.fpSetupClocks = function (root) {
    if (!root) return;
    root.querySelectorAll(".w-clock").forEach(function (el) {
      if (el.__fpClock) return;
      setupClock(el);
    });
  };

  function setupClock(el) {
    var data = {};
    try { data = JSON.parse(el.dataset.clock || "{}"); } catch (e) {}
    el.__fpClock = data;
    el.innerHTML = buildHTML(data);
    var c = {
      el: el, data: data,
      hour: el.querySelector(".wc-hour"),
      min: el.querySelector(".wc-min"),
      sec: el.querySelector(".wc-sec"),
      dateEl: el.querySelector(".wc-date"),
      subEl: el.querySelector(".wc-sub"),
    };
    if (data.facts && data.facts.length) {
      var fEl = el.querySelector(".wc-fact");
      if (fEl) fEl.textContent = data.facts[factCounter % data.facts.length];
      factCounter++;
    }
    updateText(c);
    clocks.push(c);
    startAnim();
  }

  function buildHTML(data) {
    var ticks = "";
    for (var i = 0; i < 12; i++) {
      var a = i * 30 * Math.PI / 180;
      ticks += '<line x1="' + f(50 + 40 * Math.sin(a)) + '" y1="' + f(50 - 40 * Math.cos(a)) +
        '" x2="' + f(50 + 46 * Math.sin(a)) + '" y2="' + f(50 - 46 * Math.cos(a)) + '" class="wc-tick"/>';
    }
    var marks = sunMark(data.sunrise_min, "☀", "wc-sun") + sunMark(data.sunset_min, "☾", "wc-moon");
    return '<svg class="wc-dial" viewBox="0 0 100 100">' +
      '<circle cx="50" cy="50" r="47" class="wc-face"/>' + ticks + marks +
      '<line class="wc-hand wc-hour" x1="50" y1="50" x2="50" y2="28"/>' +
      '<line class="wc-hand wc-min" x1="50" y1="50" x2="50" y2="16"/>' +
      '<line class="wc-hand wc-sec" x1="50" y1="56" x2="50" y2="13"/>' +
      '<circle cx="50" cy="50" r="2" class="wc-pin"/></svg>' +
      '<div class="wc-info"><div class="wc-date"></div><div class="wc-sub"></div></div>' +
      (data.facts && data.facts.length ? '<div class="wc-fact"></div>' : '');
  }

  function sunMark(min, glyph, cls) {
    if (min == null || min < 0) return "";
    var a = ((min / 60) % 12) * 30 * Math.PI / 180;
    return '<text x="' + f(50 + 40 * Math.sin(a)) + '" y="' + f(50 - 40 * Math.cos(a) + 2.2) +
      '" class="' + cls + '" text-anchor="middle">' + glyph + '</text>';
  }

  function startAnim() {
    if (animating) return;
    animating = true;
    requestAnimationFrame(function frame() {
      clocks = clocks.filter(function (c) { return document.body.contains(c.el); });
      if (!clocks.length) { animating = false; return; }
      var now = new Date();
      var s = now.getSeconds() + now.getMilliseconds() / 1000;
      var m = now.getMinutes() + s / 60;
      var h = (now.getHours() % 12) + m / 60;
      clocks.forEach(function (c) {
        if (c.hour) c.hour.setAttribute("transform", "rotate(" + f(h * 30) + " 50 50)");
        if (c.min) c.min.setAttribute("transform", "rotate(" + f(m * 6) + " 50 50)");
        if (c.sec) c.sec.setAttribute("transform", "rotate(" + f(s * 6) + " 50 50)");
      });
      requestAnimationFrame(frame);
    });
  }

  var dateFmt = new Intl.DateTimeFormat("nl-BE", { weekday: "long", day: "numeric", month: "long", year: "numeric" });
  function updateText(c) {
    if (!document.body.contains(c.el)) return;
    var now = new Date();
    if (c.dateEl) c.dateEl.textContent = cap(dateFmt.format(now)) + " · wk " + isoWeek(now);
    var d = c.data, parts = [];
    if (d.moon_icon) parts.push(d.moon_icon + " " + (d.moon_name || ""));
    if (d.sunrise_min >= 0) parts.push("☀ " + hhmm(d.sunrise_min));
    if (d.sunset_min >= 0) parts.push("☾ " + hhmm(d.sunset_min));
    if (d.sunrise_min >= 0 && d.sunset_min >= 0) {
      var nm = now.getHours() * 60 + now.getMinutes();
      if (nm < d.sunrise_min) parts.push("nog " + dur(d.sunrise_min - nm) + " tot zon");
      else if (nm < d.sunset_min) parts.push("nog " + dur(d.sunset_min - nm) + " zon");
      else parts.push("nacht");
    }
    if (c.subEl) c.subEl.textContent = parts.join(" · ");
  }

  setInterval(function () {
    clocks.forEach(function (c) { if (document.body.contains(c.el)) updateText(c); });
  }, 1000);

  var stage = document.getElementById("stage");
  if (stage) window.fpSetupClocks(stage);
})();
