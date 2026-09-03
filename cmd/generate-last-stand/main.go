// Command generate-last-stand builds the "swarm threshold matrix": for every
// hull in the catalog (a defender), the number of stock empire-starter
// hulls (an attacker swarm) needed to reach a >50% win rate against it,
// using pkg/combatsim's homogeneous-cohort swarm model. The result is
// written as JSON; -page is a wiring point for the HTML render (a later
// task).
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"io/fs"
	"math/rand/v2"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rsned/spacemolt-kb/pkg/combatsim"
)

// defaultMaxTicks mirrors combat-sim's --swarm mode: long grinds against
// capital defenders need more headroom than the calibration default.
const defaultMaxTicks = 4000

// Column is one attacker (empire-starter hull) column in the matrix.
type Column struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Empire     string `json:"empire"`
	Weapon     string `json:"weapon"`
	DamageType string `json:"damage_type"`
}

// starterColumnIDs returns the 5 empire-starter hull ids used as swarm
// attackers, in display order.
func starterColumnIDs() []string {
	return []string{"shard", "prospect", "cobble", "theoria", "threshold"}
}

// starterEmpire maps each empire-starter attacker id to its owning empire.
// The catalog snapshot's ShipDef does not expose faction under an exported
// field (pkg/combatsim.ShipDef deliberately keeps a minimal surface), so
// this small, stable table stands in for it — these 5 ids are fixed by the
// game's starter-ship design and do not change across catalog snapshots.
var starterEmpire = map[string]string{
	"shard":     "crimson",
	"prospect":  "nebula",
	"cobble":    "outerrim",
	"theoria":   "solarian",
	"threshold": "voidborn",
}

// starterColumns resolves the 5 empire starters against cat and returns
// their display metadata for the matrix JSON's columns array.
func starterColumns(cat *combatsim.Catalog) ([]Column, error) {
	ids := starterColumnIDs()
	cols := make([]Column, 0, len(ids))
	for _, id := range ids {
		_, c, err := resolveAttackerColumn(cat, id)
		if err != nil {
			return nil, fmt.Errorf("starter column %q: %w", id, err)
		}
		cols = append(cols, c)
	}
	return cols, nil
}

// resolveAttackerColumn resolves one attacker hull's stock fitting (used
// for battle math) and its display metadata (used for the matrix JSON's
// columns array) in a single resolve. Empire is left blank for a hull
// outside starterEmpire.
func resolveAttackerColumn(cat *combatsim.Catalog, id string) (*combatsim.StatBlock, Column, error) {
	hull, ok := cat.Ships[id]
	if !ok {
		return nil, Column{}, fmt.Errorf("unknown hull %q", id)
	}
	sb, err := combatsim.ResolveHull(id, cat, false)
	if err != nil {
		return nil, Column{}, err
	}
	col := Column{
		ID:         id,
		Name:       hull.Name,
		Empire:     starterEmpire[id],
		Weapon:     weaponSummary(cat, sb),
		DamageType: primaryDamageType(sb),
	}
	return sb, col, nil
}

// weaponSummary renders a stock fitting's weapon loadout as e.g.
// "2× Autocannon I", grouping identical weapons and preserving mount order.
func weaponSummary(cat *combatsim.Catalog, sb *combatsim.StatBlock) string {
	if len(sb.Weapons) == 0 {
		return "unarmed"
	}
	counts := make(map[string]int, len(sb.Weapons))
	var order []string
	for _, w := range sb.Weapons {
		if counts[w.Name] == 0 {
			order = append(order, w.Name)
		}
		counts[w.Name]++
	}
	parts := make([]string, 0, len(order))
	for _, id := range order {
		name := id
		if it, ok := cat.Items[id]; ok {
			name = it.Name
		}
		parts = append(parts, fmt.Sprintf("%d× %s", counts[id], name))
	}
	return strings.Join(parts, ", ")
}

// primaryDamageType returns the fitting's single damage type (resolved
// fits are always single-type; see combatsim.Resolve), or "" if unarmed.
func primaryDamageType(sb *combatsim.StatBlock) string {
	if len(sb.Weapons) == 0 {
		return ""
	}
	return sb.Weapons[0].Type
}

