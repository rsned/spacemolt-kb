package main

import (
	"fmt"
	"testing"
)

// fm builds a fixture mission; loc lists offering base ids.
func fm(id, title, typ string, procedural bool, loc ...string) *Mission {
	m := &Mission{ID: id, Title: title, Type: typ, Procedural: procedural}
	for _, b := range loc {
		m.Locations = append(m.Locations, MissionLocation{BaseID: b, BaseName: titleCase(b)})
	}
	return m
}

func familyNames(fs []*MissionFamily) map[string]int {
	out := map[string]int{}
	for _, f := range fs {
		out[f.Name] = len(f.Variants)
	}
	return out
}

func TestFamilyKeySplitsOnFirstColon(t *testing.T) {
	k, n := familyKey(fm("a", "Trade Run: Fuel Cell to Korr", "trading", true), "Odd jobs")
	if k != "trading|Trade Run" || n != "Trade Run" {
		t.Fatalf("got (%q,%q)", k, n)
	}
	_, n = familyKey(fm("b", "HEXC supply: Whale Oil", "delivery", false), "Odd jobs")
	if n != "HEXC supply" {
		t.Fatalf("got %q", n)
	}
}

func TestPartitionFamiliesRemovesChainMembersFirst(t *testing.T) {
	ms := []*Mission{fm("supply_run", "Supply Run", "smuggling", false)}
	for i := range 3 {
		ms = append(ms, fm(fmt.Sprintf("sr%d", i), fmt.Sprintf("Supply Run: Good%d to Y", i), "smuggling", true))
	}
	fams, oneOffs, fac := partitionFamilies(ms, map[string]bool{"supply_run": true})
	if got := familyNames(fams); len(got) != 1 || got["Supply Run"] != 3 {
		t.Fatalf("families = %v", got)
	}
	if len(oneOffs) != 0 || len(fac) != 0 {
		t.Fatalf("chain member leaked: oneOffs=%d faction=%d", len(oneOffs), len(fac))
	}
}

func TestPartitionFamiliesThreshold(t *testing.T) {
	ms := []*Mission{
		fm("b1", "Border Job: A", "smuggling", true),
		fm("b2", "Border Job: B", "smuggling", true),
		fm("h1", "HEXC recon: Hex Star", "exploration", false),
		fm("h2", "HEXC recon: Hex Star", "exploration", false),
		fm("h3", "HEXC recon: Hex Star", "exploration", false),
	}
	fams, oneOffs, _ := partitionFamilies(ms, nil)
	if got := familyNames(fams); len(got) != 1 || got["HEXC recon"] != 3 {
		t.Fatalf("families = %v", got)
	}
	if len(oneOffs) != 2 {
		t.Fatalf("oneOffs = %d, want 2", len(oneOffs))
	}
}

func TestPartitionFamiliesOddJobs(t *testing.T) {
	var ms []*Mission
	for i := range 3 {
		ms = append(ms, fm(fmt.Sprintf("p%d", i), "Please hold", "delivery", true))
		ms = append(ms, fm(fmt.Sprintf("c%d", i), "The chair inspection", "delivery", true))
	}
	fams, _, _ := partitionFamilies(ms, nil)
	if got := familyNames(fams); len(got) != 1 || got["Odd jobs"] != 6 {
		t.Fatalf("families = %v", got)
	}

	var ex []*Mission
	for i := range 4 {
		ex = append(ex, fm(fmt.Sprintf("w%d", i), "Anomalous Readings", "exploration", true))
	}
	fams, _, _ = partitionFamilies(ex, nil)
	if got := familyNames(fams); len(got) != 1 || got["Anomalous Readings"] != 4 {
		t.Fatalf("families = %v", got)
	}
}

func TestPartitionFamiliesFactionOneOffs(t *testing.T) {
	ms := []*Mission{
		fm("faction_abc", "ROCI Darksteel Supply Contract", "delivery", false),
		fm("story", "Scraping By", "delivery", false),
	}
	_, oneOffs, fac := partitionFamilies(ms, nil)
	if len(fac) != 1 || fac[0].ID != "faction_abc" {
		t.Fatalf("faction = %v", fac)
	}
	if len(oneOffs) != 1 || oneOffs[0].ID != "story" {
		t.Fatalf("oneOffs = %v", oneOffs)
	}
}

