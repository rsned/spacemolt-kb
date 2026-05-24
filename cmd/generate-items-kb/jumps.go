package main

import (
	"fmt"
	htmltpl "html/template"
	"math"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/hyperjump"
	"github.com/rsned/spacemolt-kb/pkg/jumpmap"
)

// jumpMargin is the Pathfinder Drive landing tolerance in galactic units.
const jumpMargin = 100.0

// secondsPerTick is the game tick duration; pathfinder travel time is ticks*10s.
const secondsPerTick = 10

// ticksDuration formats a tick count as real clock time, mm:ss.
func ticksDuration(ticks int) string {
	secs := ticks * secondsPerTick
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

// JumpRow is one destination row on a Pathfinder Jump Routes page.
type JumpRow struct {
	ID        string
	Name      string
	Bearing   float64
	Distance  float64
	Ticks     int    // travel time, ceil(Distance/10)
	Duration  string // travel time as mm:ss
	Margin    float64
	Reachable bool
}

// SweepRow is one collapsed heading-range row in the all-directions table.
type SweepRow struct {
	StartDeg  int
	EndDeg    int
	Width     int // whole-degree headings in the range (EndDeg-StartDeg+1)
	LandsAt   string
	LandsAtID string
	Void      bool
	Station   bool
	Distance  float64
	Ticks     int
	Duration  string // travel time as mm:ss
}

// JumpPageData is the data model for a system's Pathfinder Jump Routes page.
type JumpPageData struct {
	System   *System
	Arrows   htmltpl.HTML
	Wheel    htmltpl.HTML
	Direct   []JumpRow  // reachable destinations, nearest first
	Stations []JumpRow  // all station destinations, marked, nearest first
	Sweep           []SweepRow // every whole-degree heading, collapsed into ranges
	Coverage        float64    // percent of headings blocked (raw)
	CoverageDisplay float64    // percent blocked for display (capped at 99.9 when gaps exist)
	GapCount        int
}

// buildJumpReports runs the hyper-jump analysis over all systems and returns the
// per-origin reports indexed by system id plus an id->name lookup. A system has
// a station if it contains any POI of type "station".
func buildJumpReports(systems []*System) (map[string]hyperjump.OriginReport, map[string]string, []hyperjump.System) {
	hsys := make([]hyperjump.System, 0, len(systems))
	names := make(map[string]string, len(systems))
	for _, s := range systems {
		hasStation := false
		for _, p := range s.POIs {
			if p.Type == "station" {
				hasStation = true
				break
			}
		}
		hsys = append(hsys, hyperjump.System{
			ID:         s.ID,
			Name:       s.Name,
			Pos:        hyperjump.Vec{X: s.PositionX, Y: s.PositionY},
			HasStation: hasStation,
		})
		names[s.ID] = s.Name
	}

	reports := make(map[string]hyperjump.OriginReport, len(hsys))
	for _, r := range hyperjump.Analyze(hsys, jumpMargin) {
		reports[r.System] = r
	}
	return reports, names, hsys
}

// buildJumpPageData assembles the render-ready data for one system's jump page.
func buildJumpPageData(sys *System, reports map[string]hyperjump.OriginReport, names map[string]string, hsys []hyperjump.System) JumpPageData {
	report := reports[sys.ID]

	data := JumpPageData{
		System:   sys,
		Arrows:   htmltpl.HTML(jumpmap.RenderStationArrows(report, names)), //nolint:gosec // trusted internal SVG
		Wheel:    htmltpl.HTML(jumpmap.RenderCoverageWheel(report)),        //nolint:gosec // trusted internal SVG
		Coverage:        report.CoveragePct * 100,
		CoverageDisplay: jumpmap.DisplayBlockedPct(report.CoveragePct*100, len(report.Gaps)),
		GapCount:        len(report.Gaps),
	}

	var origin hyperjump.System
	for _, h := range hsys {
		if h.ID == sys.ID {
			origin = h
			break
		}
	}
	for _, r := range hyperjump.HeadingSweep(origin, hsys, jumpMargin) {
		row := SweepRow{
			StartDeg:  r.StartDeg,
			EndDeg:    r.EndDeg,
			Width:     r.EndDeg - r.StartDeg + 1,
			LandsAtID: r.LandsAt,
			LandsAt:   names[r.LandsAt],
			Void:      r.LandsAt == "",
			Station:   r.LandsAtStation,
			Distance:  r.Distance,
			Ticks:     r.Ticks,
			Duration:  ticksDuration(r.Ticks),
		}
		if row.LandsAt == "" && !row.Void {
			row.LandsAt = r.LandsAt
		}
		data.Sweep = append(data.Sweep, row)
	}

	for _, p := range report.Pairs {
		row := JumpRow{
			ID:        p.To,
			Name:      names[p.To],
			Bearing:   p.Bearing,
			Distance:  p.Distance,
			Ticks:     int(math.Ceil(p.Distance / 10)),
			Duration:  ticksDuration(int(math.Ceil(p.Distance / 10))),
			Margin:    p.AngularMargin,
			Reachable: p.Reachable,
		}
		if row.Name == "" {
			row.Name = p.To
		}
		if p.Reachable {
			data.Direct = append(data.Direct, row)
		}
		if p.DestHasStation {
			data.Stations = append(data.Stations, row)
		}
	}
	sort.Slice(data.Direct, func(i, j int) bool { return data.Direct[i].Distance < data.Direct[j].Distance })
	sort.Slice(data.Stations, func(i, j int) bool { return data.Stations[i].Distance < data.Stations[j].Distance })
	return data
}

var jumpDetailTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.System.Name}} Jump Routes - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../system.css">
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Systems</a> / <a href="./">{{.System.Name}}</a> / Jump Routes</div>

        <div class="sys-header">
            <div>
                <span class="label">Pathfinder Jump Routes</span>
                <h2 class="sys-name">{{.System.Name}}</h2>
            </div>
        </div>

        <p class="text-muted">Direct hyper-jumps fly a heading from this system until the ray passes within ` +
	"100 GU of a system's center. A closer system on that line is reached first, interrupting the path." + `</p>

        <div class="jumpmap-graphics">
            <figure class="jumpmap-figure">
                {{.Arrows}}
                <figcaption>Headings to station systems — <span class="jm-direct">white = direct</span>, <span class="jm-blocked">gray = interrupted</span></figcaption>
            </figure>
            <figure class="jumpmap-figure">
                {{.Wheel}}
                <figcaption>{{printf "%.1f" .CoverageDisplay}}% of headings blocked · {{.GapCount}} void escape gaps</figcaption>
            </figure>
        </div>

        <div class="card mt-2" style="padding:0">
          <div class="section-label">Direct Connections ({{len .Direct}})</div>
          <table class="sortable">
            <thead><tr><th class="sortable">System</th><th class="sortable" style="text-align:right">Heading</th><th class="sortable" style="text-align:right">Distance</th><th style="text-align:right">Travel (ticks)</th><th style="text-align:right">Margin</th></tr></thead>
            <tbody>
{{- range .Direct}}
            <tr>
              <td><a href="../{{.ID}}/">{{.Name}}</a></td>
              <td style="text-align:right" data-sort="{{printf "%.4f" .Bearing}}">{{printf "%.2f" .Bearing}}°</td>
              <td style="text-align:right" data-sort="{{printf "%.4f" .Distance}}">{{printf "%.0f" .Distance}}</td>
              <td style="text-align:right">{{.Ticks}} ({{.Duration}})</td>
              <td style="text-align:right">{{printf "%.3f" .Margin}}°</td>
            </tr>
{{- end}}
            </tbody>
          </table>
        </div>

        <div class="card mt-2" style="padding:0">
          <div class="section-label">Station Destinations ({{len .Stations}})</div>
          <table class="sortable">
            <thead><tr><th class="sortable">System</th><th class="sortable" style="text-align:right">Heading</th><th class="sortable" style="text-align:right">Distance</th><th style="text-align:right">Travel (ticks)</th><th>Status</th></tr></thead>
            <tbody>
{{- range .Stations}}
            <tr>
              <td><a href="../{{.ID}}/">{{.Name}}</a></td>
              <td style="text-align:right" data-sort="{{printf "%.4f" .Bearing}}">{{printf "%.2f" .Bearing}}°</td>
              <td style="text-align:right" data-sort="{{printf "%.4f" .Distance}}">{{printf "%.0f" .Distance}}</td>
              <td style="text-align:right">{{.Ticks}} ({{.Duration}})</td>
              <td>{{if .Reachable}}<span class="badge badge-frost">direct</span>{{else}}<span class="badge badge-yellow">interrupted</span>{{end}}</td>
            </tr>
{{- end}}
            </tbody>
          </table>
        </div>

        <div class="card mt-2" style="padding:0">
          <div class="section-label">Heading Sweep — All Directions ({{len .Sweep}} ranges)</div>
          <p class="text-muted" style="padding:0 12px">Where a jump lands for every whole-degree heading, collapsed into contiguous ranges. "(void)" ranges intersect no system.</p>
          <table class="sortable">
            <thead><tr><th class="sortable">Headings</th><th class="sortable" style="text-align:right">Span</th><th class="sortable">Lands At</th><th class="sortable" style="text-align:right">Distance</th><th style="text-align:right">Travel (ticks)</th></tr></thead>
            <tbody>
{{- range .Sweep}}
            <tr>
              <td data-sort="{{.StartDeg}}">{{if eq .StartDeg .EndDeg}}{{.StartDeg}}°{{else}}{{.StartDeg}}°–{{.EndDeg}}°{{end}}</td>
              <td style="text-align:right">{{.Width}}°</td>
              <td>{{if .Void}}<span class="text-muted">(void)</span>{{else}}<a href="../{{.LandsAtID}}/">{{.LandsAt}}</a>{{if .Station}} <span class="badge badge-frost">station</span>{{end}}{{end}}</td>
              <td style="text-align:right" data-sort="{{printf "%.1f" .Distance}}">{{if .Void}}<span class="text-muted">—</span>{{else}}{{printf "%.0f" .Distance}}{{end}}</td>
              <td style="text-align:right">{{if .Void}}<span class="text-muted">—</span>{{else}}{{.Ticks}} ({{.Duration}}){{end}}</td>
            </tr>
{{- end}}
            </tbody>
          </table>
        </div>

        <div class="card mt-2">
          <div class="section-label">Interrupted Paths</div>
          <p class="text-muted">The full listing of interrupted (non-station) jump paths will be added in a later phase.</p>
        </div>
    </main>
` + sortScript + `
</body>
</html>`
