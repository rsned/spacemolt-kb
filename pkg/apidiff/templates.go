package apidiff

// pageStyle is shared by the report and index pages. Colours are page-local
// tokens defined for light and html.dark; site colours are bare HSL triplets,
// so they are used through hsl(var(--x)).
const pageStyle = `<style>
:root {
  --ad-add-bg: #e4f6e6; --ad-add-fg: #17602b; --ad-add-mark: #b5e8bd;
  --ad-del-bg: #fde8e8; --ad-del-fg: #9b2226; --ad-del-mark: #f9c4c4;
  --ad-mod-bg: #fdf1d8; --ad-mod-fg: #7a5400;
  --ad-k: #7a3e9d; --ad-t: #1f6d7a; --ad-s: #8a5a00; --ad-c: #5e6977;
}
html.dark {
  --ad-add-bg: #1c3a27; --ad-add-fg: #9fe0ae; --ad-add-mark: #2d6a40;
  --ad-del-bg: #43232a; --ad-del-fg: #f4b1b1; --ad-del-mark: #7c3540;
  --ad-mod-bg: #40351c; --ad-mod-fg: #efcf85;
  --ad-k: #c8a2e6; --ad-t: #86d0dc; --ad-s: #e3c07c; --ad-c: #94a0b1;
}
.diff-nav { display: flex; justify-content: space-between; padding: 1rem 0; font-size: var(--text-ui); }
.diff-nav a, .toc a, .intro a { color: hsl(var(--primary)); }
.summary, .note { color: hsl(var(--muted-foreground)); font-size: var(--text-ui); margin: 0.5rem 0; }
.bar { position: sticky; top: 0; z-index: 5; display: flex; flex-wrap: wrap; gap: 0.6rem 1.25rem; align-items: center; justify-content: space-between;
  background: hsl(var(--card)); border: 1px solid hsl(var(--border)); padding: 0.5rem 0.75rem; margin: 1rem 0; }
.seg { display: inline-flex; border: 1px solid hsl(var(--border)); }
.seg button { font: inherit; font-size: var(--text-ui); padding: 0.3rem 0.85rem; background: none; color: hsl(var(--muted-foreground)); border: 0; cursor: pointer; }
.seg button[aria-pressed="true"] { background: hsl(var(--primary)); color: hsl(var(--primary-foreground)); }
.seg button:focus-visible, summary:focus-visible, .toc a:focus-visible { outline: 2px solid hsl(var(--primary)); outline-offset: 1px; }
.legend { display: flex; flex-wrap: wrap; gap: 0.35rem 0.9rem; font-size: var(--text-label); color: hsl(var(--muted-foreground)); }
.legend span { display: inline-flex; align-items: center; gap: 0.35rem; }
.legend i { width: 0.9rem; height: 0.9rem; border: 1px solid hsl(var(--border)); display: inline-block; }
.i-add { background: var(--ad-add-bg); } .i-del { background: var(--ad-del-bg); }
.i-mark { background: linear-gradient(90deg, var(--ad-del-mark) 50%, var(--ad-add-mark) 50%); }
.toc { list-style: none; padding: 0; margin: 0 0 1rem; display: grid; gap: 0.3rem; font-size: var(--text-ui); }
.toc li { display: flex; flex-wrap: wrap; gap: 0.3rem 0.6rem; align-items: baseline; }
h3.group { font-size: var(--text-label); text-transform: uppercase; letter-spacing: 1.5px; color: hsl(var(--muted-foreground)); margin: 1.75rem 0 0.5rem; }
.card { background: hsl(var(--card)); border: 1px solid hsl(var(--border)); margin: 1rem 0; min-width: 0; }
.card header { display: flex; flex-wrap: wrap; gap: 0.4rem 0.75rem; justify-content: space-between; align-items: baseline; padding: 0.65rem 0.85rem; }
.card h4 { font-size: var(--text-ui); font-weight: 600; margin: 0; overflow-wrap: anywhere; }
.tags { display: flex; flex-wrap: wrap; gap: 0.3rem; }
.tag { font-size: var(--text-label); padding: 0.05rem 0.45rem; }
.tag.add, .tag.new { background: var(--ad-add-bg); color: var(--ad-add-fg); }
.tag.del { background: var(--ad-del-bg); color: var(--ad-del-fg); }
.tag.mod { background: var(--ad-mod-bg); color: var(--ad-mod-fg); }
.code { overflow-x: auto; border-top: 1px solid hsl(var(--border)); font-size: var(--text-label); line-height: 1.6; }
.lines { display: inline-block; min-width: 100%; padding: 0.3rem 0; }
.l, .code summary { display: flex; white-space: pre; }
.g { flex: none; width: 1.9rem; text-align: center; color: hsl(var(--muted-foreground)); user-select: none; }
.x { flex: 1; padding-right: 1rem; }
.l.add { background: var(--ad-add-bg); } .l.add .g { color: var(--ad-add-fg); font-weight: 600; }
.l.del { background: var(--ad-del-bg); } .l.del .g { color: var(--ad-del-fg); font-weight: 600; }
.k { color: var(--ad-k); } .t { color: var(--ad-t); } .s { color: var(--ad-s); } .c { color: var(--ad-c); }
.l.add .c, .l.del .c { color: hsl(var(--foreground)); }
mark { color: inherit; padding: 0 0.12em; }
.l.add mark { background: var(--ad-add-mark); } .l.del mark { background: var(--ad-del-mark); }
details.fold { border-top: 1px dashed hsl(var(--border)); border-bottom: 1px dashed hsl(var(--border)); }
.code summary { cursor: pointer; color: hsl(var(--muted-foreground)); list-style: none; }
.code summary::-webkit-details-marker { display: none; }
.code summary:hover { color: hsl(var(--primary)); }
body[data-lang="go"] .ts, body[data-lang="ts"] .go { display: none; }
</style>`