// variant builds a family member with one objective.
func variant(id string, credits int, xp map[string]int, objType, itemID, target string, qty int, loc ...string) *Mission {
	m := fm(id, "X: "+id, "trading", true, loc...)
	m.RewardsCredits = credits
	for k, v := range xp {
		m.RewardsSkillXP = append(m.RewardsSkillXP, MissionReward{Name: k, Value: v})
	}
	if objType != "" {
		m.Objectives = []MissionObjective{{Type: objType, ItemID: itemID, ItemName: titleCase(itemID),
			Quantity: qty, TargetBaseID: target, TargetBaseName: titleCase(target)}}
	}
	return m
}

func summarized(vs ...*Mission) *MissionFamily {
	f := &MissionFamily{Name: "X", Type: "trading", Variants: vs}
	f.summarize()
	return f
}

func TestRewardSummaryOmitsZeroCredits(t *testing.T) {
	f := summarized(
		variant("a", 0, map[string]int{"trading": 20}, "", "", "", 0),
		variant("b", 0, map[string]int{"trading": 40}, "", "", "", 0),
		variant("c", 0, map[string]int{"trading": 30}, "", "", "", 0),
	)
	if got := f.Rewards.Parts(); len(got) != 1 || got[0] != "20–40 trading XP" {
		t.Fatalf("Parts = %q", got)
	}
}

func TestRewardSummaryCreditsAndXP(t *testing.T) {
	f := summarized(
		variant("a", 480, map[string]int{"trading": 20}, "", "", "", 0),
		variant("b", 1750, nil, "", "", "", 0),
		variant("c", 1125, nil, "", "", "", 0),
	)
	if got := f.Rewards.Parts(); len(got) < 1 || got[0] != "480–1,750 cr" {
		t.Fatalf("Parts = %q", got)
	}
}

func TestFamilyPatternDeliverVaryingItemAndTarget(t *testing.T) {
	f := summarized(
		variant("a", 0, nil, "deliver_item", "void_dust", "ironhearth", 5),
		variant("b", 0, nil, "deliver_item", "gutter_flux", "grand_exchange", 8),
		variant("c", 0, nil, "deliver_item", "starshine", "frontier", 5),
	)
	if f.Pattern != "Deliver <N> <good> to <station>" {
		t.Fatalf("Pattern = %q", f.Pattern)
	}
	g := summarized(
		variant("a", 0, nil, "deliver_item", "void_dust", "ironhearth_station", 5),
		variant("b", 0, nil, "deliver_item", "void_dust", "ironhearth_station", 8),
		variant("c", 0, nil, "deliver_item", "void_dust", "ironhearth_station", 6),
	)
	if g.Pattern != "Deliver <N> Void Dust to Ironhearth Station" {
		t.Fatalf("Pattern = %q", g.Pattern)
	}
}

func TestFamilyBreakdownByGoodCapsAt25(t *testing.T) {
	var vs []*Mission
	for i := range 30 {
		vs = append(vs, variant(fmt.Sprintf("v%02d", i), 100, nil, "deliver_item", fmt.Sprintf("good_%02d", i), "x", 1, "s1"))
	}
	vs = append(vs, variant("dup", 100, nil, "deliver_item", "good_29", "x", 1, "s2"))
	f := summarized(vs...)
	if f.BreakdownBy != "good" || len(f.Breakdown) != 25 || f.BreakdownMore != 5 {
		t.Fatalf("by=%q rows=%d more=%d", f.BreakdownBy, len(f.Breakdown), f.BreakdownMore)
	}
	if r := f.Breakdown[0]; r.Label != "Good 29" || r.Variants != 2 || r.Stations != 2 {
		t.Fatalf("first row = %+v", r)
	}
	if f.Breakdown[1].Label != "Good 00" {
		t.Fatalf("second row = %q, want Good 00", f.Breakdown[1].Label)
	}
}

