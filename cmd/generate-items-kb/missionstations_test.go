package main

import (
	"fmt"
	"strings"
	"testing"
)

func stationByID(pages []StationMissions, id string) *StationMissions {
	for i := range pages {
		if pages[i].BaseID == id {
			return &pages[i]
		}
	}
	return nil
}

func rowSummary(c StationChain) []string {
	var out []string
	for _, r := range c.Rows {
		switch {
		case r.Collapsed > 0:
			out = append(out, fmt.Sprintf("...%d", r.Collapsed))
		case r.Here:
			out = append(out, "*"+r.Step.Mission.ID)
		default:
			out = append(out, "-"+r.Step.Mission.ID)
		}
	}
	return out
}

func TestStationPagesChainLegsHereAndElsewhere(t *testing.T) {
	ge, cc := "grand_exchange_station", "confederacy_central_command"
	a := fm("a", "A", "delivery", false, ge)
	a.ChainNext = "b"
	b := fm("b", "B", "delivery", false, cc)
	b.ChainNext = "c"
	c := fm("c", "C", "delivery", false, ge)
	chains := buildMissionChains([]*Mission{a, b, c})
	pages := buildStationPages(chains, nil, nil)

	g := stationByID(pages, ge)
	if g == nil || len(g.Chains) != 1 {
		t.Fatalf("grand exchange page = %+v", g)
	}
	if got := rowSummary(g.Chains[0]); fmt.Sprint(got) != fmt.Sprint([]string{"*a", "-b", "*c"}) {
		t.Fatalf("rows = %v", got)
	}
	if g.Chains[0].Note != "Starts here" || g.Chains[0].Rows[1].Where != "Confederacy Central Command" {
		t.Fatalf("note=%q where=%q", g.Chains[0].Note, g.Chains[0].Rows[1].Where)
	}
	if g.Chains[0].Rows[0].Href != "../delivery/a.html" {
		t.Fatalf("href = %q", g.Chains[0].Rows[0].Href)
	}
	if n := stationByID(pages, cc).Chains[0].Note; n != "Leg 2 is here" {
		t.Fatalf("cc note = %q", n)
	}
}

func TestStationPagesCollapseLongRunElsewhere(t *testing.T) {
	ids := []string{"l1", "l2", "l3", "l4", "l5"}
	at := []string{"here", "x", "y", "z", "here"}
	var ms []*Mission
	for i, id := range ids {
		m := fm(id, strings.ToUpper(id), "exploration", false, at[i])
		if i+1 < len(ids) {
			m.ChainNext = ids[i+1]
		}
		ms = append(ms, m)
	}
	// a 2-leg run elsewhere is not collapsed
	s1 := fm("s1", "S1", "exploration", false, "here")
	s1.ChainNext = "s2"
	s2 := fm("s2", "S2", "exploration", false, "x")
	s2.ChainNext = "s3"
	s3 := fm("s3", "S3", "exploration", false, "y")
	ms = append(ms, s1, s2, s3)

	pages := buildStationPages(buildMissionChains(ms), nil, nil)
	h := stationByID(pages, "here")
	got := map[string][]string{}
	for _, c := range h.Chains {
		got[c.Rows[0].Step.Mission.ID] = rowSummary(c)
	}
	if fmt.Sprint(got["l1"]) != fmt.Sprint([]string{"*l1", "...3", "*l5"}) {
		t.Fatalf("long chain rows = %v", got["l1"])
	}
	if fmt.Sprint(got["s1"]) != fmt.Sprint([]string{"*s1", "-s2", "-s3"}) {
		t.Fatalf("short chain rows = %v", got["s1"])
	}
}

