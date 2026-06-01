# Factions & Players KB Pages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate KB HTML pages for player factions (with reconstructed member rosters) and standalone pages for every tracked player, plus add Factions/Players to site navigation.

**Architecture:** A single new Go generator `cmd/generate-factions-kb` reads `spacemolt-knowledge.db`, loads factions + players (rosters reconstructed from `seen_players`, overlaid with official `faction_members`), and writes `kb/factions/` and `kb/players/` trees in one pass using `html/template` string constants — matching the existing `generate-items-kb` pattern. Pure helpers (slug, relative-time, sighting grouping) are unit-tested; rendering is build-and-verify.

**Tech Stack:** Go 1.24, `html/template`, `modernc.org/sqlite`, `github.com/dustin/go-humanize`. Module path: `github.com/rsned/spacemolt-kb`.

---

## Background the engineer needs

- **Why rosters come from `seen_players`, not `faction_members`:** the game API reports `member_count` as 0 to outsiders and hides the official roster, so `factions.member_count` is ~always 0 and `faction_members` has only ~5 rows. `seen_players` (268 rows) is the real membership signal and joins to `factions` on `faction_id`.
- **NPC filter:** rows where `username LIKE '[%'` (e.g. `[CUSTOMS]…`) are stations/NPCs — exclude them everywhere. There are 0 `anonymous` rows.
- **Dates** are RFC3339 with `Z` suffix (e.g. `2026-05-17T19:06:12Z`).
- **`services_json`** on `faction_bases` is currently empty strings; parse best-effort and never error.
- **Output dir convention:** generators run from the repo root and write into `kb/<section>/`. The DB default path is `../spacemolt-knowledge.db` relative to where `generate-items-kb` expects it; for this generator use repo-root-relative `spacemolt-knowledge.db` with a positional override (see Task 7).
- **Existing page chrome** lives in `cmd/generate-items-kb/main.go`: `siteHeader`/`siteHeaderSub` (nav blocks), `themeScript`, `sortScript`. We replicate equivalents locally in the new command so it is self-contained.

## File Structure

```
cmd/generate-factions-kb/
  main.go        # flags, DB open, orchestration, output writing, generation timestamp
  types.go       # Faction, Member, Base, Relation, Facility, Player, ShipSeen, Sighting
  slug.go        # slugify, playerSlug
  slug_test.go
  timeago.go     # relativeTime
  timeago_test.go
  grouping.go    # groupSightings
  grouping_test.go
  load.go        # loadFactions + roster/bases/relations/facilities
  players.go     # loadPlayers (+ ships + sightings)
  render.go      # template constants, FuncMap, chrome (header/scripts), write helpers
kb/factions/factions.css   # new
kb/players/players.css      # new
kb/index.html               # modified: add Factions + Players cards and home-nav links
```

---

## Task 1: Scaffold command, types, and slug helper (TDD)

**Files:**
- Create: `cmd/generate-factions-kb/types.go`
- Create: `cmd/generate-factions-kb/slug.go`
- Test: `cmd/generate-factions-kb/slug_test.go`

- [ ] **Step 1: Create the types file**

Create `cmd/generate-factions-kb/types.go`:

```go
// Command generate-factions-kb reads the knowledge database and produces
// KB-styled HTML pages for player factions and the players sighted in them.
package main

// Faction is a player faction with its reconstructed roster and related records.
type Faction struct {
	ID                  string
	Name                string
	Tag                 string
	Slug                string
	LeaderName          string
	Treasury            int64
	OwnedBases          int
	Description         string
	Charter             string
	Emblem              string
	PrimaryColor        string
	SecondaryColor      string
	FoundedUTC          string
	OfficialMemberCount int // factions.member_count (usually 0, hidden by the API)

	Members    []*Member
	Bases      []Base
	Relations  []Relation
	Facilities []Facility
}

// MemberCount is the reconstructed roster size (distinct sighted players).
func (f *Faction) MemberCount() int { return len(f.Members) }

// Member is one player on a faction's reconstructed roster.
type Member struct {
	PlayerID    string
	Username    string
	Slug        string
	Role        string // from faction_members overlay; "" when unknown
	IsOnline    bool
	LastSeenUTC string
	Ships       []string // distinct ship class names sighted
}

// Base is a faction-owned base.
type Base struct {
	Name       string
	SystemName string
	Services   string
}

// Relation is a diplomatic relation to another faction.
type Relation struct {
	Kind       string
	TargetName string
	TargetTag  string
	Reason     string
	OurKills   int
	TheirKills int
}

// Facility is a faction facility at a base.
type Facility struct {
	Type     string
	Category string
	Level    int
	Status   string
}

// Player is a tracked player with sighting-derived history.
type Player struct {
	ID             string
	Username       string
	Slug           string
	FactionID      string
	FactionTag     string
	FactionSlug    string // "" when the faction is unknown/untracked
	ClanTag        string
	PrimaryColor   string
	SecondaryColor string
	StatusMessage  string
	FirstSeenUTC   string
	LastSeenUTC    string

	Ships     []ShipSeen
	Sightings []Sighting
}

// ShipSeen is a ship class a player was observed flying.
type ShipSeen struct {
	Class        string
	FirstSeenUTC string
	LastSeenUTC  string
}

// Sighting is a grouped where/when observation of a player.
type Sighting struct {
	SystemID    string
	SystemSlug  string // "" when no system page exists
	POIID       string
	ShipClass   string
	InCombat    bool
	LastSeenUTC string
}
```

- [ ] **Step 2: Write the failing slug test**

Create `cmd/generate-factions-kb/slug_test.go`:

```go
package main

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"STRG":            "strg",
		"End of Line":     "end-of-line",
		"Oberste Raumbehörde": "oberste-raumbehorde",
		"  Hex  Collective  ": "hex-collective",
		"[CUSTOMS]":       "customs",
		"!!!":             "",
		"":                "",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlayerSlug(t *testing.T) {
	got := playerSlug("Alice", "17a08149befb15b51a1fcf8bca325c36")
	want := "alice-17a08149"
	if got != want {
		t.Errorf("playerSlug = %q, want %q", got, want)
	}
	// Empty username falls back to the id8 prefix only.
	if got := playerSlug("!!!", "deadbeefcafef00d0000000000000000"); got != "deadbeef" {
		t.Errorf("playerSlug empty-name = %q, want %q", got, "deadbeef")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run 'TestSlugify|TestPlayerSlug' -v`
