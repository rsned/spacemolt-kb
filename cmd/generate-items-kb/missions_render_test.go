package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestCategoryPageCollapsesFamilies(t *testing.T) {
	var ms []*Mission
	for i := range 2000 {
		m := fm(fmt.Sprintf("trade_%d", i), fmt.Sprintf("Trade Run: Good%d to Station%d", i, i%7), "trading", true,
			fmt.Sprintf("origin_%d", i%11))
		m.RewardsSkillXP = []MissionReward{{Name: "trading", Value: 20 + i%3*10}}
		m.Objectives = []MissionObjective{{Type: "deliver_item", ItemID: fmt.Sprintf("good_%d", i),
			Quantity: 5, TargetBaseID: fmt.Sprintf("station_%d", i%7)}}
		ms = append(ms, m)
	}
	cats := buildMissionCategories(ms)
	if len(cats) != 1 {
		t.Fatalf("categories = %d", len(cats))
	}
	out, err := renderCategoryPage(cats[0])
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if n := strings.Count(s, `class="family-card"`); n != 1 {
		t.Fatalf("family cards = %d, want 1", n)
	}
	for _, want := range []string{"Trade Run", "20–40 trading XP", "2000 goods"} {
		if !strings.Contains(s, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(s, "trade_1999.html") {
		t.Error("page lists an individual variant")
	}
	if len(out) >= 60_000 {
		t.Errorf("page is %d bytes, want < 60000", len(out))
	}
}

func TestCategoryPageSectionsOrderAndBadges(t *testing.T) {
	a := fm("no_questions_asked", "No Questions Asked", "smuggling", false, "treasure_cache_trading_post")
	a.ChainNext = "supply_run"
	b := fm("supply_run", "Supply Run", "smuggling", false, "treasure_cache_trading_post")
	b.ExclusiveBaseID, b.ExclusiveBaseName = "treasure_cache_trading_post", "Treasure Cache Trading Post"
	one := fm("lone", "Lone Wolf", "smuggling", false, "traders_rest")
	one.Locations[0].BaseName = "Traders' Rest & Resort"
	fac := fm("faction_abc", "ROCI Darksteel Supply Contract", "smuggling", false, "x")
	ms := []*Mission{a, b, one, fac}
	for i := range 3 {
		ms = append(ms, fm(fmt.Sprintf("c%d", i), fmt.Sprintf("Contraband Run: G%d to Y", i), "smuggling", true, "y"))
	}
	// buildMissionChains wires chain links the way loadMissions does.
	a.ChainNextTitle = b.Title
	cats := buildMissionCategories(ms)
	out, err := renderCategoryPage(cats[0])
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	order := []string{"Mission Chains", "One-off Missions", "Posted by player factions", "Template Families"}
	last := -1
	for _, h := range order {
		i := strings.Index(s, h)
		if i < 0 || i < last {
			t.Fatalf("section %q at %d (previous at %d)", h, i, last)
		}
		last = i
	}
	if !strings.Contains(s, "ONLY AT Treasure Cache Trading Post") {
		t.Error("missing chain ONLY AT badge")
	}
	if !strings.Contains(s, "Traders&#39; Rest &amp; Resort") {
		t.Error("station heading not escaped")
	}
}

func TestTopIndexShowsBreakdownCounts(t *testing.T) {
	var ms []*Mission
	for i := range 3 {
		ms = append(ms, fm(fmt.Sprintf("c%d", i), fmt.Sprintf("Contraband Run: G%d", i), "smuggling", true))
	}
	ms = append(ms, fm("lone", "Lone Wolf", "smuggling", false))
	out, err := renderTopPage(buildMissionCategories(ms))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"0 chains", "1 one-offs", "1 families", "3 variants"} {
		if !strings.Contains(s, want) {
			t.Errorf("top index lacks %q", want)
		}
	}
}
