package main

import (
	"slices"
	"testing"
)

func TestDecodeRoundTrip(t *testing.T) {
	msg := "ADDITIONAL UNITS DEPLOYED TO FACILITATE ENTRY"
	h := encode(msg)
	if h[:2] != "42" { // 'A' (0x41) + 1
		t.Errorf("first byte %s, want 42", h[:2])
	}
	got, ok := decode(h)
	if !ok || got != msg {
		t.Errorf("decode(encode) = %q, %v", got, ok)
	}
}

func TestDecodeRejectsNonText(t *testing.T) {
	for _, h := range []string{"zz", "", "00ff00ff"} {
		if _, ok := decode(h); ok {
			t.Errorf("decode(%q) accepted", h)
		}
	}
}

func TestFibWrapsAt256(t *testing.T) {
	f := fib(14)
	if !slices.Equal(f[:6], []int{1, 1, 2, 3, 5, 8}) || f[13] != 377%256 {
		t.Errorf("fib = %v", f)
	}
}

func TestAssembleJoinsOnLongOverlapOnly(t *testing.T) {
	got := assemble([]string{
		"THE GLASS IS COUNTED. THE GLASS IS NOT",
		"THE GLASS IS NOT YET ENOUGH.",
		"GLASS IS COUNTED", // contained in the first: dropped
		"ENOUGH. A WALL",   // shares one word only: not joined
	})
	want := []string{"THE GLASS IS COUNTED. THE GLASS IS NOT YET ENOUGH.", "ENOUGH. A WALL"}
	if !slices.Equal(got, want) {
		t.Errorf("assemble = %q, want %q", got, want)
	}
}

func TestPlace(t *testing.T) {
	passages := []string{"CARRIER OPEN. THIS IS THE SECOND", "WE ARE ONLY COUNTING.", "A STRAY PIECE"}
	sections := []Section{
		{Title: "End", Anchors: []string{"ONLY COUNTING", "NO SUCH TEXT"}},
		{Title: "Loop", Anchors: []string{"CARRIER OPEN", "THIS IS THE SECOND"}},
	}
	placed, unplaced, stale := place(passages, sections)
	if len(placed) != 2 || placed[0].Title != "End" || placed[1].Passages[0] != passages[0] || len(placed[1].Passages) != 1 {
		t.Errorf("placed = %+v", placed)
	}
	if !slices.Equal(unplaced, []string{"A STRAY PIECE"}) || !slices.Equal(stale, []string{"NO SUCH TEXT"}) {
		t.Errorf("unplaced = %q, stale = %q", unplaced, stale)
	}
}

func TestPickJoinPrefersBalancedJoin(t *testing.T) {
	j := pickJoin([]string{
		"X WHILE WE DECIDE. IF YOU HAVE",       // nearly all shared with the next
		"WHILE WE DECIDE. IF YOU HAVE ALREADY", //
		"THE GLASS IS COUNTED. THE GLASS IS NOT",
		"THE GLASS IS NOT YET ENOUGH. A DOOR",
	})
	if j == nil || j.Shared != "THE GLASS IS NOT" {
		t.Errorf("pickJoin = %+v, want the glass join", j)
	}
}
