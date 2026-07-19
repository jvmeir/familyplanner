//go:build js && wasm

// Command kioskspa is the Family Planner kiosk client compiled to WebAssembly.
// It is the "SPA" half of the API+SPA kiosk: it fetches state + the current
// view from the JSON API (/api/kiosk/*), renders them into DOM that mirrors the
// server-rendered kiosk (so the existing app.css styles it), drives playback via
// the existing /kiosk/control endpoints, and reacts to the existing
// /kiosk/stream SSE feed for navigate/refresh. Both server and client are Go.
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"syscall/js"

	"github.com/jvmeir/familyplanner/internal/health"
	"github.com/jvmeir/familyplanner/internal/kioskapi"
)

var (
	doc = js.Global().Get("document")
	win = js.Global()
)

// app holds references to the persistent shell nodes so refreshes only swap the
// pieces that changed (stage content, footer label, jump options).
type app struct {
	dateEl    js.Value
	timeEl    js.Value
	stageEl   js.Value
	viewHost  js.Value
	healthEl  js.Value
	plNameEl  js.Value
	viewEl    js.Value
	jumpSel   js.Value
	currentID int64
	state     kioskapi.State
}

func main() {
	a := &app{}
	a.mountShell()
	a.startClock()
	go a.refreshState()
	a.startSSE()
	select {} // keep the Go runtime alive; all work is event-driven from here
}

// ---------- DOM helpers ----------

func el(tag string) js.Value { return doc.Call("createElement", tag) }

func setText(v js.Value, s string) { v.Set("textContent", s) }

func setClass(v js.Value, c string) { v.Set("className", c) }

func setAttr(v js.Value, k, val string) { v.Call("setAttribute", k, val) }

func clear(v js.Value) { v.Set("innerHTML", "") }

func appendChild(parent, child js.Value) { parent.Call("appendChild", child) }

func onClick(v js.Value, fn func()) {
	v.Call("addEventListener", "click", js.FuncOf(func(js.Value, []js.Value) any {
		fn()
		return nil
	}))
}

// ---------- shell ----------

// mountShell builds the static header/stage/footer once, mirroring kiosk.templ.
func (a *app) mountShell() {
	root := doc.Call("getElementById", "app")
	clear(root)
	setClass(root, "")
	setAttr(root, "class", "")

	header := el("header")
	setClass(header, "kheader")
	a.dateEl = el("span")
	setClass(a.dateEl, "kdate")
	a.timeEl = el("span")
	setClass(a.timeEl, "ktime")
	appendChild(header, a.dateEl)
	appendChild(header, a.timeEl)

	a.stageEl = el("div")
	a.stageEl.Set("id", "stage")
	// The view content lives in a host so the health badge (a sibling) survives
	// view swaps; the badge is a positioned corner overlay, hidden when healthy.
	a.viewHost = el("div")
	setClass(a.viewHost, "stage-host")
	a.healthEl = el("div")
	setClass(a.healthEl, "khealth")
	a.healthEl.Get("style").Set("display", "none")
	appendChild(a.stageEl, a.viewHost)
	appendChild(a.stageEl, a.healthEl)

	footer := el("footer")
	setClass(footer, "kfooter")

	label := el("span")
	setClass(label, "kfooter-label")
	a.plNameEl = el("span")
	sep := el("span")
	setClass(sep, "sep")
	setText(sep, "·")
	a.viewEl = el("span")
	appendChild(label, a.plNameEl)
	appendChild(label, sep)
	appendChild(label, a.viewEl)

	controls := el("span")
	setClass(controls, "kfooter-controls")
	appendChild(controls, a.ctlButton("⏮", "prev"))
	appendChild(controls, a.ctlButton("⏸", "pause"))
	appendChild(controls, a.ctlButton("▶", "resume"))
	appendChild(controls, a.ctlButton("⏭", "next"))

	a.jumpSel = el("select")
	a.jumpSel.Set("id", "kjump")
	a.jumpSel.Call("addEventListener", "change", js.FuncOf(func(this js.Value, _ []js.Value) any {
		if v := this.Get("value").String(); v != "" {
			go a.goTo(v)
		}
		return nil
	}))
	appendChild(controls, a.jumpSel)

	appendChild(footer, label)
	appendChild(footer, controls)

	appendChild(root, header)
	appendChild(root, a.stageEl)
	appendChild(root, footer)
}