// CellResult is one defender×attacker crossover result in the matrix JSON.
// Curve carries every swarm size probed en route to N, so the KB page can
// render a crossover curve for the cell without re-running the simulation.
type CellResult struct {
	N           int                    `json:"n"`
	PWin        float64                `json:"p_win"`
	MedianKills int                    `json:"median_kills"`
	Curve       []combatsim.CrossPoint `json:"curve"`
}

// Row is one defender hull's crossover results against every resolved
// attacker column. A column id absent from Cells means that attacker
// failed to resolve (see Matrix.Notes) rather than an actual battle
// outcome; N==0 within a present cell means the swarm required to win was
// not found within nMax (∞).
type Row struct {
	ShipID string                 `json:"ship_id"`
	Name   string                 `json:"name"`
	Tier   int                    `json:"tier"`
	Class  string                 `json:"class"`
	Cells  map[string]*CellResult `json:"cells"`
}

// Matrix is the full swarm-threshold matrix: every defender hull's
// crossover swarm size against each attacker column.
type Matrix struct {
	GeneratedUTC string   `json:"generated_utc"`
	Assumptions  []string `json:"assumptions"`
	Columns      []Column `json:"columns"`
	Rows         []Row    `json:"rows"`
	Notes        []string `json:"notes,omitempty"`
}

// cell returns the crossover swarm size N for defender defID against
// attacker atkID, or 0 both for a measured ∞ (no crossing within nMax) and
// for a row/cell that isn't present at all.
func (m Matrix) cell(defID, atkID string) int {
	for _, r := range m.Rows {
		if r.ShipID != defID {
			continue
		}
		if c := r.Cells[atkID]; c != nil {
			return c.N
		}
		return 0
	}
	return 0
}

// cellSeed derives a deterministic per-cell RNG seed from the defender and
// attacker ids, so the matrix is reproducible regardless of worker
// scheduling order.
func cellSeed(defID, atkID string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(defID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(atkID))
	return h.Sum64()
}

// BuildMatrix computes, for every defender in defenderIDs, the swarm
// crossover N against every attacker in attackerIDs. Attackers resolve via
// combatsim.ResolveHull(id, cat, false) (non-capital only, matching the
// empire starters); defenders resolve via combatsim.ResolveHull(id, cat,
// true) (capitals allowed). A hull that fails to resolve is skipped — as
// a whole attacker column, or as a single defender row — with a note
// recorded on the returned Matrix rather than aborting the run. Defenders
// are processed by a worker pool bounded by GOMAXPROCS; every cell's RNG
// seed is derived solely from the (defender, attacker) id pair, so the
// result does not depend on scheduling.
func BuildMatrix(cat *combatsim.Catalog, cal *combatsim.Calibration, attackerIDs, defenderIDs []string, nMax, runs, maxTicks int) Matrix {
	m := Matrix{
		GeneratedUTC: time.Now().UTC().Format(time.RFC3339),
		Assumptions: []string{
			"attackers use the stock (default_modules) fitting for each empire-starter hull, no skills",
			"defenders use the stock (default_modules) fitting for each hull, no skills",
			"a swarm attacker is a homogeneous cohort of identical attacker hulls (combatsim.RunSwarm), not a mixed fleet",
			"crossover N is the smallest swarm size whose measured win rate exceeds 0.5, probed up to n-max",
		},
	}

	type attacker struct {
		id string
		sb *combatsim.StatBlock
	}
	attackers := make([]attacker, 0, len(attackerIDs))
	for _, id := range attackerIDs {
		sb, col, err := resolveAttackerColumn(cat, id)
		if err != nil {
			m.Notes = append(m.Notes, fmt.Sprintf("skipped attacker %q: %v", id, err))
			continue
		}
		attackers = append(attackers, attacker{id, sb})
		m.Columns = append(m.Columns, col)
	}

	type job struct {
		defID string
		def   *combatsim.StatBlock
		hull  *combatsim.ShipDef
	}
	jobs := make([]job, 0, len(defenderIDs))
	for _, defID := range defenderIDs {
		hull, ok := cat.Ships[defID]
		if !ok {
			m.Notes = append(m.Notes, fmt.Sprintf("skipped defender %q: unknown hull", defID))
			continue
		}
		def, err := combatsim.ResolveHull(defID, cat, true)
		if err != nil {
			m.Notes = append(m.Notes, fmt.Sprintf("skipped defender %q: %v", defID, err))
			continue
		}
		jobs = append(jobs, job{defID, def, hull})
	}

	rows := make([]Row, len(jobs))
	workers := runtime.GOMAXPROCS(0)
	if workers > len(jobs) {
		workers = len(jobs)
	}
	if workers < 1 {
		workers = 1
	}
	idxCh := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range idxCh {
				j := jobs[i]
				cells := make(map[string]*CellResult, len(attackers))
				for _, a := range attackers {
					seed := cellSeed(j.defID, a.id)
					cr := combatsim.Crossover(a.sb, j.def, cal, nMax, runs, maxTicks, seed)
					cells[a.id] = &CellResult{N: cr.N, PWin: cr.PWin, MedianKills: cr.MedianKills, Curve: cr.Curve}
				}
				rows[i] = Row{ShipID: j.defID, Name: j.hull.Name, Tier: j.hull.Tier, Class: j.hull.Class, Cells: cells}
			}
		}()
	}
	for i := range jobs {
		idxCh <- i
	}
	close(idxCh)
	wg.Wait()

	sort.Slice(rows, func(i, j int) bool { return rows[i].ShipID < rows[j].ShipID })
	m.Rows = rows
	return m
}

