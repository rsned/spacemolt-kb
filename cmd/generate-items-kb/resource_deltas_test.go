package main

import (
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/resourcediff"
)

func testDeltas() *resourceDeltas {
	return &resourceDeltas{
		PrevDate: "2026-08-29", BaseDate: "2026-08-27", BaseVersion: "0.566.0",
		Prev: map[string]resourcediff.ResourceDelta{
			"chlorine_gas": {Added: 5, Removed: 1},
			"zinc_ore":     {Added: 1, Discovered: true},
			"legacy_ore":   {Removed: 2},
		},
		Base: map[string]resourcediff.ResourceDelta{
			"chlorine_gas": {Added: 12},
			"zinc_ore":     {Added: 3, Discovered: true, NewType: true},
			"nebulium":     {NewType: true},
			"legacy_ore":   {Removed: 2},
		},
		PrevSummary: resourcediff.Summary{Types: 65, Deposits: 2138, Explored: 505},
		BaseSummary: resourcediff.Summary{Types: 62, Deposits: 2084, Explored: 500},
		Cur:         resourcediff.Summary{Types: 65, Deposits: 2150, Explored: 505},
	}
}

func TestDeltaHTML(t *testing.T) {
	if got := deltaHTML(4, ""); got != `<span class="delta up">+4</span>` {
		t.Errorf("up = %q", got)
	}
	if got := deltaHTML(-2, " since x"); got != `<span class="delta down">&minus;2 since x</span>` {
		t.Errorf("down = %q", got)
	}
	if got := deltaHTML(0, ""); got != "" {
		t.Errorf("zero = %q", got)
	}
}

func TestResourceDeltasTOC(t *testing.T) {
	d := testDeltas()
	got := d.tocHTML("chlorine_gas")
	if !strings.Contains(got, `class="delta up">+4</span>`) || !strings.Contains(got, `+4 since last regen (2026-08-29); +12 since patch v0.566.0`) {
		t.Errorf("chlorine = %q", got)
	}
	if got := d.tocHTML("legacy_ore"); !strings.Contains(got, `delta down">&minus;2`) {
		t.Errorf("legacy = %q", got)
	}
	if got := d.tocHTML("zinc_ore"); !strings.Contains(got, "+1") || !strings.Contains(got, `delta new">new`) {
		t.Errorf("zinc = %q", got)
	}
	// Catalog addition that is still undiscovered: just the tag.
	if got := d.tocHTML("nebulium"); got != ` <span class="delta new">new</span>` {
		t.Errorf("nebulium = %q", got)
	}
	if got := d.tocHTML("iron_ore"); got != "" {
		t.Errorf("unchanged = %q", got)
	}
	if got := d.tocText("chlorine_gas"); got != " +4" {
		t.Errorf("tocText chlorine = %q", got)
	}
	if got := d.tocText("zinc_ore"); got != " +1 new" {
		t.Errorf("tocText zinc = %q", got)
	}
	if got := d.tocText("legacy_ore"); got != " -2" {
		t.Errorf("tocText legacy = %q", got)
	}
	var none *resourceDeltas
	if got := none.tocText("chlorine_gas"); got != "" {
		t.Errorf("nil tocText = %q", got)
	}
	if got := none.tocHTML("chlorine_gas"); got != "" {
		t.Errorf("nil deltas = %q", got)
	}
}

func TestResourceDeltasBadgeAndSummary(t *testing.T) {
	d := testDeltas()
	got := d.badgeHTML("chlorine_gas")
	if !strings.Contains(got, "+4 since 2026-08-29") || !strings.Contains(got, "+12 since v0.566.0") {
		t.Errorf("badge = %q", got)
	}
	if got := d.badgeHTML("zinc_ore"); !strings.Contains(got, "newly discovered") {
		t.Errorf("zinc badge = %q", got)
	}
	if got := d.badgeHTML("iron_ore"); got != "" {
		t.Errorf("unchanged badge = %q", got)
	}
	got = d.summaryHTML(d.PrevSummary.Deposits, d.BaseSummary.Deposits, d.Cur.Deposits)
	if !strings.Contains(got, "+12") || !strings.Contains(got, "+66") {
		t.Errorf("deposits summary = %q", got)
	}
	// Types unchanged since the previous regen, +3 since the baseline.
	got = d.summaryHTML(d.PrevSummary.Types, d.BaseSummary.Types, d.Cur.Types)
	if strings.Contains(got, "+0") || !strings.Contains(got, "+3") {
		t.Errorf("types summary = %q", got)
	}

	// When the baseline IS the previous regen, only one delta is shown.
	d.BaseDate = d.PrevDate
	if got := d.badgeHTML("chlorine_gas"); strings.Contains(got, "v0.566.0") {
		t.Errorf("same-point badge = %q", got)
	}
}

func TestResolveCapacity(t *testing.T) {
	estimates := map[string]float64{"hydrogen_gas": 3211}
	known := ResourceEntry{ResourceID: "hydrogen_gas", Remaining: 1109}
	resolveCapacity(&known, 50000, estimates)
	if !known.MaxKnown || known.MaxAmount != 50000 || known.SupportedPower != 55 || known.MaxPower != 2500 || known.DepletionPct != 98 {
		t.Errorf("known capacity = %+v", known)
	}
	est := ResourceEntry{ResourceID: "hydrogen_gas", Remaining: 1650}
	resolveCapacity(&est, 0, estimates)
	if est.MaxKnown || est.MaxAmount != 3211 || est.SupportedPower != 82 || est.MaxPower != 0 || est.DepletionPct != 49 {
		t.Errorf("estimated capacity = %+v", est)
	}
}
