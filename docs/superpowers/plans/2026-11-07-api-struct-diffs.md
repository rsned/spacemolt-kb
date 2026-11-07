# API Struct Diffs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `kb/api-diffs/` section that shows, for each OpenAPI snapshot step, what changed in the API's structs, rendered as diff-highlighted Go or TypeScript.

**Architecture:** `pkg/apidiff` turns each spec's `components.schemas` into a normalised struct model, then:
1. Matches structs across adjacent snapshots, including renames and inline-to-named moves.
2. Compares the matched pairs into typed changes.
3. Renders each changed struct as Go and TypeScript lines with add, remove and fold markers.
4. Writes one report page per step plus an index.

`cmd/generate-api-diffs` rebuilds the whole section on every run.

**Tech Stack:** Go 1.24+, standard library only (`html/template`, `encoding/json`, `compress/gzip`), `internal/kbnav` for the header. No new modules, because depguard would reject them.

**Spec:** `docs/superpowers/specs/2026-11-07-api-struct-diffs-design.md`.

The prototype `docs/superpowers/specs/2026-11-07-api-struct-diffs-prototype.py` is the behaviour reference for rendering:
- type mapping
- Go name initialisms
- sentence and word diff with 6-word trimming
- folding
- CSS
- the language toggle

## Global Constraints

- Inputs: `../spacemolt/server_docs` from the KB root.
  - Read `openapi.YYYYMMDD.json` and `archive/openapi.YYYYMMDD.json.gz`.
  - Exclude `openapi.json`, `openapi.v2.json`, every `openapi.v2.*` file and every other non-matching name.
- History starts at the 2026-02-17 spec. No reports for steps ending on or before 2026-02-17.
- Only `components.schemas` is compared. Paths are out of scope.
- Steps with nothing to report get no page and no index row.
- Prev and next links skip missing reports.
- Each run re-renders everything, deletes stale `YYYY-MM-DD.html` files in the output directory, and is byte-identical across runs. No `time.Now()` in output.
- Pages use `kbnav.Header("../")`, `../smui.css`, and the theme script from `pkg/resourcediff/report.go`.
- Colours use page-local tokens defined for light and `html.dark`, built on `hsl(var(--x))`, because KB colour variables are bare HSL triplets.
- Copy rules:
  - Summary format: `5 additions, 3 removals, 2 changes, 12 comments across 8 structs.`, with zero counts dropped.
  - Tags: `+N field(s)`, `−N field(s)`, `+N/−N enum values`, `+N/−N members`, `N retyped`, `N description(s)`, `New type · N fields`, `Removed type`.
- Every report says the structs are generated from the spec and may differ from `serverapi`.
- Tests never read `server_docs`. Fixtures live in `pkg/apidiff/testdata/`.
- Binaries go in `bin/`. Run `golangci-lint run` with no new findings, plus `go build ./...` and `go test ./...`, before each commit.

## Review Focus

1. **Hostile spec text:** a description containing `<script>`, backticks, `*/` or `&` must render as escaped text, and must not break the Go tag or the TypeScript comment. Tested in Task 5.
2. **Odd property names:** `ally-battle`, `2fa_code` and `x` must produce valid-looking Go names (`AllyBattle`, `F2faCode`, `X`), and the JSON name must stay intact in tags and the TypeScript form. Tested in Task 5.
3. **Unreadable snapshot:** a corrupt `.gz` or invalid JSON should fail the run with an error naming the file. No partial output. Tested in Task 1.
4. **Non-string enums:** integer or boolean enum values, `[1, 2]` or `[true]`, render as literals (`1`, `true`), with no crash. Tested in Task 3.
5. **Struct that is both:** a schema with both `properties` and `oneOf`/`anyOf` is reported as a struct with members, losing neither. Tested in Task 2.

---

## File Structure

| File | Responsibility |
|---|---|
| `pkg/apidiff/load.go` | Find, order and decode snapshots |
| `pkg/apidiff/model.go` | Schema JSON to `Struct` and `TypeRef`, normalisation, nested and variant naming |
| `pkg/apidiff/match.go` | Pair old and new structs: name, then near-match |
| `pkg/apidiff/compare.go` | Pairs to `StructDiff`s and the step summary |
| `pkg/apidiff/words.go` | Token LCS, sentence diff, trimming |
| `pkg/apidiff/lines.go` | `StructDiff` to Go and TypeScript `Line`s, with folding |
| `pkg/apidiff/page.go` | Report and index HTML, templates, CSS |
| `pkg/apidiff/testdata/` | Small hand-written specs, plain and gzipped |
| `cmd/generate-api-diffs/main.go` | Flags, the run loop, file writes and stale-file cleanup |

