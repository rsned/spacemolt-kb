package main

import (
	"bytes"
	"encoding/json"
	htmltpl "html/template"
	"math"
	"os"
)

// The plot is one self-contained page: the data as JSON, drawn on a canvas
// with pan and zoom. Map orientation matches the game (+y is down).

type jsSystem struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Est   bool    `json:"est,omitempty"`
	Class string  `json:"cls,omitempty"`
}

type jsReading struct {
	Obs        string      `json:"obs"`
	X          float64     `json:"x"`
	Y          float64     `json:"y"`
	Bearing    float64     `json:"b"`
	Temp       float64     `json:"t"`
	Brightness int         `json:"br"`
	Status     string      `json:"st"`
	Match      string      `json:"m,omitempty"`
	MatchClass string      `json:"mc,omitempty"`
	MatchAt    *[2]float64 `json:"ma,omitempty"`
	Err        float64     `json:"err,omitempty"`
	Dist       float64     `json:"d,omitempty"`
}

type jsMember struct {
	Obs string  `json:"obs"`
	B   float64 `json:"b"`
	Br  int     `json:"br"`
	D   float64 `json:"d"`
}

type jsJump struct {
	From    string  `json:"from"`
	Heading float64 `json:"h"`
	Tol     float64 `json:"tol"`
	D       float64 `json:"d"`
	Blocker string  `json:"blk,omitempty"`
	BlockAt float64 `json:"blkd,omitempty"`
}

type jsFit struct {
	Temp    float64    `json:"t"`
	X       float64    `json:"x"`
	Y       float64    `json:"y"`
	R       float64    `json:"r"`
	MaxErr  float64    `json:"err"`
	Major   float64    `json:"maj"`
	Minor   float64    `json:"min"`
	Angle   float64    `json:"ang"`
	Members []jsMember `json:"m"`
	Jumps   []jsJump   `json:"j"`
}