func TestFamilyBreakdownByTitleForOddJobs(t *testing.T) {
	var vs []*Mission
	for i := range 3 {
		vs = append(vs, fm(fmt.Sprintf("p%d", i), "Please hold", "delivery", true))
		vs = append(vs, fm(fmt.Sprintf("c%d", i), "The chair inspection", "delivery", true))
	}
	f := &MissionFamily{Name: "Odd jobs", Type: "delivery", Variants: vs}
	f.summarize()
	if f.BreakdownBy != "title" || len(f.Breakdown) != 2 {
		t.Fatalf("by=%q rows=%d", f.BreakdownBy, len(f.Breakdown))
	}
}

func TestFamilyOnlyAtSingleStation(t *testing.T) {
	vs := []*Mission{
		variant("a", 1, nil, "", "", "", 0, "treasure_cache_trading_post"),
		variant("b", 1, nil, "", "", "", 0, "treasure_cache_trading_post"),
		variant("c", 1, nil, "", "", "", 0, "treasure_cache_trading_post"),
	}
	f := summarized(vs...)
	if f.OnlyAt != "Treasure Cache Trading Post" || f.Stations != 1 {
		t.Fatalf("OnlyAt=%q Stations=%d", f.OnlyAt, f.Stations)
	}
	g := summarized(append(vs, variant("d", 1, nil, "", "", "", 0, "frontier_station"))...)
	if g.OnlyAt != "" || g.Stations != 2 {
		t.Fatalf("OnlyAt=%q Stations=%d", g.OnlyAt, g.Stations)
	}
}

func TestFamilyNoLocations(t *testing.T) {
	f := summarized(variant("a", 1, nil, "", "", "", 0), variant("b", 1, nil, "", "", "", 0), variant("c", 1, nil, "", "", "", 0))
	if f.Stations != 0 || f.OnlyAt != "" {
		t.Fatalf("Stations=%d OnlyAt=%q", f.Stations, f.OnlyAt)
	}
}

func TestGroupOneOffsByStationOrder(t *testing.T) {
	var many []string
	for i := range 50 {
		many = append(many, fmt.Sprintf("base_%02d", i))
	}
	ms := []*Mission{
		fm("haul", "First Haul", "mining", false, many...),
		fm("gas", "Gas Futures", "mining", false, "grand_exchange_station"),
		fm("water", "Water Rights", "mining", false, "grand_exchange_station"),
		fm("scrape", "Scraping By", "mining", false, "frontier_station"),
		fm("ghost", "Ghost Job", "mining", false),
	}
	groups := groupOneOffsByStation(ms)
	var got []string
	for _, g := range groups {
		got = append(got, g.Heading)
	}
	want := []string{"Offered at many stations", "Frontier Station", "Grand Exchange Station", "Station not recorded"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("headings = %q, want %q", got, want)
	}
	if !groups[0].Many || groups[0].Missions[0].StationCount() != 50 {
		t.Fatalf("many group = %+v", groups[0])
	}
	if len(groups[2].Missions) != 2 {
		t.Fatalf("grand exchange group has %d missions", len(groups[2].Missions))
	}
}

func TestGroupOneOffsUnknownBaseFallsBackToTitleCase(t *testing.T) {
	m := &Mission{ID: "x", Title: "X", Locations: []MissionLocation{{BaseID: "b495c6003fc83e18f6d8cecbe6929133"}}}
	groups := groupOneOffsByStation([]*Mission{m})
	if len(groups) != 1 || groups[0].Heading != titleCase("b495c6003fc83e18f6d8cecbe6929133") || groups[0].Heading == "" {
		t.Fatalf("groups = %+v", groups)
	}
}

func chainOf(ms ...*Mission) MissionChain {
	var c MissionChain
	for _, m := range ms {
		c.Steps = append(c.Steps, MissionChainStep{Mission: m})
	}
	return c
}