Expected: FAIL — `undefined: slugify` / `undefined: playerSlug`.

- [ ] **Step 4: Implement slug.go**

Create `cmd/generate-factions-kb/slug.go`:

```go
package main

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// slugify lowercases s, strips diacritics, and replaces every run of
// non-alphanumeric characters with a single '-', trimming leading/trailing '-'.
// Returns "" when nothing usable remains.
func slugify(s string) string {
	// Decompose accents (ö -> o) then drop combining marks.
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	if out, _, err := transform.String(t, s); err == nil {
		s = out
	}
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// playerSlug builds a stable, unique slug from a username and player id:
// "<slugified-username>-<first 8 of id>". Falls back to just the id8 when the
// username slugifies to empty.
func playerSlug(username, playerID string) string {
	id8 := playerID
	if len(id8) > 8 {
		id8 = id8[:8]
	}
	base := slugify(username)
	if base == "" {
		return id8
	}
	return base + "-" + id8
}
```

- [ ] **Step 5: Ensure the text module dependency is present**

Run: `cd /home/robert/spacemolt/kb && go get golang.org/x/text@latest && go mod tidy`
Expected: `golang.org/x/text` appears in `go.mod` require block.

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run 'TestSlugify|TestPlayerSlug' -v`
Expected: PASS (both tests).

- [ ] **Step 7: Commit**

```bash
git add cmd/generate-factions-kb/types.go cmd/generate-factions-kb/slug.go cmd/generate-factions-kb/slug_test.go go.mod go.sum
git commit -m "feat(factions): scaffold generator types and slug helper"
```

---

## Task 2: Relative-time helper (TDD)

**Files:**
- Create: `cmd/generate-factions-kb/timeago.go`
- Test: `cmd/generate-factions-kb/timeago_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/generate-factions-kb/timeago_test.go`:

```go
package main

