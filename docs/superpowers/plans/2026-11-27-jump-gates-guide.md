# Jump Gates Guide Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A public KB guide `guides/jump-gates.md`, rendered to `kb/guides/jump-gates.html` and listed in the Guides index.

**Architecture:** Hand-written guide on the existing `cmd/generate-guides` renderer (YAML frontmatter + markdown, like `guides/player-stations.md`). Steps carry `facts` and `notes` only — no `facility` field — so no catalog sidebar and no code change.

**Tech Stack:** Markdown + YAML frontmatter; Go renderer already in the repo.

**Spec:** docs/superpowers/specs/2026-11-27-jump-gates-guide-design.md

## Global Constraints

- **Hints only** for gathering: no system names, POI ids, deposit amounts per site, or guard lists. Allowed: "repeater-guarded nebula shells", "10 systems, ~17,700 seam glass in total", "the fold knot is a rare spire drop".
- **Never name** in the public guide: Alhena, Luminous Frame, Tinsel, Gloaming, Ash Halo, Long Ember, Old Clock, Burnt Lace, The Sanctum, The Threshold, Cloverfield/Conjunction Junction as a gate location, any agent name (craftsman-1 etc.).
- Recipe numbers come from `data/recipes_unlisted.json` (captured 2026-11-21); each recipe fact carries `basis: "measured 2026-11-21 (recipe capture)"`.
- Labels: *measured* / *player-reported* / *doc-quoted*, same convention as the station guide.
- `[[id]]` links only for ids that have item pages (`fold_shim`, `fold_knot`, `seam_glass`, `annealed_seam_glass`, `quantum_entangler`, `incoherent_entangler`, plus any other existing item such as `darksteel_plating`, `phase_matrix`, `solarian_composite`, `prismatic_lens`, `circuit_board`). Lane Anchor, Lane Anchor Prototype, Fold Lattice, Gate Regulator, Echo Cell, Mirror Facet have **no** page: write them as plain bold names (an unresolved `[[id]]` renders the raw id and a warning).
- Commit explicit paths only; the kb repo has many unrelated modified `kb/items/*.html` files.

## Review Focus

- A forbidden place or agent name slipping into notes or facts (the hints-only rule is the whole point) — the Step 3 grep pins it.
- An `[[id]]` for an item without a page renders as a bare id — the renderer's warning list must be empty for this guide.
- Unknown numbers stated as fact (part counts, upkeep, crossing cost, phase list) — each must read "not yet measured".
- The Monolith warning softened into a hint — it stays explicit (undefeatable; each kill spawns two; cannot be fled; player-reported 8 h / 354 kills, then lost).

---

### Task 1: Write and render the guide

**Files:**
- Create: `guides/jump-gates.md`
- Generated: `kb/guides/jump-gates.html`, `kb/guides/index.html`

- [ ] **Step 1: Write `guides/jump-gates.md`** following the spec's Outline (intro + 6 steps + "Not yet measured"). Frontmatter keys: `title: Jump Gates`, `summary:` (one line), `intro:`, `steps:` (each `title`, `facts`, `notes`). Values to use verbatim:
  - Recipes (inputs per craft, hand-craft time):
    Lane Anchor = 50 seam glass, 1 fold knot, 10 mirror facet, 40 darksteel plating (8.62);
    Lane Anchor Prototype = 10 seam glass, 1 fold knot, 2 mirror facet, 10 darksteel plating (1.72);
    Fold Lattice = 10 seam glass, 2 echo cell, 10 phase matrix (2.59);
    Gate Regulator = 4 echo cell, 10 solarian composite, 5 prismatic lens (3.45);
    Fold Shim = 2 annealed seam glass, 2 circuit board (2.59).
  - Recipe source: Sinter at Starfall Salvage Station, chain Taking the Machine Apart → Bench Test → The Method.
  - Repeater drops (*measured*, per kill): sliver 1 mirror facet (+ seam glass or trade crystal depending on site); pane 1 echo cell + 2 mirror facets; spire 2 echo cells + 4 mirror facets, and rarely a fold knot (1 in 4 so far). Repeater sizes: sliver 100/150, pane 400/600, spire 1,600/2,400 hull/shield. Reflection: pane flat ~15 per hit, spire a share of incoming energy damage.
  - Founding: `build_gate_site` payload `{"name": "...", "far_system": "<system id>"}`; `system_id` rejected; fee 2,500,000 cr; the server hint text from the spec's step 5.
  - Running: fold shims wear; quantum entanglers held in gate storage for upkeep, some turn incoherent (wanted by First Step's *It Hums*).

- [ ] **Step 2: Render**
  Run: `go run ./cmd/generate-guides`
  Expected: exit 0; `kb/guides/jump-gates.html` exists; no warning line mentioning `jump-gates.md`; `kb/guides/index.html` lists "Jump Gates".

- [ ] **Step 3: Hints-only check**
  Run: `grep -n -i -E 'alhena|luminous|tinsel|gloaming|ash halo|long ember|old clock|burnt lace|sanctum|threshold|cloverfield|conjunction|craftsman|explorer-|fighter-' guides/jump-gates.md kb/guides/jump-gates.html`
  Expected: no output.

- [ ] **Step 4: Commit**
  ```bash
  git add guides/jump-gates.md kb/guides/jump-gates.html kb/guides/index.html docs/superpowers/specs/2026-11-27-jump-gates-guide-design.md docs/superpowers/plans/2026-11-27-jump-gates-guide.md
  git commit -m "guides(jump-gates): new guide — recipes, rare inputs (hints only), monolith warning, founding a site"
  ```
