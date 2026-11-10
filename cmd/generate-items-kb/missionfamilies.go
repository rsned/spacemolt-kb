// Command generate-items-kb mission template families.
//
// Most missions are generated from a handful of server formulas ("Trade Run:
// X to Y" alone has ~19,000 variants). Category pages render each formula as
// one family card instead of listing every permutation; hand-written missions
// that belong to no chain stay as one-offs.
package main

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	humanize "github.com/dustin/go-humanize"
)

// familyMinVariants is the smallest group that renders as a family; smaller
// groups are listed as one-offs whatever their procedural flag says.
const familyMinVariants = 3

// oddJobsFamilyName names the family of procedural missions that carry
// whimsical, prefix-less titles ("Please hold", "The chair inspection").
const oddJobsFamilyName = "Odd jobs"

// MissionFamily is one server formula and every mission generated from it.
type MissionFamily struct {
	Key      string // type + "|" + Name
	Name     string // title prefix, e.g. "Contraband Run"
	Type     string
	Variants []*Mission

	// Filled by summarize.
	Rewards       RewardSummary
	Difficulty    IntRange
	Stations      int    // distinct offering stations across all variants
	OnlyAt        string // station name when every variant is offered at the same one
	Goods         []string
	GoodsMore     int
	Pattern       string // e.g. "Deliver <N> <good> to <station>"
	BreakdownBy   string // "good", "title", "destination" or "station"
	Breakdown     []FamilyBreakdownRow
	BreakdownMore int
}

// familyKey groups a mission by the title text before its first colon. A
// prefix-less procedural mission takes oddJobsName; a prefix-less hand-written
// one keys on its exact title.
func familyKey(m *Mission, oddJobsName string) (key, name string) {
	if before, _, ok := strings.Cut(m.Title, ":"); ok && strings.TrimSpace(before) != "" {
		name = strings.TrimSpace(before)
	} else if m.Procedural {
		name = oddJobsName
	} else {
		name = m.Title
	}
	return m.Type + "|" + name, name
}

// hasTitlePrefix reports whether a title carries a "Prefix: ..." formula name.
func hasTitlePrefix(title string) bool {
	before, _, ok := strings.Cut(title, ":")
	return ok && strings.TrimSpace(before) != ""
}