func writeHTML(path string, systems []System, obs []Observation, rs []Reading, fits []Fit, lim Limits) error {
	var js struct {
		Systems   []jsSystem  `json:"systems"`
		Observers []string    `json:"observers"`
		Readings  []jsReading `json:"readings"`
		Fits      []jsFit     `json:"fits"`
		Limits    Limits      `json:"limits"`
	}
	js.Limits = lim
	names := map[string]string{}
	for _, s := range systems {
		names[s.ID] = s.Name
		js.Systems = append(js.Systems, jsSystem{ID: s.ID, Name: s.Name, X: s.At.X, Y: s.At.Y, Est: s.Estimated, Class: s.Star.Class})
	}
	for _, o := range obs {
		js.Observers = append(js.Observers, o.System)
	}
	for _, r := range rs {
		j := jsReading{Obs: r.Obs, X: r.From.X, Y: r.From.Y, Bearing: r.S.Bearing, Temp: r.S.Temperature, Brightness: r.S.Brightness, Status: r.Status}
		if m := r.Match; m != nil {
			j.Match, j.MatchClass, j.Err, j.Dist = names[m.Star.System], m.Star.Class, round1(m.Err), math.Round(m.Distance)
			j.MatchAt = &[2]float64{m.Star.At.X, m.Star.At.Y}
		}
		js.Readings = append(js.Readings, j)
	}
	for _, f := range fits {
		j := jsFit{Temp: f.Temp, X: math.Round(f.At.X), Y: math.Round(f.At.Y), R: math.Round(math.Hypot(f.At.X, f.At.Y)),
			MaxErr: round1(f.MaxErr), Major: math.Round(f.Major), Minor: math.Round(f.Minor), Angle: f.Angle}
		for _, m := range f.Members {
			j.Members = append(j.Members, jsMember{Obs: m.Obs, B: m.S.Bearing, Br: m.S.Brightness,
				D: math.Round(math.Hypot(f.At.X-m.From.X, f.At.Y-m.From.Y))})
		}
		for _, jp := range jumpChecks(f, systems) {
			jj := jsJump{From: jp.From, Heading: math.Round(jp.Heading*100) / 100, Tol: math.Round(jp.Tol*100) / 100, D: math.Round(jp.Distance)}
			if !jp.Clear {
				jj.Blocker, jj.BlockAt = names[jp.Blocker], math.Round(jp.BlockAt)
			}
			j.Jumps = append(j.Jumps, jj)
		}
		js.Fits = append(js.Fits, j)
	}
	data, err := json.Marshal(js)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	if err := pageTmpl.Execute(&b, map[string]any{"Data": htmltpl.JS(data)}); err != nil { //nolint:gosec // our own JSON
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

var pageTmpl = htmltpl.Must(htmltpl.New("page").Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Telescope Sightings</title>
<style>
:root{--bg:#0b0f17;--panel:#111827;--fg:#d8dee9;--muted:#8b95ab;--line:#263042;--unk:#f472b6;--dbt:#f59e0b;--ok:#34d399;--obs:#38bdf8}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:14px/1.45 system-ui,sans-serif}
header{padding:12px 16px;border-bottom:1px solid var(--line)}
h1{font-size:17px;margin:0 0 4px}
.sub{color:var(--muted);font-size:13px}
.bar{display:flex;flex-wrap:wrap;gap:14px;align-items:center;padding:8px 16px;font-size:13px;color:var(--muted)}
.bar label{display:flex;gap:5px;align-items:center;cursor:pointer}
.key{display:inline-block;width:18px;height:0;border-top:2px solid;vertical-align:middle;margin-right:4px}
#wrap{position:relative;margin:0 16px;border:1px solid var(--line);border-radius:6px;overflow:hidden}
canvas{display:block;width:100%;height:72vh;cursor:grab;touch-action:none}
canvas.drag{cursor:grabbing}
#tip{position:absolute;pointer-events:none;background:var(--panel);border:1px solid var(--line);border-radius:4px;padding:4px 8px;font-size:12px;display:none;white-space:nowrap}
button{background:var(--panel);color:var(--fg);border:1px solid var(--line);border-radius:4px;padding:3px 10px;cursor:pointer}
section{padding:12px 16px 32px}
h2{font-size:15px;margin:16px 0 6px}
table{border-collapse:collapse;width:100%;font-size:13px}
th,td{text-align:left;padding:4px 8px;border-bottom:1px solid var(--line)}
th{color:var(--muted);font-weight:500}
td.r,th.r{text-align:right}
.unknown{color:var(--unk)}.doubtful{color:var(--dbt)}.match{color:var(--ok)}
tr[data-i]{cursor:pointer}tr[data-i]:hover{background:#1a2333}
</style></head><body>
<header><h1>Telescope sightings</h1>
<div class="sub">Every charted system, with the bearing line of each reading no known star clearly explains. Where lines of the same temperature agree, a fitted star is marked with its error ellipse (2σ, assuming <span id="xe"></span>° bearing error); a long ellipse means the direction is known but the distance is not. Drag to pan, scroll or pinch to zoom.</div></header>
<div class="bar">
<span><span class="key" style="border-color:var(--unk)"></span>unknown</span>
<span><span class="key" style="border-color:var(--dbt);border-top-style:dashed"></span>doubtful match</span>
<label><input type="checkbox" id="showMatch"> clear matches</label>
<label><input type="checkbox" id="showLabels" checked> labels</label>
<button id="fit">Fit galaxy</button><button id="focus">Zoom to readings</button>
</div>
<div id="wrap"><canvas id="c"></canvas><div id="tip"></div></div>
<section>
<h2>Fitted unknown stars</h2><table id="xt"><thead><tr><th>Temp</th><th class="r">Position</th><th class="r">From centre</th><th class="r">±1σ</th><th class="r">Worst miss</th><th>Readings</th><th>Pathfinder</th></tr></thead><tbody></tbody></table>
<h2>Unclear readings</h2><table id="rt"><thead><tr><th>From</th><th class="r">Bearing</th><th class="r">Temp</th><th class="r">Bright</th><th>Status</th><th>Best match</th></tr></thead><tbody></tbody></table>
</section>
<script>
const D = {{.Data}};
const cv = document.getElementById('c'), ctx = cv.getContext('2d'), tip = document.getElementById('tip');
document.getElementById('xe').textContent = '±' + D.limits.Sigma;
const showMatch = document.getElementById('showMatch'), showLabels = document.getElementById('showLabels');
let view = {cx: 0, cy: 0, s: 0.05}, hi = -1;
const obsSet = new Set(D.observers);
const unclear = D.readings.filter(r => r.st !== 'match');
function size() { const r = cv.getBoundingClientRect(), d = devicePixelRatio || 1; cv.width = r.width * d; cv.height = r.height * d; ctx.setTransform(d, 0, 0, d, 0, 0); }
function W() { return cv.width / (devicePixelRatio || 1); } function H() { return cv.height / (devicePixelRatio || 1); }
function px(x, y) { return [W() / 2 + (x - view.cx) * view.s, H() / 2 + (y - view.cy) * view.s]; }
function world(sx, sy) { return [view.cx + (sx - W() / 2) / view.s, view.cy + (sy - H() / 2) / view.s]; }
function fitTo(pts, pad) {
  const xs = pts.map(p => p[0]), ys = pts.map(p => p[1]);
  const x0 = Math.min(...xs), x1 = Math.max(...xs), y0 = Math.min(...ys), y1 = Math.max(...ys);
  view.cx = (x0 + x1) / 2; view.cy = (y0 + y1) / 2;
  view.s = Math.min(W() / (x1 - x0 + 2 * pad), H() / (y1 - y0 + 2 * pad));
}
const fitGalaxy = () => { fitTo(D.systems.map(s => [s.x, s.y]), 300); draw(); };
const fitReadings = () => {
  const pts = D.readings.map(r => [r.x, r.y]).concat(D.fits.map(c => [c.x, c.y]));
  if (pts.length) fitTo(pts, 900); draw();
};
function ray(r, col, dash, w) {
  const a = r.b * Math.PI / 180, L = 40000;
  const [x0, y0] = px(r.x, r.y), [x1, y1] = px(r.x + L * Math.cos(a), r.y + L * Math.sin(a));
  ctx.strokeStyle = col; ctx.lineWidth = w; ctx.setLineDash(dash); ctx.beginPath(); ctx.moveTo(x0, y0); ctx.lineTo(x1, y1); ctx.stroke(); ctx.setLineDash([]);
}
function css(v) { return getComputedStyle(document.documentElement).getPropertyValue(v).trim(); }
function draw() {
  const C = {unk: css('--unk'), dbt: css('--dbt'), ok: css('--ok'), obs: css('--obs'), muted: css('--muted'), fg: css('--fg')};
  ctx.clearRect(0, 0, W(), H());
  // old rim, for scale
  const [ox, oy] = px(0, 0); ctx.strokeStyle = '#ffffff1a'; ctx.setLineDash([3, 5]); ctx.beginPath(); ctx.arc(ox, oy, 7579 * view.s, 0, 7); ctx.stroke(); ctx.setLineDash([]);
  if (showMatch.checked) for (const r of D.readings) if (r.st === 'match') {
    const [a, b] = px(r.x, r.y), [c, d] = px(r.ma[0], r.ma[1]);
    ctx.strokeStyle = C.ok + '66'; ctx.lineWidth = 1; ctx.beginPath(); ctx.moveTo(a, b); ctx.lineTo(c, d); ctx.stroke();
  }
  unclear.forEach((r, i) => {
    const on = hi === -1 || hi === i;
    ray(r, (r.st === 'unknown' ? C.unk : C.dbt) + (on ? 'cc' : '33'), r.st === 'unknown' ? [] : [6, 5], hi === i ? 2.5 : 1.3);
    if (r.st === 'doubtful' && on) {
      const [a, b] = px(r.x, r.y), [c, d] = px(r.ma[0], r.ma[1]);
      ctx.strokeStyle = C.dbt + '55'; ctx.setLineDash([2, 4]); ctx.beginPath(); ctx.moveTo(a, b); ctx.lineTo(c, d); ctx.stroke(); ctx.setLineDash([]);
    }
  });
  for (const f of D.fits) {
    const [a, b] = px(f.x, f.y);
    ctx.save(); ctx.translate(a, b); ctx.rotate(f.ang);
    ctx.fillStyle = C.unk + '2a'; ctx.strokeStyle = C.unk; ctx.lineWidth = 1; ctx.beginPath();
    ctx.ellipse(0, 0, Math.max(2 * f.maj * view.s, 3), Math.max(2 * f.min * view.s, 3), 0, 0, 7); ctx.fill(); ctx.stroke(); ctx.restore();
    ctx.strokeStyle = '#fff'; ctx.lineWidth = 1.5;
    ctx.beginPath(); ctx.moveTo(a - 6, b - 6); ctx.lineTo(a + 6, b + 6); ctx.moveTo(a + 6, b - 6); ctx.lineTo(a - 6, b + 6); ctx.stroke();
    ctx.fillStyle = '#fff'; ctx.font = '600 12px system-ui'; ctx.fillText(f.t + ' K · ' + f.m.length + ' readings', a + 9, b - 8);
  }
  const labelAll = view.s > 0.12;
  for (const s of D.systems) {
    const [a, b] = px(s.x, s.y); if (a < -20 || b < -20 || a > W() + 20 || b > H() + 20) continue;
    const o = obsSet.has(s.id);
    ctx.fillStyle = o ? C.obs : s.est ? C.unk : '#8b95ab'; ctx.beginPath(); ctx.arc(a, b, o ? 4 : s.est ? 3 : 1.8, 0, 7); ctx.fill();
    if (o) { ctx.strokeStyle = C.obs; ctx.lineWidth = 1; ctx.beginPath(); ctx.arc(a, b, 8, 0, 7); ctx.stroke(); }
    if (showLabels.checked && (o || s.est || labelAll)) {
      ctx.fillStyle = o ? C.obs : s.est ? C.unk : C.muted; ctx.font = (o ? '600 ' : '') + '11px system-ui'; ctx.fillText(s.name, a + 7, b + 4);
    }
  }
}
// pan and zoom (mouse, wheel, touch)
let drag = null, pinch = null;
cv.addEventListener('pointerdown', e => { drag = {x: e.clientX, y: e.clientY, cx: view.cx, cy: view.cy}; cv.setPointerCapture(e.pointerId); cv.classList.add('drag'); });
cv.addEventListener('pointerup', () => { drag = null; cv.classList.remove('drag'); });
cv.addEventListener('pointermove', e => {
  if (drag && !pinch) { view.cx = drag.cx - (e.clientX - drag.x) / view.s; view.cy = drag.cy - (e.clientY - drag.y) / view.s; draw(); return; }
  const r = cv.getBoundingClientRect(), mx = e.clientX - r.left, my = e.clientY - r.top;
  let best = null, bd = 12;
  for (const s of D.systems) { const [a, b] = px(s.x, s.y), d = Math.hypot(a - mx, b - my); if (d < bd) { bd = d; best = s; } }
  if (best) { tip.style.display = 'block'; tip.style.left = (mx + 12) + 'px'; tip.style.top = (my + 12) + 'px'; tip.textContent = best.name + (best.cls ? ' · ' + best.cls : '') + (best.est ? ' · estimated position' : ''); }
  else tip.style.display = 'none';
});
cv.addEventListener('wheel', e => {
  e.preventDefault(); const r = cv.getBoundingClientRect(), mx = e.clientX - r.left, my = e.clientY - r.top;
  const [wx, wy] = world(mx, my); view.s *= Math.exp(-e.deltaY * 0.0015);
  view.cx = wx - (mx - W() / 2) / view.s; view.cy = wy - (my - H() / 2) / view.s; draw();
}, {passive: false});
cv.addEventListener('touchstart', e => { if (e.touches.length === 2) pinch = {d: Math.hypot(e.touches[0].clientX - e.touches[1].clientX, e.touches[0].clientY - e.touches[1].clientY), s: view.s}; }, {passive: true});
cv.addEventListener('touchmove', e => { if (pinch && e.touches.length === 2) { view.s = pinch.s * Math.hypot(e.touches[0].clientX - e.touches[1].clientX, e.touches[0].clientY - e.touches[1].clientY) / pinch.d; draw(); } }, {passive: true});
cv.addEventListener('touchend', () => { pinch = null; });
showMatch.onchange = draw; showLabels.onchange = draw;
document.getElementById('fit').onclick = fitGalaxy; document.getElementById('focus').onclick = fitReadings;
window.addEventListener('resize', () => { size(); draw(); });
// tables
const name = id => (D.systems.find(s => s.id === id) || {name: id}).name;
const xt = document.querySelector('#xt tbody');
if (!D.fits.length) xt.innerHTML = '<tr><td colspan="7" class="sub">None yet: an unknown star needs agreeing readings from two systems.</td></tr>';
for (const f of D.fits) xt.insertAdjacentHTML('beforeend', '<tr><td>' + f.t + ' K</td><td class="r">(' + f.x + ', ' + f.y + ')</td><td class="r">' + f.r + '</td><td class="r">' + f.maj + ' × ' + f.min + '</td><td class="r">' + f.err + '°</td><td>' + f.m.map(m => name(m.obs) + ' ' + m.b + '° b' + m.br).join(', ') + '</td></tr>');
const rt = document.querySelector('#rt tbody');
unclear.forEach((r, i) => rt.insertAdjacentHTML('beforeend', '<tr data-i="' + i + '"><td>' + name(r.obs) + '</td><td class="r">' + r.b + '°</td><td class="r">' + r.t + ' K</td><td class="r">' + r.br + '</td><td class="' + r.st + '">' + r.st + '</td><td>' + (r.m ? r.m + ' (' + r.mc + '), ' + r.err + '° off, ' + r.d + ' away' : '—') + '</td></tr>'));
rt.querySelectorAll('tr').forEach(tr => {
  tr.onmouseenter = () => { hi = +tr.dataset.i; draw(); };
  tr.onmouseleave = () => { hi = -1; draw(); };
});
size(); fitReadings();
</script></body></html>
`))