func (a *app) ctlButton(label, cmd string) js.Value {
	b := el("button")
	setText(b, label)
	onClick(b, func() { go a.control(cmd) })
	return b
}

// ---------- data + rendering ----------

func (a *app) refreshState() {
	var st kioskapi.State
	if err := getJSON("/api/kiosk/state", &st); err != nil {
		return
	}
	a.state = st
	setText(a.plNameEl, st.PlaylistName)
	a.renderJump()
	if a.currentID == 0 {
		a.currentID = st.CurrentID
	}
	a.renderViewLabel()
	go a.loadView(a.currentID)
	go a.refreshHealth()
}

func (a *app) renderJump() {
	clear(a.jumpSel)
	placeholder := el("option")
	placeholder.Set("value", "")
	setText(placeholder, "Ga naar…")
	appendChild(a.jumpSel, placeholder)
	for _, v := range a.state.Playlist {
		appendChild(a.jumpSel, a.option(v))
	}
	if len(a.state.All) > 0 {
		grp := el("optgroup")
		setAttr(grp, "label", "Alle weergaven")
		for _, v := range a.state.All {
			appendChild(grp, a.option(v))
		}
		appendChild(a.jumpSel, grp)
	}
}

func (a *app) option(v kioskapi.ViewRef) js.Value {
	o := el("option")
	o.Set("value", strconv.FormatInt(v.ID, 10))
	setText(o, v.Name)
	if v.ID == a.currentID {
		o.Set("selected", true)
	}
	return o
}

func (a *app) renderViewLabel() {
	name := ""
	for _, v := range append(append([]kioskapi.ViewRef{}, a.state.Playlist...), a.state.All...) {
		if v.ID == a.currentID {
			name = v.Name
			break
		}
	}
	setText(a.viewEl, name)
}

func (a *app) loadView(id int64) {
	if id == 0 {
		return
	}
	var vr kioskapi.ViewRender
	if err := getJSON("/api/kiosk/view/"+strconv.FormatInt(id, 10), &vr); err != nil {
		return
	}
	view := el("div")
	setClass(view, "view")
	setAttr(view, "data-mode", "day")
	setAttr(view, "style", vr.ThemeVars)
	appendChild(view, buildLayout(vr.Layout))

	clear(a.viewHost)
	a.stageEl.Call("setAttribute", "data-view-id", strconv.FormatInt(id, 10))
	appendChild(a.viewHost, view)
	a.renderViewLabel()
}

// refreshHealth pulls the health summary and repaints the corner badge.
func (a *app) refreshHealth() {
	var sum health.Summary
	if err := getJSON("/api/kiosk/health", &sum); err != nil {
		return
	}
	a.renderHealth(sum)
}

func (a *app) renderHealth(sum health.Summary) {
	lvl := string(sum.Level)
	if lvl != "warn" && lvl != "error" {
		a.healthEl.Get("style").Set("display", "none")
		return
	}
	setClass(a.healthEl, "khealth khealth-"+lvl)
	a.healthEl.Get("style").Set("display", "")
	clear(a.healthEl)
	appendChild(a.healthEl, span("khealth-dot", ""))
	msg := ""
	if len(sum.Issues) > 0 {
		msg = sum.Issues[0].Message
	}
	appendChild(a.healthEl, span("khealth-msg", msg))
	if sum.Count > 1 {
		appendChild(a.healthEl, span("khealth-more", "+"+strconv.Itoa(sum.Count-1)))
	}
}

