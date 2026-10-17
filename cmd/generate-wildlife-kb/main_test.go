package main

import (
	htmltpl "html/template"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/wildlife"
)

func view(name, role string, sighted bool) speciesView {
	v := speciesView{Species: wildlife.Species{Name: name, Role: role}, Slug: name}
	if sighted {
		v.Places = []wildlife.Place{{}}
	}
	return v
}

// The page lists species grouped by role, but the map's species dropdown is a
// plain A-Z list of the sighted ones.
func TestMapOptionsAlphabeticalSightedOnly(t *testing.T) {
	views := []speciesView{ // role order, as LoadSpecies returns them
		view("Belt-Grazer", "grazer", true),
		view("aeonbear", "grazer", true),
		view("Crusher-Mantis", "predator", true),
		view("Quiet-One", "predator", false),
		view("Ash-Scarab", "scavenger", true),
	}
	var got []string
	for _, v := range mapOptions(views) {
		got = append(got, v.Name)
	}
	want := []string{"aeonbear", "Ash-Scarab", "Belt-Grazer", "Crusher-Mantis"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if views[0].Name != "Belt-Grazer" {
		t.Errorf("mapOptions reordered its input: %v", views[0].Name)
	}
}

func TestCombatRowsMostDangerousFirstWithUnknownsMarked(t *testing.T) {
	leviathan := view("Rainbow Leviathan", "predator", true)
	leviathan.MaxHull = 900
	leviathan.Record = &wildlife.BattleRecord{Battles: 80, WildlifeWins: 51}
	leviathan.Combat = &wildlife.CreatureCombat{Battles: 2, HitMin: .18, HitMax: .85,
		Weapons: map[string]wildlife.CreatureWeapon{
			"beam": {DamageType: "energy", BaseMin: 130, BaseMax: 130, Shots: 6},
			"tail": {DamageType: "kinetic", BaseMin: 40, BaseMax: 60, Shots: 2},
		}}
	grazer := view("Belt-Grazer", "grazer", true)
	grazer.Record = &wildlife.BattleRecord{Battles: 13784, WildlifeWins: 73}
	grazer.Kills = make([]wildlife.Kill, 3)
	quiet := view("Quiet-One", "predator", false)
	quiet.Record = &wildlife.BattleRecord{Battles: 4, WildlifeWins: 4} // too few to rate

	rows := buildCombatRows([]speciesView{quiet, grazer, leviathan})
	if got := [3]string{rows[0].Name, rows[1].Name, rows[2].Name}; got != [3]string{"Rainbow Leviathan", "Belt-Grazer", "Quiet-One"} {
		t.Fatalf("order %v", got)
	}
	l := rows[0]
	if l.Rating != "extreme" || l.RatingRank != 5 || l.DmgMin != 40 || l.DmgMax != 130 || l.Shots != 8 || l.HitMax != .85 || l.LogBattles != 2 {
		t.Errorf("leviathan row %+v", l)
	}
	if len(l.DamageTypes) != 2 {
		t.Errorf("damage types %v", l.DamageTypes)
	}
	g := rows[1]
	if g.Rating != "minimal" || g.Kills != 3 || g.DmgMax != -1 || g.HitMax != -1 {
		t.Errorf("grazer row %+v", g)
	}
	q := rows[2]
	if q.Rating != "" || q.RatingRank != 0 || q.Battles != 4 || q.WinPct != 100 {
		t.Errorf("unrated row %+v", q)
	}
}

// TestSpeciesPageLifeStagesCard renders the species detail template directly
// (the same template main.go executes) to check that only a species with
// stage variation gets the "Life stages" card and the hull range, and that
// an unstaged species' page carries neither.
func TestSpeciesPageLifeStagesCard(t *testing.T) {
	staged := speciesView{Species: wildlife.Species{
		ID: "hoarfrost_leviathan", Name: "Hoarfrost Leviathan", Role: "predator", MaxHull: 700,
		Stages: []wildlife.StageStats{
			{Stage: "hatchling", Creatures: 4, Min: 243, Max: 324, Median: 290},
			{Stage: "juvenile", Creatures: 3, Min: 669, Max: 769, Median: 700},
		},
	}}
	staged.StageHistogram = stageHistogramSVG(staged.Species)

	unstaged := speciesView{Species: wildlife.Species{
		ID: "belt_grazer", Name: "Belt-Grazer", Role: "grazer", MaxHull: 60,
	}}

	tpl := htmltpl.Must(htmltpl.New("species").Funcs(templateFuncs()).Parse(speciesTemplate))
	render := func(v speciesView) string {
		var b strings.Builder
		if err := tpl.Execute(&b, map[string]any{"Header": htmltpl.HTML(""), "S": v, "StatsMonths": ""}); err != nil {
			t.Fatalf("render %s: %v", v.ID, err)
		}
		return b.String()
	}

	stagedHTML := render(staged)
	if !strings.Contains(stagedHTML, "Life stages") {
		t.Error("staged species page: missing \"Life stages\" card")
	}
	if !strings.Contains(stagedHTML, "243&ndash;769") {
		t.Errorf("staged species page: missing hull range 243–769")
	}
	if !strings.Contains(stagedHTML, "Hatchling") || !strings.Contains(stagedHTML, "Juvenile") {
		t.Error("staged species page: missing stage rows")
	}
	if !strings.Contains(stagedHTML, "stage-hist") {
		t.Error("staged species page: missing the stage histogram SVG")
	}

	unstagedHTML := render(unstaged)
	if strings.Contains(unstagedHTML, "Life stages") {
		t.Error("unstaged species page: unexpected \"Life stages\" card")
	}
	if strings.Contains(unstagedHTML, "varies by life stage") {
		t.Error("unstaged species page: unexpected hull-range note")
	}
	if !strings.Contains(unstagedHTML, "Hull (HP)</td><td>60</td>") {
		t.Errorf("unstaged species page: hull cell should render the plain value unchanged")
	}
}
