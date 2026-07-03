# Faction & Player Overlays — Design

**Date:** 2026-06-01
**Status:** Approved (pending spec review)

## Goal

Let people enrich faction and player KB pages with content the game does not
store — a faction **logo** / player **portrait**, a **biography / freeform body**,
and **structured stats** — by editing files in a `overlays/` directory via GitHub
PR. On each regeneration the `generate-factions-kb` generator merges any overlay
content into the matched page. A separate seed tool scaffolds overlay stubs from
the existing `personality.json` agent data.

## Background & constraints

- Faction/player pages are produced by `cmd/generate-factions-kb` from
  `spacemolt-knowledge.db`. The output lives under `kb/factions/<slug>/` and
  `kb/players/<slug>/` and is checked into git.
- `mustResetDir` wipes everything in those output dirs (except the section `.css`)
  on every run, so overlay **source** must live outside the generated tree.
- Player slugs are `username-id8` and drift when a player renames; the
  `player_id` / `faction_id` hashes are stable and are now printed on each page,
  so contributors can copy their ID directly from their page.
- The bot-agent data at `/home/robert/spacemolt/spacemolt/data/agents/*/personality.json`
  has rich fields (biography, organization, role, sub_role, motivations, traits,
  skills) but only ~5 of 112 agent `name`s currently match a player username, so
  it cannot be a reliable live source — it is used only by a one-off seed tool.

## Decisions (from brainstorming)

- **Keying:** overlay dir is named by the stable hash ID (`faction_id` /
  `player_id`).
- **Format:** one `profile.md` per entry — YAML frontmatter + markdown body.
- **personality.json:** consumed by a seed tool that writes committed stub
  overlays; the generator never reads it live.
- **v1 content:** image (logo/portrait), biography/freeform body, structured
  stats. (Links section, SVG, live personality import are out of scope.)

## Directory layout

```
overlays/                          # hand-authored SOURCE, PR-editable, NOT generated
  README.md                        # contributor guide (how to find your ID, file format)
  factions/<faction_id>/
    profile.md
    logo.png                       # png/jpg/jpeg/webp/gif; svg disallowed
  players/<player_id>/
    profile.md
    portrait.jpg
```

- Directory name = `faction_id` or `player_id` (the hash shown on each page).
- An example overlay for one faction is committed so the feature is visible and
  the format is self-documenting.

## `profile.md` schema

```markdown
---
image: logo.png                    # optional; filename in this same dir
image_alt: "Hex Collective emblem" # optional alt text for the image
stats:                             # optional ordered list of {label, value}
  - label: Homeworld
    value: Krynn Prime
  - label: Founded (lore)
    value: 2387 AE
---

## Biography

Free markdown — paragraphs, **bold**, lists, [links](https://example.com),
and headings. This body renders as the "About" section.
```

Rules:
- **All fields optional.** Body-only, frontmatter-only, or image-only entries are
  valid.
- **`stats`** is an *ordered list* of `{label, value}` (not a map) so order is
  stable and labels may repeat. Rendered as a 2-column label/value table reusing
  the existing `.faction-stats` style.
- **`image`** must name a file present in the same dir; extension in
  `{png, jpg, jpeg, webp, gif}` (case-insensitive); rejected if it contains a path
  separator or `..`. SVG disallowed (script risk).
- **Body** is everything after the closing `---`; rendered with goldmark in safe
  mode. Empty/whitespace body → no About section.
- Unknown frontmatter keys → ignored with a warning (forward-compatible).

## Architecture & data flow

New code lives in `cmd/generate-factions-kb`:

```
overlay.go     # Overlay type, loadOverlay(dir), parsing, image validation, markdown render
main.go        # load overlays, attach by ID, copy images to output, warn on orphans
render.go      # image + Profile table + About section in faction/player detail templates
```

Plus:
```
overlays/                      # source content + README + one example
cmd/seed-overlays/main.go      # personality.json -> stub profile.md (skip-if-exists, --dry-run)
```

Flow per run:
1. Load factions/players from the DB (unchanged).
2. For each entity, look for `overlays/factions/<faction_id>/profile.md` or
   `overlays/players/<player_id>/profile.md`. If present, parse it into an
   `Overlay` and attach to the struct.