// partitionFamilies drops chain members, groups the rest into families of at
// least familyMinVariants, and returns everything left over as one-offs —
// player-faction posts (ids starting "faction_") separately from the server's.
func partitionFamilies(missions []*Mission, chained map[string]bool) (families []*MissionFamily, oneOffs, factionOneOffs []*Mission) {
	var pool []*Mission
	for _, m := range missions {
		if !chained[m.ID] {
			pool = append(pool, m)
		}
	}

	// Prefix-less procedural missions of a type form one family: named after
	// their title when they all share it, otherwise "Odd jobs".
	oddTitles := map[string]map[string]bool{}
	for _, m := range pool {
		if m.Procedural && !hasTitlePrefix(m.Title) {
			if oddTitles[m.Type] == nil {
				oddTitles[m.Type] = map[string]bool{}
			}
			oddTitles[m.Type][m.Title] = true
		}
	}
	oddName := func(typ string) string {
		if t := oddTitles[typ]; len(t) == 1 {
			for title := range t {
				return title
			}
		}
		return oddJobsFamilyName
	}

	groups := map[string]*MissionFamily{}
	var order []string
	for _, m := range pool {
		key, name := familyKey(m, oddName(m.Type))
		g, ok := groups[key]
		if !ok {
			g = &MissionFamily{Key: key, Name: name, Type: m.Type}
			groups[key] = g
			order = append(order, key)
		}
		g.Variants = append(g.Variants, m)
	}

	for _, key := range order {
		g := groups[key]
		if len(g.Variants) >= familyMinVariants {
			families = append(families, g)
			continue
		}
		for _, m := range g.Variants {
			if strings.HasPrefix(m.ID, "faction_") {
				factionOneOffs = append(factionOneOffs, m)
			} else {
				oneOffs = append(oneOffs, m)
			}
		}
	}

	for _, f := range families {
		f.summarize()
	}
	slices.SortFunc(families, func(a, b *MissionFamily) int {
		if c := cmp.Compare(len(b.Variants), len(a.Variants)); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	slices.SortFunc(oneOffs, compareMissions)
	slices.SortFunc(factionOneOffs, compareMissions)
	return families, oneOffs, factionOneOffs
}

// familyBreakdownRows caps a family's breakdown table.
const familyBreakdownRows = 25

// familyGoodsChips is how many goods a family card names before "+N".
const familyGoodsChips = 4

// IntRange is a min–max span over a family's variants.
type IntRange struct{ Min, Max int }

// String renders "480–1,750", or a single number when the span is flat.
func (r IntRange) String() string {
	if r.Min == r.Max {
		return humanize.Comma(int64(r.Min))
	}
	return humanize.Comma(int64(r.Min)) + "–" + humanize.Comma(int64(r.Max))
}

// widen grows r to include v; ok reports whether r already held a value.
func (r *IntRange) widen(v int, ok bool) {
	if !ok {
		r.Min, r.Max = v, v
		return
	}
	r.Min, r.Max = min(r.Min, v), max(r.Max, v)
}

// SkillXPRange is the XP span one skill pays across a family.
type SkillXPRange struct {
	Skill string
	IntRange
}

// RewardSummary lists only the reward types a group of missions actually
// pays. A Reputation field belongs here once the scraper captures it.
type RewardSummary struct {
	Credits *IntRange // nil when no mission pays credits
	XP      []SkillXPRange
	Items   []string
}

// Parts renders each reward type as one phrase, e.g. "480–1,750 cr".
func (s RewardSummary) Parts() []string {
	var out []string
	if s.Credits != nil {
		out = append(out, s.Credits.String()+" cr")
	}
	for _, x := range s.XP {
		out = append(out, x.String()+" "+x.Skill+" XP")
	}
	if len(s.Items) > 0 {
		out = append(out, "items: "+strings.Join(s.Items, ", "))
	}
	return out
}

// summarizeRewards spans credits, XP per skill and item rewards over ms.
func summarizeRewards(ms []*Mission) RewardSummary {
	var s RewardSummary
	var credits IntRange
	haveCredits := false
	xp := map[string]*IntRange{}
	items := map[string]bool{}
	for _, m := range ms {
		if m.RewardsCredits > 0 {
			credits.widen(m.RewardsCredits, haveCredits)
			haveCredits = true
		}
		for _, r := range m.RewardsSkillXP {
			cur, ok := xp[r.Name]
			if !ok {
				cur = &IntRange{}
				xp[r.Name] = cur
			}
			cur.widen(r.Value, ok)
		}
		for _, it := range m.RewardsItems {
			name := it.ItemName
			if name == "" {
				name = titleCase(it.ItemID)
			}
			items[name] = true
		}
	}
	if haveCredits {
		s.Credits = &credits
	}
	for _, skill := range slices.Sorted(maps.Keys(xp)) {
		s.XP = append(s.XP, SkillXPRange{Skill: skill, IntRange: *xp[skill]})
	}
	s.Items = slices.Sorted(maps.Keys(items))
	return s
}

// FamilyBreakdownRow is one line of a family's breakdown table.
type FamilyBreakdownRow struct {
	Label    string
	Variants int
	Stations int
	Rewards  RewardSummary
}

// objectiveVerbs phrases an objective type at the start of a family formula.
var objectiveVerbs = map[string]string{
	"deliver_item":      "Deliver",
	"sell_item":         "Sell",
	"buy_item":          "Buy",
	"mine_resource":     "Mine",
	"kill_creature":     "Hunt",
	"kill_pirate":       "Defeat",
	"visit_system":      "Visit",
	"dock_at_base":      "Dock at",
	"traverse_wormhole": "Traverse",
	"survey_system":     "Survey",
}

// primaryObjective is the objective that names a mission's goods: the first
// one carrying an item, else the first one; nil when there are none.
func primaryObjective(m *Mission) *MissionObjective {
	for i := range m.Objectives {
		if m.Objectives[i].ItemID != "" {
			return &m.Objectives[i]
		}
	}
	if len(m.Objectives) > 0 {
		return &m.Objectives[0]
	}
	return nil
}

func locationName(l MissionLocation) string {
	if l.BaseName != "" {
		return l.BaseName
	}
	return titleCase(l.BaseID)
}

func objItemName(o *MissionObjective) string {
	if o.ItemName != "" {
		return o.ItemName
	}
	return titleCase(o.ItemID)
}

func objTargetName(o *MissionObjective) string {
	if o.TargetBaseName != "" {
		return o.TargetBaseName
	}
	return titleCase(o.TargetBaseID)
}

// distinctStations counts the offering stations across ms and returns the
// name of the only one when there is exactly one.
func distinctStations(ms []*Mission) (n int, only string) {
	seen := map[string]string{}
	for _, m := range ms {
		for _, l := range m.Locations {
			seen[l.BaseID] = locationName(l)
		}
	}
	if len(seen) == 1 {
		for _, name := range seen {
			only = name
		}
	}
	return len(seen), only
}

// summarize fills a family's card fields from its variants.
func (f *MissionFamily) summarize() {
	f.Rewards = summarizeRewards(f.Variants)
	for i, m := range f.Variants {
		f.Difficulty.widen(m.Difficulty, i > 0)
	}
	f.Stations, f.OnlyAt = distinctStations(f.Variants)

	// Goods, by how many variants name them.
	goodCount := map[string]int{}
	for _, m := range f.Variants {
		if o := primaryObjective(m); o != nil && o.ItemID != "" {
			goodCount[objItemName(o)]++
		}
	}
	goods := slices.SortedFunc(maps.Keys(goodCount), func(a, b string) int {
		if c := cmp.Compare(goodCount[b], goodCount[a]); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	if len(goods) > familyGoodsChips {
		f.Goods, f.GoodsMore = goods[:familyGoodsChips], len(goods)-familyGoodsChips
	} else {
		f.Goods = goods
	}

	f.Pattern = familyPattern(f.Variants)
	f.breakdown()
}

// familyPattern writes the formula a family's variants share, with <N>,
// <good> and <station> standing in for whatever varies between them.
func familyPattern(ms []*Mission) string {
	var objs []*MissionObjective
	for _, m := range ms {
		if o := primaryObjective(m); o != nil {
			objs = append(objs, o)
		}
	}
	if len(objs) == 0 {
		return ""
	}
	varies := func(get func(*MissionObjective) string) (val string, present, vary bool) {
		for _, o := range objs {
			v := get(o)
			if v == "" {
				continue
			}
			if present && v != val {
				vary = true
			}
			if !present {
				val, present = v, true
			}
		}
		return val, present, vary
	}

	verb, ok := objectiveVerbs[objs[0].Type]
	if !ok {
		verb = titleCase(objs[0].Type)
	}
	parts := []string{verb}
	item, hasItem, itemVaries := varies(func(o *MissionObjective) string {
		if o.ItemID == "" {
			return ""
		}
		return objItemName(o)
	})
	if q, present, vary := varies(func(o *MissionObjective) string {
		if o.Quantity <= 0 {
			return ""
		}
		return humanize.Comma(int64(o.Quantity))
	}); present && hasItem {
		if vary {
			q = "<N>"
		}
		parts = append(parts, q)
	}
	if hasItem {
		if itemVaries {
			item = "<good>"
		}
		parts = append(parts, item)
	}
	if tgt, present, vary := varies(func(o *MissionObjective) string {
		if o.TargetBaseID == "" {
			return ""
		}
		return objTargetName(o)
	}); present {
		if vary {
			tgt = "<station>"
		}
		if hasItem {
			parts = append(parts, "to")
		}
		parts = append(parts, tgt)
	} else if sys, present, vary := varies(func(o *MissionObjective) string { return o.SystemName }); present && !hasItem {
		if vary {
			sys = "<system>"
		}
		parts = append(parts, sys)
	}
	return strings.Join(parts, " ")
}

// breakdown groups the variants by title for Odd jobs, otherwise by the first
// dimension that varies — good, destination, offering station — into at most
// familyBreakdownRows rows.
func (f *MissionFamily) breakdown() {
	type dim struct {
		name  string
		label func(*Mission) string
	}
	dims := []dim{
		{"good", func(m *Mission) string {
			if o := primaryObjective(m); o != nil && o.ItemID != "" {
				return objItemName(o)
			}
			return ""
		}},
		{"destination", func(m *Mission) string {
			if o := primaryObjective(m); o != nil && o.TargetBaseID != "" {
				return objTargetName(o)
			}
			return ""
		}},
		{"station", func(m *Mission) string {
			if len(m.Locations) == 0 {
				return "Station not recorded"
			}
			return locationName(m.Locations[0])
		}},
	}
	chosen := dims[len(dims)-1]
	if f.Name == oddJobsFamilyName {
		// Odd jobs is the one family whose titles are the point.
		chosen = dim{"title", func(m *Mission) string { return m.Title }}
	}
	for _, d := range dims[:len(dims)-1] {
		if chosen.name == "title" {
			break
		}
		labels := map[string]bool{}
		for _, m := range f.Variants {
			labels[d.label(m)] = true
		}
		if len(labels) > 1 {
			chosen = d
			break
		}
	}

	groups := map[string][]*Mission{}
	for _, m := range f.Variants {
		l := chosen.label(m)
		if l == "" {
			l = "—"
		}
		groups[l] = append(groups[l], m)
	}
	rows := make([]FamilyBreakdownRow, 0, len(groups))
	for label, ms := range groups {
		n, _ := distinctStations(ms)
		rows = append(rows, FamilyBreakdownRow{Label: label, Variants: len(ms), Stations: n, Rewards: summarizeRewards(ms)})
	}
	slices.SortFunc(rows, func(a, b FamilyBreakdownRow) int {
		if c := cmp.Compare(b.Variants, a.Variants); c != 0 {
			return c
		}
		return cmp.Compare(a.Label, b.Label)
	})
	f.BreakdownBy = chosen.name
	if len(rows) > familyBreakdownRows {
		f.Breakdown, f.BreakdownMore = rows[:familyBreakdownRows], len(rows)-familyBreakdownRows
	} else {
		f.Breakdown = rows
	}
}

// Station-group headings that are not station names.
const (
	manyStationsHeading = "Offered at many stations"
	noStationHeading    = "Station not recorded"
)

// StationGroup is one heading of one-off missions on a category page.
type StationGroup struct {
	Heading  string
	Many     bool // the "Offered at many stations" group, which shows counts
	Missions []*Mission
}

// StationCount is how many distinct stations offer m.
func (m *Mission) StationCount() int {
	n, _ := distinctStations([]*Mission{m})
	return n
}

// OnlyAt names the station the server marks m exclusive to, or "".
func (m *Mission) OnlyAt() string {
	if m.ExclusiveBaseID == "" {
		return ""
	}
	return m.ExclusiveBaseName
}

// groupOneOffsByStation files one-offs under the station that offers them:
// missions offered at several stations first, then one group per station by
// name, then missions whose offering station was never recorded.
func groupOneOffsByStation(oneOffs []*Mission) []StationGroup {
	var many, none []*Mission
	byStation := map[string][]*Mission{}
	for _, m := range oneOffs {
		switch n, only := distinctStations([]*Mission{m}); {
		case n == 0:
			none = append(none, m)
		case n > 1:
			many = append(many, m)
		default:
			byStation[only] = append(byStation[only], m)
		}
	}
	var out []StationGroup
	if len(many) > 0 {
		out = append(out, StationGroup{Heading: manyStationsHeading, Many: true, Missions: many})
	}
	for _, name := range slices.Sorted(maps.Keys(byStation)) {
		out = append(out, StationGroup{Heading: name, Missions: byStation[name]})
	}
	if len(none) > 0 {
		out = append(out, StationGroup{Heading: noStationHeading, Missions: none})
	}
	for i := range out {
		slices.SortFunc(out[i].Missions, compareMissions)
	}
	return out
}

// chainOnlyAt names the station a chain is exclusive to: the server-marked
// station of any step, else the one station every step shares, else "".
func chainOnlyAt(c MissionChain) string {
	for _, s := range c.Steps {
		if at := s.Mission.OnlyAt(); at != "" {
			return at
		}
	}
	shared := ""
	for _, s := range c.Steps {
		n, only := distinctStations([]*Mission{s.Mission})
		if n != 1 || (shared != "" && only != shared) {
			return ""
		}
		shared = only
	}
	return shared
}

// GoodsTotal is how many distinct goods the family's variants name.
func (f *MissionFamily) GoodsTotal() int { return len(f.Goods) + f.GoodsMore }