Also touched: `internal/kbnav/kbnav.go`, `kb/nav.js`, `pkg/gamediff/report.go`, `kb/diffs/index.html`, `README.md`, and the generated `kb/api-diffs/`.

Set up before Task 1: create branch `api-diffs` from `main`. Commit the design, the prototype and this plan as the first commit, using `docs(api-diffs): design, prototype and plan`.

---

### Task 1: Load snapshots

**Files:**
- Create: `pkg/apidiff/load.go`
- Create: `pkg/apidiff/load_test.go`
- Create: `pkg/apidiff/testdata/docs/`

**Interfaces:**
- Produces:
  - `type Snapshot struct { Date string; Schemas map[string]any }`. `Date` is `YYYYMMDD`.
  - `func LoadSnapshots(dir string) ([]Snapshot, error)` returns snapshots sorted by date ascending.
  - `func ISODate(d string) string` turns `20261107` into `2026-11-07`.

- [ ] **Step 1: Write the fixture directory.** It needs:
  - `openapi.20260301.json`
  - `archive/openapi.20260215.json.gz`
  - Decoys: `openapi.json`, `openapi.v2.20260301.json`, `archive/openapi.v2.20260215.json.gz`, `.openapi.20260301.json.swp`
  
  Each fixture is `{"components":{"schemas":{"A":{"type":"object","properties":{"x":{"type":"string"}}}}}}`.
- [ ] **Step 2: Write the failing tests.**
  - `TestLoadSnapshotsFindsDatedFilesOnly` asserts dates `[20260215, 20260301]`, and that `Schemas["A"]` is present in both.
  - `TestLoadSnapshotsBadFileNamesIt` writes a corrupt `.gz` in a `t.TempDir()`, then asserts that the error is non-nil and its message contains the filename.
  - `TestISODate` asserts `20261107` gives `2026-11-07`.
- [ ] **Step 3: Run the tests.**
  - Run: `go test ./pkg/apidiff/ -run 'TestLoad|TestISODate'`
  - Expected: FAIL, with an undefined `LoadSnapshots`.
- [ ] **Step 4: Implement `LoadSnapshots` and `ISODate`.**
  - Match file names with `^openapi\.(\d{8})\.json(\.gz)?$` in `dir` and `dir/archive`.
  - Return an error if the same date appears twice.
  - Decode with `json.Decoder.UseNumber()` so integer enums keep their form.
- [ ] **Step 5: Run the tests.** Same command. Expected: PASS.
- [ ] **Step 6: Commit** with `feat(apidiff): load dated OpenAPI snapshots`.

### Task 2: Struct model

**Files:**
- Create: `pkg/apidiff/model.go`
- Create: `pkg/apidiff/model_test.go`

**Interfaces:**
- Consumes: `Snapshot.Schemas`.
- Produces:

```go
type TypeKind int // KindScalar, KindArray, KindMap, KindRef, KindNested, KindAny
type TypeRef struct {
	Kind     TypeKind
	Scalar   string   // "integer" | "number" | "boolean" | "string"
	Elem     *TypeRef // KindArray, KindMap
	Ref      string   // KindRef: schema name; KindNested: the nested Struct's ID
	Nullable bool
}
type Field struct {
	Name     string
	Type     TypeRef
	Required bool
	Desc     string
	Enum     []string // literal form: strings quoted with %q, numbers/bools bare
}
type Struct struct {
	ID      string   // unique key: "Name", "Parent.field", "Parent.field[]", "Parent#v<idx>"
	Display string   // card title
	Desc    string
	Fields  []Field  // spec order
	Members []string // oneOf/anyOf member IDs, in spec order
}
func BuildModel(schemas map[string]any) map[string]*Struct
```

