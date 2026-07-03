# Factions & Players KB Pages — Design

**Date:** 2026-05-31
**Status:** Approved (pending spec review)

## Goal

Generate KB pages for player **factions** and **players**, formatted in the existing
KB style. Each faction gets a page with its metadata and a reconstructed member
roster; each member (and every other tracked player) gets a standalone player page.
Add **Factions** and **Players** to the site navigation.

## Background & Key Constraint

Faction/player data lives in `spacemolt-knowledge.db` (not the crafting DB used by
`generate-items-kb`).

The game API **reports `member_count` as 0 to outsiders** and only exposes a member
roster to faction members. Consequently:

- `factions.member_count` is stale/zero for almost every faction.
- `faction_members` has only ~5 rows total.
- `seen_players` (268 rows) is the **real source of truth** for membership — it joins
  cleanly to `factions` on `faction_id` (e.g. STRG shows 33 seen players vs.
  `member_count` 0).

Therefore rosters are **reconstructed from sightings** (`seen_players`), with official
`faction_members` data overlaid where it exists. Pages display a note explaining the
reconstructed nature.

NPC/system noise in `seen_players` is minimal: exactly 1 bracket-prefixed entry
(`[CUSTOMS]…`, a station) and 0 anonymous rows. Filter rule: skip
`username LIKE '[%'`. This leaves ~267 real players.

## Data Sources (spacemolt-knowledge.db)

| Table | Use |
|-------|-----|
| `factions` | faction metadata: name, tag, leader, treasury, owned_bases, description, charter, emblem, primary_color, secondary_color, founded_utc |
| `seen_players` | primary roster + player identity: username, faction_id, faction_tag, clan_tag, colors, status_message, first_seen_utc, last_seen_utc |
| `faction_members` | official overlay (when present): role, joined_utc, is_online |
| `faction_bases` | faction bases: base_name, system_name, services_json |
| `faction_relations` | allies/wars/NAP: kind, target_name, target_tag, reason, our_kills, their_kills |
| `faction_facilities` | facilities: facility_type, category, level, status |
| `seen_player_ships` | ship classes seen per player, with first/last seen |
| `seen_player_sightings` | where/when seen: system_id, poi_id, ship_class, in_combat, last_seen_utc |

Join key for membership: `seen_players.faction_id = factions.faction_id`.

## Architecture

One new generator, matching the existing per-domain generator pattern
(`generate-items-kb`, `generate-galaxy-map`). It loads `seen_players`/ships/sightings
once, builds cross-reference maps in memory, and writes both output trees in a single
pass (faction pages list members → player pages; player pages link back to factions).

```
cmd/generate-factions-kb/
  main.go      # flags, DB open, orchestration, output writing
  load.go      # query factions + roster + bases/relations/facilities
  players.go   # build per-player records from seen_players + ships + sightings
  slug.go      # slugify + collision handling (+ slug_test.go)
  render.go    # html/template definitions + helpers
  *_test.go    # unit tests for slug + relative-time + sighting-grouping helpers
```

- **Input DB**: default `../spacemolt-knowledge.db`, overridable by first positional arg
  (same convention as `generate-items-kb`).
- **Output**: writes `kb/factions/` and `kb/players/` trees, plus `factions.css` and
  `players.css`. Reuses shared `smui.css` and the standard header/nav block with theme
  toggle.
- **Nav edit**: add **Factions** and **Players** links to the nav. The nav block is
  duplicated across each generator's templates; this design only adds the two links to
  the factions/players generator's own templates. (Updating other sections' nav is a
  follow-up; see Out of Scope.)
- **Determinism**: a single generation timestamp is passed in once and used for all
  relative-time formatting, so repeated runs over unchanged data produce identical
  output.

## Pages

### 1. Faction index — `kb/factions/index.html`
Card grid (like the items category index). One card per faction, sorted by
reconstructed member count (desc). Card accent border uses `primary_color`. Card shows:
name, `[tag]`, member count ("N members" = distinct seen players), bases, treasury.
Page intro includes the "rosters reconstructed from sightings; API reports 0 to
outsiders" note.

