package main

import (
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

const stationCoverExactK = 3
const stationCoverBatch = 10

// galaxyCover is one target's galaxy-wide (no distance limit) BoM min-station
// cover, with ids resolved to display names, for the build-cost detail banner.
type galaxyCover struct {
	Feasible bool
	Count    int
	Exact    bool
	Stations []string // cover station display names (when Feasible)
	Missing  []string // input display names with no depth anywhere (when !Feasible)
}

// galaxyCoverFor computes the galaxy-wide BoM min-station cover for one target.
func galaxyCoverFor(t buildcost.Target, depth buildcost.StationDepth, ids []string,
	stationNames, itemNames map[string]string) galaxyCover {
	c := buildcost.MinStationCover(t.BoM, depth, ids, stationCoverExactK)
	return galaxyCover{
		Feasible: c.Feasible,
		Count:    c.Count,
		Exact:    c.Exact,
		Stations: displayNames(c.Stations, stationNames),
		Missing:  displayNames(c.Missing, itemNames),
	}
}

type coverEntry struct {
	ID, Name, Category, Kind string
	BoM                      buildcost.CoverResult
	Recipe                   buildcost.CoverResult
	RecipeNA                 bool
	RecipeNAReason           string
	BatchSensitive           bool
	ExampleCover             []string // BoM cover, station display names
}

type unbuildableEntry struct {
	ID, Name, Category, Kind string
	Missing                  []string // input display names
}

type histBar struct {
	Stations, Count, HeightPct int
}

type stationCoverPage struct {
	Total               int
	SingleStation       int
	MaxStations         int
	UnbuildableCount    int
	BatchSensitiveCount int
	HardestName         string
	HardestID           string
	AvgStations         float64
	Buildable           []coverEntry
	Unbuildable         []unbuildableEntry
	BoMHistogram        []histBar
	RecipeHistogram     []histBar
}

// stationDepthFromBooks sums each station's sell-ladder quantities per item.
func stationDepthFromBooks(books map[string]*buildcost.Book) buildcost.StationDepth {
	depth := make(buildcost.StationDepth, len(books))
	for st, b := range books {
		if b == nil {
			continue
		}
		m := map[string]float64{}
		for item, ladder := range b.Sell {
			var sum float64
			for _, o := range ladder {
				sum += o.Qty
			}
			m[item] = sum
		}
		depth[st] = m
	}
	return depth
}

// bestRecipeCover returns the feasible recipe cover with the smallest station
// count (ties: fewer stations already handled; then recipe order as given), and
// whether the target's Recipe mode is N/A.
func bestRecipeCover(t buildcost.Target, depth buildcost.StationDepth, ids []string) (buildcost.CoverResult, bool) {
	if t.RecipeNA != "" || len(t.Recipes) == 0 {
		return buildcost.CoverResult{}, true
	}
	var best buildcost.CoverResult
	found := false
	for _, r := range t.Recipes {
		c := buildcost.MinStationCover(r.Inputs, depth, ids, stationCoverExactK)
		if !c.Feasible {
			continue
		}
		if !found || c.Count < best.Count {
			best = c
			found = true
		}
	}
	if !found {
		return buildcost.CoverResult{}, true // no recipe sourceable
	}
	return best, false
}

func scaleReqs(reqs []buildcost.Requirement, factor float64) []buildcost.Requirement {
	out := make([]buildcost.Requirement, len(reqs))
	for i, r := range reqs {
		out[i] = buildcost.Requirement{ItemID: r.ItemID, Qty: r.Qty * factor}
	}
	return out
}

func displayNames(ids []string, names map[string]string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		if n := names[id]; n != "" {
			out[i] = n
		} else {
			out[i] = id
		}
	}
	return out
}

func buildStationCoverPage(targets []buildcost.Target, depth buildcost.StationDepth, stationIDs []string,
	stationNames, itemNames, categories map[string]string) stationCoverPage {
	p := stationCoverPage{Total: len(targets)}
	var sumStations int

	for _, t := range targets {
		bom := buildcost.MinStationCover(t.BoM, depth, stationIDs, stationCoverExactK)
		name := itemNames[t.ID]
		if name == "" {
			name = t.ID
		}
		if !bom.Feasible {
			p.Unbuildable = append(p.Unbuildable, unbuildableEntry{
				ID: t.ID, Name: name, Category: categories[t.ID], Kind: t.Kind,
				Missing: displayNames(bom.Missing, itemNames),
			})
			continue
		}
		rec, recNA := bestRecipeCover(t, depth, stationIDs)
		batch := buildcost.MinStationCover(scaleReqs(t.BoM, stationCoverBatch), depth, stationIDs, stationCoverExactK)
		e := coverEntry{
			ID: t.ID, Name: name, Category: categories[t.ID], Kind: t.Kind,
			BoM: bom, Recipe: rec, RecipeNA: recNA,
			RecipeNAReason: t.RecipeNA,
			BatchSensitive: batch.Feasible && batch.Count > bom.Count,
			ExampleCover:   displayNames(bom.Stations, stationNames),
		}
		p.Buildable = append(p.Buildable, e)
		sumStations += bom.Count
		if bom.Count == 1 {
			p.SingleStation++
		}
		if bom.Count > p.MaxStations {
			p.MaxStations = bom.Count
			p.HardestID = t.ID
			p.HardestName = name
		}
		if e.BatchSensitive {
			p.BatchSensitiveCount++
		}
	}

	p.UnbuildableCount = len(p.Unbuildable)
	if len(p.Buildable) > 0 {
		p.AvgStations = float64(sumStations) / float64(len(p.Buildable))
	}

	// Sort buildable by BoM count desc, then name; unbuildable by name.
	sort.Slice(p.Buildable, func(i, j int) bool {
		if p.Buildable[i].BoM.Count != p.Buildable[j].BoM.Count {
			return p.Buildable[i].BoM.Count > p.Buildable[j].BoM.Count
		}
		if p.Buildable[i].Name != p.Buildable[j].Name {
			return p.Buildable[i].Name < p.Buildable[j].Name
		}
		return p.Buildable[i].ID < p.Buildable[j].ID
	})
	sort.Slice(p.Unbuildable, func(i, j int) bool {
		if p.Unbuildable[i].Name != p.Unbuildable[j].Name {
			return p.Unbuildable[i].Name < p.Unbuildable[j].Name
		}
		return p.Unbuildable[i].ID < p.Unbuildable[j].ID
	})

	p.BoMHistogram = histogram(p.Buildable, func(e coverEntry) (int, bool) { return e.BoM.Count, true })
	p.RecipeHistogram = histogram(p.Buildable, func(e coverEntry) (int, bool) {
		return e.Recipe.Count, !e.RecipeNA && e.Recipe.Feasible
	})
	return p
}

// histogram buckets buildable entries by a count selector into bars 1..max, with
// zero-count buckets filled in and HeightPct scaled to the tallest bar.
func histogram(entries []coverEntry, sel func(coverEntry) (int, bool)) []histBar {
	counts := map[int]int{}
	maxN := 0
	for _, e := range entries {
		n, ok := sel(e)
		if !ok || n < 1 {
			continue
		}
		counts[n]++
		if n > maxN {
			maxN = n
		}
	}
	if maxN == 0 {
		return nil
	}
	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}
	bars := make([]histBar, 0, maxN)
	for n := 1; n <= maxN; n++ {
		h := 0
		if maxCount > 0 {
			h = int(100 * float64(counts[n]) / float64(maxCount))
		}
		bars = append(bars, histBar{Stations: n, Count: counts[n], HeightPct: h})
	}
	return bars
}
