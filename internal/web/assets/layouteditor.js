// Layout editor: drag the gutters between sibling panes to resize. Structural
// edits (split/merge/assign) are HTMX innerHTML swaps of #editor-canvas, so this
// wrapper element (and these listeners) persist across them.
(function () {
  const canvas = document.getElementById("editor-canvas");
  if (!canvas) return;
  const viewID = canvas.dataset.viewId;

  function csrf() {
    const m = document.querySelector('meta[name=csrf-token]');
    return m ? m.content : "";
  }

  let drag = null;

  canvas.addEventListener("pointerdown", function (e) {
    const gutter = e.target.closest(".egutter");
    if (!gutter) return;
    const prev = gutter.previousElementSibling;
    const next = gutter.nextElementSibling;
    if (!prev || !next) return;

    const horizontal = gutter.dataset.dir === "row";
    const pr = prev.getBoundingClientRect();
    const nr = next.getBoundingClientRect();
    const w1 = parseFloat(prev.style.flexGrow) || 1;
    const w2 = parseFloat(next.style.flexGrow) || 1;

    drag = {
      split: gutter.parentElement,
      prev: prev,
      next: next,
      horizontal: horizontal,
      sumPx: horizontal ? pr.width + nr.width : pr.height + nr.height,
      sumW: w1 + w2,
      startPos: horizontal ? pr.left : pr.top,
    };
    gutter.setPointerCapture(e.pointerId);
    e.preventDefault();
  });

  canvas.addEventListener("pointermove", function (e) {
    if (!drag) return;
    const pos = drag.horizontal ? e.clientX : e.clientY;
    let frac = (pos - drag.startPos) / drag.sumPx;
    frac = Math.max(0.08, Math.min(0.92, frac));
    drag.prev.style.flexGrow = (frac * drag.sumW).toFixed(4);
    drag.next.style.flexGrow = ((1 - frac) * drag.sumW).toFixed(4);
  });

  function endDrag() {
    if (!drag) return;
    const split = drag.split;
    drag = null;
    const wraps = Array.from(split.children).filter(function (c) {
      return c.classList.contains("epane-wrap");
    });
    const weights = wraps
      .map(function (wp) { return (parseFloat(wp.style.flexGrow) || 1).toFixed(4); })
      .join(",");
    fetch("/admin/views/" + viewID + "/layout/weights", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-CSRF-Token": csrf() },
      body: "path=" + encodeURIComponent(split.dataset.path || "") + "&weights=" + encodeURIComponent(weights),
    }).catch(function () {});
  }

  canvas.addEventListener("pointerup", endDrag);
  canvas.addEventListener("pointercancel", endDrag);
})();
