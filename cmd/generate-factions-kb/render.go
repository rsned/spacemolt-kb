package main

import (
	htmltpl "html/template"
	"strings"
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
		"join": strings.Join,
		"dash": func(s string) string {
			if s == "" {
				return "—"
			}
			return s
		},
		"richText": richText,
		"inline":   func(s string) htmltpl.HTML { return htmltpl.HTML(inlineMarkup(s)) },
	}
}

// richText renders free-text (charter/description) as HTML paragraphs: a blank
// line ("\n\n") starts a new <p>, a single "\n" becomes a <br>, and **bold**
// spans become <strong>. All text is HTML-escaped; only the wrapper tags we
// emit are trusted. class is applied to each <p> (empty for an unclassed one).
func richText(class, s string) htmltpl.HTML {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	open := "<p>"
	if class != "" {
		open = `<p class="` + htmltpl.HTMLEscapeString(class) + `">`
	}
	var b strings.Builder
	for para := range strings.SplitSeq(s, "\n\n") {
		para = strings.Trim(para, "\n")
		if strings.TrimSpace(para) == "" {
			continue
		}
		b.WriteString(open)
		for i, line := range strings.Split(para, "\n") {
			if i > 0 {
				b.WriteString("<br>")
			}
			b.WriteString(inlineMarkup(line))
		}
		b.WriteString("</p>")
	}
	return htmltpl.HTML(b.String())
}

// inlineMarkup HTML-escapes a single line of free text and converts paired
// **bold** markers into <strong> spans.
func inlineMarkup(line string) string {
	return boldify(htmltpl.HTMLEscapeString(line))
}

// boldify converts paired ** markers in already-escaped text into <strong>
// spans. An unmatched trailing ** is left as literal text.
func boldify(s string) string {
	parts := strings.Split(s, "**")
	if len(parts) < 3 {
		return s // no complete pair of markers
	}
	last := len(parts) - 1
	var b strings.Builder
	for i, p := range parts {
		switch {
		case i%2 == 1 && i != last:
			b.WriteString("<strong>")
			b.WriteString(p)
			b.WriteString("</strong>")
		case i%2 == 1 && i == last:
			b.WriteString("**") // unmatched opener — restore literal markers
			b.WriteString(p)
		default:
			b.WriteString(p)
		}
	}
	return b.String()
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
            <a href="{{.Slug}}/" class="faction-card"{{if or .PrimaryColor .SecondaryColor}} style="{{if .PrimaryColor}}--faction-accent:{{.PrimaryColor}};{{end}}{{if .SecondaryColor}}--faction-accent2:{{.SecondaryColor}};{{end}}"{{end}}>
                <div class="fc-name">{{.Name}} <span class="fc-tag">[{{.Tag}}]</span></div>
                <div class="fc-stats">{{.MemberCount}} members &middot; {{.OwnedBases}} bases</div>
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
    <link rel="stylesheet" href="../../factions/factions.css">
</head>
<body>
` + siteHeader2 + `
    <main class="container page-content detail-page">
        <div class="faction-banner"{{if or .PrimaryColor .SecondaryColor}} style="{{if .PrimaryColor}}--faction-accent:{{.PrimaryColor}};{{end}}{{if .SecondaryColor}}--faction-accent2:{{.SecondaryColor}};{{end}}"{{end}}>
            {{if and .Overlay .Overlay.ImageFile}}<img class="overlay-logo" src="{{.Overlay.ImageFile}}" alt="{{.Overlay.ImageAlt}}">{{end}}
            <h2>{{.Name}} <span class="fb-tag">[{{.Tag}}]</span></h2>
            <div class="fb-id text-muted">{{.ID}}</div>
            {{if .FoundedUTC}}<div class="text-muted fb-founded">Founded {{shortDate .FoundedUTC}}</div>{{end}}
            {{if .Description}}<h4 class="fb-label">Description</h4>{{richText "fb-desc" .Description}}{{end}}
            {{if .Charter}}<h4 class="fb-label">Charter</h4>{{richText "fb-charter text-muted" .Charter}}{{end}}
        </div>

        <dl class="faction-stats">
            <dt>Leader</dt><dd>{{dash .LeaderName}}</dd>
            <dt>Members</dt><dd>{{.MemberCount}}</dd>
            <dt>Bases</dt><dd>{{.OwnedBases}}</dd>
        </dl>
        <p class="api-note">Official API member_count: {{.OfficialMemberCount}} (hidden from outsiders); roster below is reconstructed from sightings.</p>

{{if and .Overlay .Overlay.Stats}}
        <h3>Profile</h3>
        <dl class="faction-stats overlay-stats">
{{- range .Overlay.Stats}}
            <dt>{{.Label}}</dt><dd>{{.Value}}</dd>
{{- end}}
        </dl>
{{end}}
{{if and .Overlay .Overlay.BodyHTML}}
        <h3>About</h3>
        <p class="overlay-credit text-muted">Community-contributed profile.</p>
        <div class="overlay-body">{{.Overlay.BodyHTML}}</div>
{{end}}
        <h3>Members ({{.MemberCount}})</h3>
{{if .Members}}
        <ul class="member-list">
{{- range .Members}}
            <li><a href="../../players/{{.Slug}}/">{{if .IsOnline}}<span class="online-dot">&#9679;</span> {{end}}{{.Username}}</a></li>
{{- end}}
        </ul>
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
    <main class="container page-content detail-page">
        <div class="player-banner"{{if .PrimaryColor}} style="--player-accent:{{.PrimaryColor}}"{{end}}>
            {{if and .Overlay .Overlay.ImageFile}}<img class="overlay-portrait" src="{{.Overlay.ImageFile}}" alt="{{.Overlay.ImageAlt}}">{{end}}
            <h2>{{.Username}}{{if .FactionSlug}} <a class="pb-faction" href="../../factions/{{.FactionSlug}}/">[{{.FactionTag}}]</a>{{else if .FactionTag}}<span class="pb-faction">[{{.FactionTag}}]</span>{{end}}</h2>
            <div class="pb-id">{{.ID}}</div>
            {{if .ClanTag}}<div class="pb-clan">clan {{.ClanTag}}</div>{{end}}
            {{if .StatusMessage}}<div class="pb-status">{{inline .StatusMessage}}</div>{{end}}
        </div>

{{if and .Overlay .Overlay.Stats}}
        <h3>Profile</h3>
        <dl class="faction-stats overlay-stats">
{{- range .Overlay.Stats}}
            <dt>{{.Label}}</dt><dd>{{.Value}}</dd>
{{- end}}
        </dl>
{{end}}
{{if and .Overlay .Overlay.BodyHTML}}
        <h3>About</h3>
        <p class="overlay-credit text-muted">Community-contributed profile.</p>
        <div class="overlay-body">{{.Overlay.BodyHTML}}</div>
{{end}}
        <div class="stat-strip">
            <div class="ss-item"><strong>{{shortDate .FirstSeenUTC}}</strong> first seen</div>
            <div class="ss-item"><strong>{{rel .LastSeenUTC}}</strong> last seen</div>
        </div>
    </main>
` + themeScript + `
</body>
</html>
`