// highEndFitPath is the reconstructed fitting for the "High-End Setup"
// callout's Combat Drone gunline Opus Magna — extracted from played battle
// 4c1e005e81bb1969a06103be96afa65c via craftsman-boss + `combat-sim
// --extract-fits`. Its only combat-relevant difference from the stock
// default_modules Opus Magna is flat damage reduction (35% stock -> the 75%
// cap, from damage_control_system + quantum_shield_iv + 2x
// adaptive_shield_iii); shields/armor/recharge/8 guns are ~stock.
const highEndFitPath = "data/combat-sim/fits/high_end_opus_drone.json"

// HighEndRow is one empire starter's crossover N against the stock Opus
// Magna and against the high-end Combat Drone fit of the same hull, for the
// High-End Setup callout.
type HighEndRow struct {
	StarterID   string
	StarterName string
	Empire      string
	StockN      int // always finite (n>0) — see buildHighEndData
	HighEndN    int // 0 = ∞
}

// HighEndData is the High-End Setup callout's computed dataset: the same
// Opus Magna hull, stock vs. the reconstructed high-end fit at
// highEndFitPath, crossover'd against every empire starter.
type HighEndData struct {
	FitName string
	Rows    []HighEndRow
}

// buildHighEndData resolves the high-end fit at fitPath (highEndFitPath in
// production; a test passes its own relative path — see render_test.go)
// and computes its swarm crossover against every empire starter, pairing
// each with the stock Opus Magna's crossover already computed into m (see
// BuildMatrix) rather than recomputing it — the two must read the exact
// same number the main matrix table shows for the opus_magna row, which a
// second Crossover call with a different seed would not guarantee. Returns
// nil if the fit fails to resolve, or m has no opus_magna row to source the
// stock column from, so the page renders without this callout rather than
// aborting the whole run.
func buildHighEndData(cat *combatsim.Catalog, cal *combatsim.Calibration, m Matrix, fitPath string, runs, nMax, maxTicks int) *HighEndData {
	fit, err := combatsim.LoadFit(fitPath)
	if err != nil {
		return nil
	}
	droneStock, err := combatsim.ResolveFit(fit, cat, true)
	if err != nil {
		return nil
	}
	cols, err := starterColumns(cat)
	if err != nil {
		return nil
	}
	data := &HighEndData{FitName: fit.Name}
	for _, c := range cols {
		stockN := m.cell(opusMagnaID, c.ID)
		if stockN == 0 {
			continue // no opus_magna row/cell in m for this starter — see doc comment
		}
		starterSB, _, err := resolveAttackerColumn(cat, c.ID)
		if err != nil {
			continue
		}
		droneCross := combatsim.Crossover(starterSB, droneStock, cal, nMax, runs, maxTicks, cellSeed("high_end_opus_drone", c.ID))
		data.Rows = append(data.Rows, HighEndRow{
			StarterID: c.ID, StarterName: c.Name, Empire: c.Empire,
			StockN: stockN, HighEndN: droneCross.N,
		})
	}
	if len(data.Rows) == 0 {
		return nil
	}
	return data
}