### 2. Faction detail — `kb/factions/<tag-slug>/index.html`
- **Banner**: name, `[tag]`, `primary_color` accent, founded date.
- **Charter/description** text.
- **Stat strip**: treasury, bases, leader, member count. Inline note that official API
  `member_count` is 0 (hidden from outsiders).
- **Members (N)** table — `Username · Role · Ships seen · Last seen`. Username links to
  the player page. `Role` / online dot come from `faction_members` overlay when present,
  else blank. Sorted online-then-last-seen.
- **Bases** table — `Name · System · Services`. Omitted if none.
- **Relations** table — `Kind · Faction · Reason · Kills (us/them)`. Omitted if none.
- **Facilities** table — `Type · Category · Level · Status`. Omitted if none.

Sections with zero rows are omitted entirely.

### 3. Player index — `kb/players/index.html`
Static table of all ~267 real players, default sort last seen (desc):
`Username · Faction · Ships seen · Last seen`. Username links to the player page;
faction tag links to the faction page ("—" if unaffiliated).

### 4. Player detail — `kb/players/<username-slug>-<id8>/index.html`
- **Header**: username, faction `[tag]` link, `primary_color` accent, status message,
  clan tag.
- **Identity strip**: `First seen · Last seen` (no counts).
- **Ships seen** — list of `<ship class> — first <date>, last <relative>` (no ×N counts).
- **Activity (where seen)** table — `System · POI · Ship · Combat · Last seen` (no count
  column). Rows from `seen_player_sightings` grouped by system+poi+ship, keeping the
  latest `last_seen_utc`. System name links to `systems/<system_id>/` when that page
  exists, else plain text.

## Slugs

- **Faction**: `slugify(tag)` (e.g. `STRG` → `strg`). On collision, append a short
  `faction_id` suffix.
- **Player**: `slugify(username)-<first 8 hex of player_id>` — guarantees uniqueness and
  a stable URL even if the display username changes.
- `slugify` lowercases, replaces non-alphanumeric runs with `-`, trims leading/trailing
  `-`, and handles empty results (fall back to id). Covered by `slug_test.go`.

## Helpers & Cross-links

- **Relative time**: `last_seen_utc` → "2h ago / 1d ago / 5d ago", computed against the
  passed-in generation timestamp. Absolute UTC shown as `title=` hover. Unit tested.
- **Sighting grouping**: group `seen_player_sightings` rows by (system, poi, ship),
  keep latest `last_seen_utc` and OR the `in_combat` flag. Unit tested.
- **System link resolution**: build the set of existing system slugs first; render a
  link only when the sighting's `system_id` is in that set, else plain text.
- **Member count**: distinct `seen_players.player_id` per `faction_id`.

## Error Handling

- Missing/empty optional fields (charter, colors, leader, status message) render as
  blank or "—"; never error.
- Malformed `services_json` / `details_json` is tolerated (rendered as raw text or
  skipped) rather than aborting the run.
- A faction with zero reconstructed members still renders a page (empty roster note).
- DB open / required-table failures abort with a clear error (consistent with existing
  generators).

## Testing

Unit tests (following the `cmd/generate-items-kb/*_test.go` convention):
- `slug.go`: collision handling, weird/empty usernames, unicode.
- relative-time helper: boundaries (minutes/hours/days), future timestamps.
- sighting-grouping helper: dedupe + latest-wins + combat-flag OR.

Manual verification: run the generator against `spacemolt-knowledge.db`, confirm
`kb/factions/index.html`, a faction detail page, `kb/players/index.html`, and a player
detail page render and cross-link correctly; `go build ./...`, `go test ./...`, and
`golangci-lint` clean.

## Out of Scope (follow-ups)

- Updating the shared nav block in *other* section generators to include Factions/Players
  (those pages regenerate independently; nav parity is a separate sweep).
- Time-series / historical activity charts.
- Combat/kill leaderboards.
- Pages for NPC/station entities filtered out of `seen_players`.
