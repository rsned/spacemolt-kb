package main

import (
	"encoding/json"
	htmltpl "html/template"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// gateMaxGU is the longest jump gate a faction can build, in galaxy units
// (the game's build text, relayed by players 2026-11-16).
const gateMaxGU = 1235

// distanceSystem is one selectable system on the distance calculator. GU are
// map units: a lane's length in the connections table is the straight-line
// distance between its two systems' positions.
type distanceSystem struct {
	ID     string  `json:"id"`
	Name   string  `json:"n"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Approx bool    `json:"a,omitempty"` // position is a screenshot estimate
}

// distanceSystems lists every system with a usable position, sorted by name
// (then id, so duplicate names stay in a stable order). Systems with no
// position, or seen only on someone else's screenshot, are left out.
func distanceSystems(systems []*System) []distanceSystem {
	out := make([]distanceSystem, 0, len(systems))
	for _, s := range systems {
		if s.PositionUnknown || s.Sighted {
			continue
		}
		out = append(out, distanceSystem{
			ID: s.ID, Name: s.Name,
			X: math.Round(s.PositionX*10) / 10, Y: math.Round(s.PositionY*10) / 10,
			Approx: s.PositionApprox,
		})
	}
	slices.SortFunc(out, func(a, b distanceSystem) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// writeDistancePage writes kb/systems/distance.html: two system pickers and
// the straight-line distance between them.
func writeDistancePage(outDir string, systems []*System) error {
	data, err := json.Marshal(distanceSystems(systems))
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(outDir, "distance.html"))
	if err != nil {
		return err
	}
	err = distanceTmpl.Execute(f, struct {
		Data    htmltpl.JS
		GateMax int
	}{htmltpl.JS(data), gateMaxGU})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

var distanceTmpl = htmltpl.Must(htmltpl.New("distance").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Distance Calculator - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../system.css">
    <link rel="stylesheet" href="../items/items.css">
    <style>
        .dist-form { display: grid; grid-template-columns: auto 1fr; gap: 10px 14px; align-items: center; max-width: 560px; margin: 20px 0; }
        .rng-table { max-width: 560px; }
        .rng-table td.num-cell, .rng-table th.num-cell { text-align: right; font-variant-numeric: tabular-nums; }
        .dist-form input, .dist-form select { width: 100%; box-sizing: border-box; padding: 7px 10px; font: inherit; background: hsl(var(--card)); color: hsl(var(--foreground)); border: 1px solid hsl(var(--input)); border-radius: 6px; cursor: pointer; }
        .dist-form input { cursor: text; }
        .dist-form input:hover, .dist-form select:hover { border-color: hsl(var(--smui-border-hover, var(--border))); }
        .dist-form input:focus, .dist-form select:focus { outline: 2px solid hsl(var(--ring)); outline-offset: 1px; }
        .dist-out { background: hsl(var(--card)); border: 1px solid hsl(var(--border)); border-radius: 8px; padding: 16px 20px; max-width: 560px; box-sizing: border-box; }
        .tool-sep { border: 0; border-top: 1px solid hsl(var(--border)); margin: 36px 0 24px; }
        .dist-num { font-size: 2em; font-weight: 700; font-variant-numeric: tabular-nums; }
        .dist-note { color: hsl(var(--muted-foreground)); font-size: 0.9em; margin-top: 6px; }
        .gate-ok { color: hsl(var(--smui-green)); }
        .gate-no { color: hsl(var(--smui-red)); }
    </style>
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <h2>Distance Calculator</h2>
        <p>Pick two systems to get the straight-line distance between them in GU (galaxy map units, the same units as lane lengths).</p>
        <div class="dist-form">
            <label for="dist-a">From</label><select id="dist-a"></select>
            <label for="dist-b">To</label><select id="dist-b"></select>
        </div>
        <div class="dist-out" id="dist-out" aria-live="polite"></div>

        <hr class="tool-sep">
        <h2>Systems Within Range</h2>
        <p>Pick a system and a distance to list every system within that many GU of it, nearest first.</p>
        <div class="dist-form">
            <label for="rng-c">System</label><select id="rng-c"></select>
            <label for="rng-r">Distance (GU)</label><input id="rng-r" type="number" min="0" step="1" value="{{.GateMax}}">
        </div>
        <div id="rng-out" aria-live="polite"></div>
    </main>
    <script>
    (function() {
        var S = {{.Data}}, GATE = {{.GateMax}};
        var a = document.getElementById('dist-a'), b = document.getElementById('dist-b');
        var out = document.getElementById('dist-out'), byId = {};
        S.forEach(function(s) {
            byId[s.id] = s;
            var label = s.n + (s.a ? ' (approx.)' : '');
            a.add(new Option(label, s.id)); b.add(new Option(label, s.id));
        });
        var c = document.getElementById('rng-c'), rg = document.getElementById('rng-r');
        var rout = document.getElementById('rng-out');
        S.forEach(function(s) { c.add(new Option(s.n + (s.a ? ' (approx.)' : ''), s.id)); });
        var q = new URLSearchParams(location.hash.slice(1));
        a.value = byId[q.get('a')] ? q.get('a') : 'sol';
        b.value = byId[q.get('b')] ? q.get('b') : 'sol';
        c.value = byId[q.get('c')] ? q.get('c') : 'sol';
        if (q.get('r') && !isNaN(parseFloat(q.get('r')))) rg.value = q.get('r');
        function esc(t) { var d = document.createElement('div'); d.textContent = t; return d.innerHTML; }
        function fmt(d) { return d.toLocaleString(undefined, {maximumFractionDigits: 1}); }
        function saveHash() {
            history.replaceState(null, '', '#a=' + encodeURIComponent(a.value) + '&b=' + encodeURIComponent(b.value) +
                '&c=' + encodeURIComponent(c.value) + '&r=' + encodeURIComponent(rg.value));
        }
        function range() {
            var o = byId[c.value], lim = parseFloat(rg.value);
            saveHash();
            if (!o || isNaN(lim) || lim < 0) { rout.textContent = ''; return; }
            var hits = [];
            S.forEach(function(s) {
                if (s.id === o.id) return;
                var d = Math.hypot(s.x - o.x, s.y - o.y);
                if (d <= lim) hits.push({s: s, d: d});
            });
            hits.sort(function(x, y) { return x.d - y.d; });
            if (!hits.length) { rout.innerHTML = '<p class="dist-note">No systems within ' + fmt(lim) + ' GU of ' + esc(o.n) + '.</p>'; return; }
            var html = '<p>' + hits.length + ' system' + (hits.length === 1 ? '' : 's') + ' within ' + fmt(lim) + ' GU of ' + esc(o.n) + '.</p>' +
                '<table class="rng-table"><thead><tr><th>System</th><th class="num-cell">GU</th></tr></thead><tbody>';
            hits.forEach(function(h) {
                html += '<tr><td><a href="' + encodeURIComponent(h.s.id) + '/index.html">' + esc(h.s.n) + '</a>' +
                    (h.s.a ? ' <span class="dist-note">(approx.)</span>' : '') + '</td><td class="num-cell">' + fmt(h.d) + '</td></tr>';
            });
            rout.innerHTML = html + '</tbody></table>' +
                (o.a || hits.some(function(h) { return h.s.a; }) ? '<p class="dist-note">Approximate entries are placed from map screenshots, usually within a few GU.</p>' : '');
        }
        c.addEventListener('change', range); rg.addEventListener('input', range);
        function show() {
            var p = byId[a.value], r = byId[b.value];
            if (!p || !r) { out.textContent = ''; return; }
            saveHash();
            var d = Math.hypot(r.x - p.x, r.y - p.y);
            var html = '<div class="dist-num">' + d.toLocaleString(undefined, {maximumFractionDigits: 1}) + ' GU</div>' +
                '<div>' + esc(p.n) + ' → ' + esc(r.n) + '</div>';
            if (p.id !== r.id) {
                html += d <= GATE
                    ? '<div class="dist-note gate-ok">Within the ' + GATE + ' GU jump gate limit</div>'
                    : '<div class="dist-note gate-no">Beyond the ' + GATE + ' GU jump gate limit</div>';
            }
            if (p.a || r.a) {
                html += '<div class="dist-note">Approximate: uncharted systems are placed from map screenshots, usually within a few GU. Check in game before building anything permanent.</div>';
            }
            out.innerHTML = html;
        }
        a.addEventListener('change', show); b.addEventListener('change', show);
        show(); range();
    })();
    </script>
` + themeScript + `
</body>
</html>
`))
