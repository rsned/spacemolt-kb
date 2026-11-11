package main

import (
	"fmt"
	"strings"
	"testing"
)

// branchFixture: a -> b; b unlocks c and d (no chain_next); c -> e.
// Mirrors The Cover Story -> The Listening Foundry -> {It Hums, The Foundry
// Points Somewhere}.
func branchFixture() []*Mission {
	a := fm("a", "Alpha", "exploration", false, "home")
	a.ChainNext = "b"
	b := fm("b", "Bravo", "delivery", false, "home")
	b.Unlocks = []ChainLink{{ID: "d", Title: "Delta"}, {ID: "c", Title: "Charlie"}}
	c := fm("c", "Charlie", "exploration", false, "home")
	c.ChainNext = "e"
	d := fm("d", "Delta", "delivery", false, "home")
	e := fm("e", "Echo", "exploration", false, "far")
	return []*Mission{a, b, c, d, e}
}

func chainShape(c MissionChain) string {
	var ids []string
	for _, s := range c.Steps {
		ids = append(ids, s.Mission.ID)
	}
	out := fmt.Sprintf("%d:%s", c.Start, strings.Join(ids, ">"))
	if len(c.Branches) > 0 {
		var bs []string
		for _, b := range c.Branches {
			bs = append(bs, chainShape(b))
		}
		out += "{" + strings.Join(bs, "|") + "}"
	}
	return out
}

func TestBuildMissionChainsBranches(t *testing.T) {
	chains := buildMissionChains(branchFixture())
	if len(chains) != 1 {
		t.Fatalf("chains = %d, want 1 (branch heads must not stand alone)", len(chains))
	}
	if got, want := chainShape(chains[0]), "0:a>b{2:c>e|2:d}"; got != want {
		t.Fatalf("shape = %s, want %s (branches ordered by title)", got, want)
	}
	m := chainMemberIDs(chains)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		if !m[id] {
			t.Errorf("%s not a chain member", id)
		}
	}
}

// A single unlock with no chain_next continues the chain in a straight line.
func TestBuildMissionChainsSingleUnlockIsLinear(t *testing.T) {
	a := fm("a", "Alpha", "exploration", false)
	a.Unlocks = []ChainLink{{ID: "b", Title: "Bravo"}}
	b := fm("b", "Bravo", "exploration", false)
	chains := buildMissionChains([]*Mission{a, b})
	if len(chains) != 1 || chainShape(chains[0]) != "0:a>b" {
		t.Fatalf("chains = %v", chains)
	}
}

// An unlocked mission not scraped yet becomes a placeholder branch, titled
// from the unlock record.
func TestBuildMissionChainsUnscrapedBranch(t *testing.T) {
	a := fm("a", "Alpha", "exploration", false)
	a.Unlocks = []ChainLink{{ID: "x", Title: "X-Ray Vision"}, {ID: "b", Title: "Bravo"}}
	b := fm("b", "Bravo", "exploration", false)
	chains := buildMissionChains([]*Mission{a, b})
	if len(chains) != 1 || len(chains[0].Branches) != 2 {
		t.Fatalf("chains = %v", chains)
	}
	ph := chains[0].Branches[1].Steps[0]
	if !ph.Undiscovered || ph.Mission.Title != "X-Ray Vision" {
		t.Fatalf("placeholder = %+v", ph)
	}
}

func TestCategoryPageRendersBranches(t *testing.T) {
	cats := buildMissionCategories(branchFixture())
	var ex MissionCategoryInfo
	for _, c := range cats {
		if c.Name == "exploration" {
			ex = c
		}
	}
	out, err := renderCategoryPage(ex)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"unlocks 2 branches", "Branch 1 of 2", "Branch 2 of 2", `href="c.html"`, `href="../delivery/d.html"`} {
		if !strings.Contains(s, want) {
			t.Errorf("category page lacks %q", want)
		}
	}
	// c continues as leg 3 inside its branch.
	if !strings.Contains(s, `<td class="chain-step-col">3</td>`) {
		t.Error("branch legs are not numbered on from the stem")
	}
}

func TestStationPageBranchNote(t *testing.T) {
	pages := buildStationPages(buildMissionChains(branchFixture()), nil, nil)
	far := stationByID(pages, "far")
	if far == nil || len(far.Chains) != 1 {
		t.Fatalf("far = %+v", far)
	}
	if n := far.Chains[0].Note; n != "Alpha — leg 4 (branch 1) is here" {
		t.Errorf("note = %q", n)
	}
	home := stationByID(pages, "home")
	if len(home.Chains) != 1 || len(home.Chains[0].Branches) != 2 {
		t.Fatalf("home chains = %+v", home.Chains)
	}
	out, err := renderStationPage(*home)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"unlocks 2 branches", "Branch 2 of 2", `href="../exploration/e.html"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("station page lacks %q", want)
		}
	}
}

func TestDetailPageListsUnlocks(t *testing.T) {
	ms := branchFixture()
	linkMissionChains(ms)
	out, err := renderMissionDetail(ms[1])
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	i := strings.Index(s, ">Unlocks<")
	if i < 0 {
		t.Fatal("no Unlocks row")
	}
	row := s[i : i+400]
	if !strings.Contains(row, `href="../exploration/c.html"`) || !strings.Contains(row, "Delta") {
		t.Errorf("unlocks row = %s", row)
	}
	if strings.Index(row, "Charlie") > strings.Index(row, "Delta") {
		t.Error("unlocks not sorted by title")
	}
}
