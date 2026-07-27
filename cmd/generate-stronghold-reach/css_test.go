package main

import (
	"strings"
	"testing"
)

func TestReachCSSHidesEveryFrameByDefault(t *testing.T) {
	css := ReachCSS(3)

	for _, want := range []string{"#reach-map .rb-1", "#reach-map .rb-2", "#reach-map .rb-3"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing base hide selector %q", want)
		}
	}
	if !strings.Contains(css, "display:none") {
		t.Errorf("base rule should hide frames")
	}
}

func TestReachCSSRevealIsCumulative(t *testing.T) {
	css := ReachCSS(4)

	// The data-r="3" block must list rb-1 through rb-3 and stop there.
	for _, want := range []string{
		`#reach-map[data-r="3"] .rb-1`,
		`#reach-map[data-r="3"] .rb-2`,
		`#reach-map[data-r="3"] .rb-3`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("missing cumulative selector %q", want)
		}
	}
	if strings.Contains(css, `#reach-map[data-r="3"] .rb-4`) {
		t.Errorf("frame 3 must not reveal frame 4")
	}
}

func TestReachCSSOmitsRadiusZeroFrameRules(t *testing.T) {
	css := ReachCSS(3)

	if strings.Contains(css, ".rb-0") {
		t.Errorf("radius 0 blob geometry is always visible and needs no rule")
	}
	if strings.Contains(css, `[data-r="0"]`) {
		t.Errorf("there is no radius 0 frame")
	}
}

func TestReachCSSHasStaticStrongholdDotRule(t *testing.T) {
	css := ReachCSS(3)

	if !strings.Contains(css, "#reach-map .sr-0") {
		t.Errorf("strongholds need an always-on dot rule")
	}
	if strings.Contains(css, `[data-r="2"] .sr-0`) {
		t.Errorf("sr-0 must not appear in per-frame rules")
	}
}

func TestReachCSSBrightensInReachDots(t *testing.T) {
	css := ReachCSS(2)

	if !strings.Contains(css, `#reach-map[data-r="2"] .sr-1`) {
		t.Errorf("in-reach dots should be brightened per frame")
	}
}

func TestReachCSSZeroMaxProducesNoFrameRules(t *testing.T) {
	css := ReachCSS(0)

	if strings.Contains(css, "[data-r=") {
		t.Errorf("no frames means no frame rules, got:\n%s", css)
	}
}

func TestReachCSSEmitsEmpireDotColors(t *testing.T) {
	css := ReachCSS(3)

	for _, want := range []string{
		"#reach-map .emp-crimson{fill:#DC143C;}",
		"#reach-map .emp-nebula{fill:#00CED1;}",
		"#reach-map .emp-outerrim{fill:#2E8B57;}",
		"#reach-map .emp-solarian{fill:#FFD700;}",
		"#reach-map .emp-voidborn{fill:#9932CC;}",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("missing empire dot rule %q", want)
		}
	}
}

func TestReachCSSEmpireRulesPrecedeStrongholdRule(t *testing.T) {
	css := ReachCSS(3)

	// Empire rules and the stronghold rule have equal specificity
	// (id + class), so source order decides. Strongholds are neutral and
	// never carry an emp- class today, but if that ever changes the
	// stronghold color must win.
	emp := strings.Index(css, "#reach-map .emp-crimson")
	sr0 := strings.Index(css, "#reach-map .sr-0")
	if emp < 0 || sr0 < 0 {
		t.Fatalf("expected both rules present; emp=%d sr0=%d", emp, sr0)
	}
	if emp > sr0 {
		t.Errorf("empire rules must come before the stronghold rule (emp=%d, sr-0=%d)", emp, sr0)
	}
}

func TestReachCSSFrameRulesOutrankEmpireColors(t *testing.T) {
	css := ReachCSS(3)

	// Covering a system must repaint it. Frame rules add an attribute
	// selector on top of id+class, so they outrank the empire rules by
	// specificity regardless of source order.
	if !strings.Contains(css, `#reach-map[data-r="2"] .sr-1`) {
		t.Fatalf("frame rule missing")
	}
	if strings.Contains(css, `#reach-map .sr-1{`) {
		t.Errorf("frame reveal must be attribute-qualified, not a bare id+class rule")
	}
}
