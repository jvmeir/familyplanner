// Voice clock audio: synthesises an open/public-domain chime on each quarter-hour
// and, on the hour, speaks the Dutch time (SpeechSynthesis, nl-BE). The synth is
// exposed on window (fpChime/fpSpeak/fpTestChime) so the admin settings page can
// preview it; the SSE listener at the bottom only runs on the kiosk (needs fpES).
(function () {
  // Browsers block audio until a user gesture; on an untouched wall the context
  // stays suspended until a tap, or launch Chromium with
  // --autoplay-policy=no-user-gesture-required.
  var ctx = null;
  function audio() {
    if (!ctx) {
      var AC = window.AudioContext || window.webkitAudioContext;
      if (!AC) return null;
      ctx = new AC();
    }
    if (ctx.state === "suspended") ctx.resume();
    return ctx;
  }

  // A struck-bell tone: fundamental + soft octave, fast attack, long decay.
  function bell(ac, freq, start, dur, peak) {
    for (var i = 0; i < 2; i++) {
      var o = ac.createOscillator(), g = ac.createGain();
      o.type = "triangle";
      o.frequency.value = freq * (i ? 2 : 1);
      o.connect(g); g.connect(ac.destination);
      var pk = i ? peak * 0.3 : peak;
      g.gain.setValueAtTime(0.0001, start);
      g.gain.exponentialRampToValueAtTime(pk, start + 0.012);
      g.gain.exponentialRampToValueAtTime(0.0001, start + dur);
      o.start(start); o.stop(start + dur + 0.05);
    }
  }

  function beep(ac, freq, start, dur) {
    var o = ac.createOscillator(), g = ac.createGain();
    o.type = "sine"; o.frequency.value = freq;
    o.connect(g); g.connect(ac.destination);
    g.gain.setValueAtTime(0.0001, start);
    g.gain.exponentialRampToValueAtTime(0.3, start + 0.005);
    g.gain.setValueAtTime(0.3, start + dur - 0.02);
    g.gain.exponentialRampToValueAtTime(0.0001, start + dur);
    o.start(start); o.stop(start + dur + 0.02);
  }

  // Westminster Quarters (public-domain, "Big Ben"). Five 4-note changes in E
  // major; per quarter it plays a growing set, and on the hour it adds the hour
  // strikes on a low bell.
  var N = { E4: 329.63, FS4: 369.99, GS4: 415.30, B3: 246.94, E3: 164.81 };
  var CH = [
    [N.GS4, N.FS4, N.E4, N.B3],
    [N.E4, N.GS4, N.FS4, N.B3],
    [N.E4, N.FS4, N.GS4, N.E4],
    [N.GS4, N.E4, N.FS4, N.B3],
    [N.B3, N.FS4, N.GS4, N.E4],
  ];
  var QSEQ = { 0: [1, 2, 3, 4], 1: [0], 2: [1, 2], 3: [3, 4, 0] };

  function westminster(ac, quarter, hour) {
    var t = ac.currentTime, step = 0.62, gap = 0.45;
    (QSEQ[quarter] || [0]).forEach(function (ci) {
      CH[ci].forEach(function (f) { bell(ac, f, t, 2.0, 0.32); t += step; });
      t += gap;
    });
    if (quarter === 0) {
      t += 0.5;
      var strikes = (hour % 12) || 12;
      for (var i = 0; i < strikes; i++) { bell(ac, N.E3, t, 2.6, 0.42); t += 1.2; }
    }
    return t - ac.currentTime;
  }

  function gong(ac) {
    var t = ac.currentTime;
    [880.0, 587.33].forEach(function (f, i) { bell(ac, f, t + i * 0.3, 1.6, 0.34); });
    return 1.9;
  }

  function pips(ac) {
    var t = ac.currentTime;
    for (var i = 0; i < 3; i++) { beep(ac, 1000, t, 0.09); t += 0.2; }
    beep(ac, 1500, t, 0.5);
    return t + 0.5 - ac.currentTime;
  }

  // playChime returns the sound's duration in seconds (so the caller can time
  // the spoken announcement to follow it).
  function playChime(d) {
    var ac = audio();
    if (!ac) return 0;
    d = d || {};
    if (d.style === "gong") return gong(ac);
    if (d.style === "pips") return pips(ac);
    return westminster(ac, d.quarter || 0, d.hour || 0);
  }

  function speak(text) {
    if (!("speechSynthesis" in window) || !text) return;
    var u = new SpeechSynthesisUtterance("Het is " + text);
    u.lang = "nl-BE";
    u.rate = 0.85; // calm, measured — station-announcer cadence
    var voices = window.speechSynthesis.getVoices();
    var nl = voices.find(function (v) { return /nl[-_]BE/i.test(v.lang); }) ||
             voices.find(function (v) { return /^nl/i.test(v.lang); });
    if (nl) u.voice = nl;
    window.speechSynthesis.speak(u);
  }

  var HOURS = ["twaalf", "één", "twee", "drie", "vier", "vijf",
               "zes", "zeven", "acht", "negen", "tien", "elf"];
  function dutchHour(h) { return HOURS[((h % 12) + 12) % 12] + " uur"; }

  // Exposed for the admin tester + reuse.
  window.fpChime = playChime;
  window.fpSpeak = speak;
  window.fpTestChime = function (style) {
    var h = new Date().getHours();
    var dur = playChime({ style: style || "westminster", quarter: 0, hour: h });
    setTimeout(function () { speak(dutchHour(h)); }, (dur + 0.4) * 1000);
  };

  // First interaction unlocks audio for the session.
  function unlock() {
    audio();
    document.removeEventListener("pointerdown", unlock);
    document.removeEventListener("keydown", unlock);
  }
  document.addEventListener("pointerdown", unlock);
  document.addEventListener("keydown", unlock);

  // ---- kiosk only: react to the server's quarter-hour chime event ----
  var es = window.fpES;
  if (es) {
    es.addEventListener("chime", function (e) {
      var d = {};
      try { d = JSON.parse(e.data); } catch (_) {}
      var dur = playChime(d);
      if (d.announce && d.text) {
        setTimeout(function () { speak(d.text); }, (dur + 0.4) * 1000);
      }
    });
  }
})();
