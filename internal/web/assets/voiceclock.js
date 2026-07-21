// Global voice clock: reacts to the server's quarter-hour "chime" SSE event
// (shared EventSource from kiosk.js), plays a short Web-Audio chime, and — on the
// hour — speaks the Dutch time via SpeechSynthesis. Runs on every kiosk screen,
// independent of the current view. The server gates timing + quiet hours, so
// this stays dumb: chime, maybe speak.
(function () {
  const es = window.fpES;
  if (!es) return;

  // Browsers block audio until a user gesture; on a wall with no interaction the
  // AudioContext stays suspended (silent) until someone touches the screen, or
  // until Chromium is launched with --autoplay-policy=no-user-gesture-required.
  let ctx = null;
  function audio() {
    if (!ctx) {
      const AC = window.AudioContext || window.webkitAudioContext;
      if (!AC) return null;
      ctx = new AC();
    }
    if (ctx.state === "suspended") ctx.resume();
    return ctx;
  }

  // A celesta/tubular-bell-style ascending station gong, evoking the NMBS/SNCB
  // platform-announcement jingle (an approximation of the style — not their exact
  // proprietary sample). Each note = fundamental + soft octave, gentle attack and
  // a long bell tail.
  const JINGLE = [
    { f: 587.33, at: 0.0 },  // D5
    { f: 880.0, at: 0.26 },  // A5
    { f: 1108.73, at: 0.52 }, // C#6
    { f: 1174.66, at: 0.9 },  // D6 (resolve)
  ];

  function bell(ac, freq, start) {
    [1, 2].forEach(function (mult, i) {
      const osc = ac.createOscillator();
      const gain = ac.createGain();
      osc.type = "triangle";
      osc.frequency.value = freq * mult;
      osc.connect(gain);
      gain.connect(ac.destination);
      const peak = i === 0 ? 0.34 : 0.12; // octave quieter
      gain.gain.setValueAtTime(0.0001, start);
      gain.gain.exponentialRampToValueAtTime(peak, start + 0.015);
      gain.gain.exponentialRampToValueAtTime(0.0001, start + 1.6);
      osc.start(start);
      osc.stop(start + 1.7);
    });
  }

  function chime() {
    const ac = audio();
    if (!ac) return;
    const now = ac.currentTime;
    JINGLE.forEach(function (n) {
      bell(ac, n.f, now + n.at);
    });
  }

  function speak(text) {
    if (!("speechSynthesis" in window) || !text) return;
    const u = new SpeechSynthesisUtterance("Het is " + text);
    u.lang = "nl-BE";
    u.rate = 0.85; // calm, measured — station-announcer cadence
    u.pitch = 1.0;
    // Prefer an installed Flemish (nl-BE) voice, else any Dutch voice.
    const voices = window.speechSynthesis.getVoices();
    const nlBE = voices.find(function (v) { return /nl[-_]BE/i.test(v.lang); });
    const nl = nlBE || voices.find(function (v) { return /^nl/i.test(v.lang); });
    if (nl) u.voice = nl;
    window.speechSynthesis.speak(u);
  }

  es.addEventListener("chime", function (e) {
    let d = {};
    try {
      d = JSON.parse(e.data);
    } catch (_) {}
    chime();
    if (d.announce && d.text) {
      // Let the jingle ring out, then announce (station-announcer cadence).
      setTimeout(function () {
        speak(d.text);
      }, 2000);
    }
  });

  // First interaction unlocks audio for the rest of the session.
  function unlock() {
    audio();
    document.removeEventListener("pointerdown", unlock);
    document.removeEventListener("keydown", unlock);
  }
  document.addEventListener("pointerdown", unlock);
  document.addEventListener("keydown", unlock);
})();