- [ ] **Step 1: Write the failing tests.** Use inline JSON literals:
  - `TestBuildModelNullable`:
    - 3.0.3 `{"type":"string","nullable":true}` and 3.1 `{"type":["string","null"]}` both give `Scalar string, Nullable true`.
    - `{"type":["array","null"],"items":{"type":"integer"}}` gives an array of integer, nullable.
  - `TestBuildModelBoolSchema`: `"x": true` gives `KindAny`, with no panic.
  - `TestBuildModelNested`: an inline object at `P.properties.rewards` gives a struct with ID `P.rewards`, and the field type is `KindNested` with `Ref "P.rewards"`. An inline object under `items` gives ID `P.list[]`.
  - `TestBuildModelVariants`:
    - `oneOf: [{$ref: A}, {inline props with action enum ["list"]}]` gives `Members ["A", "P#v1"]`.
    - `P#v1` has Display `P (list)`.
    - Without an action, Display is `P (variant 1)`.
  - `TestBuildModelPropsAndUnion` (Review Focus 5): a schema with `properties` and `anyOf` keeps both `Fields` and `Members`.
  - `TestBuildModelEnumLiterals`: `["a"]` gives `["\"a\""]`, and `[1, true]` gives `["1", "true"]`.
- [ ] **Step 2: Run the tests.**
  - Run: `go test ./pkg/apidiff/ -run TestBuildModel`
  - Expected: FAIL.
- [ ] **Step 3: Implement `BuildModel`.**
  - Recursive walk.
  - `allOf` members merge their properties into the owner.
  - A schema with neither properties nor members, such as a map-only schema, produces no `Struct`.
  - `Required` comes from the owning object's `required` list.
- [ ] **Step 4: Run the tests.** Same command. Expected: PASS.
- [ ] **Step 5: Commit** with `feat(apidiff): normalised struct model from schemas`.

### Task 3: Match and compare

**Files:**
- Create: `pkg/apidiff/match.go`
- Create: `pkg/apidiff/compare.go`
- Create: `pkg/apidiff/compare_test.go`

**Interfaces:**
- Consumes: `BuildModel`.
- Produces:

```go
type Pair struct{ Old, New *Struct } // nil Old = new struct, nil New = removed
func Match(old, new map[string]*Struct) []Pair // sorted by New.ID, else Old.ID

type FieldChange struct {
	Name            string
	Old, New        *Field // nil = added / removed
	Retyped, Reword bool
	EnumAdded, EnumRemoved []string
}
type StructDiff struct {
	Pair
	Fields                       []FieldChange // every field, unchanged ones included, in render order
	MembersAdded, MembersRemoved []string
	DescChanged                  bool
}
func (d StructDiff) Structural() bool // anything but description changes
type Counts struct{ Additions, Removals, Changes, Comments, Structs int }
type Step struct {
	From, To string // YYYYMMDD
	Diffs    []StructDiff // structural first, then wording; each group sorted by display name
	Counts   Counts
}
func CompareStep(from, to Snapshot) Step
func (c Counts) Summary() string // copy per Global Constraints; "" when Structs == 0
```

**Matching rules.** This algorithm isn't fixed by the spec, so it is stated here:

1. Same ID matches.
2. Each remaining new struct is compared with each remaining old struct by Jaccard similarity of field-name sets.
3. Candidate pairs with a score of 0.8 or more are taken greedily: highest score first, ties broken by `New.ID` then `Old.ID`.
4. Anything left over is new or removed.

**Type equality.** A nested type and a ref are equal when Match paired their structs: `KindNested Ref X` against `KindRef Ref Y`, where X was matched to Y. Compare types through a resolver built from the match result.

**Counting.**
- New struct: 1 addition.
- Removed struct: 1 removal.
- Each field added or removed counts once.
- Each enum value added or removed counts once.
- Each member added or removed counts once, unless that member is itself a new or removed struct in this step. Such a member shows on the union card but isn't counted.
- Retyped: 1 change.
- Reworded field or struct description: 1 comment.
- `Structs` is the number of `StructDiff`s with any change.

- [ ] **Step 1: Write the failing tests.** Each builds two small schema maps:
  - `TestCompareFieldAddRemove`: adding `flagship_bounty` gives `Counts{Additions: 1, Structs: 1}`, and the summary is `1 addition across 1 struct.`
  - `TestCompareEnumRemoved`: removing 4 enum values gives `Removals 4`, with `EnumRemoved` in spec order.
  - `TestCompareEnumNonString` (Review Focus 4): changing `[1, 2]` to `[1, 3]` gives `EnumAdded ["3"]` and `EnumRemoved ["2"]`.
  - `TestCompareRetypeAndRequired`: a change of type, or a change between required and optional, gives `Retyped` and `Changes 1`.
  - `TestCompareInlineToRefNotRetype`: the old field is inline `{a,b}`, and the new field is a `$ref` to `B{a,b}`. Expect no `FieldChange` with `Retyped`, and `B` is not reported as a new struct.
  - `TestCompareRenameNearMatch`: `Old X{a,b,c,d,e}` against `New Y{a,b,c,d,e,f}` gives one pair with a field addition, not a new plus a removed struct.
  - `TestCompareVariantsByFieldsNotPosition`: two variants swap positions, which gives no diffs.
  - `TestCompareUnionMembers`:
    - Existing struct `C` joining a union gives `Additions 1`.
    - New struct `D` joining gives `Additions 1` in total, because it counts only as a new struct.
  - `TestCompareOrdering`: structural diffs come before wording-only ones, each group alphabetical.
