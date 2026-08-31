package main

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

var allStances = []Stance{StanceFire, StanceBrace, StanceEvade, StanceFlee}

type Cell struct {
	StanceA, StanceB Stance
	Counts           map[Outcome]int
	Runs             int
}

// RunTable runs `runs` battles per stance pair, deterministic under seed.
func RunTable(a, b *StatBlock, cal *Calibration, runs int, seed uint64) []Cell {
	cells := make([]Cell, 0, 16)
	for i, sa := range allStances {
		for j, sb := range allStances {
			rng := rand.New(rand.NewPCG(seed, uint64(i*len(allStances)+j)))
			c := Cell{StanceA: sa, StanceB: sb, Counts: map[Outcome]int{}, Runs: runs}
			for range runs {
				c.Counts[RunBattle(a, b, sa, sb, cal, rng)]++
			}
			cells = append(cells, c)
		}
	}
	return cells
}

func cellAssumed(c Cell, cal *Calibration) bool {
	assumed := func(name string) bool {
		for _, x := range cal.Assumed {
			if x == name {
				return true
			}
		}
		return false
	}
	if (c.StanceA == StanceEvade || c.StanceB == StanceEvade) && assumed("evade_in_mult") {
		return true
	}
	if (c.StanceA == StanceFlee || c.StanceB == StanceFlee) && assumed("flee_escape_per_tick") {
		return true
	}
	return false
}

func dominant(c Cell) (Outcome, float64) {
	var best Outcome
	bn := -1
	for _, o := range []Outcome{OutAKill, OutBKill, OutAFled, OutBFled, OutMutual, OutStalemate} {
		if c.Counts[o] > bn {
			best, bn = o, c.Counts[o]
		}
	}
	return best, 100 * float64(bn) / float64(c.Runs)
}

// FormatTable renders the 4×4 stance table (A rows × B cols).
func FormatTable(a, b *StatBlock, cells []Cell, cal *Calibration) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "A = %s   vs   B = %s   (%d runs/cell)\n\n", a.Name, b.Name, cells[0].Runs)
	fmt.Fprintf(&sb, "%-10s", "A \\ B")
	for _, s := range allStances {
		fmt.Fprintf(&sb, "%-18s", s)
	}
	sb.WriteString("\n")
	for i, sa := range allStances {
		fmt.Fprintf(&sb, "%-10s", sa)
		for j := range allStances {
			c := cells[i*len(allStances)+j]
			o, pct := dominant(c)
			mark := ""
			if cellAssumed(c, cal) {
				mark = "*"
			}
			fmt.Fprintf(&sb, "%-18s", fmt.Sprintf("%s %.0f%%%s", o, pct, mark))
		}
		sb.WriteString("\n")
	}
	if len(cal.Assumed) > 0 {
		fmt.Fprintf(&sb, "\n* cell depends on ASSUMED calibration: %s (see data/combat-sim/calibration.json)\n",
			strings.Join(cal.Assumed, ", "))
	}
	return sb.String()
}