// multiOpusDMax is the largest titan count D probed by the Multi-Opus
// Effect callout.
const multiOpusDMax = 4

// multiOpusNMax bounds the Prospect swarm size multiCrossover probes.
// Unlike combatsim.Crossover's homogeneous-cohort swarm model (O(1) per
// tick regardless of n), RunMultiShipModes simulates every ship
// individually, so the search must stay bounded well below Crossover's
// nMax — the expected crossing here is on the order of 100 ships.
const multiOpusNMax = 4000

// MultiOpusRow is one titan-count D's crossover N (Prospect swarm size)
// against D stock Opus Magnas, under both defending-team targeting modes,
// for the Multi-Opus Effect callout.
type MultiOpusRow struct {
	D        int
	DogpileN int // 0 = ∞
	SpreadN  int // 0 = ∞
}

// MultiOpusData is the Multi-Opus Effect callout's computed dataset: the
// Prospect-swarm crossover N against 1..multiOpusDMax stock Opus Magnas,
// split by whether the titans dogpile (concentrate fire on one attacker) or
// spread (fan out across distinct attackers) their targeting.
type MultiOpusData struct {
	N1   int            // crossover N against a single Opus Magna (dogpile == spread when D=1)
	Rows []MultiOpusRow // D=2..multiOpusDMax
}

// buildMultiOpusData resolves the stock Prospect and Opus Magna hulls, then
// computes the Prospect-swarm crossover N against D stock Opus Magnas for
// D=1..multiOpusDMax under both targeting modes (D=1 has only one entry,
// N1, since a lone titan's own targeting choice among attackers is moot).
// Returns nil if either hull fails to resolve, or if even the D=1 crossover
// never dominates within multiOpusNMax — the page renders without this
// callout rather than aborting the whole run.
func buildMultiOpusData(cat *combatsim.Catalog, cal *combatsim.Calibration, runs, maxTicks int) *MultiOpusData {
	prospect, err := combatsim.ResolveHull("prospect", cat, false)
	if err != nil {
		return nil
	}
	opus, err := combatsim.ResolveHull(opusMagnaID, cat, true)
	if err != nil {
		return nil
	}
	n1 := multiCrossover(prospect, opus, 1, combatsim.TargetConcentrate, cal, multiOpusNMax, runs, maxTicks,
		cellSeed("multiopus-d1", "prospect_vs_opus_magna"))
	if n1 == 0 {
		return nil
	}
	data := &MultiOpusData{N1: n1}
	for d := 2; d <= multiOpusDMax; d++ {
		dogpile := multiCrossover(prospect, opus, d, combatsim.TargetConcentrate, cal, multiOpusNMax, runs, maxTicks,
			cellSeed(fmt.Sprintf("multiopus-d%d-dogpile", d), "prospect_vs_opus_magna"))
		spread := multiCrossover(prospect, opus, d, combatsim.TargetSpread, cal, multiOpusNMax, runs, maxTicks,
			cellSeed(fmt.Sprintf("multiopus-d%d-spread", d), "prospect_vs_opus_magna"))
		data.Rows = append(data.Rows, MultiOpusRow{D: d, DogpileN: dogpile, SpreadN: spread})
	}
	return data
}