- [ ] **Step 2: Run the tests.**
  - Run: `go test ./pkg/apidiff/ -run TestCompare`
  - Expected: FAIL.
- [ ] **Step 3: Implement `Match`, `CompareStep` and `Summary`.**
  - Field render order: new-snapshot order, with each removed field inserted after its old predecessor.
  - Pluralise per count: `1 addition`, `2 additions`, `1 struct`.
- [ ] **Step 4: Run the tests.** Same command. Expected: PASS.
- [ ] **Step 5: Commit** with `feat(apidiff): match structs across snapshots and compare`.

### Task 4: Word and sentence diff

**Files:**
- Create: `pkg/apidiff/words.go`
- Create: `pkg/apidiff/words_test.go`

**Interfaces:**
- Produces:

```go
type Token struct{ Text string; Marked bool }
type DescRow struct{ Kind LineKind; Tokens []Token } // Kind: LineDel, LineAdd, LineCtx (a "…" row)
func DescDiff(old, new string) []DescRow
```

`LineKind` is defined in Task 5. Declare it in `words.go` as `type LineKind int` with `LineCtx`, `LineAdd`, `LineDel`, `LineHdr` and `LineFold`.

- [ ] **Step 1: Write the failing tests.**
  - `TestDescDiffWordMark`: `An owner-written description …` against `A per-facility description …` gives a del row with `An` and `owner-written` marked, and an add row with `A` and `per-facility` marked.
  - `TestDescDiffUnchangedSentencesCollapse`: when only the second of three sentences changes, the rows are ctx, del, add, ctx.
  - `TestDescDiffTrim`: a 40-word sentence with one word changed in the middle keeps 6 words each side, with `…` tokens at both ends.
  - `TestDescDiffLowSimilarity`: totally different sentences, with similarity below 0.35, give no marks.
- [ ] **Step 2: Run the tests.**
  - Run: `go test ./pkg/apidiff/ -run TestDescDiff`
  - Expected: FAIL.
- [ ] **Step 3: Implement it.**
  - Split sentences after `.`, `!`, `?`, `;` or `:` followed by whitespace.
  - Use an LCS over sentences, then over words.
  - Similarity is `2*LCS/(len(a)+len(b))`.
  - Wrapping to 84 columns is Task 5's job.
- [ ] **Step 4: Run the tests.** Same command. Expected: PASS.
- [ ] **Step 5: Commit** with `feat(apidiff): sentence and word-level description diff`.

### Task 5: Go and TypeScript lines

**Files:**
- Create: `pkg/apidiff/lines.go`
- Create: `pkg/apidiff/lines_test.go`

**Interfaces:**
- Consumes: `StructDiff`, `DescDiff`, `LineKind`.
- Produces:

```go
type Lang string // "go" | "ts"
type Line struct {
	Kind  LineKind
	HTML  htmltpl.HTML // escaped, with <span class="k|t|n|s|c"> and <mark> markup
	Folded []Line      // LineFold only
	Label string       // LineFold only: "N unchanged fields" / "N unchanged values" / "N unchanged members"
}
func StructLines(d StructDiff, lang Lang) []Line // already folded
func GoName(json string) string
```

Rendering rules, from the spec's Rendering section. The prototype shows the exact output.

**Go**
- `integer` → `int`, `number` → `float64`, `boolean` → `bool`, `string` → `string`.
- Arrays → `[]T`. Maps → `map[string]T`, or `map[string]any`.
- Refs and nested types → their type name. Optional refs → `*T`.
- Enum fields → `<Struct><Field>`, with a `const` block after the struct.
- Unions → a `// X is one of:` comment block.
- Column widths are computed across both snapshots.

**TypeScript**
- `?` on optional fields, `| null` on nullable ones.
- Enums → `export type <Struct><Field> =` followed by `| value` lines.
- Unions → `export type X =` followed by `| Member` lines.

