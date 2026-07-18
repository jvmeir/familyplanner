package server

import "net/http"

// spaBootstrap is the minimal HTML shell that loads the Go→WASM kiosk client.
// The client (cmd/kioskspa, built to /static/app.wasm) takes over #app and
// renders everything from the JSON API + the existing SSE stream.
const spaBootstrap = `<!DOCTYPE html>
<html lang="nl">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>Family Planner</title>
<meta name="theme-color" content="#0f1220">
<link rel="icon" href="/static/icon-192.png">
<link rel="apple-touch-icon" href="/static/icon-192.png">
<link rel="manifest" href="/manifest.kiosk.webmanifest">
<link rel="stylesheet" href="/static/app.css">
</head>
<body class="spa">
<div id="app" class="spa-loading">Laden…</div>
<script>
if ("serviceWorker" in navigator) {
  window.addEventListener("load", function () {
    navigator.serviceWorker.register("/sw.js").catch(function () {});
  });
}
</script>
<script src="/static/wasm_exec.js"></script>
<script>
(function () {
  if (!("WebAssembly" in window)) {
    document.getElementById("app").textContent = "WebAssembly niet ondersteund.";
    return;
  }
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch("/static/app.wasm"), go.importObject)
    .then(function (res) { go.run(res.instance); })
    .catch(function (err) {
      document.getElementById("app").textContent = "Kiosk laden mislukt: " + err;
    });
})();
</script>
</body>
</html>`

// handleSPA serves the WASM kiosk client's HTML bootstrap. Device-cookie auth is
// enforced by the surrounding kiosk route group, same as /kiosk.
func (s *Server) handleSPA(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(spaBootstrap))
}
