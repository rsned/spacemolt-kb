# API Struct Diffs: design

Date: 2026-11-07. Status: awaiting owner approval. Builder check done.

## Intent

The KB already shows what changed in the game's catalogs (`kb/diffs/`). This
adds the same thing for the server's API: when a new `openapi.json` lands, the
owner wants to see what changed in the response structures, in a form that reads
like a code review of the structs.

The main question the section answers is "what changed in the latest update?"
History of one response is a side benefit, not the job.

Why it matters: real mechanics show up here first. The 11-06 spec documented
that a `trade_authenticator` market on a player station goes offline when
faction storage can't cover its upkeep. The catalog diff showed only that a
description changed.

## Decided by the owner

- A landing page similar to the catalog diffs: one row per snapshot, newest
  first, with a one-line summary such as "5 additions, 3 removals, 12 comments".
- A details page per snapshot using the approved sample's card view.
- A list of every changed struct at the top of each details page.
- The look of the approved sample (https://claude.ai/artifact/Nivq39Xhe9QZmaYTJLkrFE):
  - Go / TypeScript toggle.
  - Light red background on removed lines, light green on added lines.
  - Word-level highlight inside changed descriptions.
  - Unchanged fields folded into a clickable row.
  - Summary tags on each card.
- Backfilled across the full snapshot history.
- A spec-only first version. "Do we decode it" and "have we received it" are
  later work.

The owner approved these calls when they were played back as guesses:

- Section at `kb/api-diffs/`, with an "API Changes" entry in the nav drawer and
  the shared site header.
- Previous/next snapshot links on each report, as the catalog diffs have.
- "Comments" in the summary means description changes.
- Removed enum values count as removals and also get their own tag.
- A new Go generator that runs in the normal KB refresh.

## Goals

1. `kb/api-diffs/index.html` lists every snapshot step that has changes, newest
   first, each with a one-line summary.
2. `kb/api-diffs/YYYY-MM-DD.html` shows every changed struct for that step.
3. Output is reproducible: two runs on the same inputs give identical bytes.
4. Adding a new snapshot takes one command with no arguments.

## Non-goals

- Decoded-by-client or received-in-practice columns.
- Per-struct history pages, or search across snapshots.
- Endpoint-level changes: added or removed paths, request parameters and
  operation descriptions. Only `components.schemas` is compared. Note this
  includes every named schema, so request types such as `FactionEditUpdates`
  and the `Notification_*` types appear alongside responses.
- The v2 spec (`openapi.v2.*`). It is a separate API, with far fewer snapshots.
- Compilable output. The Go and TypeScript are a reading format.

## Anti-goals

These would make it a failure even if it technically works:

- **Refactors reported as change.** The 07-19 step split the spec from 265 to
  615 schemas. If that report claims hundreds of new structs, the history can't
  be trusted.
- **Wording noise burying structure.** Field, type and enum changes must be
  easier to find than description edits.
- **Generated code passed off as the client's.** Pages must say the structs are
  built from the spec, and may differ from `serverapi`.

## Inputs

- Directory: `../spacemolt/server_docs` from the KB root (flag `-docs`), which
  resolves to `/home/robert/spacemolt/spacemolt/server_docs`.
- 93 main specs: 38 `openapi.YYYYMMDD.json` plus 55
  `archive/openapi.YYYYMMDD.json.gz`, running 2026-02-10 to 2026-11-07. No date
  appears in both places.
- Exclude `openapi.json` and `openapi.v2.json` (symlinks), all `openapi.v2.*`
  files, and editor swap files.
- `info.version` is always `1.0.0`, so the snapshot date is the only identifier.
- 20 steps have byte-identical `components.schemas` once key order is ignored.

The specs live in the game client repo and are not copied into the KB. Reports
are the committed output, so CI and the Pages deploy never need the specs.

The history starts at the 2026-02-17 spec. Earlier specs keep almost everything
inline in `paths`, which is out of scope, and only 1 of the 151 schemas added that
day matches anything there, so that step would claim 151 new structs. No reports
are produced for steps up to and including 02-16→02-17. The landing page's last
line says the history starts at 2026-02-17, when the spec first documented its
schemas. (Builder's call.)

### Shape changes in the history

The comparison has to absorb these so they don't appear as changes:

| Date | What happened |
|---|---|
| 2026-02-10 to 02-16 | Only 4 schemas, with the rest inline in `paths`. |
| 2026-02-17 | Schemas jump from 4 to 155. This is the baseline: see below. |
| Through 2026-07-14 | OpenAPI 3.0.3. Up to 24 schemas are a `oneOf` of unnamed inline variants. `FacilityResponse.oneOf[5]`, for example, has no name, and variant positions shift between snapshots (5 → 7 → 5, 16 → 18 → 16 → 17). |
| 2026-07-19 | Move to OpenAPI 3.1.0. Inline variants become named schemas, so `FacilityResponse` becomes a pure `oneOf` of `$ref`s. |
| From 2026-07-19 | Nullable may be written as `type: ["array","null"]`. Five properties are bare `true`/`false` schemas rather than objects. |

## Comparison

Compare two adjacent snapshots at the level of a **struct**: a named schema with
properties, an inline `oneOf`/`anyOf` object variant, or an inline object nested
under a property or `items`. The walker is recursive. (Builder's call on nested
objects: real changes live only there, such as `rewards.pirate_rep` on 03-22 and
`V2GameState.player.towing_wreck_id` on 05-30, and 18 nested objects remain in
the 11-07 spec. Treating them as structs also accounts for most of the 07-19
refactor.)

A nested object gets its own card titled `Parent.field`, or `Parent.field[]` for
array items. In the parent, its field type shows as `<Parent><Field>` in both
languages. A nested object stops being a separate struct once it becomes a named
schema, and the two are matched as described below.

Each struct is reported where its fields are declared. Do not repeat a change
through every response that reaches it. The 11-06 `maintenance_per_cycle` edit
is one change on `FacilityEntry`, not five on `FacilityResponse`.

Change kinds, in the order they rank:

| Kind | Meaning | Summary word |
|---|---|---|
| New struct | Struct did not exist before | additions |
| Removed struct | Struct no longer exists | removals |
| Field added | | additions |
| Field removed | | removals |
| Field retyped | Type, `$ref`, nullability or required-ness changed | changes |
| Enum value added or removed | Counted per value | additions / removals |
| Union member added or removed | A type joined or left a `oneOf`/`anyOf` list | additions / removals |
| Description changed | Field-level or struct-level | comments |

**Unions.** The owner wants these reported. A schema that is a pure `oneOf` or
`anyOf`, such as `NotificationPayload` or `CraftCommandResponse`, has no fields,
so it gets its own small card listing its members, folded and coloured the same
way. Members are compared by name, never position. Member changes after 07-19
number 10 or more. Before then, unnamed variants are compared as structs and
produce no union entry, and the 07-19 change of variants into `$ref`s is not a
member change.

To avoid double counting, a member that is itself a new struct counts once, as
the new struct. It still appears in the union card, and the summary number
doesn't change. (Builder's call.)

The two schemas that are only a map have no fields or members and aren't
compared. (Builder's call.)

### Matching structs across snapshots

Name is the first match. Then handle refactors, so that a struct that only moved
or got a name is not reported as new:

- A newly named schema whose field set matches a struct that disappeared, or an
  unnamed inline variant, in the previous snapshot is the same struct. Report
  only its real differences. Exact-match threshold is left to the builder, but
  see the acceptance test below.
- Unnamed variants are matched to each other by field set, never by position.
- Unnamed variants display as parent name plus their variant, using the literal
  `action` value where one exists, otherwise the position in that snapshot, for
  example `FacilityResponse (variant 5)`.
- An inline object replaced by a `$ref` to a schema with the same fields is not
  a retype. 137 of the 264 schema names kept across 07-19 differ, mostly for this
  reason.
- Normalise before comparing: 3.0.3 `nullable: true` and 3.1 type lists are the
  same thing, and bare boolean schemas don't crash the walker.

## Rendering

The approved sample defines the look. The prototype script is the reference for
behaviour: copy it to `docs/superpowers/specs/2026-11-07-api-struct-diffs-prototype.py`
with this design, because its scratchpad copy will be deleted.

### Go form

- `type X struct { ... }` with exported names. `id`, `url`, `xp`, `api`, `poi`,
  `hp` and `npc` are upper-cased initialisms.
- `integer` → `int`, `number` → `float64`, `boolean` → `bool`.
- Arrays → `[]T`. `additionalProperties` → `map[string]T`, otherwise
  `map[string]any`.
- Optional fields get `omitempty`. An optional `$ref` is a pointer.
- Enum fields use a named type, `<Struct><Field>`, and changed values render in
  a `const ( ... )` block after the struct.

### TypeScript form

- `export interface X { ... }`, with the JSON field names unchanged.
- Optional fields use `?`. Nullable adds `| null`.
- Changed enums render as `export type <Struct><Field> = | "a" | "b";`.

### Unions

- Go has no union, so a comment block names it, for clarity in the reading
  format: `// FooResponse is one of:` followed by a `//   BarResponse` line per
  member.
- TypeScript: `export type FooResponse =` followed by `| BarResponse` lines.

### Both forms

- Fields keep the newer snapshot's order, with removed fields shown where they
  used to be.
- Compare by field, never by rendered line. Go column widths are computed across
  both snapshots, so alignment can't create a false diff.
- Comments appear only on added, removed or changed fields.
- Comments wrap at 84 characters.
- A changed description shows only the sentences that changed, with `// …` for
  the rest. A long sentence is trimmed to 6 words either side of the change.
  Adjacent changed words merge into one highlight.
- Unchanged fields collapse to a `<details>` row reading "N unchanged fields", or
  "N unchanged values" in an enum block. Two context lines stay around each
  change. Folds of fewer than 3 lines are not collapsed.
- `<`, `>` and `&` in generated text are escaped, as with all spec text.

## Pages

Both use `kbnav.Header("../")`, the site's theme toggle script and `smui.css`.
Colours come from page-local tokens defined for light mode and for `html.dark`.
Check both themes: KB colour variables are bare HSL triplets, so use
`hsl(var(--x))`.

### Landing page: `kb/api-diffs/index.html`

- Table: date, summary. Newest first. Same style as `kb/diffs/index.html`.
- Example summary: `5 additions, 3 removals, 2 changes, 12 comments across 8 structs.`
  Zero counts are dropped.
- Steps with nothing to report get no row and no report.
- A short intro says the pages are built from the spec, and links to the catalog
  change log.

### Report page: `kb/api-diffs/YYYY-MM-DD.html`

Top to bottom:

1. Previous / All Reports / next links, as on `kb/diffs/`.
2. Title with both dates, and the same summary line as the landing page.
3. Sticky bar with the Go / TypeScript toggle and the colour legend. The toggle
   choice persists in `localStorage`, wrapped in try/catch. The page must work
   with the Go form when storage is unavailable.
4. The struct list: each changed struct as an anchor link with its summary tags.
5. Cards, one per struct, in two groups:
   - Structural changes (anything other than descriptions).
   - Wording only.
   Each group is alphabetical. This ordering puts structure first, per the
   anti-goal.

A line in the header area says the structs are generated from the spec and may
differ from `serverapi`.

"Previous" and "next" link to the adjacent report that exists, so a skipped step
doesn't produce a dead link. The title names the two snapshots actually compared,
which are always adjacent dates in the history.

## Code

- `pkg/apidiff`: load specs, build the struct model, compare, render. HTML output
  lives here, as in `pkg/gamediff` and `pkg/resourcediff`.
- `cmd/generate-api-diffs`: flags `-docs` (default `../spacemolt/server_docs`)
  and `-output` (default `kb/api-diffs`). Each run re-renders all reports and the
  index, and removes report files that no longer belong, which is cheap and
  avoids nav patching. This is how `generate-resource-diffs` already works.
- The whole section is rebuilt from the specs, so there's no snapshot state to
  rotate and no ordering rule to follow.

## Site integration

- Add `{"api-diffs", "API Changes"}` to `kbnav.Items` after Change Log and run
  `go run ./cmd/generate-nav`.
- `internal/kbnav/sitenav_test.go` already covers new page trees. Add no
  exemption.
- Add a line on `kb/diffs/index.html` linking to the section, via the template in
  `pkg/gamediff/report.go` and a hand patch to the committed page, as was done
  for the resource change log.
- Add `generate-api-diffs` to the README's generator table and tree.
- The KB refresh chain gains one step, after `generate-diffs`.

## Testing

- Unit tests with small hand-written spec fixtures for each change kind, each
  shape in the history table, and the Go and TypeScript forms.
- No test may read `server_docs`, because CI doesn't have it. Tests skip nothing
  and use fixtures in `pkg/apidiff/testdata/`.
- Acceptance checks, run by hand against the real specs before merging:
  - **07-19 step:** no field of a `FacilityResponse` variant appears as added
    because of the refactor, and the report is under 50 changed structs. This
    number is a sanity check on the matching, to be revisited with evidence.
  - **11-02 → 11-06:** `EmpireGiftResponse` shows `flagship_bounty` added,
    `FactionInfoResponse` and `FactionEditUpdates` show `ally_battle_opt_out`,
    and `Notification_pirate_radio` shows `discord_policy` removed.
  - **11-06 → 11-07:** `GiftGoodwill` shows 4 enum values removed.
  - **Unions:** a step after 07-19 where a real member joined a union, such as
    `NotificationPayload`, shows it on a union card.
  - **Totals:** 92 steps, less 20 with identical schemas, leaves at most 72, less
    the steps before the 02-17 baseline. Any other difference is explained.
  - **Nested:** 03-20 → 03-22 shows `rewards.pirate_rep`, and 05-27 → 05-30 shows
    `V2GameState.player.towing_wreck_id`.
  - Two consecutive runs are byte-identical.
  - Both themes and phone width checked in a real browser, with the toggle and a
    fold clicked.

## Left to the builder

- How word-level diffing is implemented in Go, and whether that needs a
  dependency. Prefer none.
- The exact field-set similarity rule, subject to the 07-19 acceptance check.
  Exact matching alone leaves 73 unmatched new schemas on that step, so expect to
  need a near-match rule, and revisit the "under 50" figure with evidence.
- HTML structure, as long as it matches the sample's look.

## Open follow-ups, not part of this work

- A "do we decode it" column, using `pkg/game/serverapi` tags.
- A "first seen" column from recorded replies such as
  `data/stations/cloverfield/events.jsonl`.
- Added and removed endpoints.