const themeScript = `<script>
(function() {
    var toggle = document.getElementById('theme-toggle');
    var root = document.documentElement;
    if (localStorage.getItem('theme') === 'dark') root.classList.add('dark');
    if (toggle) toggle.addEventListener('click', function() {
        root.classList.toggle('dark');
        localStorage.setItem('theme', root.classList.contains('dark') ? 'dark' : 'light');
    });
})();
</script>`

const langScript = `<script>
(function() {
    var buttons = document.querySelectorAll('.seg button');
    function set(l) {
        document.body.dataset.lang = l;
        buttons.forEach(function(b) { b.setAttribute('aria-pressed', b.dataset.l === l ? 'true' : 'false'); });
        try { localStorage.setItem('api-diffs-lang', l); } catch (e) {}
    }
    buttons.forEach(function(b) { b.addEventListener('click', function() { set(b.dataset.l); }); });
    try { var s = localStorage.getItem('api-diffs-lang'); if (s === 'go' || s === 'ts') set(s); } catch (e) {}
})();
</script>`

const linesTemplate = `{{define "lines"}}<div class="lines">{{range .}}{{if isFold .Kind}}<details class="fold"><summary><span class="g">⋯</span><span class="x">{{.Label}}</span></summary>{{range .Folded}}<div class="l"><span class="g"> </span><span class="x">{{.HTML}}</span></div>{{end}}</details>{{else}}<div class="l {{cls .Kind}}"><span class="g">{{sign .Kind}}</span><span class="x">{{.HTML}}</span></div>{{end}}{{end}}</div>{{end}}
{{define "card"}}<article class="card" id="{{.Anchor}}"><header><h4>{{.Title}}</h4><div class="tags">{{range .Tags}}<span class="tag {{.Class}}">{{.Text}}</span>{{end}}</div></header>
<div class="code go">{{template "lines" .Go}}</div><div class="code ts">{{template "lines" .TS}}</div></article>
{{end}}
{{define "toc"}}{{range .}}<li><a class="toc-link" href="#{{.Anchor}}">{{.Title}}</a>{{range .Tags}}<span class="tag {{.Class}}">{{.Text}}</span>{{end}}</li>{{end}}{{end}}`

const reportTemplate = linesTemplate + `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>API Changes &mdash; {{long .Step.To}}</title>
<link rel="stylesheet" href="../smui.css">
` + pageStyle + `
</head>
<body data-lang="go">
{{.Header}}
<main class="container page-content">
<div class="diff-nav">
    <span>{{if .Prev}}<a href="{{.Prev}}.html">&larr; {{.Prev}}</a>{{else}}&nbsp;{{end}}</span>
    <a href="index.html">All Reports</a>
    <span>{{if .Next}}<a href="{{.Next}}.html">{{.Next}} &rarr;</a>{{else}}&nbsp;{{end}}</span>
</div>
<h2>API Changes &mdash; {{long .Step.To}} vs {{long .Step.From}}</h2>
<p class="summary">{{.Summary}}</p>
<p class="note">Structs here are generated from the server's OpenAPI spec, so names and types may differ from <code>serverapi</code>. Unchanged fields are folded; click a fold to open it.</p>
<div class="bar">
    <div class="seg" role="group" aria-label="Language">
        <button type="button" data-l="go" aria-pressed="true">Go</button>
        <button type="button" data-l="ts" aria-pressed="false">TypeScript</button>
    </div>
    <div class="legend"><span><i class="i-add"></i>added</span><span><i class="i-del"></i>removed</span><span><i class="i-mark"></i>changed words</span></div>
</div>
<ul class="toc">{{template "toc" .Structural}}{{template "toc" .Wording}}</ul>
{{if .Structural}}<h3 class="group">Structural changes</h3>
{{range .Structural}}{{template "card" .}}{{end}}{{end}}
{{if .Wording}}<h3 class="group">Wording only</h3>
{{range .Wording}}{{template "card" .}}{{end}}{{end}}
</main>
` + themeScript + langScript + `
</body>
</html>`

const indexTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>API Change Log</title>
<link rel="stylesheet" href="../smui.css">
` + pageStyle + `
</head>
<body>
{{.Header}}
<main class="container">
<div class="page-content intro">
<h2>API Change Log</h2>
<p class="text-muted mt-1">What changed in the server API's structs between snapshots of its OpenAPI spec: fields, types, enum values, union members and descriptions. The structs are generated from the spec, so names and types may differ from the client's.</p>
<p class="text-muted">Changes to game data (recipes, items, ships, skills, map) are in the <a href="../diffs/index.html">catalog change log</a>.</p>
<table class="mt-2">
<thead><tr><th>Date</th><th>Changes</th></tr></thead>
<tbody>
{{range .Steps}}<tr><td><a href="{{iso .To}}.html">{{iso .To}}</a></td><td>{{.Counts.Summary}}</td></tr>
{{end}}</tbody>
</table>
<p class="text-muted mt-2">History starts at 2026-02-17, when the spec first documented its schemas.</p>
</div>
</main>
` + themeScript + `
</body>
</html>`
