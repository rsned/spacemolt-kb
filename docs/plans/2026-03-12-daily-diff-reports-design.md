# Daily Game Data Diff Reports

**Date**: 2026-03-12
**Status**: Design complete

## Overview

Automate tracking of day-to-day changes in the 5 static game catalogs that feed
the KB (recipes, items, ships, skills, map). A new `generate-diffs` tool in the
KB repo diffs fresh JSON snapshots against the previous day's data and produces
HTML report pages viewable at `kb/diffs/`.

## Tracked Catalogs

| Catalog | File | Key Field | Array Field |
|---------|------|-----------|-------------|
| Recipes | `catalog_recipes.json` | `id` | `items` |
| Items | `catalog_items.json` | `id` | `items` |
| Ships | `catalog_ships.json` | `id` | `items` |
| Skills | `catalog_skills.json` | `id` | `items` |
| Map | `get_map.json` | `system_id` | `systems` |

## Tool: `cmd/generate-diffs`

Standalone Go binary. Invoked as:

```bash
generate-diffs --input /path/to/fresh/json/files
```

### What it does on each run

1. Creates `data/snapshots/YYYYMMDD/` with today's date.
2. Copies the 5 catalog files from `--input` into that directory.
3. Reads `previous` symlink to find yesterday's snapshot.
4. Diffs each catalog using comparison logic ported from
   `spacemolt/cmd/data/data-diff`.
5. Generates `kb/diffs/YYYY-MM-DD.html` with all 5 diffs in sections.
6. Regenerates `kb/diffs/index.html` (day listing with summary counts).
7. Updates prev/next links in adjacent day pages.
8. Rotates symlinks: `previous` -> old `latest`, `latest` -> today.

### First run handling

If no `previous` symlink exists, the tool imports the snapshot and creates
`latest` only — no diff report generated (nothing to compare against). Next run
will have a baseline.

### No-change days

If all 5 catalogs are identical, still generate a page saying "No changes
detected" so the prev/next chain remains unbroken.

## Snapshot Storage

```
data/snapshots/
├── 20260311/
│   ├── catalog_items.json
│   ├── catalog_recipes.json
│   ├── catalog_ships.json
│   ├── catalog_skills.json
│   └── get_map.json
├── 20260312/
│   └── ... (same 5 files)
├── previous -> 20260311
└── latest -> 20260312
```

Dated directories are kept for historical reference. The `previous` and `latest`
symlinks point to the two most recent directories and are the only ones the tool
reads during diffing.

## HTML Report Page: `kb/diffs/YYYY-MM-DD.html`

Uses `smui.css` dark theme. Not linked from main KB nav — accessed directly at
`/diffs/`.

### Layout

```
┌─────────────────────────────────────────────┐
│  < 2026-03-11          2026-03-13 >         │  prev/next nav
│                                             │
│  Game Data Changes - March 12, 2026         │  h1
│                                             │
│  Summary: 19 additions, 33 deletions,       │  top-level counts
│  10 modifications across 3 catalogs         │
│                                             │
│  ┌─ Recipes ──────────────────────────────┐ │
│  │  +19  -33  ~10                         │ │  section header
│  │                                        │ │
│  │  Additions                             │ │
│  │    + Build Afterburner II              │ │
│  │    + Build Afterburner III             │ │
│  │                                        │ │
│  │  Deletions                             │ │
│  │    - Build Advanced Drone Bay          │ │
│  │                                        │ │
│  │  Modified                              │ │
│  │    prepare_gourmet_rations             │ │
│  │      outputs: 3 -> 1                   │ │
│  └────────────────────────────────────────┘ │
│                                             │
│  ┌─ Items ────────────────────────────────┐ │
│  │  No changes                            │ │
│  └────────────────────────────────────────┘ │
│                                             │
│  ... (Ships, Skills, Map sections)          │
└─────────────────────────────────────────────┘
```

Catalogs always appear in fixed order: Recipes, Items, Ships, Skills, Map.
Sections with no changes show "No changes" to confirm the catalog was checked.

Prev/next links navigate to the adjacent existing report day, not calendar days.

## Index Page: `kb/diffs/index.html`

Reverse-chronological list of all report days. Each row shows the date (linked)
and a one-line summary like "19 additions, 33 deletions, 10 modifications" or
"No changes".

## File Layout

```
kb/
├── cmd/generate-diffs/main.go    # CLI tool
├── pkg/gamediff/                 # Diff logic
│   ├── diff.go                   # Comparison functions
│   └── report.go                 # HTML generation
├── data/snapshots/               # JSON archives (gitignored)
│   ├── YYYYMMDD/
│   ├── previous -> YYYYMMDD
│   └── latest -> YYYYMMDD
└── kb/diffs/                     # Generated HTML (gitignored)
    ├── index.html
    └── YYYY-MM-DD.html
```

## Diff Logic: `pkg/gamediff`

Ported from `spacemolt/cmd/data/data-diff/main.go`. Core types:

```go
type CatalogDiff struct {
    Name      string   // "Recipes", "Items", etc.
    File      string   // "catalog_recipes.json"
    Additions []Entry  // name + id
    Deletions []Entry
    Changes   []Change // id, field, old, new
}

type Entry struct {
    Name string
    ID   string
}

type Change struct {
    ID     string
    Field  string
    OldVal any
    NewVal any
}
```

Detection strategy per file:
- `catalog_*.json` -> keyed by `id` in `items` array
- `get_map.json` -> keyed by `system_id` in `systems` array

Field-level changes are always computed (no flag needed — these are historical
reports, not quick terminal checks).
