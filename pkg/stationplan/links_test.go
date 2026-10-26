package stationplan

import (
	"slices"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

func testLinker() *Linker {
	return NewLinker(
		[]catalog.Facility{{ID: "locker_bay", Name: "Locker Bay", Category: "service"}},
		[]catalog.Item{{ID: "flex_polymer", Name: "Flex Polymer", Category: "refined"}},
		"../",
	)
}

func TestHref_FacilityAndItem(t *testing.T) {
	l := testLinker()
	if h, ok := l.Href("locker_bay"); !ok || h != "../facilities/service/locker_bay.html" {
		t.Errorf("locker_bay → %q %v", h, ok)
	}
	if h, ok := l.Href("flex_polymer"); !ok || h != "../items/refined/flex_polymer.html" {
		t.Errorf("flex_polymer → %q %v", h, ok)
	}
	if _, ok := l.Href("nope"); ok {
		t.Error("unknown id resolved")
	}
	if l.Name("flex_polymer") != "Flex Polymer" || l.Name("nope") != "nope" {
		t.Errorf("Name: %q %q", l.Name("flex_polymer"), l.Name("nope"))
	}
}

func TestExpandRefs(t *testing.T) {
	got, unresolved := testLinker().ExpandRefs("Stock [[flex_polymer]] first")
	if got != "Stock [Flex Polymer](../items/refined/flex_polymer.html) first" || len(unresolved) != 0 {
		t.Errorf("got %q, unresolved %v", got, unresolved)
	}
}

func TestExpandRefs_Unknown(t *testing.T) {
	got, unresolved := testLinker().ExpandRefs("[[nope]] and [[nope]]")
	if got != "nope and nope" {
		t.Errorf("got %q", got)
	}
	if !slices.Equal(unresolved, []string{"nope"}) {
		t.Errorf("unresolved = %v", unresolved)
	}
	if strings.Contains(got, "](") {
		t.Error("unknown ref produced a link")
	}
}
