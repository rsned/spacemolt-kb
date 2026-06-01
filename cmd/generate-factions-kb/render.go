package main

import (
	htmltpl "html/template"
	"time"
)

// templateFuncs returns helpers shared by all templates. genTime is the single
// generation timestamp so relative-time output is deterministic per run.
func templateFuncs(genTime time.Time) htmltpl.FuncMap {
	return htmltpl.FuncMap{
		"rel": func(utc string) string { return relativeTime(genTime, utc) },
		"shortDate": func(utc string) string {
			t, err := time.Parse(time.RFC3339, utc)
			if err != nil {
				return "—"
			}
			return t.Format("2006-01-02")
		},
		"join": func(parts []string, sep string) string {
			out := ""
			for i, p := range parts {
				if i > 0 {
					out += sep
				}
				out += p
			}
			return out
		},
		"dash": func(s string) string {
			if s == "" {
				return "—"
			}
			return s
		},
	}
}

// nav link list shared by both header depths.
const navLinks1 = `
            <a href="../">Home</a>
            <a href="../systems/index.html">Systems</a>
            <a href="../items/index.html">Items</a>
            <a href="../ships/index.html">Ships</a>
            <a href="../factions/index.html">Factions</a>
            <a href="../players/index.html">Players</a>`

const navLinks2 = `
            <a href="../../">Home</a>
            <a href="../../systems/index.html">Systems</a>
            <a href="../../items/index.html">Items</a>
            <a href="../../ships/index.html">Ships</a>
            <a href="../../factions/index.html">Factions</a>
            <a href="../../players/index.html">Players</a>`

const themeBtn = `
            <button class="theme-toggle" id="theme-toggle" aria-label="Toggle theme">
                <svg class="icon-sun" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
                <svg class="icon-moon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
            </button>`

var siteHeader1 = `    <header class="site-header">
        <h1><a href="../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>` + navLinks1 + themeBtn + `
        </nav>
    </header>`

var siteHeader2 = `    <header class="site-header">
        <h1><a href="../../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>` + navLinks2 + themeBtn + `
        </nav>
    </header>`

var themeScript = `    <script>
    (function() {
        var toggle = document.getElementById('theme-toggle');
        var root = document.documentElement;
        if (localStorage.getItem('theme') === 'dark') root.classList.add('dark');
        toggle.addEventListener('click', function() {
            root.classList.toggle('dark');
            localStorage.setItem('theme', root.classList.contains('dark') ? 'dark' : 'light');
        });
    })();
    </script>`

var sortScript = `    <script>
    document.querySelectorAll("table.sortable").forEach(function(table) {
      var headers = table.querySelectorAll("th.sortable");
      var sortCol = -1, sortAsc = true;
      headers.forEach(function(th) {
        var idx = th.cellIndex;
        th.addEventListener("click", function() {
          if (sortCol === idx) { sortAsc = !sortAsc; } else { sortCol = idx; sortAsc = true; }
          table.querySelectorAll("th .sort-arrow").forEach(function(a) { a.remove(); });
          var arrow = document.createElement("span");
          arrow.className = "sort-arrow";
          arrow.textContent = sortAsc ? "▲" : "▼";
          th.appendChild(arrow);
          var tbody = table.querySelector("tbody");
          var rows = Array.from(tbody.querySelectorAll("tr"));
          rows.sort(function(a, b) {
            var at = a.cells[idx].getAttribute("data-sort") || a.cells[idx].textContent.trim();
            var bt = b.cells[idx].getAttribute("data-sort") || b.cells[idx].textContent.trim();
            var an = parseFloat(at), bn = parseFloat(bt);
            if (!isNaN(an) && !isNaN(bn)) return sortAsc ? an - bn : bn - an;
            return sortAsc ? at.localeCompare(bt) : bt.localeCompare(at);
          });
          rows.forEach(function(r) { tbody.appendChild(r); });
        });
      });
    });
    </script>`