**Both**
- Nested type names: `<Parent><Field>` from `GoName`.
- Comments only on added, removed or changed fields, wrapped at 84.
- 2 context lines around changes. Folds only when 3 or more lines.

- [ ] **Step 1: Write the failing tests.**
  - `TestGoName` (Review Focus 2): `ally-battle` → `AllyBattle`, `2fa_code` → `F2faCode`, `x` → `X`, `base_id` → `BaseID`, `xp_gained` → `XPGained`.
  - `TestStructLinesGoAdd`: an added optional int `flagship_bounty` gives a `LineAdd` whose text, with tags stripped, is `FlagshipBounty int `json:"flagship_bounty,omitempty"`` after padding is trimmed.
  - `TestStructLinesTSNullable`: gives `contraband_items?: string[] | null;`.
  - `TestStructLinesEnumBlock`: a removed value gives a `LineDel` holding `"boss_flagship_copy"`, and the fold label is `N unchanged values`.
  - `TestStructLinesFold`: 20 unchanged fields around one change give 1 fold before it labelled `17 unchanged fields`, and 2 context lines on each side.
  - `TestStructLinesNoFalseDiffFromAlignment`: a new field with a long name causes no `LineDel` or `LineAdd` on the other fields.
  - `TestStructLinesEscaping` (Review Focus 1): a description of `<script>x</script> */ `tag` & co` appears escaped in the HTML, with no raw `<script` anywhere.
  - `TestStructLinesUnion`: in TypeScript, `| D` is a `LineAdd`.
- [ ] **Step 2: Run the tests.**
  - Run: `go test ./pkg/apidiff/ -run 'TestGoName|TestStructLines'`
  - Expected: FAIL.
- [ ] **Step 3: Implement `StructLines` and `GoName`.**
  - `GoName` upper-cases these initialisms: `id`, `url`, `xp`, `api`, `poi`, `hp`, `npc`.
  - A name starting with a digit gets an `F` prefix.
- [ ] **Step 4: Run the tests.** Same command. Expected: PASS.
- [ ] **Step 5: Commit** with `feat(apidiff): Go and TypeScript diff lines`.

### Task 6: Pages and generator

**Files:**
- Create: `pkg/apidiff/page.go`
- Create: `pkg/apidiff/page_test.go`
- Create: `cmd/generate-api-diffs/main.go`

**Interfaces:**
- Consumes: `LoadSnapshots`, `CompareStep`, `StructLines`, `ISODate`.
- Produces:
  - `func RenderReport(s Step, prev, next string) (string, error)`. `prev` and `next` are ISO dates, or `""`.
  - `func RenderIndex(steps []Step) (string, error)`. `steps` is newest first, and only steps with changes are passed.
  - `func Steps(snaps []Snapshot, baseline string) []Step`. It returns reportable steps, oldest first, with `To > baseline` and `Counts.Structs > 0`.
  - `const Baseline = "20260217"`.
- CLI: `generate-api-diffs [-docs ../spacemolt/server_docs] [-output kb/api-diffs]`.

Report layout, top to bottom, as in the spec:
1. Previous / All Reports / next links.
2. `<h2>API Changes — <long date> vs <long date></h2>`.
3. The summary line.
4. The "generated from the spec" note.
5. Sticky bar with the `Go`/`TypeScript` buttons, `aria-pressed`, and the legend.
6. The struct list, as anchor links plus tags.
7. Cards in two groups, "Structural changes" and "Wording only", with headings shown only for groups that have cards.

The index has an intro saying the pages are built from the spec, with a link to `../diffs/index.html`, then a date and summary table, newest first. Its last line reads: `History starts at 2026-02-17, when the spec first documented its schemas.`

The CSS and toggle script come from the prototype template. Swap its standalone tokens for page-local tokens defined in `:root` and redefined in `html.dark`. Keep the same light red and green and the same dark variants.

- [ ] **Step 1: Write the failing tests.**
  - `TestStepsSkipsBaselineAndEmpty`: fixture snapshots dated 0210, 0217, 0301 and 0302, where 0302 is identical to 0301. Only the step 0217→0301 is returned.
  - `TestRenderReportHeaderAndNav`: the output contains `kbnav.Header("../")`, `href="2026-02-01.html"` for prev, no next link when `next == ""`, and `aria-pressed="true"` on the Go button.
  - `TestRenderReportGroups`: the "Structural changes" heading comes before "Wording only", and the struct list has one `href="#` link per card.
  - `TestRenderIndex`: the rows are newest first, the summary text matches `Counts.Summary()`, and the history line is present.
