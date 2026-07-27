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
