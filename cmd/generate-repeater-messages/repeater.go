package main

import (
	"cmp"
	"encoding/hex"
	"slices"
	"strings"
)

// minOverlap is the shortest shared text that joins two pieces. A repeater
// rotates its window through the message, so neighbouring windows share a
// long run of text; one shared word is too easily a coincidence.
const minOverlap = 12

// decode reverses the repeater cipher: each byte had the next Fibonacci
// number (1, 1, 2, 3, 5, ... mod 256, restarting with every message) added
// to it. It reports false for anything that is not hex or does not decode to
// printable ASCII.
func decode(h string) (string, bool) {
	b, err := hex.DecodeString(strings.TrimSpace(h))
	if err != nil || len(b) == 0 {
		return "", false
	}
	out := make([]byte, len(b))
	x, y := 1, 1
	for i, v := range b {
		c := byte((int(v) - x + 256) % 256)
		if c < 0x20 || c > 0x7e {
			return "", false
		}
		out[i] = c
		x, y = y, (x+y)%256
	}
	return string(out), true
}

// encode is decode's inverse, used for the worked example and tests.
func encode(s string) string {
	out := make([]byte, len(s))
	x, y := 1, 1
	for i := range len(s) {
		out[i] = byte((int(s[i]) + x) % 256)
		x, y = y, (x+y)%256
	}
	return hex.EncodeToString(out)
}

// fib returns the first n keystream values (Fibonacci mod 256).
func fib(n int) []int {
	out := make([]int, n)
	x, y := 1, 1
	for i := range n {
		out[i] = x
		x, y = y, (x+y)%256
	}
	return out
}

// overlap is the length of the longest suffix of a that is a prefix of b,
// if it is at least minOverlap; otherwise 0.
func overlap(a, b string) int {
	for k := min(len(a), len(b)) - 1; k >= minOverlap; k-- {
		if strings.HasSuffix(a, b[:k]) {
			return k
		}
	}
	return 0
}

// assemble joins decoded pieces into passages: it drops pieces contained in
// another, then repeatedly joins the pair with the longest overlap. Ties go to
// the alphabetically first pair, so the result is deterministic. Passages
// come back longest first.
func assemble(pieces []string) []string {
	set := map[string]bool{}
	for _, p := range pieces {
		if p = strings.TrimSpace(p); p != "" {
			set[p] = true
		}
	}
	var ps []string
	for p := range set {
		contained := false
		for q := range set {
			if q != p && strings.Contains(q, p) {
				contained = true
				break
			}
		}
		if !contained {
			ps = append(ps, p)
		}
	}
	slices.Sort(ps)
	for {
		bk, bi, bj := 0, -1, -1
		for i, a := range ps {
			for j, b := range ps {
				if i != j {
					if k := overlap(a, b); k > bk {
						bk, bi, bj = k, i, j
					}
				}
			}
		}
		if bk == 0 {
			break
		}
		joined := ps[bi] + ps[bj][bk:]
		var next []string
		for n, p := range ps {
			if n != bi && n != bj {
				next = append(next, p)
			}
		}
		ps = append(next, joined)
		slices.Sort(ps)
	}
	slices.SortStableFunc(ps, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	return ps
}

// Section is one stretch of the message in our reading order; Anchors are
// snippets that pick out its passages, in order.
type Section struct {
	Title   string   `json:"title"`
	Note    string   `json:"note,omitempty"`
	Anchors []string `json:"anchors"`
}

// PlacedSection is a section with the passages its anchors found.
type PlacedSection struct {
	Section
	Passages []string
}

// place orders passages by the sections' anchors. Each passage goes to the
// first anchor it contains (so a passage that has grown to cover two anchors
// is shown once); passages no anchor finds are returned as unplaced, longest
// first. Anchors that match nothing are returned too, so the order file can
// be kept tidy.
func place(passages []string, sections []Section) (placed []PlacedSection, unplaced, stale []string) {
	used := map[int]bool{}
	for _, s := range sections {
		ps := PlacedSection{Section: s}
		for _, a := range s.Anchors {
			found := false
			for i, p := range passages {
				if strings.Contains(p, a) {
					found = true
					if !used[i] {
						used[i] = true
						ps.Passages = append(ps.Passages, p)
					}
					break
				}
			}
			if !found {
				stale = append(stale, a)
			}
		}
		if len(ps.Passages) > 0 {
			placed = append(placed, ps)
		}
	}
	for i, p := range passages {
		if !used[i] {
			unplaced = append(unplaced, p)
		}
	}
	return placed, unplaced, stale
}
