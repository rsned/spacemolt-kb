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
	// Label is the leg number as shown: "4", "2…" for a folded run, or "4/5"
	// for a leg shared by routes that reach it at different depths.
	Label string
	// Routes is set on the single row standing in for the differing opening
	// legs of chains merged because they share every leg from this station on.
	Routes []StationRoute
}

// StationRoute is one way into a merged chain: where it starts and how many
// legs come before the first one offered at this station.
type StationRoute struct {
	Head   *Mission
	Href   string
	Where  string
	Before int
}

// BeforeText describes the merged routes' opening legs, e.g. "3 or 4 legs".
func (r StationChainRow) BeforeText() string {
	var ns []int
	for _, rt := range r.Routes {
		ns = append(ns, rt.Before)
	}
	parts := numberList(ns)
	noun := "legs"
	if len(parts) == 1 && parts[0] == "1" {
		noun = "leg"
	}
	return orList(parts) + " " + noun
}

// orList joins items as "a", "a or b", "a, b or c".
func orList(items []string) string {
	if len(items) <= 1 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}

// StationChain is a chain as shown on one station's page.
type StationChain struct {
	Note       string // "<first mission> — starts here" or "… — leg N is here"
	StartsHere bool
	Head       *Mission // the chain's first mission; Rows[0] may be a folded row
	Rows       []StationChainRow

	// Branches follow the last row when the chain splits (see
	// MissionChain.Branches); BranchLabel is "Branch k of n" on a branch.
	Branches    []StationChain
	BranchLabel string

	firstHere int    // stem step index of the first leg offered here (-1 if none, or if it is in a branch)
	hereRow   int    // index in Rows of that leg
	tail      string // mission ids from the first leg here to the end
	headWhere string // where the first leg is offered
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
		touched := map[string]string{}
		c.walk(func(s MissionChainStep) {
			if !s.Undiscovered {
				maps.Copy(touched, missionStations(s.Mission))
			}
		})
		for id, name := range touched {
			page(id, name).Chains = append(page(id, name).Chains, stationChain(c, id))
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
		p.Chains = mergeSharedTails(p.Chains)
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

// hereMark records the first leg offered at the station, depth first.
type hereMark struct {
	found   bool
	leg     int    // 1-based leg number
	branch  string // "" in the stem, else "1", "2", "1.2" ...
	stepIdx int    // index into the stem's Steps when branch == ""
	row     int    // index into the stem's Rows when branch == ""
}

// stationChain lays out chain c for station id: legs here shown normally,
// legs elsewhere greyed, runs of stationCollapseRun or more consecutive
// discovered legs elsewhere folded, and branches laid out the same way.
func stationChain(c MissionChain, id string) StationChain {
	sc := StationChain{Head: c.Head(), firstHere: -1}
	var mark hereMark
	sc.Rows, sc.Branches = stationSegment(c, id, &mark, "")
	if mark.found {
		switch {
		case mark.branch == "" && mark.leg == 1:
			sc.StartsHere = true
			sc.Note = sc.Head.Title + " — starts here"
		case mark.branch == "":
			sc.Note = sc.Head.Title + " — leg " + strconv.Itoa(mark.leg) + " is here"
		default:
			sc.Note = sc.Head.Title + " — leg " + strconv.Itoa(mark.leg) + " (branch " + mark.branch + ") is here"
		}
	}
	if s := c.Steps[0]; !s.Undiscovered {
		sc.headWhere = legWhere(missionStations(s.Mission))
	}
	if mark.found && mark.branch == "" {
		sc.firstHere, sc.hereRow = mark.stepIdx, mark.row
		ids := make([]string, 0, len(c.Steps)-mark.stepIdx)
		for _, s := range c.Steps[mark.stepIdx:] {
			ids = append(ids, s.Mission.ID)
		}
		sc.tail = strings.Join(ids, ">") + branchSignature(c.Branches)
	}
	return sc
}

// branchSignature identifies a set of branches by their missions, so two
// routes merge only when everything after the shared leg matches.
func branchSignature(bs []MissionChain) string {
	if len(bs) == 0 {
		return ""
	}
	parts := make([]string, len(bs))
	for i, b := range bs {
		var ids []string
		for _, s := range b.Steps {
			ids = append(ids, s.Mission.ID)
		}
		parts[i] = strings.Join(ids, ">") + branchSignature(b.Branches)
	}
	return "{" + strings.Join(parts, "|") + "}"
}

// stationSegment lays out one run of legs (the stem, or a branch) and the
// branches that follow it. branch names this run for the "is here" note.
func stationSegment(c MissionChain, id string, mark *hereMark, branch string) ([]StationChainRow, []StationChain) {
	var rows, run []StationChainRow
	flush := func() {
		if len(run) >= stationCollapseRun {
			rows = append(rows, StationChainRow{Index: run[0].Index, Collapsed: len(run), Label: run[0].Label + "…"})
		} else {
			rows = append(rows, run...)
		}
		run = nil
	}
	for i, s := range c.Steps {
		leg := c.LegNumber(i)
		row := StationChainRow{Index: leg, Step: s, Label: strconv.Itoa(leg)}
		var at map[string]string
		if !s.Undiscovered {
			row.Href = stationHref(s.Mission)
			at = missionStations(s.Mission)
			_, row.Here = at[id]
		}
		if row.Here || s.Undiscovered {
			flush()
			if row.Here && !mark.found {
				*mark = hereMark{found: true, leg: leg, branch: branch, stepIdx: i, row: len(rows)}
			}
			rows = append(rows, row)
			continue
		}
		row.Where = legWhere(at)
		run = append(run, row)
	}
	flush()
	var branches []StationChain
	for bi, b := range c.Branches {
		name := strconv.Itoa(bi + 1)
		if branch != "" {
			name = branch + "." + name
		}
		r, bb := stationSegment(b, id, mark, name)
		branches = append(branches, StationChain{
			BranchLabel: "Branch " + strconv.Itoa(bi+1) + " of " + strconv.Itoa(len(c.Branches)),
			Rows:        r, Branches: bb,
		})
	}
	return rows, branches
}

// legWhere names where a leg is offered, for a leg not offered here.
func legWhere(stations map[string]string) string {
	switch len(stations) {
	case 0:
		return "station not recorded"
	case 1:
		for _, name := range stations {
			return name
		}
	}
	return "many stations"
}

// mergeSharedTails folds chains that reach this station by different routes
// but share every leg from the first one here to the end. The merged chain
// opens with one row naming each route's start, then the shared legs, each
// numbered for every route ("4/5"). Chains starting here never merge: a chain
// head has no incoming route, so no other chain can share its whole length.
func mergeSharedTails(chains []StationChain) []StationChain {
	groups := map[string][]StationChain{}
	var order []string
	for _, c := range chains {
		key := c.tail
		if c.firstHere <= 0 {
			key = "\x00" + strconv.Itoa(len(order)) // never merged
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], c)
	}
	out := make([]StationChain, 0, len(order))
	for _, key := range order {
		g := groups[key]
		if len(g) == 1 {
			out = append(out, g[0])
			continue
		}
		out = append(out, mergeRoutes(g))
	}
	return out
}

// mergeRoutes builds the single block for chains sharing a tail.
func mergeRoutes(g []StationChain) StationChain {
	routes := make([]StationRoute, len(g))
	var legs []int
	for i, c := range g {
		routes[i] = StationRoute{Head: c.Head, Href: stationHref(c.Head), Where: c.headWhere, Before: c.firstHere}
		legs = append(legs, c.firstHere+1)
	}
	slices.SortFunc(routes, func(a, b StationRoute) int {
		return cmp.Or(cmp.Compare(a.Head.Title, b.Head.Title), cmp.Compare(a.Head.ID, b.Head.ID))
	})
	heads := make([]string, len(routes))
	for i, r := range routes {
		heads[i] = r.Head.Title
	}

	m := StationChain{
		Note:      orList(heads) + " — leg " + orList(numberList(legs)) + " is here · " + strconv.Itoa(len(g)) + " routes lead here",
		Head:      routes[0].Head,
		firstHere: -1,
		Rows:      []StationChainRow{{Label: "1…", Routes: routes}},
	}
	tail := g[0].Rows[g[0].hereRow:]
	for j, row := range tail {
		var nums []int
		for _, c := range g {
			nums = append(nums, c.Rows[c.hereRow+j].Index)
		}
		row.Label = strings.Join(numberList(nums), "/")
		if row.Collapsed > 0 {
			row.Label += "…"
		}
		m.Rows = append(m.Rows, row)
	}
	all := make([][]StationChain, len(g))
	for i, c := range g {
		all[i] = c.Branches
	}
	m.Branches = mergeBranchLabels(all)
	return m
}

// mergeBranchLabels returns the first route's branches with each leg labelled
// for every route ("12/13"). Merged routes share their branches (the merge key
// includes them), so the shapes match row for row.
func mergeBranchLabels(routes [][]StationChain) []StationChain {
	if len(routes) == 0 || len(routes[0]) == 0 {
		return nil
	}
	out := make([]StationChain, len(routes[0]))
	for k, b := range routes[0] {
		nb := StationChain{BranchLabel: b.BranchLabel}
		for j, row := range b.Rows {
			var nums []int
			for _, r := range routes {
				nums = append(nums, r[k].Rows[j].Index)
			}
			row.Label = strings.Join(numberList(nums), "/")
			if row.Collapsed > 0 {
				row.Label += "…"
			}
			nb.Rows = append(nb.Rows, row)
		}
		sub := make([][]StationChain, len(routes))
		for i, r := range routes {
			sub[i] = r[k].Branches
		}
		nb.Branches = mergeBranchLabels(sub)
		out[k] = nb
	}
	return out
}

// numberList sorts and de-duplicates ns and formats them as strings.
func numberList(ns []int) []string {
	ns = slices.Compact(slices.Sorted(slices.Values(ns)))
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = strconv.Itoa(n)
	}
	return out
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
	t = htmltpl.Must(t.Parse(htmlStationChainBodyTemplate))
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
    <main class="container page-content kbm">
        <div class="breadcrumb"><a href="../">Missions</a> / By Station</div>
        <h2>Missions by Station</h2>
        <p class="text-muted mt-1">Every story mission a station hands out — chains with a leg there and one-off missions — on one page per station. Generated template missions and player-faction posts are not listed.</p>
{{- range .}}
        <div class="kbm-label">{{.Empire}} <span class="text-muted">{{len .Stations}}</span></div>
        <div class="kbm-index">
{{- range .Stations}}
            <a href="{{.BaseID}}.html"><span class="n">{{plural (len .Chains) "chain" "chains"}} · {{plural (len .OneOffs) "one-off" "one-offs"}}</span><span class="t">{{.Name}}</span></a>
{{- end}}
        </div>
{{- end}}
    </main>
` + themeScript + `
</body>
</html>
`

var htmlStationChainBodyTemplate = `
{{define "stationChainBody"}}
                <table class="kbm-rows chain-table kbm-chain">
                    <colgroup><col class="c-step"><col><col class="c-diff"><col class="c-cr"><col class="c-where"></colgroup>
                    <tbody>
{{- range .Rows}}
{{- if .Routes}}
                        <tr class="leg-elsewhere route-row"><td class="chain-step-col">{{.Label}}</td><td colspan="4">… {{.BeforeText}} elsewhere first, starting with {{range $i, $r := .Routes}}{{if $i}} or {{end}}<a href="{{$r.Href}}">{{$r.Head.Title}}</a> <span class="text-muted">({{$r.Where}})</span>{{end}}</td></tr>
{{- else if .Collapsed}}
                        <tr class="leg-elsewhere"><td class="chain-step-col">{{.Label}}</td><td colspan="4">… {{.Collapsed}} more legs elsewhere</td></tr>
{{- else if .Step.Undiscovered}}
                        <tr class="chain-undiscovered-row"><td class="chain-step-col">{{.Label}}</td><td><span class="chain-undiscovered" title="Not yet discovered — mission id {{.Step.Mission.ID}}">{{.Step.Mission.Title}}</span></td><td colspan="3" class="text-muted">not yet scanned or logged</td></tr>
{{- else}}
                        <tr{{if not .Here}} class="leg-elsewhere"{{end}}>
                            <td class="chain-step-col">{{.Label}}</td>
                            <td><span class="chain-type-icon" title="{{titleCase .Step.Mission.Type}} mission">{{typeIcon .Step.Mission.Type}}</span> <a href="{{.Href}}">{{.Step.Mission.Title}}</a></td>
                            <td>{{difficulty .Step.Mission.Difficulty}}</td>
                            <td class="value">{{fmtValue .Step.Mission.RewardsCredits}}</td>
                            <td class="where">{{if .Here}}here{{else}}{{.Where}}{{end}}</td>
                        </tr>
{{- end}}
{{- end}}
{{- if .Branches}}
                        <tr class="chain-fork-row"><td class="chain-step-col">&#8627;</td><td colspan="4">unlocks {{len .Branches}} branches</td></tr>
{{- end}}
                    </tbody>
                </table>
{{- if .Branches}}
                <div class="chain-branches">
{{- range .Branches}}
                    <div class="chain-branch-label">{{.BranchLabel}}</div>
                    {{template "stationChainBody" .}}
{{- end}}
                </div>
{{- end}}
{{end}}
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
    <main class="container page-content kbm">
        <div class="breadcrumb"><a href="../">Missions</a> / <a href="index.html">By Station</a> / {{.Name}}</div>
        <h2>{{.Name}} <span class="text-muted">{{.Empire}} · {{plural (len .Chains) "chain" "chains"}} · {{plural (len .OneOffs) "one-off" "one-offs"}}</span></h2>
{{- $p := .}}
{{if .Chains}}
        <section class="mission-section">
            <h3>Chains with a leg here <span class="text-muted">{{len .Chains}}</span></h3>
            <p class="text-muted section-note">Legs offered at this station are shown normally; legs offered elsewhere are greyed out with where to pick them up.</p>
{{- range .Chains}}
            <div class="station-chain">
                <div class="station-chain-note text-muted">{{.Note}}</div>
                {{template "stationChainBody" .}}
            </div>
{{- end}}
        </section>
{{end}}
{{if .OneOffs}}
        <section class="mission-section">
            <h3>One-off missions here <span class="text-muted">{{len .OneOffs}}</span></h3>
            <table class="kbm-rows">
                <colgroup><col><col class="c-type"><col class="c-diff"><col class="c-cr"></colgroup>
                <tbody>
{{- range .OneOffs}}
                    <tr>
                        <td><a href="{{.Href}}">{{.M.Title}}</a>{{if .Others}} <span class="text-muted">· also at {{plural .Others "other station" "other stations"}}</span>{{end}}</td>
                        <td class="giver">{{$p.TypeIcon .M.Type}} {{.M.Type}}</td>
                        <td>{{difficulty .M.Difficulty}}</td>
                        <td class="value">{{fmtValue .M.RewardsCredits}}</td>
                    </tr>
{{- end}}
                </tbody>
            </table>
        </section>
{{end}}
    </main>
` + themeScript + `
</body>
</html>
`
