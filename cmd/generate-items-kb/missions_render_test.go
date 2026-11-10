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
	for _, want := range []string{"Trade Run", "20–40 trading XP", "2,000 goods"} {
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
	if !strings.Contains(s, "Starts at Treasure Cache Trading Post") {
		t.Error("missing chain start-station heading")
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
	for _, want := range []string{"1 one-off ", "1 family", "3 template variants"} {
		if !strings.Contains(s, want) {
			t.Errorf("top index lacks %q", want)
		}
	}
	for _, bad := range []string{"0 chains", "1 one-offs", "1 families"} {
		if strings.Contains(s, bad) {
			t.Errorf("top index has %q", bad)
		}
	}
}

// Categories whose template variants outnumber their story missions are
// listed under "Mostly templates"; the rest under "Story-heavy".
func TestTopIndexGroupsStoryAndTemplateCategories(t *testing.T) {
	var ms []*Mission
	for i := range 3 {
		ms = append(ms, fm(fmt.Sprintf("c%d", i), fmt.Sprintf("Contraband Run: G%d", i), "smuggling", true))
	}
	ms = append(ms, fm("lone", "Lone Wolf", "smuggling", false), fm("duel", "Duel", "combat", false))
	out, err := renderTopPage(buildMissionCategories(ms))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	story, tmpl := strings.Index(s, "Story-heavy"), strings.Index(s, "Mostly templates")
	combat, smuggling := strings.Index(s, `href="combat/"`), strings.Index(s, `href="smuggling/"`)
	if story < 0 || tmpl < 0 || story >= combat || combat >= tmpl || tmpl >= smuggling {
		t.Fatalf("story=%d combat=%d templates=%d smuggling=%d", story, combat, tmpl, smuggling)
	}
	if !strings.Contains(s, `class="container page-content kbm"`) {
		t.Error("top page lacks the kbm style scope")
	}
}

// A family breakdown shows its first five rows; the rest sit behind an expander.
func TestFamilyBreakdownTopFiveThenExpander(t *testing.T) {
	var ms []*Mission
	for i := range 8 {
		for j := range i + 1 {
			ms = append(ms, fm(fmt.Sprintf("r%d_%d", i, j), fmt.Sprintf("Recon: Spot %d", j), "exploration", true,
				fmt.Sprintf("st%d", i)))
		}
	}
	out, err := renderCategoryPage(buildMissionCategories(ms)[0])
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	i := strings.Index(s, "<details")
	if i < 0 {
		t.Fatal("no expander")
	}
	if n := strings.Count(s[:i], `<tr class="bd-row">`); n != 5 {
		t.Fatalf("rows before expander = %d, want 5", n)
	}
	if n := strings.Count(s[i:], `<tr class="bd-row">`); n != 3 || !strings.Contains(s[i:], "3 more") {
		t.Fatalf("rows in expander = %d (want 3, labelled \"3 more\")", n)
	}
}

// Chains are compact: no repeated header row and no Faction/Objectives columns.
func TestCategoryChainsCompact(t *testing.T) {
	a := fm("a", "Alpha", "mining", false, "s")
	a.ChainNext = "b"
	b := fm("b", "Bravo", "mining", false, "s")
	out, err := renderCategoryPage(buildMissionCategories([]*Mission{a, b})[0])
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, bad := range []string{"<thead>", ">Faction<", ">Objectives<"} {
		if strings.Contains(s, bad) {
			t.Errorf("category page has %q", bad)
		}
	}
	if !strings.Contains(s, `href="a.html"`) || !strings.Contains(s, `href="b.html"`) {
		t.Error("chain legs not linked")
	}
}

// The one-offs heading counts missions; the "many stations" group is not a station.
func TestOneOffHeadingCountsMissions(t *testing.T) {
	ms := []*Mission{
		fm("a", "Alpha", "combat", false, "s1", "s2"),
		fm("b", "Bravo", "combat", false, "s1"),
		fm("c", "Charlie", "combat", false, "s1"),
	}
	out, err := renderCategoryPage(buildMissionCategories(ms)[0])
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `One-off Missions <span class="text-muted">3 missions</span>`) {
		i := strings.Index(s, "One-off Missions")
		t.Fatalf("heading = %q", s[i:i+80])
	}
}

// A one-off whose chain_next has not been scraped is a chain opener: it is
// listed under the chains with a placeholder leg that is not a link.
func TestChainOpenerWithUnscrapedNextRendersPlaceholder(t *testing.T) {
	m := fm("prismatic_prospecting", "Prismatic Prospecting", "mining", false, "grand_exchange_station")
	m.ChainNext = "nebula_extractor_problem"
	out, err := renderCategoryPage(buildMissionCategories([]*Mission{m})[0])
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	chains, oneOffs := s, ""
	if i := strings.Index(s, "One-off Missions"); i >= 0 {
		chains, oneOffs = s[:i], s[i:]
	}
	if !strings.Contains(chains, "Prismatic Prospecting") || strings.Contains(oneOffs, "Prismatic Prospecting") {
		t.Fatal("opener not listed under chains")
	}
	if !strings.Contains(chains, "Nebula Extractor Problem") {
		t.Fatal("placeholder leg missing")
	}
	if strings.Contains(s, `href="nebula_extractor_problem.html"`) {
		t.Fatal("placeholder leg is a link")
	}
}
