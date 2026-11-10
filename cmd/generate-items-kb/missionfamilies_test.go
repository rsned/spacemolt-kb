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