import (
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		utc  string
		want string
	}{
		{"2026-05-31T11:59:30Z", "just now"},
		{"2026-05-31T11:30:00Z", "30m ago"},
		{"2026-05-31T10:00:00Z", "2h ago"},
		{"2026-05-29T12:00:00Z", "2d ago"},
		{"2026-05-31T12:30:00Z", "just now"}, // future clamps to "just now"
		{"", "—"},
		{"not-a-date", "—"},
	}
	for _, c := range cases {
		if got := relativeTime(now, c.utc); got != c.want {
			t.Errorf("relativeTime(%q) = %q, want %q", c.utc, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run TestRelativeTime -v`
Expected: FAIL — `undefined: relativeTime`.

- [ ] **Step 3: Implement timeago.go**

Create `cmd/generate-factions-kb/timeago.go`:

```go
package main

import (
	"fmt"
	"time"
)

// relativeTime renders utc (RFC3339) as a short human delta from now:
// "just now", "30m ago", "2h ago", "2d ago". Future or unparseable times
// return "just now" / "—" respectively so templates never error.
func relativeTime(now time.Time, utc string) string {
	if utc == "" {
		return "—"
	}
	ts, err := time.Parse(time.RFC3339, utc)
	if err != nil {
		return "—"
	}
	d := now.Sub(ts)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run TestRelativeTime -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-factions-kb/timeago.go cmd/generate-factions-kb/timeago_test.go
git commit -m "feat(factions): add relative-time helper"
```

---

## Task 3: Sighting grouping helper (TDD)

**Files:**
- Create: `cmd/generate-factions-kb/grouping.go`
- Test: `cmd/generate-factions-kb/grouping_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/generate-factions-kb/grouping_test.go`:

```go
package main

import "testing"

func TestGroupSightings(t *testing.T) {
	in := []Sighting{
		{SystemID: "sol", POIID: "stationA", ShipClass: "Frigate", InCombat: false, LastSeenUTC: "2026-05-31T10:00:00Z"},
		{SystemID: "sol", POIID: "stationA", ShipClass: "Frigate", InCombat: true, LastSeenUTC: "2026-05-31T12:00:00Z"},
		{SystemID: "vega", POIID: "", ShipClass: "Hauler", InCombat: false, LastSeenUTC: "2026-05-30T09:00:00Z"},
	}
	got := groupSightings(in)
	if len(got) != 2 {
		t.Fatalf("got %d groups, want 2", len(got))
	}
	// Groups are sorted by LastSeenUTC desc; sol/Frigate is most recent.
	if got[0].SystemID != "sol" || !got[0].InCombat || got[0].LastSeenUTC != "2026-05-31T12:00:00Z" {
		t.Errorf("first group = %+v; want sol/Frigate combat=true latest 12:00", got[0])
	}
	if got[1].SystemID != "vega" {
		t.Errorf("second group = %+v; want vega", got[1])
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run TestGroupSightings -v`
Expected: FAIL — `undefined: groupSightings`.

- [ ] **Step 3: Implement grouping.go**

Create `cmd/generate-factions-kb/grouping.go`:

```go
package main

import (
	"sort"
	"strings"
)

// groupSightings collapses raw sightings by (system, poi, ship), keeping the
// latest LastSeenUTC and OR-ing the InCombat flag. Result is sorted by
// LastSeenUTC descending (string compare is valid for RFC3339 UTC).
func groupSightings(in []Sighting) []Sighting {
	type key struct{ sys, poi, ship string }
	byKey := map[key]*Sighting{}
	order := []key{}
	for i := range in {
		s := in[i]
		k := key{s.SystemID, s.POIID, s.ShipClass}
		g, ok := byKey[k]
		if !ok {
			cp := s
			byKey[k] = &cp
			order = append(order, k)
			continue
		}
		g.InCombat = g.InCombat || s.InCombat
		if s.LastSeenUTC > g.LastSeenUTC {
			g.LastSeenUTC = s.LastSeenUTC
		}
	}
	out := make([]Sighting, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LastSeenUTC != out[j].LastSeenUTC {
			return out[i].LastSeenUTC > out[j].LastSeenUTC
		}
		return strings.Compare(out[i].SystemID, out[j].SystemID) < 0
	})
	return out
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run TestGroupSightings -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-factions-kb/grouping.go cmd/generate-factions-kb/grouping_test.go
git commit -m "feat(factions): add sighting grouping helper"
```

---

## Task 4: Faction loaders (load.go)

**Files:**
- Create: `cmd/generate-factions-kb/load.go`

This task adds DB loading. It compiles as part of the package but is exercised end-to-end in Task 7. No new unit test (DB-backed); correctness verified at run time.

- [ ] **Step 1: Implement load.go**

Create `cmd/generate-factions-kb/load.go`:

```go
package main

import (
	"database/sql"
	"fmt"
	"sort"
)

// loadFactions loads every faction with its reconstructed roster (from
// seen_players, overlaid with official faction_members), bases, relations, and
// facilities. shipsByPlayer maps player_id -> distinct ship class names.
func loadFactions(db *sql.DB, shipsByPlayer map[string][]string) ([]*Faction, error) {
	rows, err := db.Query(`
		SELECT faction_id, name, tag, leader_username, treasury, member_count,
		       owned_bases, description, charter, emblem,
		       primary_color, secondary_color, founded_utc
		FROM factions`)
	if err != nil {
		return nil, fmt.Errorf("query factions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var factions []*Faction
	seenSlug := map[string]bool{}
	for rows.Next() {
		f := &Faction{}
		var name, tag, leader, desc, charter, emblem, pc, sc, founded sql.NullString
		var treasury, memberCount, bases sql.NullInt64
		if err := rows.Scan(&f.ID, &name, &tag, &leader, &treasury, &memberCount,
			&bases, &desc, &charter, &emblem, &pc, &sc, &founded); err != nil {
			return nil, fmt.Errorf("scan faction: %w", err)
		}
		f.Name = name.String
		f.Tag = tag.String
		f.LeaderName = leader.String
		f.Treasury = treasury.Int64
		f.OfficialMemberCount = int(memberCount.Int64)
		f.OwnedBases = int(bases.Int64)
		f.Description = desc.String
		f.Charter = charter.String
		f.Emblem = emblem.String
		f.PrimaryColor = pc.String
		f.SecondaryColor = sc.String
		f.FoundedUTC = founded.String

		// Stable, unique slug from tag (fall back to name, then id8).
		base := slugify(f.Tag)
		if base == "" {
			base = slugify(f.Name)
		}
		if base == "" {
			base = f.ID
		}
		slug := base
		for n := 2; seenSlug[slug]; n++ {
			slug = fmt.Sprintf("%s-%d", base, n)
		}
		seenSlug[slug] = true
		f.Slug = slug

		factions = append(factions, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	byID := map[string]*Faction{}
	for _, f := range factions {
		byID[f.ID] = f
	}

	if err := loadRosters(db, byID, shipsByPlayer); err != nil {
		return nil, err
	}
	if err := loadBases(db, byID); err != nil {
		return nil, err
	}
	if err := loadRelations(db, byID); err != nil {
		return nil, err
	}
	if err := loadFacilities(db, byID); err != nil {
		return nil, err
	}

	// Roster ordering: online first, then most-recently-seen.
	for _, f := range factions {
		sort.SliceStable(f.Members, func(i, j int) bool {
			a, b := f.Members[i], f.Members[j]
			if a.IsOnline != b.IsOnline {
				return a.IsOnline
			}
			return a.LastSeenUTC > b.LastSeenUTC
		})
	}
	// Faction ordering: largest reconstructed roster first.
	sort.SliceStable(factions, func(i, j int) bool {
		if factions[i].MemberCount() != factions[j].MemberCount() {
			return factions[i].MemberCount() > factions[j].MemberCount()
		}
		return factions[i].Name < factions[j].Name
	})
	return factions, nil
}

// loadRosters fills each faction's Members from seen_players, overlaying role /
// online state from faction_members where present.
func loadRosters(db *sql.DB, byID map[string]*Faction, shipsByPlayer map[string][]string) error {
	// Official overlay: (faction_id, player_id) -> (role, online).
	type overlay struct {
		role   string
		online bool
	}
	ov := map[string]overlay{} // key: factionID + "|" + playerID
	orows, err := db.Query(`SELECT faction_id, player_id, role, is_online FROM faction_members`)
	if err != nil {
		return fmt.Errorf("query faction_members: %w", err)
	}
	for orows.Next() {
		var fid, pid string
		var role sql.NullString
		var online sql.NullInt64
		if err := orows.Scan(&fid, &pid, &role, &online); err != nil {
			_ = orows.Close()
			return fmt.Errorf("scan faction_members: %w", err)
		}
		ov[fid+"|"+pid] = overlay{role: role.String, online: online.Int64 != 0}
	}
	_ = orows.Close()
	if err := orows.Err(); err != nil {
		return err
	}

	rows, err := db.Query(`
		SELECT faction_id, player_id, username, last_seen_utc
		FROM seen_players
		WHERE faction_id IS NOT NULL AND faction_id != '' AND username NOT LIKE '[%'`)
	if err != nil {
		return fmt.Errorf("query seen_players rosters: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var fid, pid string
		var uname, lastSeen sql.NullString
		if err := rows.Scan(&fid, &pid, &uname, &lastSeen); err != nil {
			return fmt.Errorf("scan roster: %w", err)
		}
		f, ok := byID[fid]
		if !ok {
			continue // sighted faction we have no factions-table row for
		}
		m := &Member{
			PlayerID:    pid,
			Username:    uname.String,
			Slug:        playerSlug(uname.String, pid),
			LastSeenUTC: lastSeen.String,
			Ships:       shipsByPlayer[pid],
		}
		if o, ok := ov[fid+"|"+pid]; ok {
			m.Role = o.role
			m.IsOnline = o.online
		}
		f.Members = append(f.Members, m)
	}
	return rows.Err()
}

func loadBases(db *sql.DB, byID map[string]*Faction) error {
	rows, err := db.Query(`SELECT faction_id, base_name, system_name, services_json FROM faction_bases`)
	if err != nil {
		return fmt.Errorf("query faction_bases: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var fid string
		var name, sys, svc sql.NullString
		if err := rows.Scan(&fid, &name, &sys, &svc); err != nil {
			return fmt.Errorf("scan base: %w", err)
		}
		if f, ok := byID[fid]; ok {
			f.Bases = append(f.Bases, Base{
				Name:       name.String,
				SystemName: sys.String,
				Services:   parseServices(svc.String),
			})
		}
	}
	return rows.Err()
}

func loadRelations(db *sql.DB, byID map[string]*Faction) error {
	rows, err := db.Query(`SELECT faction_id, kind, target_name, target_tag, reason, our_kills, their_kills FROM faction_relations`)
	if err != nil {
		return fmt.Errorf("query faction_relations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var fid string
		var kind, tname, ttag, reason sql.NullString
		var ours, theirs sql.NullInt64
		if err := rows.Scan(&fid, &kind, &tname, &ttag, &reason, &ours, &theirs); err != nil {
			return fmt.Errorf("scan relation: %w", err)
		}
		if f, ok := byID[fid]; ok {
			f.Relations = append(f.Relations, Relation{
				Kind:       kind.String,
				TargetName: tname.String,
				TargetTag:  ttag.String,
				Reason:     reason.String,
				OurKills:   int(ours.Int64),
				TheirKills: int(theirs.Int64),
			})
		}
	}
	return rows.Err()
}

func loadFacilities(db *sql.DB, byID map[string]*Faction) error {
	rows, err := db.Query(`SELECT faction_id, facility_type, category, level, status FROM faction_facilities`)
	if err != nil {
		return fmt.Errorf("query faction_facilities: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var fid string
		var ftype, cat, status sql.NullString
		var level sql.NullInt64
		if err := rows.Scan(&fid, &ftype, &cat, &level, &status); err != nil {
			return fmt.Errorf("scan facility: %w", err)
		}
		if f, ok := byID[fid]; ok {
			f.Facilities = append(f.Facilities, Facility{
				Type:     ftype.String,
				Category: cat.String,
				Level:    int(level.Int64),
				Status:   status.String,
			})
		}
	}
	return rows.Err()
}
```

- [ ] **Step 2: Add the parseServices helper to load.go**

Append to `cmd/generate-factions-kb/load.go`:

```go
import "encoding/json" // add to the existing import block, not a second block

// parseServices renders services_json best-effort. Accepts a JSON array of
// strings, a JSON object (uses its keys), or falls back to the raw trimmed
// string. Never errors.
func parseServices(raw string) string {
	if raw == "" {
		return ""
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return strings.Join(arr, ", ")
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return strings.Join(keys, ", ")
	}
	return strings.TrimSpace(raw)
}
```

Note: merge `encoding/json` and `strings` into the single import block at the top of `load.go` (the file already imports `database/sql`, `fmt`, `sort`; add `encoding/json` and `strings`).

- [ ] **Step 3: Verify it compiles**

Run: `go build ./cmd/generate-factions-kb/`
Expected: builds (no `main` yet is fine — package compiles; if Go complains about no main in package main, that's resolved in Task 7. To check now use `go vet ./cmd/generate-factions-kb/` which only needs the package to type-check). Run: `go vet ./cmd/generate-factions-kb/`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add cmd/generate-factions-kb/load.go
git commit -m "feat(factions): add faction/roster/bases/relations/facilities loaders"
```

---

## Task 5: Player loader (players.go)

**Files:**
- Create: `cmd/generate-factions-kb/players.go`

- [ ] **Step 1: Implement players.go**

Create `cmd/generate-factions-kb/players.go`:

```go
package main

import (
	"database/sql"
	"fmt"
	"sort"
)

// loadShips returns, per player_id, the distinct ship classes sighted (sorted),
// and the full ShipSeen records per player for player pages.
func loadShips(db *sql.DB) (classes map[string][]string, detail map[string][]ShipSeen, err error) {
	rows, err := db.Query(`SELECT player_id, ship_class, first_seen_utc, last_seen_utc FROM seen_player_ships`)
	if err != nil {
		return nil, nil, fmt.Errorf("query seen_player_ships: %w", err)
	}
	defer func() { _ = rows.Close() }()
	classes = map[string][]string{}
	detail = map[string][]ShipSeen{}
	for rows.Next() {
		var pid string
		var class, first, last sql.NullString
		if err := rows.Scan(&pid, &class, &first, &last); err != nil {
			return nil, nil, fmt.Errorf("scan ship: %w", err)
		}
		classes[pid] = append(classes[pid], class.String)
		detail[pid] = append(detail[pid], ShipSeen{
			Class:        class.String,
			FirstSeenUTC: first.String,
			LastSeenUTC:  last.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	for pid := range classes {
		sort.Strings(classes[pid])
	}
	return classes, detail, nil
}

// loadSightings returns grouped sightings per player_id, resolving system slugs
// against knownSystemSlugs (system_id -> slug; "" means no page exists).
func loadSightings(db *sql.DB, knownSystemSlugs map[string]string) (map[string][]Sighting, error) {
	rows, err := db.Query(`SELECT player_id, system_id, poi_id, ship_class, in_combat, last_seen_utc FROM seen_player_sightings`)
	if err != nil {
		return nil, fmt.Errorf("query seen_player_sightings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	raw := map[string][]Sighting{}
	for rows.Next() {
		var pid, sysID string
		var poi, ship, last sql.NullString
		var combat sql.NullInt64
		if err := rows.Scan(&pid, &sysID, &poi, &ship, &combat, &last); err != nil {
			return nil, fmt.Errorf("scan sighting: %w", err)
		}
		raw[pid] = append(raw[pid], Sighting{
			SystemID:    sysID,
			SystemSlug:  knownSystemSlugs[sysID],
			POIID:       poi.String,
			ShipClass:   ship.String,
			InCombat:    combat.Int64 != 0,
			LastSeenUTC: last.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := map[string][]Sighting{}
	for pid, list := range raw {
		out[pid] = groupSightings(list)
	}
	return out, nil
}

// loadPlayers loads all real players (NPC/station rows excluded), attaching
// their ships and grouped sightings. factionSlugByID maps faction_id -> slug
// for back-links ("" when untracked).
func loadPlayers(db *sql.DB, shipDetail map[string][]ShipSeen, sightings map[string][]Sighting, factionSlugByID map[string]string) ([]*Player, error) {
	rows, err := db.Query(`
		SELECT player_id, username, faction_id, faction_tag, clan_tag,
		       primary_color, secondary_color, status_message,
		       first_seen_utc, last_seen_utc
		FROM seen_players
		WHERE username NOT LIKE '[%'`)
	if err != nil {
		return nil, fmt.Errorf("query seen_players: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var players []*Player
	for rows.Next() {
		var pid string
		var uname, fid, ftag, clan, pc, sc, status, first, last sql.NullString
		if err := rows.Scan(&pid, &uname, &fid, &ftag, &clan, &pc, &sc, &status, &first, &last); err != nil {
			return nil, fmt.Errorf("scan player: %w", err)
		}
		p := &Player{
			ID:             pid,
			Username:       uname.String,
			Slug:           playerSlug(uname.String, pid),
			FactionID:      fid.String,
			FactionTag:     ftag.String,
			FactionSlug:    factionSlugByID[fid.String],
			ClanTag:        clan.String,
			PrimaryColor:   pc.String,
			SecondaryColor: sc.String,
			StatusMessage:  status.String,
			FirstSeenUTC:   first.String,
			LastSeenUTC:    last.String,
			Ships:          shipDetail[pid],
			Sightings:      sightings[pid],
		}
		sort.SliceStable(p.Ships, func(i, j int) bool { return p.Ships[i].Class < p.Ships[j].Class })
		players = append(players, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(players, func(i, j int) bool {
		if players[i].LastSeenUTC != players[j].LastSeenUTC {
			return players[i].LastSeenUTC > players[j].LastSeenUTC
		}
		return players[i].Username < players[j].Username
	})
	return players, nil
}

// knownSystemSlugs scans kb/systems/ for existing per-system page directories so
// sighting rows can link to them. The KB writes systems as kb/systems/<id>/.
func knownSystemSlugs(systemsDir string) map[string]string {
	out := map[string]string{}
	entries, err := osReadDir(systemsDir)
	if err != nil {
		return out // no systems dir -> no links, not an error
	}
	for _, e := range entries {
		if e.IsDir() {
			out[e.Name()] = e.Name()
		}
	}
	return out
}
```

Note: `osReadDir` is a thin wrapper added in `main.go` (Task 7) as `var osReadDir = os.ReadDir` to keep this file import-light; alternatively import `os` here and call `os.ReadDir` directly. Use `os.ReadDir` directly and add `"os"` to this file's imports — drop the `osReadDir` indirection:

```go
import "os"
// ...
entries, err := os.ReadDir(systemsDir)
```

- [ ] **Step 2: Verify it type-checks**

Run: `go vet ./cmd/generate-factions-kb/`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/generate-factions-kb/players.go
git commit -m "feat(factions): add player/ships/sightings loaders"
```

---

## Task 6: CSS files

**Files:**
- Create: `kb/factions/factions.css`
- Create: `kb/players/players.css`

These reuse `smui.css` variables (the KB exposes `--smui-*` HSL custom properties; existing pages use `hsl(var(--smui-frost-2))` etc.). Keep these small — layout leans on shared classes.

- [ ] **Step 1: Create kb/factions/factions.css**

```css
/* Faction KB pages. Builds on smui.css. */
.faction-cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: 1rem;
    margin-top: 1.25rem;
}
.faction-card {
    display: block;
    padding: 1rem 1.1rem;
    border-radius: 8px;
    border: 1px solid hsl(var(--smui-border, 220 13% 80%));
    border-left: 4px solid var(--faction-accent, hsl(var(--smui-frost-2)));
    text-decoration: none;
    color: inherit;
    background: hsl(var(--smui-surface, 0 0% 100%) / 0.4);
    transition: transform .08s ease, box-shadow .08s ease;
}
.faction-card:hover { transform: translateY(-2px); box-shadow: 0 4px 14px rgba(0,0,0,.12); }
.faction-card .fc-name { font-weight: 600; font-size: 1.05rem; }
.faction-card .fc-tag { opacity: .65; font-family: monospace; margin-left: .35rem; }
.faction-card .fc-stats { margin-top: .5rem; font-size: .85rem; opacity: .8; }

.faction-banner {
    border-left: 5px solid var(--faction-accent, hsl(var(--smui-frost-2)));
    padding: .75rem 1rem;
    border-radius: 6px;
    background: hsl(var(--smui-surface, 0 0% 100%) / 0.4);
    margin-bottom: 1rem;
}
.faction-banner .fb-tag { font-family: monospace; opacity: .7; }
.stat-strip { display: flex; flex-wrap: wrap; gap: 1.25rem; margin: .75rem 0; font-size: .9rem; }
.stat-strip .ss-item strong { display: block; font-size: 1.1rem; }
.api-note { font-size: .8rem; opacity: .7; font-style: italic; margin-top: .25rem; }
.online-dot { color: hsl(140 70% 45%); }
.kills { font-family: monospace; }
```

- [ ] **Step 2: Create kb/players/players.css**

```css
/* Player KB pages. Builds on smui.css. */
.player-banner {
    border-left: 5px solid var(--player-accent, hsl(var(--smui-frost-2)));
    padding: .75rem 1rem;
    border-radius: 6px;
    background: hsl(var(--smui-surface, 0 0% 100%) / 0.4);
    margin-bottom: 1rem;
}
.player-banner .pb-faction { font-family: monospace; opacity: .8; margin-left: .5rem; }
.player-banner .pb-status { font-style: italic; opacity: .8; margin-top: .35rem; }
.player-banner .pb-clan { opacity: .6; font-size: .85rem; }
.combat-flag { color: hsl(0 70% 55%); }
.muted { opacity: .6; }
```

- [ ] **Step 3: Commit**

```bash
git add kb/factions/factions.css kb/players/players.css
git commit -m "feat(factions): add faction and player page styles"
```

---

## Task 7: Templates, rendering, and orchestration (render.go + main.go)

**Files:**
- Create: `cmd/generate-factions-kb/render.go`
- Create: `cmd/generate-factions-kb/main.go`

- [ ] **Step 1: Create render.go (chrome + FuncMap + templates)**

Create `cmd/generate-factions-kb/render.go`. The nav blocks add **Factions** and **Players** to the standard KB nav. `siteHeader2` is for pages two levels deep (`kb/factions/<slug>/index.html`).

```go
package main

import (
	htmltpl "html/template"
	"time"
)

// templateFuncs returns helpers shared by all templates. genTime is the single
// generation timestamp so relative-time output is deterministic per run.
func templateFuncs(genTime time.Time) htmltpl.FuncMap {
	return htmltpl.FuncMap{
		"rel": func(utc string) string { return relativeTime(genTime, utc) },
		"shortDate": func(utc string) string {
			t, err := time.Parse(time.RFC3339, utc)
			if err != nil {
				return "—"
			}
			return t.Format("2006-01-02")
		},
		"join": func(parts []string, sep string) string {
			out := ""
			for i, p := range parts {
				if i > 0 {
					out += sep
				}
				out += p
			}
			return out
		},
		"dash": func(s string) string {
			if s == "" {
				return "—"
			}
			return s
		},
	}
}

// nav link list shared by both header depths.
const navLinks1 = `
            <a href="../">Home</a>
            <a href="../systems/index.html">Systems</a>
            <a href="../items/index.html">Items</a>
            <a href="../ships/index.html">Ships</a>
            <a href="../factions/index.html">Factions</a>
            <a href="../players/index.html">Players</a>`

const navLinks2 = `
            <a href="../../">Home</a>
            <a href="../../systems/index.html">Systems</a>
            <a href="../../items/index.html">Items</a>
            <a href="../../ships/index.html">Ships</a>
            <a href="../../factions/index.html">Factions</a>
            <a href="../../players/index.html">Players</a>`

const themeBtn = `
            <button class="theme-toggle" id="theme-toggle" aria-label="Toggle theme">
                <svg class="icon-sun" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
                <svg class="icon-moon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
            </button>`

var siteHeader1 = `    <header class="site-header">
        <h1><a href="../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>` + navLinks1 + themeBtn + `
        </nav>
    </header>`

var siteHeader2 = `    <header class="site-header">
        <h1><a href="../../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>` + navLinks2 + themeBtn + `
        </nav>
    </header>`

var themeScript = `    <script>
    (function() {
        var toggle = document.getElementById('theme-toggle');
        var root = document.documentElement;
        if (localStorage.getItem('theme') === 'dark') root.classList.add('dark');
        toggle.addEventListener('click', function() {
            root.classList.toggle('dark');
            localStorage.setItem('theme', root.classList.contains('dark') ? 'dark' : 'light');
        });
    })();
    </script>`

// --- Faction index ---
var factionIndexTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Factions - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../factions/factions.css">
</head>
<body>
` + siteHeader1 + `
    <main class="container page-content">
        <h2>Factions</h2>
        <p class="text-muted mt-1">{{len .}} player factions. Member rosters are reconstructed from sightings &mdash; the game API reports member counts as 0 to outsiders.</p>
        <div class="faction-cards">
{{- range .}}
            <a href="{{.Slug}}/" class="faction-card"{{if .PrimaryColor}} style="--faction-accent:{{.PrimaryColor}}"{{end}}>
                <div class="fc-name">{{.Name}}<span class="fc-tag">[{{.Tag}}]</span></div>
                <div class="fc-stats">{{.MemberCount}} members &middot; {{.OwnedBases}} bases &middot; {{.Treasury}} cr</div>
            </a>
{{- end}}
        </div>
    </main>
` + themeScript + `
</body>
</html>
`

// --- Faction detail ---
var factionDetailTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} [{{.Tag}}] - Factions - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../players/players.css">
    <link rel="stylesheet" href="../../factions/factions.css">
</head>
<body>
` + siteHeader2 + `
    <main class="container page-content">
        <div class="faction-banner"{{if .PrimaryColor}} style="--faction-accent:{{.PrimaryColor}}"{{end}}>
            <h2>{{.Name}} <span class="fb-tag">[{{.Tag}}]</span></h2>
            {{if .FoundedUTC}}<div class="text-muted">Founded {{shortDate .FoundedUTC}}</div>{{end}}
            {{if .Description}}<p>{{.Description}}</p>{{end}}
            {{if .Charter}}<p class="text-muted">{{.Charter}}</p>{{end}}
        </div>

        <div class="stat-strip">
            <div class="ss-item"><strong>{{.Treasury}}</strong> treasury (cr)</div>
            <div class="ss-item"><strong>{{.OwnedBases}}</strong> bases</div>
            <div class="ss-item"><strong>{{dash .LeaderName}}</strong> leader</div>
            <div class="ss-item"><strong>{{.MemberCount}}</strong> members (sighted)</div>
        </div>
        <p class="api-note">Official API member_count: {{.OfficialMemberCount}} (hidden from outsiders); roster below is reconstructed from sightings.</p>

        <h3>Members ({{.MemberCount}})</h3>
        <table class="sortable">
            <thead><tr><th class="sortable">Username</th><th class="sortable">Role</th><th>Ships seen</th><th class="sortable">Last seen</th></tr></thead>
            <tbody>
{{- range .Members}}
                <tr>
                    <td><a href="../../players/{{.Slug}}/">{{if .IsOnline}}<span class="online-dot">&#9679;</span> {{end}}{{.Username}}</a></td>
                    <td>{{dash .Role}}</td>
                    <td>{{if .Ships}}{{join .Ships ", "}}{{else}}&mdash;{{end}}</td>
                    <td data-sort="{{.LastSeenUTC}}" title="{{.LastSeenUTC}}">{{rel .LastSeenUTC}}</td>
                </tr>
{{- end}}
            </tbody>
        </table>

{{if .Bases}}
        <h3>Bases ({{len .Bases}})</h3>
        <table>
            <thead><tr><th>Name</th><th>System</th><th>Services</th></tr></thead>
            <tbody>
{{- range .Bases}}
                <tr><td>{{dash .Name}}</td><td>{{dash .SystemName}}</td><td>{{dash .Services}}</td></tr>
{{- end}}
            </tbody>
        </table>
{{end}}

{{if .Relations}}
        <h3>Relations</h3>
        <table>
            <thead><tr><th>Kind</th><th>Faction</th><th>Reason</th><th>Kills (us/them)</th></tr></thead>
            <tbody>
{{- range .Relations}}
                <tr><td>{{.Kind}}</td><td>{{.TargetName}} {{if .TargetTag}}[{{.TargetTag}}]{{end}}</td><td>{{dash .Reason}}</td><td class="kills">{{.OurKills}} / {{.TheirKills}}</td></tr>
{{- end}}
            </tbody>
        </table>
{{end}}

{{if .Facilities}}
        <h3>Facilities</h3>
        <table>
            <thead><tr><th>Type</th><th>Category</th><th>Level</th><th>Status</th></tr></thead>
            <tbody>
{{- range .Facilities}}
                <tr><td>{{dash .Type}}</td><td>{{dash .Category}}</td><td>{{.Level}}</td><td>{{dash .Status}}</td></tr>
{{- end}}
            </tbody>
        </table>
{{end}}
    </main>
` + themeScript + `
</body>
</html>
`

// --- Player index ---
var playerIndexTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Players - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../players/players.css">
</head>
<body>
` + siteHeader1 + `
    <main class="container page-content">
        <h2>Players</h2>
        <p class="text-muted mt-1">{{len .}} players tracked from sightings.</p>
        <table class="sortable">
            <thead><tr><th class="sortable">Username</th><th class="sortable">Faction</th><th>Ships seen</th><th class="sortable">Last seen</th></tr></thead>
            <tbody>
{{- range .}}
                <tr>
                    <td><a href="{{.Slug}}/">{{.Username}}</a></td>
                    <td>{{if .FactionSlug}}<a href="../factions/{{.FactionSlug}}/">[{{.FactionTag}}]</a>{{else if .FactionTag}}[{{.FactionTag}}]{{else}}<span class="muted">&mdash;</span>{{end}}</td>
                    <td>{{if .Ships}}{{range $i, $s := .Ships}}{{if $i}}, {{end}}{{$s.Class}}{{end}}{{else}}&mdash;{{end}}</td>
                    <td data-sort="{{.LastSeenUTC}}" title="{{.LastSeenUTC}}">{{rel .LastSeenUTC}}</td>
                </tr>
{{- end}}
            </tbody>
        </table>
    </main>
` + themeScript + `
</body>
</html>
`

// --- Player detail ---
var playerDetailTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Username}} - Players - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../players/players.css">
</head>
<body>
` + siteHeader2 + `
    <main class="container page-content">
        <div class="player-banner"{{if .PrimaryColor}} style="--player-accent:{{.PrimaryColor}}"{{end}}>
            <h2>{{.Username}}{{if .FactionSlug}} <a class="pb-faction" href="../../factions/{{.FactionSlug}}/">[{{.FactionTag}}]</a>{{else if .FactionTag}}<span class="pb-faction">[{{.FactionTag}}]</span>{{end}}</h2>
            {{if .ClanTag}}<div class="pb-clan">clan {{.ClanTag}}</div>{{end}}
            {{if .StatusMessage}}<div class="pb-status">{{.StatusMessage}}</div>{{end}}
        </div>

        <div class="stat-strip">
            <div class="ss-item"><strong>{{shortDate .FirstSeenUTC}}</strong> first seen</div>
            <div class="ss-item"><strong>{{rel .LastSeenUTC}}</strong> last seen</div>
        </div>

{{if .Ships}}
        <h3>Ships seen</h3>
        <table>
            <thead><tr><th>Class</th><th>First seen</th><th>Last seen</th></tr></thead>
            <tbody>
{{- range .Ships}}
                <tr><td>{{.Class}}</td><td>{{shortDate .FirstSeenUTC}}</td><td title="{{.LastSeenUTC}}">{{rel .LastSeenUTC}}</td></tr>
{{- end}}
            </tbody>
        </table>
{{end}}

{{if .Sightings}}
        <h3>Activity (where seen)</h3>
        <table class="sortable">
            <thead><tr><th class="sortable">System</th><th>POI</th><th>Ship</th><th>Combat</th><th class="sortable">Last seen</th></tr></thead>
            <tbody>
{{- range .Sightings}}
                <tr>
                    <td>{{if .SystemSlug}}<a href="../../systems/{{.SystemSlug}}/">{{.SystemID}}</a>{{else}}{{.SystemID}}{{end}}</td>
                    <td>{{if .POIID}}{{.POIID}}{{else}}&mdash;{{end}}</td>
                    <td>{{dash .ShipClass}}</td>
                    <td>{{if .InCombat}}<span class="combat-flag">&#9876;</span>{{else}}&mdash;{{end}}</td>
                    <td data-sort="{{.LastSeenUTC}}" title="{{.LastSeenUTC}}">{{rel .LastSeenUTC}}</td>
                </tr>
{{- end}}
            </tbody>
        </table>
{{end}}
    </main>
` + themeScript + `
</body>
</html>
`
```

- [ ] **Step 2: Create main.go (orchestration + output)**

Create `cmd/generate-factions-kb/main.go`:

```go
package main

import (
	"database/sql"
	"flag"
	htmltpl "html/template"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	flag.Parse()

	dbPath := "spacemolt-knowledge.db"
	factionsOut := "kb/factions"
	playersOut := "kb/players"
	systemsDir := "kb/systems"

	if args := flag.Args(); len(args) > 0 {
		dbPath = args[0]
	}

	genTime := time.Now().UTC()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Ships first (rosters and player pages both need them).
	shipClasses, shipDetail, err := loadShips(db)
	if err != nil {
		log.Fatalf("load ships: %v", err)
	}
	sysSlugs := knownSystemSlugs(systemsDir)

	factions, err := loadFactions(db, shipClasses)
	if err != nil {
		log.Fatalf("load factions: %v", err)
	}
	factionSlugByID := map[string]string{}
	for _, f := range factions {
		factionSlugByID[f.ID] = f.Slug
	}

	sightings, err := loadSightings(db, sysSlugs)
	if err != nil {
		log.Fatalf("load sightings: %v", err)
	}
	players, err := loadPlayers(db, shipDetail, sightings, factionSlugByID)
	if err != nil {
		log.Fatalf("load players: %v", err)
	}

	funcs := templateFuncs(genTime)
	fIdx := htmltpl.Must(htmltpl.New("fidx").Funcs(funcs).Parse(factionIndexTmpl))
	fDet := htmltpl.Must(htmltpl.New("fdet").Funcs(funcs).Parse(factionDetailTmpl))
	pIdx := htmltpl.Must(htmltpl.New("pidx").Funcs(funcs).Parse(playerIndexTmpl))
	pDet := htmltpl.Must(htmltpl.New("pdet").Funcs(funcs).Parse(playerDetailTmpl))

	// Clean + recreate output dirs, preserving the .css files.
	mustResetDir(factionsOut, "factions.css")
	mustResetDir(playersOut, "players.css")

	mustWrite(filepath.Join(factionsOut, "index.html"), fIdx, factions)
	for _, f := range factions {
		dir := filepath.Join(factionsOut, f.Slug)
		mustMkdir(dir)
		mustWrite(filepath.Join(dir, "index.html"), fDet, f)
	}

	mustWrite(filepath.Join(playersOut, "index.html"), pIdx, players)
	for _, p := range players {
		dir := filepath.Join(playersOut, p.Slug)
		mustMkdir(dir)
		mustWrite(filepath.Join(dir, "index.html"), pDet, p)
	}

	log.Printf("generated %d factions and %d players", len(factions), len(players))
}

// mustResetDir removes generated HTML under dir (recursively for subdirs) but
// keeps the named file (the section CSS) and the dir itself.
func mustResetDir(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			mustMkdir(dir)
			return
		}
		log.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		if e.Name() == keep {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if err := os.RemoveAll(p); err != nil {
			log.Fatalf("remove %s: %v", p, err)
		}
	}
}

func mustMkdir(dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", dir, err)
	}
}

func mustWrite(path string, tmpl *htmltpl.Template, data any) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("create %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	if err := tmpl.Execute(f, data); err != nil {
		log.Fatalf("render %s: %v", path, err)
	}
}
```

- [ ] **Step 3: Build the command**

Run: `cd /home/robert/spacemolt/kb && go build ./cmd/generate-factions-kb/`
Expected: builds with no errors.

- [ ] **Step 4: Run the generator against the real DB**

Run: `cd /home/robert/spacemolt/kb && go run ./cmd/generate-factions-kb ../spacemolt-knowledge.db`
Expected log: `generated 18 factions and 267 players` (counts may differ slightly as data refreshes). `kb/factions/index.html`, `kb/factions/strg/index.html`, `kb/players/index.html`, and a `kb/players/<slug>/index.html` exist.

Verify: `ls kb/factions kb/players | head` and `grep -c faction-card kb/factions/index.html` (>0).

- [ ] **Step 5: Spot-check rendered output**

Run: `grep -o '<title>[^<]*' kb/factions/strg/index.html` → shows `STRG`.
Run: `grep -c 'players/' kb/factions/strg/index.html` → member rows link to player pages (>0).
Run: `grep -o 'reconstructed from sightings' kb/factions/index.html` → present (the API note).
Open `kb/players/index.html` and a player page in a browser; confirm nav shows Factions + Players, theme toggle works, faction tag links resolve, and no sighting-count numbers appear.

- [ ] **Step 6: Commit**

```bash
git add cmd/generate-factions-kb/render.go cmd/generate-factions-kb/main.go kb/factions kb/players
git commit -m "feat(factions): render faction and player pages + orchestration"
```

---

## Task 8: Home page links + final verification

**Files:**
- Modify: `kb/index.html` (add Factions + Players cards and header-nav links)

- [ ] **Step 1: Add header-nav links on the home page**

In `kb/index.html`, inside the `<header class="site-header"><nav>…</nav>` block, after the existing `Ships`/`Missions` links and before the theme-toggle button, add:

```html
            <a href="factions/index.html">Factions</a>
            <a href="players/index.html">Players</a>
```

(Home-page nav uses paths without `../` since it sits at the KB root.)

- [ ] **Step 2: Add two category cards**

In `kb/index.html`, inside `<div class="categories">`, after the last existing `</a>` card (card 08 — Contracts), add:

```html
            <a href="factions/index.html" class="cat" style="--cat-accent: hsl(var(--smui-purple))">
                <div class="cat-top">
                    <span class="cat-label">09 &mdash; Politics</span>
                    <span class="cat-stat">18 factions</span>
                </div>
                <div class="cat-name">
                    <span class="cat-glyph">&#x2691;</span>
                    Factions
                    <span class="cat-arrow">&rarr;</span>
                </div>
                <p class="cat-desc">Player factions, their charters and treasuries, bases, diplomacy, and reconstructed member rosters.</p>
                <div class="cat-tags">
                    <span class="cat-tag">Rosters</span>
                    <span class="cat-tag">Bases</span>
                    <span class="cat-tag">Relations</span>
                    <span class="cat-tag">Treasury</span>
                </div>
            </a>

            <a href="players/index.html" class="cat" style="--cat-accent: hsl(var(--smui-frost-3))">
                <div class="cat-top">
                    <span class="cat-label">10 &mdash; People</span>
                    <span class="cat-stat">267 players</span>
                </div>
                <div class="cat-name">
                    <span class="cat-glyph">&#x265F;</span>
                    Players
                    <span class="cat-arrow">&rarr;</span>
                </div>
                <p class="cat-desc">Every player sighted across the galaxy. Ships flown, where they have been seen, and faction ties.</p>
                <div class="cat-tags">
                    <span class="cat-tag">Sightings</span>
                    <span class="cat-tag">Ships</span>
                    <span class="cat-tag">Factions</span>
                    <span class="cat-tag">Activity</span>
                </div>
            </a>
```

(If `--smui-purple` / `--smui-frost-3` are not defined in `smui.css`, substitute an existing accent var used by neighbouring cards — check with `grep -o 'smui-[a-z0-9-]*' kb/smui.css | sort -u`.)

- [ ] **Step 3: Verify links resolve**

Run: `grep -c 'factions/index.html\|players/index.html' kb/index.html`
Expected: ≥ 3 (one nav link + one card each, players appears twice).
Open `kb/index.html` in a browser; click the two new cards → land on the new index pages.

- [ ] **Step 4: Full build, test, and lint**

Run: `cd /home/robert/spacemolt/kb && go build ./... && go test ./...`
Expected: all pass (`cmd/generate-factions-kb` tests green).

Run golangci-lint on the new package (use the golangci-lint tool). Expected: no new findings.

- [ ] **Step 5: Commit**

```bash
git add kb/index.html
git commit -m "feat(factions): link Factions and Players from the KB home page"
```

---

## Self-Review Notes (for the implementer)

- **Spec coverage:** faction index/detail (Task 7), player index/detail (Task 7), merged roster source seen_players⊎faction_members (Task 4 `loadRosters`), NPC filter `NOT LIKE '[%'` (Tasks 4 & 5), no sighting counts anywhere (templates omit them), slugs (Task 1), relative time (Task 2), sighting grouping (Task 3), system cross-links only when page exists (Task 5 `knownSystemSlugs` + templates), nav additions (Task 7 headers + Task 8 home), CSS (Task 6), error tolerance for empty/malformed fields (NullString scans + `parseServices`).
- **Type consistency:** `Faction.MemberCount()` method used in templates and ordering; `Member.Slug`/`Player.Slug` produced by `playerSlug`; `factionSlugByID` built in `main` and passed to `loadPlayers`; `sysSlugs` (system_id→slug) built once and shared.
- **Determinism:** single `genTime` threaded through `templateFuncs`.

## Out of Scope (follow-ups)

- Updating other section generators' nav blocks for Factions/Players parity (those regenerate independently).
- Historical/time-series activity charts; kill leaderboards; pages for filtered NPC/station entities.