// buildLayout mirrors web.LayoutPane: a leaf renders a widget cell; a split
// renders weighted nested panes.
func buildLayout(n kioskapi.Layout) js.Value {
	if n.Cell != nil {
		return buildCell(*n.Cell)
	}
	split := el("div")
	setClass(split, "split dir-"+n.Dir)
	for _, c := range n.Children {
		pane := el("div")
		setClass(pane, "pane")
		w := c.Weight
		if w <= 0 {
			w = 1
		}
		setAttr(pane, "style", "flex:"+strconv.FormatFloat(w, 'f', -1, 64)+" 1 0;min-width:0;min-height:0;")
		appendChild(pane, buildLayout(c.Node))
		appendChild(split, pane)
	}
	return split
}

// buildCell mirrors web.widgetCell (cell.templ), reusing the same class names so
// app.css styles it identically to the server-rendered kiosk.
func buildCell(c kioskapi.Cell) js.Value {
	if c.Kind == "empty" {
		w := el("div")
		setClass(w, "widget empty")
		return w
	}
	if c.IframeURL != "" {
		w := el("div")
		setClass(w, "widget")
		f := el("iframe")
		setClass(f, "w-iframe")
		f.Set("src", c.IframeURL)
		setAttr(f, "sandbox", "allow-scripts allow-same-origin allow-popups allow-forms")
		setAttr(f, "loading", "lazy")
		appendChild(w, f)
		return w
	}
	if c.ImageURL != "" {
		w := el("div")
		setClass(w, "widget photo")
		img := el("img")
		setClass(img, "w-photo")
		img.Set("src", c.ImageURL)
		setAttr(img, "alt", "")
		appendChild(w, img)
		return w
	}

	w := el("div")
	setClass(w, "widget")
	if c.Title != "" {
		w.Call("appendChild", div("w-title", c.Title))
	}
	if c.Big != "" {
		w.Call("appendChild", div("w-big", c.Big))
	}
	if c.Sub != "" {
		w.Call("appendChild", div("w-sub", c.Sub))
	}
	if c.Body != "" {
		p := el("p")
		setClass(p, "w-body")
		setText(p, c.Body)
		appendChild(w, p)
	}
	if len(c.Lines) > 0 {
		ul := el("ul")
		setClass(ul, "w-list")
		setAttr(ul, "style", "--n:"+strconv.Itoa(len(c.Lines)))
		for _, l := range c.Lines {
			li := el("li")
			setText(li, l)
			appendChild(ul, li)
		}
		appendChild(w, ul)
	}
	if len(c.Schedule) > 0 {
		appendChild(w, buildSchedule(c))
	}
	if c.Month != nil {
		appendChild(w, buildMonth(c.Month))
	}
	if c.Stale {
		b := el("span")
		setClass(b, "badge stale")
		setText(b, "verouderd")
		appendChild(w, b)
	}
	return w
}

func buildSchedule(c kioskapi.Cell) js.Value {
	if c.ScheduleTable {
		tbl := el("table")
		setClass(tbl, "w-schedtable")
		tb := el("tbody")
		for _, d := range c.Schedule {
			tr := el("tr")
			if d.Today {
				setClass(tr, "today")
			}
			th := el("th")
			setText(th, d.Label)
			appendChild(tr, th)
			td := el("td")
			if len(d.Events) == 0 {
				sp := el("span")
				setClass(sp, "sched-empty")
				setText(sp, "—")
				appendChild(td, sp)
			} else {
				for _, ev := range d.Events {
					sp := el("span")
					setClass(sp, "sched-ev")
					setText(sp, ev)
					appendChild(td, sp)
				}
			}
			appendChild(tr, td)
			appendChild(tb, tr)
		}
		appendChild(tbl, tb)
		return tbl
	}
	wrap := el("div")
	setClass(wrap, "w-sched")
	for _, d := range c.Schedule {
		row := el("div")
		cls := "sched-day"
		if d.Today {
			cls += " today"
		}
		setClass(row, cls)
		lbl := el("span")
		setClass(lbl, "sched-label")
		setText(lbl, d.Label)
		appendChild(row, lbl)
		if len(d.Events) == 0 {
			sp := el("span")
			setClass(sp, "sched-empty")
			setText(sp, "—")
			appendChild(row, sp)
		} else {
			evs := el("span")
			setClass(evs, "sched-events")
			for _, ev := range d.Events {
				sp := el("span")
				setClass(sp, "sched-ev")
				setText(sp, ev)
				appendChild(evs, sp)
			}
			appendChild(row, evs)
		}
		appendChild(wrap, row)
	}
	return wrap
}

