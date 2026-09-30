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
