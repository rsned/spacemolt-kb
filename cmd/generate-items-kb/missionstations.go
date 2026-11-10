// Command generate-items-kb mission "By Station" view.
//
// Docked at a station, a player wants every story mission it hands out in one
// place rather than checking nine category pages. These pages list, per
// station, every chain with a leg offered there (other legs greyed out) and
// every story one-off offered there. Template families and player-faction
// posts are not story content and are left out.
package main

import (
	"bytes"
	"cmp"
	htmltpl "html/template"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// stationCollapseRun is the shortest run of consecutive legs offered
// elsewhere that a station page folds into one "… N more legs" row.
const stationCollapseRun = 3

// StationChainRow is one line of a chain on a station page: a leg, or a
// folded run of legs offered elsewhere (Collapsed > 0).
type StationChainRow struct {
	Index     int // 1-based leg number
	Step      MissionChainStep
	Here      bool   // offered at this station
	Where     string // where an elsewhere leg is offered
	Href      string // detail page, relative to kb/missions/stations/
	Collapsed int
}

// StationChain is a chain as shown on one station's page.
type StationChain struct {
	Note       string // "Starts here" or "Leg N is here"
	StartsHere bool
	Head       *Mission // the chain's first mission; Rows[0] may be a folded row
	Rows       []StationChainRow
}

// StationOneOff is a story one-off offered at a station.
type StationOneOff struct {
	M      *Mission
	Href   string
	Others int // other stations offering it
}

// StationMissions is everything one station page shows.
type StationMissions struct {
	BaseID  string
	Name    string
	Empire  string
	Chains  []StationChain
	OneOffs []StationOneOff
}

// TypeIcon exposes the mission-type glyphs to the station templates.
func (StationMissions) TypeIcon(typ string) string { return missionTypeIcons[typ] }

// stationHref links a mission's detail page from kb/missions/stations/.
func stationHref(m *Mission) string { return "../" + m.Type + "/" + m.ID + ".html" }

// missionStations maps a mission's offering stations by base id to name.
func missionStations(m *Mission) map[string]string {
	out := map[string]string{}
	for _, l := range m.Locations {
		out[l.BaseID] = locationName(l)
	}
	return out
}

// buildStationPages assembles a page for every station offering at least one
// chain leg or story one-off. empires maps base id to its empire label.
func buildStationPages(chains []MissionChain, oneOffs []*Mission, empires map[string]string) []StationMissions {
	pages := map[string]*StationMissions{}
	page := func(id, name string) *StationMissions {
		p, ok := pages[id]
		if !ok {
			emp := empires[id]
			if emp == "" {
				emp = "Unknown"
			}
			p = &StationMissions{BaseID: id, Name: name, Empire: emp}
			pages[id] = p
		}
		return p
	}

	for _, c := range chains {
		stationsByLeg := make([]map[string]string, len(c.Steps))
		touched := map[string]string{}
		for i, s := range c.Steps {
			if s.Undiscovered {
				continue
			}
			stationsByLeg[i] = missionStations(s.Mission)
			maps.Copy(touched, stationsByLeg[i])
		}
		for id, name := range touched {
			page(id, name).Chains = append(page(id, name).Chains, stationChain(c, stationsByLeg, id))
		}
	}

	for _, m := range oneOffs {
		at := missionStations(m)
		for id, name := range at {
			p := page(id, name)
			p.OneOffs = append(p.OneOffs, StationOneOff{M: m, Href: stationHref(m), Others: len(at) - 1})
		}
	}

	out := make([]StationMissions, 0, len(pages))
	for _, p := range pages {
		slices.SortFunc(p.Chains, func(a, b StationChain) int {
			if a.StartsHere != b.StartsHere {
				if a.StartsHere {
					return -1
				}
				return 1
			}
			ha, hb := a.Head, b.Head
			if c := cmp.Compare(ha.Difficulty, hb.Difficulty); c != 0 {
				return c
			}
			return cmp.Compare(ha.Title, hb.Title)
		})
		slices.SortFunc(p.OneOffs, func(a, b StationOneOff) int { return compareMissions(a.M, b.M) })
		out = append(out, *p)
	}
	slices.SortFunc(out, func(a, b StationMissions) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

// stationChain lays out chain c for station id, folding runs of
// stationCollapseRun or more consecutive discovered legs offered elsewhere.
func stationChain(c MissionChain, stationsByLeg []map[string]string, id string) StationChain {
	sc := StationChain{Head: c.Head()}
	var run []StationChainRow
	flush := func() {
		if len(run) >= stationCollapseRun {
			sc.Rows = append(sc.Rows, StationChainRow{Index: run[0].Index, Collapsed: len(run)})
		} else {
			sc.Rows = append(sc.Rows, run...)
		}
		run = nil
	}
	for i, s := range c.Steps {
		row := StationChainRow{Index: i + 1, Step: s}
		if !s.Undiscovered {
			row.Href = stationHref(s.Mission)
			_, row.Here = stationsByLeg[i][id]
		}
		if row.Here && sc.Note == "" {
			sc.StartsHere = i == 0
			if sc.StartsHere {
				sc.Note = "Starts here"
			} else {
				sc.Note = "Leg " + strconv.Itoa(i+1) + " is here"
			}
		}
		if row.Here || s.Undiscovered {
			flush()
			sc.Rows = append(sc.Rows, row)
			continue
		}
		switch n := len(stationsByLeg[i]); {
		case n == 0:
			row.Where = "station not recorded"
		case n > 1:
			row.Where = "many stations"
		default:
			for _, name := range stationsByLeg[i] {
				row.Where = name
			}
		}
		run = append(run, row)
	}
	flush()
	return sc
}

// StationEmpireGroup is one empire heading on the station index.
type StationEmpireGroup struct {
	Empire   string
	Stations []StationMissions
}

// groupStationsByEmpire orders the index: empires alphabetically, with
// Independent and Unknown last; stations by name inside each.
func groupStationsByEmpire(pages []StationMissions) []StationEmpireGroup {
	by := map[string][]StationMissions{}
	for _, p := range pages {
		by[p.Empire] = append(by[p.Empire], p)
	}
	rank := func(e string) int {
		switch e {
		case "Independent":
			return 1
		case "Unknown":
			return 2
		}
		return 0
	}
	empires := slices.SortedFunc(maps.Keys(by), func(a, b string) int {
		if c := cmp.Compare(rank(a), rank(b)); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	out := make([]StationEmpireGroup, 0, len(empires))
	for _, e := range empires {
		ss := by[e]
		slices.SortFunc(ss, func(a, b StationMissions) int { return cmp.Compare(a.Name, b.Name) })
		out = append(out, StationEmpireGroup{Empire: e, Stations: ss})
	}
	return out
}

func stationTemplates() *htmltpl.Template {
	t := htmltpl.Must(htmltpl.New("station").Funcs(missionTemplateFuncs()).Parse(htmlStationPageTemplate))
	return htmltpl.Must(t.New("stationIndex").Parse(htmlStationIndexTemplate))
}

// renderStationPage renders kb/missions/stations/<base_id>.html.
func renderStationPage(p StationMissions) ([]byte, error) {
	var b bytes.Buffer
	err := stationTemplates().ExecuteTemplate(&b, "station", p)
	return b.Bytes(), err
}

// renderStationIndex renders kb/missions/stations/index.html.
func renderStationIndex(groups []StationEmpireGroup) ([]byte, error) {
	var b bytes.Buffer
	err := stationTemplates().ExecuteTemplate(&b, "stationIndex", groups)
	return b.Bytes(), err
}

// writeStationPages replaces kb/missions/stations/ with the current pages.
func writeStationPages(outDir string, pages []StationMissions) error {
	dir := filepath.Join(outDir, "stations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".html") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	idx, err := renderStationIndex(groupStationsByEmpire(pages))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), idx, 0o644); err != nil {
		return err
	}
	for _, p := range pages {
		page, err := renderStationPage(p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, p.BaseID+".html"), page, 0o644); err != nil {
			return err
		}
	}
	return nil
}

var htmlStationIndexTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Missions by Station - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../missions/missions.css">
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Missions</a> / By Station</div>
        <h2>Missions by Station</h2>
        <p class="text-muted mt-1">Every story mission a station hands out — chains with a leg there and one-off missions — on one page per station. Generated template missions and player-faction posts are not listed.</p>
{{- range .}}
        <section class="mission-section station-index">
            <h3>{{.Empire}} <span class="text-muted">{{len .Stations}}</span></h3>
            <ul class="station-list">
{{- range .Stations}}
                <li><a href="{{.BaseID}}.html">{{.Name}}</a> <span class="text-muted">{{len .Chains}} chain{{if ne (len .Chains) 1}}s{{end}} · {{len .OneOffs}} one-off{{if ne (len .OneOffs) 1}}s{{end}}</span></li>
{{- end}}
            </ul>
        </section>
{{- end}}
    </main>
` + themeScript + `
</body>
</html>
`

var htmlStationPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} Missions - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../missions/missions.css">
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Missions</a> / <a href="index.html">By Station</a> / {{.Name}}</div>
        <h2>{{.Name}} <span class="text-muted">{{.Empire}} · {{len .Chains}} chains · {{len .OneOffs}} one-offs</span></h2>
{{- $p := .}}
{{if .Chains}}
        <section class="mission-section">
            <h3>Chains with a leg here <span class="text-muted">{{len .Chains}}</span></h3>
            <p class="text-muted section-note">Legs offered at this station are shown normally; legs offered elsewhere are greyed out with where to pick them up.</p>
{{- range .Chains}}
            <div class="station-chain">
                <div class="station-chain-note text-muted">{{.Note}}</div>
                <table class="chain-table">
                    <colgroup><col class="c-step"><col><col class="c-diff"><col class="c-cr"><col class="c-where"></colgroup>
                    <tbody>
{{- range .Rows}}
{{- if .Collapsed}}
                        <tr class="leg-elsewhere"><td class="chain-step-col">{{.Index}}…</td><td colspan="4">… {{.Collapsed}} more legs elsewhere</td></tr>
{{- else if .Step.Undiscovered}}
                        <tr class="chain-undiscovered-row"><td class="chain-step-col">{{.Index}}</td><td><span class="chain-undiscovered" title="Not yet discovered — mission id {{.Step.Mission.ID}}">{{.Step.Mission.Title}}</span></td><td colspan="3" class="text-muted">not yet scanned or logged</td></tr>
{{- else}}
                        <tr{{if not .Here}} class="leg-elsewhere"{{end}}>
                            <td class="chain-step-col">{{.Index}}</td>
                            <td><span class="chain-type-icon" title="{{titleCase .Step.Mission.Type}} mission">{{$p.TypeIcon .Step.Mission.Type}}</span> <a href="{{.Href}}">{{.Step.Mission.Title}}</a></td>
                            <td>{{difficulty .Step.Mission.Difficulty}}</td>
                            <td class="value">{{fmtValue .Step.Mission.RewardsCredits}}</td>
                            <td>{{if .Here}}<span class="text-muted">here</span>{{else}}{{.Where}}{{end}}</td>
                        </tr>
{{- end}}
{{- end}}
                    </tbody>
                </table>
            </div>
{{- end}}
        </section>
{{end}}
{{if .OneOffs}}
        <section class="mission-section">
            <h3>One-off missions here <span class="text-muted">{{len .OneOffs}}</span></h3>
            <div class="table-container">
                <table class="sortable">
                    <thead><tr><th class="sortable">Mission</th><th class="sortable">Type</th><th class="sortable">Difficulty</th><th class="sortable">Credits</th></tr></thead>
                    <tbody>
{{- range .OneOffs}}
                        <tr>
                            <td><a href="{{.Href}}">{{.M.Title}}</a>{{if .Others}} <span class="text-muted">· also at {{.Others}} other station{{if ne .Others 1}}s{{end}}</span>{{end}}</td>
                            <td data-sort="{{.M.Type}}">{{$p.TypeIcon .M.Type}} {{.M.Type}}</td>
                            <td data-sort="{{.M.Difficulty}}">{{difficulty .M.Difficulty}}</td>
                            <td data-sort="{{.M.RewardsCredits}}" class="value">{{fmtValue .M.RewardsCredits}}</td>
                        </tr>
{{- end}}
                    </tbody>
                </table>
            </div>
        </section>
{{end}}
    </main>
` + sortScript + themeScript + `
</body>
</html>
`