func TestStationPagesPlaceholderLegKept(t *testing.T) {
	m := fm("prismatic_prospecting", "Prismatic Prospecting", "exploration", false, "grand_exchange_station")
	m.ChainNext = "nebula_extractor_problem"
	pages := buildStationPages(buildMissionChains([]*Mission{m}), nil, nil)
	if len(pages) != 1 {
		t.Fatalf("pages = %d (placeholder must not create a station)", len(pages))
	}
	rows := pages[0].Chains[0].Rows
	if len(rows) != 2 || !rows[1].Step.Undiscovered || rows[1].Href != "" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestStationPagesChainOrderStartsHereFirst(t *testing.T) {
	x := fm("x1", "Zulu Opener", "mining", false, "s")
	x.Difficulty = 1
	y0 := fm("y0", "Alpha Elsewhere", "mining", false, "t")
	y0.Difficulty = 1
	y0.ChainNext = "y1"
	y1 := fm("y1", "Alpha Leg Here", "mining", false, "s")
	x.ChainNext = "x2"
	x2 := fm("x2", "Zulu Two", "mining", false, "t")
	pages := buildStationPages(buildMissionChains([]*Mission{x, x2, y0, y1}), nil, nil)
	s := stationByID(pages, "s")
	if s.Chains[0].Rows[0].Step.Mission.ID != "x1" {
		t.Fatalf("first chain starts with %s, want the one starting here", s.Chains[0].Rows[0].Step.Mission.ID)
	}
}

func TestStationPagesOneOffsCountOtherStations(t *testing.T) {
	bounty := fm("pirate_bounty", "Pirate Bounty", "combat", false, "a", "b", "c")
	local := fm("gas", "Gas Futures", "mining", false, "a")
	pages := buildStationPages(nil, []*Mission{bounty, local}, nil)
	a := stationByID(pages, "a")
	if a == nil || len(a.OneOffs) != 2 {
		t.Fatalf("station a = %+v", a)
	}
	for _, o := range a.OneOffs {
		if o.M.ID == "pirate_bounty" && o.Others != 2 {
			t.Fatalf("Others = %d, want 2", o.Others)
		}
		if o.M.ID == "gas" && o.Others != 0 {
			t.Fatalf("Others = %d, want 0", o.Others)
		}
	}
	if len(pages) != 3 {
		t.Fatalf("pages = %d, want 3", len(pages))
	}
}

func TestStationIndexGroupsByEmpire(t *testing.T) {
	pages := buildStationPages(nil, []*Mission{
		fm("a", "A", "mining", false, "frontier_station"),
		fm("b", "B", "mining", false, "grand_exchange_station"),
		fm("c", "C", "mining", false, "market_prime_exchange"),
		fm("d", "D", "mining", false, "hex_star"),
	}, map[string]string{
		"frontier_station": "Outer Rim", "grand_exchange_station": "Nebula",
		"market_prime_exchange": "Nebula", "hex_star": "Independent",
	})
	groups := groupStationsByEmpire(pages)
	var got []string
	for _, g := range groups {
		var names []string
		for _, s := range g.Stations {
			names = append(names, s.BaseID)
		}
		got = append(got, g.Empire+":"+strings.Join(names, ","))
	}
	want := []string{"Nebula:grand_exchange_station,market_prime_exchange", "Outer Rim:frontier_station", "Independent:hex_star"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("groups = %q, want %q", got, want)
	}
}

func TestStationPageRenders(t *testing.T) {
	a := fm("a", "First Links", "delivery", false, "ge")
	a.Locations[0].BaseName = "Traders' Rest & Resort"
	a.ChainNext = "b"
	b := fm("b", "Crossing Borders", "delivery", false, "sable")
	pages := buildStationPages(buildMissionChains([]*Mission{a, b}), []*Mission{fm("g", "Gas Futures", "mining", false, "ge")}, nil)
	out, err := renderStationPage(*stationByID(pages, "ge"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		"Traders&#39; Rest &amp; Resort", `href="../delivery/a.html"`, `class="leg-elsewhere"`,
		"Starts here", "Chains with a leg here", "One-off missions here", `href="../mining/g.html"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("station page lacks %q", want)
		}
	}
	idx, err := renderStationIndex(groupStationsByEmpire(pages))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(idx), `href="ge.html"`) {
		t.Error("index lacks station link")
	}
}

// A chain whose first legs are all elsewhere opens with a folded row; sorting
// and rendering must not read a mission from it.
func TestStationPagesChainOpeningWithFoldedLegs(t *testing.T) {
	var ms []*Mission
	at := []string{"x", "y", "z", "here"}
	for i, st := range at {
		m := fm(fmt.Sprintf("f%d", i), fmt.Sprintf("F%d", i), "exploration", false, st)
		if i+1 < len(at) {
			m.ChainNext = fmt.Sprintf("f%d", i+1)
		}
		ms = append(ms, m)
	}
	o := fm("o1", "Opener", "exploration", false, "here")
	o.ChainNext = "o2"
	ms = append(ms, o, fm("o2", "O2", "exploration", false, "x"))

	pages := buildStationPages(buildMissionChains(ms), nil, nil)
	h := stationByID(pages, "here")
	if len(h.Chains) != 2 || h.Chains[0].Head.ID != "o1" {
		t.Fatalf("chains = %+v", h.Chains)
	}
	if got := rowSummary(h.Chains[1]); fmt.Sprint(got) != fmt.Sprint([]string{"...3", "*f3"}) {
		t.Fatalf("rows = %v", got)
	}
	if _, err := renderStationPage(*h); err != nil {
		t.Fatal(err)
	}
}