3. While writing the entity's output dir, copy any validated overlay image into
   it (e.g. `kb/factions/<slug>/logo.png`) and reference it relatively.
4. Render the image, Profile (stats) table, and About (body) sections when the
   overlay is present; otherwise the page is byte-for-byte as today.
5. After processing, any `overlays/<kind>/<id>/` whose ID matched no entity is
   logged as a warning (catches typos / stale IDs).

`Overlay` struct (shape):
```go
type Overlay struct {
    ImageFile string      // validated filename, or "" 
    ImageAlt  string
    Stats     []OverlayStat // {Label, Value}
    BodyHTML  htmltpl.HTML  // goldmark-rendered, safe
}
type OverlayStat struct{ Label, Value string }
```

## Page rendering

Faction detail (player detail symmetric):
- **Image**: logo/portrait in the banner, floated to the right of the name/id
  block, capped (≤120px) with rounded corners. No image → banner unchanged.
- **Profile (stats)**: 2-column label/value table under the DB stat block, with an
  `<h3>` heading. Omitted when `stats` is empty.
- **About (body)**: rendered markdown under an `<h3>About</h3>`, wrapped in
  `.overlay-body` (comfortable measure; markdown headings sized down so they don't
  compete with the page's own `<h3>`s). A small "community-contributed" caption
  notes the content is player-supplied, not game data. Placed after the stat
  blocks and before Members (faction) / before the seen strip (player). Omitted
  when the body is empty.

CSS additions: `.overlay-logo` / `.overlay-portrait`, `.overlay-body` (in
`factions.css` and `players.css`).

## Markdown & image safety

Overlays arrive through reviewed PRs (semi-trusted), but rendering still hardens:
- **goldmark** configured to escape raw HTML (no `html.WithUnsafe`), so embedded
  `<script>`/HTML is inert. Links allowed; rendered with `rel="nofollow"`.
- **Images** validated as above (extension allowlist, no path traversal, file must
  exist). Oversized images (configurable threshold, default ~512 KB) → warning but
  still copied. Wrong type/missing → warning, image skipped, text still renders.

## Seed tool — `cmd/seed-overlays`

- Reads `personality.json` files from a configurable agents dir (default
  `../spacemolt/data/agents`).
- Matches an agent to a player when `name` equals a `seen_players.username`
  (case-insensitive, NPC/bracket rows excluded); optionally matches `organization`
  (with/without a leading "The") to a faction `name` for faction stubs.
- For each match with **no existing overlay**, writes a stub
  `overlays/players/<player_id>/profile.md` (or faction equivalent): biography from
  `biography`; `stats` from organization/role/sub_role and a few notable
  motivations/traits.
- **Never overwrites** an existing overlay. Supports `--dry-run` (reports what it
  would write, changes nothing).
- Output is committed; contributors then own and edit those files.

## Dependencies

- `github.com/yuin/goldmark` (pure Go, safe-by-default markdown).
- `gopkg.in/yaml.v3` (frontmatter parsing).

Both are pure Go with light transitive footprints.

## Testing

- **Unit (overlay.go):** frontmatter split (with/without `---`, empty body,
  frontmatter-only); `stats` parsing (ordered, repeated labels); image validation
  (good extension, bad extension, `..`/separator traversal rejected, missing file);
  goldmark raw-HTML escaping (an XSS string stays inert); orphan-ID handling.
- **Unit (seed-overlays):** name-match logic (case-insensitive, NPC excluded);
  organization→faction normalization; skip-if-exists; `--dry-run` writes nothing.
- **Integration:** run the generator with the committed example overlay; assert the
  image is copied into the output dir and the Profile/About sections render; assert
  an entity with no overlay is byte-for-byte unchanged.
- `go build ./...`, `go test ./...`, and `golangci-lint` clean.

## Out of scope (v1, possible follow-ups)

- Links section; SVG images (needs sanitization); live `personality.json` import;
  custom section ordering beyond markdown order; overlay content for the index
  pages (logos on the faction landing cards) — could be a fast follow.