// --- Faction index ---
var factionIndexTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Factions - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../factions/factions.css">
</head>
<body>
` + siteHeader1 + `
    <main class="container page-content">
        <h2>Factions</h2>
        <p class="text-muted mt-1">{{len .}} player factions. Member rosters are reconstructed from sightings &mdash; the game API reports member counts as 0 to outsiders.</p>
        <div class="faction-cards">
{{- range .}}
            <a href="{{.Slug}}/" class="faction-card"{{if .PrimaryColor}} style="--faction-accent:{{.PrimaryColor}}"{{end}}>
                <div class="fc-name">{{.Name}}<span class="fc-tag">[{{.Tag}}]</span></div>
                <div class="fc-stats">{{.MemberCount}} members &middot; {{.OwnedBases}} bases &middot; {{.Treasury}} cr</div>
            </a>
{{- end}}
        </div>
    </main>
` + themeScript + `
</body>
</html>
`

// --- Faction detail ---
var factionDetailTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} [{{.Tag}}] - Factions - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../players/players.css">
    <link rel="stylesheet" href="../../factions/factions.css">
</head>
<body>
` + siteHeader2 + `
    <main class="container page-content">
        <div class="faction-banner"{{if .PrimaryColor}} style="--faction-accent:{{.PrimaryColor}}"{{end}}>
            <h2>{{.Name}} <span class="fb-tag">[{{.Tag}}]</span></h2>
            {{if .FoundedUTC}}<div class="text-muted">Founded {{shortDate .FoundedUTC}}</div>{{end}}
            {{if .Description}}<p>{{.Description}}</p>{{end}}
            {{if .Charter}}<p class="text-muted">{{.Charter}}</p>{{end}}
        </div>

        <div class="stat-strip">
            <div class="ss-item"><strong>{{.Treasury}}</strong> treasury (cr)</div>
            <div class="ss-item"><strong>{{.OwnedBases}}</strong> bases</div>
            <div class="ss-item"><strong>{{dash .LeaderName}}</strong> leader</div>
            <div class="ss-item"><strong>{{.MemberCount}}</strong> members (sighted)</div>
        </div>
        <p class="api-note">Official API member_count: {{.OfficialMemberCount}} (hidden from outsiders); roster below is reconstructed from sightings.</p>

        <h3>Members ({{.MemberCount}})</h3>
{{if .Members}}
        <table class="sortable">
            <thead><tr><th class="sortable">Username</th><th class="sortable">Role</th><th>Ships seen</th><th class="sortable">Last seen</th></tr></thead>
            <tbody>
{{- range .Members}}
                <tr>
                    <td><a href="../../players/{{.Slug}}/">{{if .IsOnline}}<span class="online-dot">&#9679;</span> {{end}}{{.Username}}</a></td>
                    <td>{{dash .Role}}</td>
                    <td>{{if .Ships}}{{join .Ships ", "}}{{else}}&mdash;{{end}}</td>
                    <td data-sort="{{.LastSeenUTC}}" title="{{.LastSeenUTC}}">{{rel .LastSeenUTC}}</td>
                </tr>
{{- end}}
            </tbody>
        </table>
{{else}}
        <p class="text-muted">No members sighted yet.</p>
{{end}}

{{if .Bases}}
        <h3>Bases ({{len .Bases}})</h3>
        <table>
            <thead><tr><th>Name</th><th>System</th><th>Services</th></tr></thead>
            <tbody>
{{- range .Bases}}
                <tr><td>{{dash .Name}}</td><td>{{dash .SystemName}}</td><td>{{dash .Services}}</td></tr>
{{- end}}
            </tbody>
        </table>
{{end}}

{{if .Relations}}
        <h3>Relations</h3>
        <table>
            <thead><tr><th>Kind</th><th>Faction</th><th>Reason</th><th>Kills (us/them)</th></tr></thead>
            <tbody>
{{- range .Relations}}
                <tr><td>{{.Kind}}</td><td>{{.TargetName}} {{if .TargetTag}}[{{.TargetTag}}]{{end}}</td><td>{{dash .Reason}}</td><td class="kills">{{.OurKills}} / {{.TheirKills}}</td></tr>
{{- end}}
            </tbody>
        </table>
{{end}}

{{if .Facilities}}
        <h3>Facilities</h3>
        <table>
            <thead><tr><th>Type</th><th>Category</th><th>Level</th><th>Status</th></tr></thead>
            <tbody>
{{- range .Facilities}}
                <tr><td>{{dash .Type}}</td><td>{{dash .Category}}</td><td>{{.Level}}</td><td>{{dash .Status}}</td></tr>
{{- end}}
            </tbody>
        </table>
{{end}}
    </main>
` + themeScript + sortScript + `
</body>
</html>
`

