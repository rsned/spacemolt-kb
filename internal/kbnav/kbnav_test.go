package kbnav

import (
	"strings"
	"testing"
)

func TestHeaderDepthPrefix(t *testing.T) {
	h1 := Header("../")
	h2 := Header("../../")

	// Home link points at the kb root for each depth.
	if !strings.Contains(h1, `<a href="../">Home</a>`) {
		t.Errorf("depth-1 header missing Home link:\n%s", h1)
	}
	if !strings.Contains(h2, `<a href="../../">Home</a>`) {
		t.Errorf("depth-2 header missing Home link:\n%s", h2)
	}
	// Section links carry the prefix + /index.html.
	if !strings.Contains(h1, `<a href="../passengers/index.html">Passengers</a>`) {
		t.Errorf("depth-1 header missing Passengers link:\n%s", h1)
	}
	if !strings.Contains(h2, `<a href="../../passengers/index.html">Passengers</a>`) {
		t.Errorf("depth-2 header missing Passengers link:\n%s", h2)
	}
}

func TestHeaderRendersEveryNavItem(t *testing.T) {
	h := Header("../")
	for _, it := range Items {
		if !strings.Contains(h, ">"+it.Label+"</a>") {
			t.Errorf("header missing nav item %q", it.Label)
		}
	}
	// The theme toggle must be present and inside the nav.
	if !strings.Contains(h, `id="theme-toggle"`) {
		t.Error("header missing theme toggle")
	}
	if strings.Count(h, "<header") != 1 || strings.Count(h, "</header>") != 1 {
		t.Errorf("header should be a single well-formed element:\n%s", h)
	}
}