// multiCrossover finds the smallest Prospect swarm size N whose measured
// win rate against a fixed group of d Opus Magnas exceeds 0.5, via
// exponential doubling then bisection (mirrors combatsim.Crossover, but
// over a heterogeneous RunMultiShipModes battle rather than the homogeneous
// swarm model, since 2+ titans on the defending team must be simulated as
// individual ships — see multiOpusNMax). mode controls how the d titans
// (team 1) pick their own targets among the swarm each tick; the swarm
// (team 0) always dogpiles, matching the page's framing of the titans'
// targeting as the player choice under study.
func multiCrossover(prospect, opus *combatsim.StatBlock, d int, mode combatsim.TargetMode, cal *combatsim.Calibration, nMax, runs, maxTicks int, seed uint64) int {
	var teamMode map[int]combatsim.TargetMode
	if mode == combatsim.TargetSpread {
		teamMode = map[int]combatsim.TargetMode{1: combatsim.TargetSpread}
	}
	winRate := func(n int) float64 {
		wins := 0
		for s := range uint64(runs) {
			ships := make([]combatsim.Ship, 0, n+d)
			for range n {
				ships = append(ships, combatsim.Ship{Stats: prospect, Team: 0})
			}
			for range d {
				ships = append(ships, combatsim.Ship{Stats: opus, Team: 1})
			}
			rng := rand.New(rand.NewPCG(seed+s+1, uint64(n)*2654435761))
			r := combatsim.RunMultiShipModes(ships, teamMode, cal, maxTicks, rng)
			if r.WinningTeam == 0 {
				wins++
			}
		}
		return float64(wins) / float64(runs)
	}
	lo, hi, lastPow := 0, 0, 0
	for n := 1; ; n *= 2 {
		if n > nMax {
			if nMax >= 1 && winRate(nMax) > 0.5 {
				hi, lo = nMax, lastPow
			}
			break
		}
		if winRate(n) > 0.5 {
			hi, lo = n, lastPow
			break
		}
		lastPow = n
	}
	if hi == 0 {
		return 0 // ∞: never dominated within nMax
	}
	for lo+1 < hi {
		mid := lo + (hi-lo)/2
		if winRate(mid) > 0.5 {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi
}

// allHullIDs returns every ship id in cat, sorted for a deterministic
// default row order.
func allHullIDs(cat *combatsim.Catalog) []string {
	ids := make([]string, 0, len(cat.Ships))
	for id := range cat.Ships {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func run(catalogDir, calPath, out, page string, runs, nMax, limit int) error {
	cat, err := combatsim.LoadCatalog(catalogDir)
	if err != nil {
		return err
	}
	cal, err := combatsim.LoadCalibration(calPath)
	if errors.Is(err, fs.ErrNotExist) {
		cal, err = combatsim.DefaultCalibration(), nil
	}
	if err != nil {
		return err
	}

	defenderIDs := allHullIDs(cat)
	if limit > 0 && limit < len(defenderIDs) {
		defenderIDs = defenderIDs[:limit]
	}

	m := BuildMatrix(cat, cal, starterColumnIDs(), defenderIDs, nMax, runs, defaultMaxTicks)

	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %d rows x %d columns\n", out, len(m.Rows), len(m.Columns))

	if page != "" {
		highEnd := buildHighEndData(cat, cal, m, highEndFitPath, runs, nMax, defaultMaxTicks)
		multiOpus := buildMultiOpusData(cat, cal, runs, defaultMaxTicks)
		html, err := RenderPage(m, highEnd, multiOpus)
		if err != nil {
			return fmt.Errorf("render page: %w", err)
		}
		if err := os.WriteFile(page, []byte(html), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", page)
	}
	return nil
}

func main() {
	catalog := flag.String("catalog", "data/combat-sim/catalog", "catalog snapshot dir")
	calPath := flag.String("calibration", "data/combat-sim/calibration.json", "calibration file (missing = built-in defaults)")
	runs := flag.Int("runs", 300, "battles per probed swarm size")
	nMax := flag.Int("n-max", 25000, "largest swarm size probed before reporting ∞")
	out := flag.String("out", "data/combat-sim/last_stand_matrix.json", "matrix JSON output path")
	page := flag.String("page", "", "matrix HTML page output path (empty = skip; render not yet implemented)")
	limit := flag.Int("limit", 0, "limit to the first N defenders in catalog-id order (0 = all)")
	flag.Parse()

	if err := run(*catalog, *calPath, *out, *page, *runs, *nMax, *limit); err != nil {
		fmt.Fprintln(os.Stderr, "generate-last-stand:", err)
		os.Exit(1)
	}
}
