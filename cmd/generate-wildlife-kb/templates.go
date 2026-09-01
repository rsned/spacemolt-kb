package main

// Shared styling for the section, on top of smui.css/system.css.
const wildlifeStyle = `<style>
        .summary-cards { display: flex; gap: 16px; margin: 16px 0; flex-wrap: wrap; }
        .summary-card { background: var(--bg-card); border: 1px solid var(--border); border-radius: 8px; padding: 12px 20px; text-align: center; }
        .summary-card .num { font-size: 1.8em; font-weight: 700; }
        .summary-card .label { font-size: 0.8em; color: var(--text-muted); text-transform: uppercase; }
        .toc { columns: 3; column-gap: 24px; margin: 8px 0 16px; }
        .toc a { display: block; padding: 2px 0; color: var(--link); text-decoration: none; font-size: 0.95em; }
        .toc a:hover { text-decoration: underline; }
        .toc-role { break-inside: avoid; margin-bottom: 12px; }
        .toc-role h4 { margin: 0 0 4px; font-size: 0.8em; text-transform: uppercase; letter-spacing: 1px; color: var(--text-muted); }
        .species-section { margin-top: 32px; scroll-margin-top: 16px; }
        .species-section h3 { margin-bottom: 8px; border-bottom: 1px solid var(--border); padding-bottom: 4px; }
        .species-section table, .wl-table { width: 100%; font-size: 0.9em; }
        .species-section th, .wl-table th { text-align: left; white-space: nowrap; }
        .species-section td, .wl-table td { padding: 4px 8px; vertical-align: top; }
        .num { text-align: right; font-variant-numeric: tabular-nums; }
        .back-top { font-size: 0.8em; margin-left: 8px; color: var(--text-muted); }
        .unsighted { background: var(--bg-card); border: 1px solid var(--border); border-left: 4px solid #999; padding: 16px; margin-top: 16px; border-radius: 4px; color: var(--text-muted); font-size: 0.9em; }
        #wl-map-wrap { width: 100%; margin: 16px 0 24px; background: var(--bg-card); border: 1px solid var(--border); border-radius: 8px; padding: 12px; }
        #wl-map-wrap svg { width: 100%; height: auto; display: block; border-radius: 4px; }
        #wl-map-wrap select { width: 100%; margin-bottom: 10px; padding: 6px; }
        .wl-map-empty { display: none; font-size: 0.85em; color: var(--text-muted); margin-top: 8px; }
        #wl-map[data-empty="1"] + .wl-map-empty { display: block; }
        #wl-map line { stroke: #8593ad; stroke-width: 1.4; opacity: 0.62; }
        #wl-map .galaxy-sys-dot { fill: #e6ecf5; r: 5; stroke: #0a0e1a; stroke-width: 0.6; transition: none; }
        #wl-map .galaxy-sys-label { fill: #e8eef7; font-size: 24px; }
        .abund-scarce { color: #c0a030; } .abund-moderate { color: #50a050; } .abund-abundant { color: #2e9ad0; font-weight: 600; }
        .bloom { font-size: 0.8em; border: 1px solid #c0a030; color: #c0a030; border-radius: 3px; padding: 0 4px; }
        .hero { position: relative; width: 100%; aspect-ratio: 16 / 7; border-radius: 8px; overflow: hidden; background: #0a0e1a; border: 1px solid var(--border); margin: 12px 0 20px; }
        .hero img { width: 100%; height: 100%; object-fit: cover; display: block; }
        .hero.placeholder { display: flex; align-items: center; justify-content: center; flex-direction: column; color: #8593ad; background: repeating-linear-gradient(135deg, #0a0e1a 0 18px, #10162a 18px 36px); }
        .hero.placeholder .glyph { font-size: 3em; opacity: 0.6; }
        .hero.placeholder .note { font-size: 0.85em; letter-spacing: 1px; text-transform: uppercase; margin-top: 6px; }
        .hero.placeholder .path { font-size: 0.75em; opacity: 0.6; margin-top: 4px; font-family: monospace; }
        .lore { margin-top: 12px; }
        .lore p { margin: 6px 0; }
        .lore .lore-intro { font-style: italic; color: var(--text-muted); }
        .lore dt { font-weight: 600; margin-top: 10px; }
        .lore dd { margin: 2px 0 0 0; }
        .empty { color: var(--text-muted); font-style: italic; }
        .codex { font-size: 1.05em; line-height: 1.5; margin: 0; padding: 4px 0 8px; }
        .codex-src { font-size: 0.8em; color: var(--text-muted); }
</style>`

