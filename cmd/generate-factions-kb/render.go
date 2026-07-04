package main

import (
	htmltpl "html/template"
	"strings"
	"time"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
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
		"silhouette": func(seed, primary, secondary string) htmltpl.HTML {
			return silhouetteSVG(seed, primary, secondary)
		},
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

// siteHeader1/siteHeader2 are the top-nav headers for player/faction/passenger
// pages at depth 1 and 2. Both come from the single shared definition in
// internal/kbnav, so the bar matches the rest of the site.
var siteHeader1 = kbnav.Header("../")

var siteHeader2 = kbnav.Header("../../")

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

// factionSortScript reorders the faction cards in place by name or member count.
// Clicking the active button toggles its direction. Default render is members
// descending, matching the Members button's initial pressed/arrow state.
var factionSortScript = `    <script>
    (function() {
        var grid = document.querySelector('.faction-cards');
        if (!grid) return;
        var buttons = document.querySelectorAll('.sort-bar .sort-btn');
        function apply(key, dir) {
            var cards = Array.prototype.slice.call(grid.querySelectorAll('.faction-card'));
            cards.sort(function(a, b) {
                var cmp;
                if (key === 'members') {
                    cmp = (parseInt(a.dataset.members, 10) || 0) - (parseInt(b.dataset.members, 10) || 0);
                } else {
                    cmp = (a.dataset.name || '').toLowerCase().localeCompare((b.dataset.name || '').toLowerCase());
                }
                if (cmp === 0) {
                    cmp = (a.dataset.name || '').toLowerCase().localeCompare((b.dataset.name || '').toLowerCase());
                }
                return dir === 'asc' ? cmp : -cmp;
            });
            cards.forEach(function(c) { grid.appendChild(c); });
        }
        buttons.forEach(function(btn) {
            btn.addEventListener('click', function() {
                var dir = btn.dataset.dir;
                if (btn.getAttribute('aria-pressed') === 'true') {
                    dir = dir === 'asc' ? 'desc' : 'asc';
                }
                btn.dataset.dir = dir;
                buttons.forEach(function(b) {
                    b.setAttribute('aria-pressed', b === btn ? 'true' : 'false');
                    var a = b.querySelector('.sort-arrow');
                    if (a) a.remove();
                });
                var arrow = document.createElement('span');
                arrow.className = 'sort-arrow';
                arrow.innerHTML = dir === 'asc' ? ' &#9650;' : ' &#9660;';
                btn.appendChild(arrow);
                apply(btn.dataset.sort, dir);
            });
        });
    })();
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
        <div class="sort-bar">
            <span class="sort-label">Sort:</span>
            <button type="button" class="sort-btn" data-sort="members" data-dir="desc" aria-pressed="true">Members <span class="sort-arrow">&#9660;</span></button>
            <button type="button" class="sort-btn" data-sort="name" data-dir="asc" aria-pressed="false">Name</button>
        </div>
        <div class="faction-cards">
{{- range .}}
            <a href="{{.Slug}}/" class="faction-card" data-name="{{.Name}}" data-members="{{.MemberCount}}"{{if or .PrimaryColor .SecondaryColor}} style="{{if .PrimaryColor}}--faction-accent:{{.PrimaryColor}};{{end}}{{if .SecondaryColor}}--faction-accent2:{{.SecondaryColor}};{{end}}"{{end}}>
                <div class="fc-name">{{.Name}} <span class="fc-tag">[{{.Tag}}]</span></div>
                <div class="fc-stats">{{.MemberCount}} members &middot; {{.OwnedBases}} bases</div>
            </a>
{{- end}}
        </div>
    </main>
` + themeScript + factionSortScript + `
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
        <div class="faction-banner{{if and .Overlay .Overlay.ImageFile}} has-overlay-image{{end}}"{{if or .PrimaryColor .SecondaryColor}} style="{{if .PrimaryColor}}--faction-accent:{{.PrimaryColor}};{{end}}{{if .SecondaryColor}}--faction-accent2:{{.SecondaryColor}};{{end}}"{{end}}>
            <h2>{{.Name}} <span class="fb-tag">[{{.Tag}}]</span></h2>
            <div class="fb-id text-muted">{{.ID}}</div>
            {{if and .Overlay .Overlay.ImageFile}}<figure class="overlay-figure"><img class="overlay-logo" src="{{.Overlay.ImageFile}}" alt="{{.Overlay.ImageAlt}}">{{if .Overlay.ImageAlt}}<figcaption class="overlay-caption">{{.Overlay.ImageAlt}}</figcaption>{{end}}</figure>{{end}}
            {{if .FoundedUTC}}<div class="text-muted fb-founded">Founded {{shortDate .FoundedUTC}}</div>{{end}}
            {{if .Description}}<h4 class="fb-label">Description</h4>{{richText "fb-desc" .Description}}{{end}}
            {{if .Charter}}<h4 class="fb-label">Charter</h4>{{richText "fb-charter text-muted" .Charter}}{{end}}
        </div>

        <dl class="faction-stats">
            <dt>Leader</dt><dd>{{if .LeaderSlug}}<a href="../../players/{{.LeaderSlug}}/">{{.LeaderName}}</a>{{else}}{{dash .LeaderName}}{{end}}</dd>
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
            <thead><tr><th class="sortable">Username</th><th class="sortable">Faction</th></tr></thead>
            <tbody>
{{- range .}}
                <tr>
                    <td><a href="{{.Slug}}/">{{.Username}}</a></td>
                    <td>{{if .FactionSlug}}<a href="../factions/{{.FactionSlug}}/">[{{.FactionTag}}]</a>{{else if .FactionTag}}[{{.FactionTag}}]{{else}}<span class="muted">&mdash;</span>{{end}}</td>
                </tr>
{{- end}}
            </tbody>
        </table>
    </main>
` + themeScript + sortScript + `
</body>
</html>
`

// --- Passenger index ---
var passengerIndexTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Passengers - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../passengers/passengers.css">
</head>
<body>
` + siteHeader1 + `
    <main class="container page-content">
        <h2>Passengers</h2>
        <p class="text-muted mt-1">{{len .}} ship passengers sighted in transit.</p>
        <table class="sortable">
            <thead><tr><th class="sortable">Name</th><th class="sortable">Citizenship</th><th class="sortable">Class</th><th class="sortable">Sightings</th></tr></thead>
            <tbody>
{{- range .}}
                <tr>
                    <td class="pname">{{if .ThumbFile}}<img class="pthumb" src="{{.Slug}}/{{.ThumbFile}}" width="32" height="32" loading="lazy" decoding="async" alt="">{{else}}<span class="pthumb pthumb-none" aria-hidden="true"></span>{{end}}<a href="{{.Slug}}/">{{.Name}}</a></td>
                    <td>{{dash .Citizenship}}</td>
                    <td>{{dash .Class}}</td>
                    <td data-sort="{{.SightingCount}}">{{.SightingCount}}</td>
                </tr>
{{- end}}
            </tbody>
        </table>
    </main>
` + themeScript + sortScript + `
</body>
</html>
`

// --- Passenger detail ---
// Portrait precedence: contributor overlay image > generated AI portrait >
// deterministic silhouette.
var passengerDetailTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} - Passengers - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../passengers/passengers.css">
</head>
<body>
` + siteHeader2 + `
    <main class="container page-content detail-page">
        <aside class="infobox"{{if .EmpireColor}} style="--player-accent:{{.EmpireColor}}"{{end}}>
            <div class="infobox-title">{{.Name}}</div>
            <div class="infobox-id">{{.ID}}</div>
            {{if .Citizenship}}<div class="infobox-subtitle">{{.Citizenship}}</div>{{end}}
{{if and .Overlay .Overlay.ImageFile}}
            <img class="infobox-image" src="{{.Overlay.ImageFile}}" alt="{{.Overlay.ImageAlt}}">
            {{if .Overlay.ImageAlt}}<figcaption class="infobox-caption">{{.Overlay.ImageAlt}}</figcaption>{{end}}
{{else if .PortraitFile}}
            <img class="infobox-image" src="{{.PortraitFile}}" alt="Generated portrait of {{.Name}}">
{{else}}
            <div class="infobox-silhouette">{{silhouette .ID .EmpireColor ""}}</div>
{{end}}
            <dl class="infobox-data">
                {{if .Class}}<dt>Class</dt><dd>{{.Class}}</dd>{{end}}
                <dt>First seen</dt><dd>{{shortDate .FirstSeenUTC}}</dd>
                <dt>Sightings</dt><dd>{{.SightingCount}}</dd>
            </dl>
        </aside>
{{if .Bio}}
        <h3>About</h3>
        {{richText "" .Bio}}
{{end}}
{{if and .Overlay .Overlay.BodyHTML}}
        <h3>Profile</h3>
        <p class="overlay-credit text-muted">Community-contributed profile.</p>
        <div class="overlay-body">{{.Overlay.BodyHTML}}</div>
{{end}}
{{if and .PortraitFile .PortraitPrompt (not (and .Overlay .Overlay.ImageFile))}}
        <p class="portrait-prompt-note text-muted" style="margin-top:1.75rem"><small>AI portrait prompt: <code>{{.PortraitPrompt}}</code></small></p>
{{end}}
    </main>
` + themeScript + `
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
{{if and .Overlay .Overlay.ImageFile}}
        <aside class="infobox"{{if .PrimaryColor}} style="--player-accent:{{.PrimaryColor}}"{{end}}>
            <div class="infobox-title">{{.Username}}</div>
            {{if .FactionTag}}<div class="infobox-subtitle">{{if .FactionSlug}}<a href="../../factions/{{.FactionSlug}}/">{{.FactionTag}}</a>{{else}}{{.FactionTag}}{{end}}</div>{{end}}
            <img class="infobox-image" src="{{.Overlay.ImageFile}}" alt="{{.Overlay.ImageAlt}}">
            {{if .Overlay.ImageAlt}}<figcaption class="infobox-caption">{{.Overlay.ImageAlt}}</figcaption>{{end}}
            <dl class="infobox-data">
{{- range .Overlay.Stats}}
                <dt>{{.Label}}</dt><dd>{{.Value}}</dd>
{{- end}}
                {{if .ClanTag}}<dt>Clan</dt><dd>{{.ClanTag}}</dd>{{end}}
                <dt>First seen</dt><dd>{{shortDate .FirstSeenUTC}}</dd>
                <dt>Last seen</dt><dd>{{rel .LastSeenUTC}}</dd>
            </dl>
            <div class="infobox-id">{{.ID}}</div>
        </aside>
        {{if .StatusMessage}}<div class="pb-status">{{inline .StatusMessage}}</div>{{end}}
{{else if .PortraitFile}}
        <aside class="infobox"{{if .PrimaryColor}} style="--player-accent:{{.PrimaryColor}}"{{end}}>
            <div class="infobox-title">{{.Username}}</div>
            {{if .FactionTag}}<div class="infobox-subtitle">{{if .FactionSlug}}<a href="../../factions/{{.FactionSlug}}/">{{.FactionTag}}</a>{{else}}{{.FactionTag}}{{end}}</div>{{end}}
            <img class="infobox-image" src="{{.PortraitFile}}" alt="Generated portrait of {{.Username}}">
            <figcaption class="infobox-caption">AI-generated placeholder portrait.</figcaption>
            <dl class="infobox-data">
                {{if .ClanTag}}<dt>Clan</dt><dd>{{.ClanTag}}</dd>{{end}}
                <dt>First seen</dt><dd>{{shortDate .FirstSeenUTC}}</dd>
                <dt>Last seen</dt><dd>{{rel .LastSeenUTC}}</dd>
            </dl>
            <div class="infobox-id">{{.ID}}</div>
        </aside>
        {{if .StatusMessage}}<div class="pb-status">{{inline .StatusMessage}}</div>{{end}}
{{if and .Overlay .Overlay.Stats}}
        <h3>Profile</h3>
        <dl class="faction-stats overlay-stats">
{{- range .Overlay.Stats}}
            <dt>{{.Label}}</dt><dd>{{.Value}}</dd>
{{- end}}
        </dl>
{{end}}
{{else}}
        <aside class="infobox"{{if .PrimaryColor}} style="--player-accent:{{.PrimaryColor}}"{{end}}>
            <div class="infobox-title">{{.Username}}</div>
            {{if .FactionTag}}<div class="infobox-subtitle">{{if .FactionSlug}}<a href="../../factions/{{.FactionSlug}}/">{{.FactionTag}}</a>{{else}}{{.FactionTag}}{{end}}</div>{{end}}
            <div class="infobox-silhouette">{{silhouette .ID .PrimaryColor .SecondaryColor}}</div>
            <dl class="infobox-data">
                {{if .ClanTag}}<dt>Clan</dt><dd>{{.ClanTag}}</dd>{{end}}
                <dt>First seen</dt><dd>{{shortDate .FirstSeenUTC}}</dd>
                <dt>Last seen</dt><dd>{{rel .LastSeenUTC}}</dd>
            </dl>
            <div class="infobox-id">{{.ID}}</div>
        </aside>
        {{if .StatusMessage}}<div class="pb-status">{{inline .StatusMessage}}</div>{{end}}
{{if and .Overlay .Overlay.Stats}}
        <h3>Profile</h3>
        <dl class="faction-stats overlay-stats">
{{- range .Overlay.Stats}}
            <dt>{{.Label}}</dt><dd>{{.Value}}</dd>
{{- end}}
        </dl>
{{end}}
{{end}}
{{if and .Overlay .Overlay.BodyHTML}}
        <h3>About</h3>
        <p class="overlay-credit text-muted">Community-contributed profile.</p>
        <div class="overlay-body">{{.Overlay.BodyHTML}}</div>
{{end}}
    </main>
` + themeScript + `
</body>
</html>
`