func buildMonth(m *kioskapi.Month) js.Value {
	wrap := el("div")
	setClass(wrap, "w-month")
	appendChild(wrap, div("wm-title", m.Title))
	tbl := el("table")
	setClass(tbl, "wm-grid")
	head := el("thead")
	htr := el("tr")
	for _, wd := range m.Weekdays {
		th := el("th")
		setText(th, wd)
		appendChild(htr, th)
	}
	appendChild(head, htr)
	appendChild(tbl, head)
	body := el("tbody")
	for _, week := range m.Weeks {
		tr := el("tr")
		for _, day := range week {
			td := el("td")
			cls := "wm-day"
			if !day.InMonth {
				cls += " out"
			}
			if day.Today {
				cls += " today"
			}
			setClass(td, cls)
			appendChild(td, span("wm-num", strconv.Itoa(day.Day)))
			for i, ev := range day.Events {
				if i >= 2 {
					break
				}
				appendChild(td, span("wm-ev", ev))
			}
			if len(day.Events) > 2 {
				appendChild(td, span("wm-more", "+"+strconv.Itoa(len(day.Events)-2)))
			}
			appendChild(tr, td)
		}
		appendChild(body, tr)
	}
	appendChild(tbl, body)
	appendChild(wrap, tbl)
	return wrap
}

func div(class, text string) js.Value {
	d := el("div")
	setClass(d, class)
	setText(d, text)
	return d
}

func span(class, text string) js.Value {
	s := el("span")
	setClass(s, class)
	setText(s, text)
	return s
}

// ---------- controls ----------

func (a *app) control(cmd string) {
	post("/kiosk/control/" + cmd)
}

func (a *app) goTo(viewID string) {
	post("/kiosk/control/goto?view=" + viewID)
}

// ---------- clock ----------

func (a *app) startClock() {
	tick := js.FuncOf(func(js.Value, []js.Value) any {
		a.tickClock()
		return nil
	})
	a.tickClock()
	win.Call("setInterval", tick, 1000)
}

func (a *app) tickClock() {
	d := win.Get("Date").New()
	dateOpts := map[string]any{"weekday": "long", "day": "numeric", "month": "long"}
	timeOpts := map[string]any{"hour": "2-digit", "minute": "2-digit"}
	setText(a.dateEl, d.Call("toLocaleDateString", "nl-BE", js.ValueOf(dateOpts)).String())
	setText(a.timeEl, d.Call("toLocaleTimeString", "nl-BE", js.ValueOf(timeOpts)).String())
}

// ---------- SSE ----------

func (a *app) startSSE() {
	es := win.Get("EventSource").New("/kiosk/stream")
	es.Call("addEventListener", "navigate", js.FuncOf(func(_ js.Value, args []js.Value) any {
		data := args[0].Get("data").String()
		if id, err := strconv.ParseInt(data, 10, 64); err == nil && id != a.currentID {
			a.currentID = id
			go a.loadView(id)
		}
		return nil
	}))
	es.Call("addEventListener", "refresh", js.FuncOf(func(_ js.Value, args []js.Value) any {
		// Periodic in-view data refresh (e.g. the clock widget). Reload current
		// view + re-check health so the badge tracks live sync/auth state.
		go a.loadView(a.currentID)
		go a.refreshHealth()
		return nil
	}))
}

// ---------- HTTP ----------

func getJSON(url string, out any) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func post(url string) {
	resp, err := http.Post(url, "text/plain", nil)
	if err == nil {
		resp.Body.Close()
	}
}