const themeScript = `    <script>
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

// mapSyncScript keeps the species highlight in sync between the dropdown,
// the URL hash, the map, and which section is shown.
const mapSyncScript = `    <script>
    (function () {
      var map = document.getElementById('wl-map');
      var sel = document.getElementById('wl-map-select');
      if (!map || !sel) return;
      var valid = {};
      for (var i = 0; i < sel.options.length; i++) valid[sel.options[i].value] = true;
      var sections = document.querySelectorAll('.species-section');
      var byId = {};
      for (var j = 0; j < sections.length; j++) byId[sections[j].id] = sections[j];
      function showOnly(slug) { for (var k = 0; k < sections.length; k++) sections[k].hidden = (sections[k].id !== slug); }
      function apply(slug, updateHash) {
        if (!byId[slug]) slug = sel.value;
        if (valid[slug]) { map.removeAttribute('data-empty'); map.setAttribute('data-active', slug); if (sel.value !== slug) sel.value = slug; }
        else { map.setAttribute('data-empty', '1'); map.removeAttribute('data-active'); }
        showOnly(slug);
        if (updateHash && location.hash.slice(1) !== slug) history.replaceState(null, '', '#' + slug);
      }
      sel.addEventListener('change', function () { apply(sel.value, true); });
      window.addEventListener('hashchange', function () { apply(location.hash.slice(1), false); });
      var initial = location.hash.slice(1);
      apply(byId[initial] ? initial : sel.value, false);
    })();
    </script>`

// placesTable renders a species' per-system estimates. Shared by the index
// sections and the species pages via {{template "places" .}}.
const placesTemplate = `{{define "places"}}
            <table class="wl-table sortable">
                <thead>
                    <tr>
                        <th>System</th>
                        <th class="num" title="Latest system survey count, or the sum of the latest counts at each POI">Est. count</th>
                        <th>Abundance</th>
                        <th>Bloom</th>
                        <th>POIs (latest count)</th>
                        <th>Ranched</th>
                        <th>Branded</th>
                        <th class="num">Sightings</th>
                        <th>Last seen</th>
                    </tr>
                </thead>
                <tbody>
{{- range .}}
                    <tr>
                        <td><a href="../systems/{{.SystemID}}/index.html">{{.SystemName}}</a> <code style="font-size:0.85em">{{.SystemID}}</code></td>
                        <td class="num">{{.Count}}</td>
                        <td>{{if .Abundance}}<span class="abund-{{.Abundance}}">{{.Abundance}}</span>{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                        <td>{{if .Bloom}}<span class="bloom">{{.Bloom}}</span>{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                        <td>{{if .POIs}}{{poiList .POIs}}{{else}}<span class="text-muted">system survey only</span>{{end}}</td>
                        <td>{{if .Ranched}}<span class="badge badge-green">Yes</span>{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                        <td>{{if .Branded}}<span class="badge badge-yellow">Yes</span>{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                        <td class="num">{{.Sightings}}</td>
                        <td title="tick {{.LastTick}}">{{date .LastSeen}}</td>
                    </tr>
{{- end}}
                </tbody>
            </table>
{{end}}`

const indexTemplate = placesTemplate + `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Wildlife - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../system.css">
` + wildlifeStyle + `
    <style>
    {{.HighlightCSS}}
    </style>
</head>
<body>
{{.Header}}
    <main class="container page-content">
        <h2>Wildlife</h2>
        <p>Every species the survey scans have recorded, with estimated populations by system. <span class="text-muted">Counts are the latest system survey (or the sum of the latest counts at each POI); wildlife moves, blooms, and gets hunted, so treat them as a snapshot. Each species has a <a href="#jump">detail page</a> with hull, attacks, kills, and field notes.</span></p>

        <div id="wl-map-wrap">
            <select id="wl-map-select" aria-label="Highlight species on map">
{{- range .Species}}
{{- if gt (len .Places) 0}}
                <option value="{{.Slug}}">{{.Name}} ({{.SystemCount}} {{plural .SystemCount "system"}}, ~{{.EstimatedTotal}})</option>
{{- end}}
{{- end}}
            </select>
            <div id="wl-map" data-active="{{.FirstSlug}}">{{.MapSVG}}</div>
            <div class="wl-map-empty">No systems with this species have been surveyed yet.</div>
        </div>

        <div class="summary-cards">
            <div class="summary-card"><div class="num">{{len .Species}}</div><div class="label">Species</div></div>
            <div class="summary-card"><div class="num">{{.Sighted}}</div><div class="label">Sighted</div></div>
            <div class="summary-card"><div class="num">~{{.Estimated}}</div><div class="label">Creatures (est.)</div></div>
            <div class="summary-card"><div class="num">{{.Coverage.SystemsWithWildlife}}</div><div class="label">Systems with wildlife</div></div>
            <div class="summary-card"><div class="num">{{.Coverage.SystemsSurveyed}} / {{.Coverage.TotalSystems}}</div><div class="label">Systems surveyed</div></div>
            <div class="summary-card"><div class="num">{{.Coverage.PlacesSurveyed}}</div><div class="label">Places surveyed</div></div>
        </div>

        <div class="card" style="padding: 12px 16px" id="jump">
            <div class="section-label">Jump To Species</div>
            <div class="toc">
{{- range .Groups}}
                <div class="toc-role">
                <h4>{{title .Role}}s</h4>
{{- range .Species}}
                <a href="#{{.Slug}}">{{.Name}} ({{if eq (len .Places) 0}}Unsighted{{else}}{{.SystemCount}} {{plural .SystemCount "system"}} &middot; ~{{.EstimatedTotal}}{{end}})</a>
{{- end}}
                </div>
{{- end}}
            </div>
        </div>

{{- range .Species}}
        <div id="{{.Slug}}" class="species-section"{{if ne .Slug $.FirstSlug}} hidden{{end}}>
            <h3>{{.Name}} <span class="badge {{roleClass .Role}}" style="font-size:0.7em; vertical-align:middle;">{{.Role}}</span> <span class="badge" style="font-size:0.7em; vertical-align:middle;">{{if eq (len .Places) 0}}Unsighted{{else}}~{{.EstimatedTotal}} in {{.SystemCount}} {{plural .SystemCount "system"}}{{end}}</span> <small style="font-size:0.8em; font-weight:normal;"><a href="{{.ID}}.html">Details</a></small> <a href="#" class="back-top">[top]</a></h3>
            <p class="text-muted" style="font-size:0.9em">Hull {{.MaxHull}}{{if .MaxShield}} &middot; Shield {{.MaxShield}}{{end}}{{if .Danger}} &middot; {{.Danger}}{{end}}{{if .Record}}{{if .Record.Rating}} &middot; <span class="badge {{ratingClass .Record.Rating}}" style="font-size:0.85em">danger: {{.Record.Rating}}</span>{{end}}{{end}}{{if .Combat}}{{range .Combat.DamageTypes}} &middot; {{.}} attack{{end}}{{end}}{{if .Habitats}} &middot; {{range $i, $h := .Habitats}}{{if $i}}, {{end}}{{habitat $h}}{{end}}{{end}}{{if .Ranchable}} &middot; ranchable{{end}}</p>
{{- if eq (len .Places) 0}}
            <div class="unsighted">Scanned in the roster but not yet sighted anywhere. Survey agents may find it in uncharted regions.</div>
{{- else}}
{{template "places" .Places}}
{{- end}}
        </div>
{{- end}}
        <p class="text-muted" style="margin-top:32px;font-size:0.85em">Generated {{.Generated}} from the survey ledger.</p>
    </main>
` + mapSyncScript + `
` + themeScript + `
</body>
</html>
`

const speciesTemplate = placesTemplate + `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.S.Name}} - Wildlife - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../system.css">
` + wildlifeStyle + `
</head>
<body>
{{.Header}}
    <main class="container page-content">
        <p class="text-muted" style="font-size:0.85em"><a href="index.html">Wildlife</a> &rsaquo; {{title .S.Role}}s</p>
        <h2>{{.S.Name}} <span class="badge {{roleClass .S.Role}}" style="font-size:0.6em; vertical-align:middle;">{{.S.Role}}</span>{{if .S.Ranchable}} <span class="badge badge-green" style="font-size:0.6em; vertical-align:middle;">ranchable</span>{{end}}</h2>
        <code>{{.S.ID}}</code>

{{- if .S.HasImage}}
        <div class="hero"><img src="{{.S.ImagePath}}" alt="{{.S.Name}}"></div>
{{- else}}
        <div class="hero placeholder" title="Hero image not yet available">
            <div class="glyph">&#x1F41A;</div>
            <div class="note">Hero image pending</div>
            <div class="path">wildlife/{{.S.ImagePath}}</div>
        </div>
{{- end}}

{{- if .S.Description}}
        <div class="card" style="padding:12px 16px">
            <div class="section-label">Codex entry</div>
            <p class="codex">{{.S.Description}}</p>
            <div class="codex-src">Official species entry returned by scanning a creature{{if eq .S.CodexSource "codex"}} &mdash; recorded by hand{{with .S.CodexTick}} at tick {{.}}{{end}}{{else if eq .S.CodexSource "lore"}} &mdash; quoted from the lore document{{end}}.</div>
        </div>
{{- else}}
        <div class="card" style="padding:12px 16px">
            <div class="section-label">Codex entry</div>
            <p class="empty">Not read yet. Since v0.571.0 scanning any creature of this species returns its entry (the <code>description</code> field); the server does not keep it, so record it in <code>data/wildlife/codex.json</code> or let the knowledge DB capture it.</p>
        </div>
{{- end}}

        <div class="card mt-2" style="padding:0">
            <div class="section-label">General</div>
            <table>
                <tr><td class="kv-label">Class</td><td><span class="badge {{roleClass .S.Role}}">{{.S.Role}}</span></td></tr>
                <tr><td class="kv-label">Hull (HP)</td><td>{{.S.MaxHull}}</td></tr>
                <tr><td class="kv-label">Shield</td><td>{{if .S.MaxShield}}{{.S.MaxShield}}{{else}}<span class="text-muted">none</span>{{end}}</td></tr>
                <tr><td class="kv-label">Danger</td><td>{{if .S.Danger}}{{.S.Danger}}{{else}}<span class="text-muted">not yet scanned</span>{{end}}</td></tr>
                <tr><td class="kv-label">Battle record</td><td>{{if .S.Record}}{{.S.Record.Battles}} battles, wildlife won {{.S.Record.WildlifeWins}} ({{printf "%.1f" .S.Record.WinPct}}%){{if .S.Record.Rating}} &mdash; <span class="badge {{ratingClass .S.Record.Rating}}">{{.S.Record.Rating}} danger</span>{{else}} &mdash; too few battles to rate{{end}} <span class="text-muted">(public battle feed, {{$.StatsMonths}})</span>{{else}}<span class="text-muted">no battles in the public feed</span>{{end}}</td></tr>
                <tr><td class="kv-label">Attack</td><td>{{if .S.Combat}}{{if .S.Combat.Weapons}}{{range $n, $w := .S.Combat.Weapons}}<span class="badge">{{$w.DamageType}}</span> {{intRange $w.BaseMin $w.BaseMax}} base dmg/shot ({{$w.Shots}} {{plural $w.Shots "shot"}} observed) {{end}}{{if .S.Combat.HitMax}}&middot; hit chance {{printf "%.2f" .S.Combat.HitMin}}&ndash;{{printf "%.2f" .S.Combat.HitMax}} {{end}}{{else}}never seen attacking {{end}}<span class="text-muted">({{.S.Combat.Battles}} exported {{plural .S.Combat.Battles "battle"}})</span>{{else}}<span class="text-muted">no exported battle logs yet</span>{{end}}</td></tr>
                <tr><td class="kv-label">Habitats</td><td>{{if .S.Habitats}}{{range $i, $h := .S.Habitats}}{{if $i}}, {{end}}{{habitat $h}}{{end}}{{else}}<span class="text-muted">unknown</span>{{end}}</td></tr>
                <tr><td class="kv-label">Ranchable</td><td>{{yesno .S.Ranchable}}</td></tr>
{{- if .S.ScanTraits}}
                <tr><td class="kv-label">Scan traits</td><td>{{.S.ScanTraits}}</td></tr>
{{- end}}
                <tr><td class="kv-label">Population (est.)</td><td>{{if .S.Places}}~{{.S.EstimatedTotal}} across {{.S.SystemCount}} {{plural .S.SystemCount "system"}}{{else}}<span class="text-muted">not yet sighted</span>{{end}}</td></tr>
                <tr><td class="kv-label">First seen</td><td>{{if .S.FirstSeen}}{{datetime .S.FirstSeen}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td></tr>
                <tr><td class="kv-label">Last seen</td><td>{{if .S.LastSeen}}{{datetime .S.LastSeen}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td></tr>
            </table>
        </div>

        <div class="card mt-2" style="padding:0">
            <div class="section-label">Attacks</div>
{{- if .S.Attacks}}
            <table class="wl-table">
                <thead><tr><th>Weapon</th><th>Damage type</th><th>Shot</th><th class="num">Battles</th><th class="num">Shots</th><th class="num">Hits</th><th class="num">Accuracy</th><th class="num">Damage / hit</th><th class="num">Total damage</th><th>Last observed</th></tr></thead>
                <tbody>
{{- range .S.Attacks}}
                    <tr>
                        <td>{{.Weapon}}</td>
                        <td>{{.DamageType}}</td>
                        <td>{{.ShotKind}}</td>
                        <td class="num">{{.Battles}}</td>
                        <td class="num">{{.Shots}}</td>
                        <td class="num">{{.Hits}}</td>
                        <td class="num">{{fmtF .Accuracy}}%</td>
                        <td class="num">{{if .Hits}}{{fmtF .DamagePerHit}}{{if ne .DamageMin .DamageMax}} <span class="text-muted">({{fmtF .DamageMin}}&ndash;{{fmtF .DamageMax}})</span>{{end}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                        <td class="num">{{fmtF .DamageTotal}}</td>
                        <td>{{date .LastSeen}}</td>
                    </tr>
{{- end}}
                </tbody>
            </table>
{{- else}}
            <p class="empty" style="padding:12px 16px">No attacks recorded{{if eq .S.Role "predator"}} yet &mdash; every engagement is logged when it happens{{else}}; {{if .S.Danger}}scanned as &ldquo;{{.S.Danger}}&rdquo;{{else}}it has never been seen to fight{{end}}{{end}}.</p>
{{- end}}
        </div>

        <div class="card mt-2" style="padding:0">
            <div class="section-label">Where found</div>
{{- if .S.Places}}
{{template "places" .S.Places}}
{{- else}}
            <p class="empty" style="padding:12px 16px">Not yet sighted in any surveyed system.</p>
{{- end}}
        </div>

        <div class="card mt-2" style="padding:0">
            <div class="section-label">Kills &amp; drops</div>
{{- if .S.Kills}}
            <table class="wl-table">
                <thead><tr><th>When</th><th>Where</th><th class="num">Duration (ticks)</th><th class="num">Damage dealt</th><th class="num">Damage taken</th><th class="num">Salvage value</th><th>Drops</th></tr></thead>
                <tbody>
{{- range .S.Kills}}
                    <tr>
                        <td>{{datetime .KilledAt}}</td>
                        <td><a href="../systems/{{.SystemID}}/index.html">{{.SystemName}}</a>{{if .POIName}} &middot; {{.POIName}}{{end}}</td>
                        <td class="num">{{.DurationTicks}}</td>
                        <td class="num">{{.DamageDealt}}</td>
                        <td class="num">{{.DamageTaken}}</td>
                        <td class="num">{{.SalvageValue}}</td>
                        <td>{{if .Drops}}{{range $i, $d := .Drops}}{{if $i}}, {{end}}{{if $d.ItemCategory}}<a href="../items/{{$d.ItemCategory}}/{{$d.ItemID}}.html">{{$d.ItemName}}</a>{{else}}{{$d.ItemName}}{{end}} &times;{{fmtF $d.Quantity}}{{end}}{{else}}<span class="text-muted">none read</span>{{end}}</td>
                    </tr>
{{- end}}
                </tbody>
            </table>
{{- else}}
            <p class="empty" style="padding:12px 16px">No kills recorded yet. Carcass drops will be listed here once a kill is logged.</p>
{{- end}}
        </div>

{{- with .S.Lore}}
        <div class="card mt-2" style="padding:12px 16px">
            <div class="section-label">Field notes <span class="codex-src" style="text-transform:none;letter-spacing:0">(KB lore, unofficial)</span></div>
            <div class="lore">
                <p class="lore-intro">{{.Intro}}</p>
                <dl>
{{- if .Changed}}
                    <dt>Changed</dt><dd>{{.Changed}}</dd>
{{- end}}
{{- if .Feeds}}
                    <dt>Feeds</dt><dd>{{.Feeds}}</dd>
{{- end}}
{{- if .Defends}}
                    <dt>Defends</dt><dd>{{.Defends}}</dd>
{{- end}}
                </dl>
            </div>
        </div>
{{- end}}
        <p style="margin-top:24px"><a href="index.html#{{.S.Slug}}">&larr; All wildlife</a></p>
    </main>
` + themeScript + `
</body>
</html>
`
