package main

import (
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