- [ ] **Step 2: Run the tests.**
  - Run: `go test ./pkg/apidiff/ -run 'TestSteps|TestRender'`
  - Expected: FAIL.
- [ ] **Step 3: Implement `page.go` and `main.go`.**
  - `main` loads snapshots and computes steps.
  - It writes `ISODate(To).html` for each step, with prev and next taken from the neighbouring reportable steps, and writes `index.html`.
  - It then deletes any `????-??-??.html` in the output directory that wasn't written.
  - It logs a single line: `api-diffs: N reports from M snapshots → <dir>`.
- [ ] **Step 4: Run the tests.** Same command. Expected: PASS.
- [ ] **Step 5: Run the generator twice and check determinism.**
  - Run `go build -o bin/generate-api-diffs ./cmd/generate-api-diffs && bin/generate-api-diffs && sha256sum kb/api-diffs/* > /tmp/a && bin/generate-api-diffs && sha256sum kb/api-diffs/* | diff - /tmp/a`.
  - Expected: no diff output.
- [ ] **Step 6: Commit** the code only, with `feat(api-diffs): report pages and generator`. The generated pages go in Task 7.

### Task 7: Site integration, backfill and acceptance

**Files:**
- Modify: `internal/kbnav/kbnav.go` (`Items`)
- Modify: `kb/nav.js` (generated)
- Modify: `pkg/gamediff/report.go` (index template)
- Modify: `kb/diffs/index.html`
- Modify: `README.md`
- Create: `kb/api-diffs/` (generated)

- [ ] **Step 1: Add the nav entry.**
  - Add `{"api-diffs", "API Changes"}` to `kbnav.Items`, after `{"diffs", "Change Log"}`.
  - Run `go run ./cmd/generate-nav`.
- [ ] **Step 2: Add the link from the catalog diffs.**
  - In `pkg/gamediff/report.go`'s index template, after the resource change log line, add: `<p class="text-muted">Changes to the API's response structures are in the <a href="../api-diffs/index.html">API change log</a>.</p>`.
  - Splice the same line into the committed `kb/diffs/index.html`, which is not regenerated.
- [ ] **Step 3: Update the README.**
  - Add a `generate-api-diffs` row to the generator table: what it does, plus the `-docs` and `-output` flags.
  - Add `api-diffs/` to the `kb/` tree, under `diffs/`.
  - Add `go run ./cmd/generate-api-diffs` to the regen list, after `generate-diffs`.
- [ ] **Step 4: Backfill.** Run `go run ./cmd/generate-api-diffs`. Expect one log line with the report count.
- [ ] **Step 5: Run the acceptance checks against the generated pages.** Record each result in the commit message.
  - **Totals:** report count is 66 or fewer. 92 steps, less 20 identical and 6 before the baseline, leaves 66. Explain any shortfall, such as steps whose only changes are to map-only schemas.
  - **07-19:** `kb/api-diffs/2026-07-19.html` has no added field on any `FacilityResponse` variant, and fewer than 50 cards. If it has more, tune the 0.8 threshold or explain the figure with examples. Don't just raise the limit.
  - **11-06:** has `flagship_bounty` on `EmpireGiftResponse`, `ally_battle_opt_out` on `FactionInfoResponse` and `FactionEditUpdates`, and a removed `discord_policy`.
  - **11-07:** has 4 removed `GiftGoodwill` enum values.
  - **Nested:** `2026-03-22.html` shows `pirate_rep` inside `rewards`, and `2026-05-30.html` shows `towing_wreck_id`.
  - **Unions:** at least one report after 07-19 has a union card with an added member.
- [ ] **Step 6: Check in the browser.**
  - Open `kb/api-diffs/2026-11-06.html` in headless Chrome, as in earlier KB work.
  - Check light and dark mode at 1100px and at 420px.
  - Click the TypeScript toggle and a fold.
  - Check that nothing overflows the page body, and that code blocks scroll inside their own container.
- [ ] **Step 7: Run the full checks.**
  - Run: `go build ./... && go test ./... && golangci-lint run`
  - Expected: all pass, 0 new lint issues. `TestGeneratedPagesMatchNav` covers `kb/api-diffs/`.
- [ ] **Step 8: Commit.**

```bash
git add internal/kbnav/kbnav.go kb/nav.js pkg/gamediff/report.go kb/diffs/index.html README.md kb/api-diffs
git commit -m "feat(kb): API change log section, backfilled from 2026-02-17"
```

Merging and pushing wait for the owner.