// --- Player index ---
var playerIndexTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Players - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../players/players.css">
</head>
<body>
` + siteHeader1 + `
    <main class="container page-content">
        <h2>Players</h2>
        <p class="text-muted mt-1">{{len .}} players tracked from sightings.</p>
        <table class="sortable">
            <thead><tr><th class="sortable">Username</th><th class="sortable">Faction</th><th>Ships seen</th><th class="sortable">Last seen</th></tr></thead>
            <tbody>
{{- range .}}
                <tr>
                    <td><a href="{{.Slug}}/">{{.Username}}</a></td>
                    <td>{{if .FactionSlug}}<a href="../factions/{{.FactionSlug}}/">[{{.FactionTag}}]</a>{{else if .FactionTag}}[{{.FactionTag}}]{{else}}<span class="muted">&mdash;</span>{{end}}</td>
                    <td>{{if .Ships}}{{range $i, $s := .Ships}}{{if $i}}, {{end}}{{$s.Class}}{{end}}{{else}}&mdash;{{end}}</td>
                    <td data-sort="{{.LastSeenUTC}}" title="{{.LastSeenUTC}}">{{rel .LastSeenUTC}}</td>
                </tr>
{{- end}}
            </tbody>
        </table>
    </main>
` + themeScript + sortScript + `
</body>
</html>
`

// --- Player detail ---
var playerDetailTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Username}} - Players - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../players/players.css">
</head>
<body>
` + siteHeader2 + `
    <main class="container page-content">
        <div class="player-banner"{{if .PrimaryColor}} style="--player-accent:{{.PrimaryColor}}"{{end}}>
            <h2>{{.Username}}{{if .FactionSlug}} <a class="pb-faction" href="../../factions/{{.FactionSlug}}/">[{{.FactionTag}}]</a>{{else if .FactionTag}}<span class="pb-faction">[{{.FactionTag}}]</span>{{end}}</h2>
            {{if .ClanTag}}<div class="pb-clan">clan {{.ClanTag}}</div>{{end}}
            {{if .StatusMessage}}<div class="pb-status">{{.StatusMessage}}</div>{{end}}
        </div>

        <div class="stat-strip">
            <div class="ss-item"><strong>{{shortDate .FirstSeenUTC}}</strong> first seen</div>
            <div class="ss-item"><strong>{{rel .LastSeenUTC}}</strong> last seen</div>
        </div>

{{if .Ships}}
        <h3>Ships seen</h3>
        <table>
            <thead><tr><th>Class</th><th>First seen</th><th>Last seen</th></tr></thead>
            <tbody>
{{- range .Ships}}
                <tr><td>{{.Class}}</td><td>{{shortDate .FirstSeenUTC}}</td><td title="{{.LastSeenUTC}}">{{rel .LastSeenUTC}}</td></tr>
{{- end}}
            </tbody>
        </table>
{{end}}

{{if .Sightings}}
        <h3>Activity (where seen)</h3>
        <table class="sortable">
            <thead><tr><th class="sortable">System</th><th>POI</th><th>Ship</th><th>Combat</th><th class="sortable">Last seen</th></tr></thead>
            <tbody>
{{- range .Sightings}}
                <tr>
                    <td>{{if .SystemSlug}}<a href="../../systems/{{.SystemSlug}}/">{{.SystemID}}</a>{{else}}{{.SystemID}}{{end}}</td>
                    <td>{{if .POIID}}{{.POIID}}{{else}}&mdash;{{end}}</td>
                    <td>{{dash .ShipClass}}</td>
                    <td>{{if .InCombat}}<span class="combat-flag">&#9876;</span>{{else}}&mdash;{{end}}</td>
                    <td data-sort="{{.LastSeenUTC}}" title="{{.LastSeenUTC}}">{{rel .LastSeenUTC}}</td>
                </tr>
{{- end}}
            </tbody>
        </table>
{{end}}
    </main>
` + themeScript + sortScript + `
</body>
</html>
`
