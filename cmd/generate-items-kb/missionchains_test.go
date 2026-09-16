package main

import "testing"

// chainFixture mirrors the shape the knowledge DB actually produces: one long
// single-type chain, a chain that crosses types, a merge where two heads run
// into a shared tail, a head whose continuation has not been scraped, and
// procedural filler.
func chainFixture() []*Mission {
	m := func(id, title, typ string, diff int, next string, procedural bool) *Mission {
		return &Mission{ID: id, Title: title, Type: typ, Difficulty: diff, ChainNext: next, Procedural: procedural}
	}
	return []*Mission{
		// smuggling: one chain of three, head difficulty 2
		m("no_questions_asked", "No Questions Asked", "smuggling", 2, "across_the_line", false),
		m("across_the_line", "Across the Line", "smuggling", 4, "an_introduction", false),
		m("an_introduction", "An Introduction", "smuggling", 5, "", false),
		// mining chain crossing into exploration, head difficulty 1
		m("first_cargo", "First Cargo", "mining", 1, "scouting_the_lanes", false),
		m("scouting_the_lanes", "Scouting the Lanes", "exploration", 2, "", false),
		// two exploration heads merging into a shared tail
		m("consult_rim", "Consulting the Outer Rim", "exploration", 5, "impossible_list", false),
		m("consult_void", "Consulting the Voidborn", "exploration", 3, "impossible_list", false),
		m("impossible_list", "Impossible List", "exploration", 5, "", false),
		// non-procedural, no links at all
		m("lone_wolf", "Lone Wolf", "smuggling", 7, "", false),
		// non-procedural head whose continuation is not in the table
		m("dangling_head", "Dangling Head", "smuggling", 3, "never_scraped", false),
		// procedural filler
		m("proc_a", "Proc A", "smuggling", 9, "", true),
		m("proc_b", "Proc B", "smuggling", 1, "", true),
	}
}

func chainTitles(c MissionChain) []string {
	out := make([]string, len(c.Steps))
	for i, s := range c.Steps {
		out[i] = s.Mission.Title
	}
	return out
}

func eq(t *testing.T, got, want []string, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
}

func TestBuildMissionChainsFollowsChainOrder(t *testing.T) {
	chains := buildMissionChains(chainFixture())
	// Four chains: smuggling trio, mining->exploration pair, and the two
	// merging exploration routes. Single missions are not chains.
	if len(chains) != 4 {
		var got []string
		for _, c := range chains {
			got = append(got, c.Head().Title)
		}
		t.Fatalf("built %d chains (heads %v), want 4", len(chains), got)
	}
	byHead := map[string]MissionChain{}
	for _, c := range chains {
		byHead[c.Head().ID] = c
	}
	eq(t, chainTitles(byHead["no_questions_asked"]),
		[]string{"No Questions Asked", "Across the Line", "An Introduction"}, "smuggling chain")
	eq(t, chainTitles(byHead["first_cargo"]),
		[]string{"First Cargo", "Scouting the Lanes"}, "cross-type chain")
}

func TestBuildMissionChainsRepeatsSharedTail(t *testing.T) {
	byHead := map[string]MissionChain{}
	for _, c := range buildMissionChains(chainFixture()) {
		byHead[c.Head().ID] = c
	}
	eq(t, chainTitles(byHead["consult_rim"]),
		[]string{"Consulting the Outer Rim", "Impossible List"}, "merge route A")
	eq(t, chainTitles(byHead["consult_void"]),
		[]string{"Consulting the Voidborn", "Impossible List"}, "merge route B")
}

func TestChainsForTypeSortsByHeadDifficultyAndMarksForeign(t *testing.T) {
	chains := buildMissionChains(chainFixture())

	// Exploration sees three chains: the mining-headed one (difficulty 1),
	// then the two merge routes (difficulty 3 and 5).
	got := chainsForType(chains, "exploration")
	var heads []string
	for _, c := range got {
		heads = append(heads, c.Head().Title)
	}
	eq(t, heads, []string{"First Cargo", "Consulting the Voidborn", "Consulting the Outer Rim"},
		"exploration chain order")

	// In the mining-headed chain, the head is foreign to this page and links
	// across; the exploration step is local.
	cross := got[0]
	if !cross.Steps[0].Foreign {
		t.Error("First Cargo should be foreign on the exploration page")
	}
	if cross.Steps[0].Href != "../mining/first_cargo.html" {
		t.Errorf("foreign href = %q, want ../mining/first_cargo.html", cross.Steps[0].Href)
	}
	if cross.Steps[0].TypeIcon == "" {
		t.Error("foreign step should carry a type icon")
	}
	if cross.Steps[1].Foreign {
		t.Error("Scouting the Lanes is exploration, should not be foreign")
	}
	if cross.Steps[1].Href != "scouting_the_lanes.html" {
		t.Errorf("local href = %q, want scouting_the_lanes.html", cross.Steps[1].Href)
	}
}

func TestChainsForTypeExcludesUnrelatedChains(t *testing.T) {
	chains := buildMissionChains(chainFixture())
	for _, c := range chainsForType(chains, "smuggling") {
		if c.Head().ID == "first_cargo" {
			t.Error("smuggling page should not show the mining/exploration chain")
		}
	}
}

func TestPartitionMissionsSplitsStandaloneFromProcedural(t *testing.T) {
	all := chainFixture()
	chains := buildMissionChains(all)
	var smuggling []*Mission
	for _, m := range all {
		if m.Type == "smuggling" {
			smuggling = append(smuggling, m)
		}
	}
	standalone, procedural := partitionMissions(smuggling, chainMemberIDs(chains))

	// Chained smuggling missions are excluded; the dangling head and the
	// unlinked mission remain, sorted by difficulty.
	eq(t, missionTitles(standalone), []string{"Dangling Head", "Lone Wolf"}, "standalone")
	eq(t, missionTitles(procedural), []string{"Proc B", "Proc A"}, "procedural")
}

func missionTitles(ms []*Mission) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Title
	}
	return out
}

func TestMissionTitleFromID(t *testing.T) {
	cases := []struct{ id, want string }{
		{"bid_and_ask", "Bid and Ask"},
		{"network_expansion", "Network Expansion"},
		{"the_anvils_stamp", "The Anvils Stamp"},
		{"return_journey", "Return Journey"},
		{"silicon_supply_for_science", "Silicon Supply for Science"},
		{"into_the_unknown", "Into the Unknown"},
		{"end_of_the_line", "End of the Line"},
		{"patrol_to_valor", "Patrol to Valor"},
		{"phase_two_clearance", "Phase Two Clearance"},
		{"", ""},
	}
	for _, c := range cases {
		if got := missionTitleFromID(c.id); got != c.want {
			t.Errorf("missionTitleFromID(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}
