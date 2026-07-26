// PDF widget renderer (pdf.js). Renders a widget's uploaded PDF into its .w-pdf
// container, either as a scrollable document (fit-width, zoomable, uses the
// generic cell-scroll for auto/manual scroll) or, when data-pdf-interval > 0, as
// a slideshow/movie: one page per interval, playing to the last page and then
// (in an advance-on-end view) signalling the rotation to move on.
import * as pdfjsLib from "/static/pdf.min.mjs";
pdfjsLib.GlobalWorkerOptions.workerSrc = "/static/pdf.worker.min.mjs";

(function () {
  function clamp(z) { return Math.max(0.3, Math.min(5, z)); }
  function range(a, b) { var out = []; for (var i = a; i <= b; i++) out.push(i); return out; }

  // (Re)initialize every not-yet-set-up .w-pdf in root. opts.onEnd fires when a
  // slideshow pdf plays past its last page (used for advance-on-end views).
  window.fpSetupPdfs = function (root, opts) {
    opts = opts || {};
    if (!root) return;
    root.querySelectorAll(".w-pdf[data-pdf-url]").forEach(function (el) {
      if (el.__fpPdf) return;
      setupPdf(el, opts.onEnd || null);
    });
  };

  function setupPdf(el, onEnd) {
    var st = {
      url: el.dataset.pdfUrl,
      fit: el.dataset.pdfFit || "width",
      interval: parseInt(el.dataset.pdfInterval, 10) || 0,
      zoom: 1, page: 1, doc: null, pages: 0, timer: null, onEnd: onEnd, rendering: false,
    };
    st.slideshow = st.interval > 0;
    el.__fpPdf = st;
    el.innerHTML = '<div class="w-pdf-empty">…</div>';
    pdfjsLib.getDocument(st.url).promise.then(function (doc) {
      st.doc = doc; st.pages = doc.numPages;
      render(el);
      if (st.slideshow) scheduleSlide(el);
    }).catch(function () {
      el.innerHTML = '<div class="w-pdf-empty">PDF laden mislukt</div>';
    });
  }

  // Render the current state: all pages (document) or one page (slideshow).
  function render(el) {
    var st = el.__fpPdf;
    if (!st || !st.doc || st.rendering) return;
    st.rendering = true;
    var cw = el.clientWidth || 800, ch = el.clientHeight || 600;
    var pageNums = st.slideshow ? [st.page] : range(1, st.pages);
    var frag = document.createDocumentFragment();
    var chain = Promise.resolve();
    pageNums.forEach(function (n) {
      chain = chain.then(function () {
        return st.doc.getPage(n).then(function (page) {
          var vp1 = page.getViewport({ scale: 1 });
          var base = (st.slideshow || st.fit === "page")
            ? Math.min(cw / vp1.width, ch / vp1.height)
            : cw / vp1.width;
          var vp = page.getViewport({ scale: base * st.zoom });
          var canvas = document.createElement("canvas");
          canvas.className = "w-pdf-page" + (st.slideshow ? " w-pdf-slide" : "");
          canvas.width = Math.floor(vp.width); canvas.height = Math.floor(vp.height);
          frag.appendChild(canvas);
          return page.render({ canvasContext: canvas.getContext("2d"), viewport: vp }).promise;
        });
      });
    });
    chain.then(function () {
      el.innerHTML = "";
      el.appendChild(frag);
      st.rendering = false;
    }).catch(function () { st.rendering = false; });
  }

  function scheduleSlide(el) {
    var st = el.__fpPdf;
    if (!st) return;
    clearTimeout(st.timer);
    st.timer = setTimeout(function () {
      if (!document.body.contains(el)) return;
      if (st.page >= st.pages) {
        if (st.onEnd) { st.onEnd(); return; } // play to end → advance the rotation
        st.page = 1; // otherwise loop
      } else {
        st.page++;
      }
      render(el);
      scheduleSlide(el);
    }, Math.max(2, st.interval) * 1000);
  }

  // Zoom: delta +1 in, -1 out, 0 = reset to fit. Called from the control buttons
  // (btn is the button) and from fpPdfZoomEl (the .w-pdf element directly).
  window.fpPdfZoom = function (btn, delta) {
    var w = btn.closest(".widget");
    var el = w && w.querySelector(".w-pdf");
    if (el) window.fpPdfZoomEl(el, delta);
  };
  window.fpPdfZoomEl = function (el, delta) {
    var st = el.__fpPdf;
    if (!st) return;
    st.zoom = delta === 0 ? 1 : clamp(st.zoom * (delta > 0 ? 1.25 : 0.8));
    render(el);
  };

  // Manually step a slideshow PDF to the next/prev page (focused-widget control),
  // resetting the auto-advance timer. No-op in document (scroll) mode.
  window.fpPdfStep = function (el, dir) {
    var st = el.__fpPdf;
    if (!st || !st.slideshow || !st.doc) return;
    if (dir >= 0) st.page = st.page >= st.pages ? 1 : st.page + 1;
    else st.page = st.page <= 1 ? st.pages : st.page - 1;
    render(el);
    scheduleSlide(el);
  };

  // Render the initially inline screen once the module has loaded (kiosk.js runs
  // before this deferred module, so its first fpSetupPdfs call is a no-op).
  var stage = document.getElementById("stage");
  if (stage) {
    var v = stage.querySelector(".view");
    var onEnd = v && v.dataset.advanceOnEnd === "1" ? function () { if (window.fpCtl) window.fpCtl("next"); } : null;
    window.fpSetupPdfs(stage, { onEnd: onEnd });
  }
})();