func TestGroupChainsByStartStation(t *testing.T) {
	ge, cc := "grand_exchange_station", "confederacy_central_command"
	chains := []MissionChain{
		// later legs elsewhere do not matter, only the opener's station
		chainOf(fm("first_cargo", "First Cargo", "mining", false, ge), fm("lanes", "Scouting the Lanes", "exploration", false, cc)),
		chainOf(fm("welcome", "Welcome to Sol Central", "mining", false, cc), fm("sirius", "Sirius Expedition", "exploration", false, cc)),
		chainOf(fm("gold", "Scouting the Gold Run", "exploration", false, ge), fm("std", "The Gold Standard", "mining", false, ge)),
		chainOf(fm("haul", "First Haul", "mining", false, ge, cc), fm("h2", "Second Haul", "mining", false)),
		chainOf(fm("ghost", "Ghost Lead", "mining", false), fm("g2", "Ghost Tail", "mining", false, ge)),
	}
	groups := groupChainsByStart(chains)
	var got []string
	for _, g := range groups {
		got = append(got, fmt.Sprintf("%s:%d", g.Heading, len(g.Chains)))
	}
	want := []string{"Starts at many stations:1", "Starts at Confederacy Central Command:1",
		"Starts at Grand Exchange Station:2", "Station not recorded:1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("groups = %q, want %q", got, want)
	}
	// input order (difficulty) is kept inside a group
	if groups[2].Chains[0].Head().ID != "first_cargo" || groups[2].Chains[1].Head().ID != "gold" {
		t.Fatalf("grand exchange order = %s, %s", groups[2].Chains[0].Head().ID, groups[2].Chains[1].Head().ID)
	}
}

func TestFamilyGoodsTotalCountsAllGoods(t *testing.T) {
	var vs []*Mission
	for i := range 7 {
		vs = append(vs, variant(fmt.Sprintf("v%d", i), 1, nil, "deliver_item", fmt.Sprintf("good_%d", i), "x", 1))
	}
	f := summarized(vs...)
	if f.GoodsTotal() != 7 || len(f.Goods) != 4 || f.GoodsMore != 3 {
		t.Fatalf("total=%d chips=%d more=%d", f.GoodsTotal(), len(f.Goods), f.GoodsMore)
	}
}

// Wreck Recovery: no goods, a unique title per variant, a varying target —
// the breakdown must group by destination, not list every title.
func TestFamilyBreakdownSkipsTitleOutsideOddJobs(t *testing.T) {
	var vs []*Mission
	for i := range 6 {
		m := fm(fmt.Sprintf("w%d", i), fmt.Sprintf("Wreck Recovery: Wreck %d", i), "salvage_recovery", true, "s1")
		m.Objectives = []MissionObjective{{Type: "dock_at_base", Quantity: 1, TargetBaseID: fmt.Sprintf("yard_%d", i%2)}}
		vs = append(vs, m)
	}
	f := &MissionFamily{Name: "Wreck Recovery", Type: "salvage_recovery", Variants: vs}
	f.summarize()
	if f.BreakdownBy != "destination" || len(f.Breakdown) != 2 {
		t.Fatalf("by=%q rows=%d", f.BreakdownBy, len(f.Breakdown))
	}
	if f.Pattern != "Dock at <station>" {
		t.Fatalf("Pattern = %q", f.Pattern)
	}
}

// Real odd jobs carry item objectives; they must still break down by title.
func TestFamilyBreakdownOddJobsWithItemsUsesTitle(t *testing.T) {
	var vs []*Mission
	for i := range 3 {
		for _, title := range []string{"Please hold", "The chair inspection"} {
			m := fm(fmt.Sprintf("%s%d", title, i), title, "delivery", true)
			m.Objectives = []MissionObjective{{Type: "deliver_item", ItemID: fmt.Sprintf("odd_%s_%d", title, i), Quantity: 1, TargetBaseID: "x"}}
			vs = append(vs, m)
		}
	}
	f := &MissionFamily{Name: oddJobsFamilyName, Type: "delivery", Variants: vs}
	f.summarize()
	if f.BreakdownBy != "title" || len(f.Breakdown) != 2 {
		t.Fatalf("by=%q rows=%d", f.BreakdownBy, len(f.Breakdown))
	}
}

func TestFamilyPatternOmitsQuantityWithoutItem(t *testing.T) {
	var vs []*Mission
	for i := range 3 {
		m := fm(fmt.Sprintf("h%d", i), "HEXC recon: Hex Star", "exploration", false)
		m.Objectives = []MissionObjective{{Type: "visit_system", Quantity: 1, SystemName: "Dheneb"}}
		vs = append(vs, m)
	}
	f := &MissionFamily{Name: "HEXC recon", Type: "exploration", Variants: vs}
	f.summarize()
	if f.Pattern != "Visit Dheneb" {
		t.Fatalf("Pattern = %q", f.Pattern)
	}
}
