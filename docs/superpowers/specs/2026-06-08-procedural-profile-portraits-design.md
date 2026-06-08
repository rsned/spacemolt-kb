# Procedural Profile Portraits — Design

**Date:** 2026-06-08
**Status:** Approved design, ready for implementation planning
**Component:** `cmd/generate-factions-kb` (KB site generator)

## Problem

Scanned agents (players) and ship passengers shown in the KB have no profile
photo until a contributor hand-authors a portrait overlay. Most profiles are
therefore a bare text banner. We want every profile to carry a visual
identity *automatically*, while still letting the real owner (or a curator)
replace it later — "until they provide their own."

Two populations, two very different data shapes:

- **Players (scanned agents):** `username`, stable `id`, faction/player colors.
  No bio. Potentially many of them; the KB regenerates frequently. Cheap,
  zero-cost placeholders are essential.
- **Passengers:** a *coming-soon* game feature being wired in. Minimal data —
  `id`, `name`, and a `bio`. The bio is rich enough to drive a text-to-image
  prompt. Far fewer of them than players.

## Solution Overview

A single **precedence model** resolves a portrait for every profile, backed by
two generators tuned to each population:

- **Players → inline sci-fi silhouette SVG.** Deterministic from the player ID,
  tinted by the player's colors, embedded directly in the page. No model call,
  no files, no cache — regenerates free on every build.
- **Passengers → AI portraits.** A prompt is built from the bio and sent to a
  **pluggable CLI image generator** (Stable Diffusion or any tool wrappable in a
  script). Results are cached, committed, and only regenerated when the prompt
  changes. Generation is an explicit opt-in step, decoupled from routine regen.

A contributor-authored overlay portrait always wins over either generator, so
the existing overlay system (`overlays/players/<id>/portrait.webp`) remains the
"provide your own" escape hatch with no code change.

## Precedence Model

For every profile, the generator resolves a portrait in this fixed order:

1. **Contributor overlay portrait** — human-authored `portrait.webp` in
   `overlays/.../{id}/` (existing system). Always wins.
2. **Generated placeholder:**
   - *Player* → inline sci-fi silhouette SVG (always available, zero cost).
   - *Passenger* → cached AI portrait (available once the batch has run).
3. **Last-resort fallback** — the existing plain banner. Only reached by a
   passenger with no generated portrait *yet* and no overlay. Players never hit
   this, because their silhouette is always available.

A single resolver function encapsulates this order and is used by both the
player and passenger templates.

## Component 1 — Player Silhouette Builder (inline SVG)

A new pure component in the generator produces a self-contained `<svg>` string
from a player.

**Determinism & seeding**
- Seed from the player **ID** (stable across renames, unlike username).
- The seed selects from a small set of discrete variations: visor/helmet shape
  (4–6 variants), antenna/badge presence, collar style. Same agent → identical
  SVG on every rebuild; different agents → reliably different.
- Colors come from the player's existing `PrimaryColor` / `SecondaryColor`.
  When color data is empty, derive a deterministic palette from the ID hash
  (never a flat gray) so every agent still looks distinct.

**Form**
- A stylized head-and-shoulders crew silhouette inside the existing `.infobox`
  frame: visor glow + faction-tinted field. Clearly a placeholder, on-theme,
  not a real face.

**Why inline SVG (not files)**
- Zero model calls; regenerates free on every KB build, fitting the frequent
  `chore(kb): refresh` cadence. No caching, no committed binaries, no copy step.
- Crisp at any size, tiny, themeable via the CSS custom properties already in
  `smui.css`.

**Integration**
- In `render.go`, the player detail template's "no overlay" branch swaps the
  bare banner for the inline SVG inside the infobox. The overlay branch is
  untouched.

**Defaults confirmed**
- Empty-color players → deterministic palette from ID hash (not gray). **Yes.**
- Players-index silhouette thumbnails → **deferred to a follow-up phase**
  (detail pages first).

## Component 2 — Passenger AI Portrait Pipeline

### a) Data source (coming-soon-aware)
A thin `Passenger` loader (`id`, `name`, `bio`) follows the same pattern as
`loadPlayers`, reading from wherever the new game feature lands (table/JSON —
exact source TBD when wired). Until then it returns empty, so nothing breaks.
The pipeline ships built but dormant, ready to light up when data arrives.

### b) Prompt builder
A pure function `bio → prompt`:
- Combines the passenger bio with a fixed **style suffix** that keeps the whole
  gallery visually consistent (e.g. `"…, character portrait, sci-fi crew
  member, painterly, neutral background, head and shoulders"`).
- The style suffix lives in a single config constant so the whole gallery's look
  can be tuned in one place.
- An empty/garbage bio still yields a valid generic prompt — never empty.
- The exact prompt is persisted alongside the image for cache invalidation.

### c) Pluggable CLI contract
The generator shells out to a command configured via env var / config field
(e.g. `SMKB_PORTRAIT_CMD`). The contract is intentionally minimal — the command
receives:
- the **prompt** (via stdin or `--prompt`),
- an **output file path** to write the image to,
- a deterministic **seed** derived from the passenger ID (reproducible per
  passenger).

Any backend wrappable as "take a prompt + seed, write an image to this path"
works: SD CLI, ComfyUI, a Replicate curl wrapper, etc. If the command is unset,
the pipeline no-ops (falls back to banner) — so a normal KB regen never requires
a model to be installed.

### d) Caching & regeneration
Generated portraits are **committed** (they are expensive to produce).
- Cache key = passenger **ID**.
- A sidecar next to each image stores the **prompt hash**.
- On build: image exists AND prompt hash unchanged → **skip** (no model call).
  Bio/prompt changed (hash differs) or no image → **regenerate**.
- Routine regens cost nothing; only new or changed passengers hit the model.

### e) Storage location
Generated content lives in a **separate tree** from human overlays so curated
and machine content are never confused:

```
overlays/generated/passengers/<id>/portrait.webp   # the image (committed)
overlays/generated/passengers/<id>/prompt.txt      # prompt + hash sidecar
```

Human overlays stay in `overlays/players/<id>/` and always win (precedence
model). At build, generated portraits copy into `kb/...` exactly like overlay
images do today (`copyOverlayImage`).

### f) Cadence (opt-in generation)
Because SD is slow, image generation is **decoupled** from the normal generator:
- A dedicated step/flag (e.g. `generate-factions-kb --portraits`, or a small
  sibling command) runs the passenger batch.
- The everyday `chore(kb): refresh` regen merely *consumes* whatever portraits
  already exist on disk.
- Generation is therefore always an explicit opt-in.

## Error Handling

- **Missing/failed portrait command** → log warning, skip that passenger, fall
  back to banner. Never abort the build. (A broken SD setup must not break KB
  regen.)
- **Invalid generated image** (wrong dimensions, unreadable) → reuse existing
  overlay image validation (≤320×320, allowed extensions: png/jpg/jpeg/webp/gif);
  on failure, warn + skip.
- **Empty/garbage bio** → prompt builder emits a valid generic prompt; never
  empty.
- **Player SVG** is failure-proof: pure string building, always returns valid
  SVG even with empty colors/ID.

## Testing

- **Silhouette builder:** golden tests — same ID+colors → byte-identical SVG;
  different IDs → different output; empty colors → deterministic palette
  (not gray).
- **Prompt builder:** table tests — bio in → expected prompt (with style
  suffix); empty bio → generic-but-valid prompt; prompt-hash stability.
- **Cache logic:** unchanged hash → skip (assert the command is *not* invoked,
  via a fake command script); changed hash → regenerate. No real model used.
- **Precedence resolver:** overlay > generated > banner, asserted for both
  player and passenger.

## Phasing

Each phase is independently shippable.

1. **Phase 1 — Player silhouettes.** Fully buildable today; immediate visible
   payoff on every player detail page. A complete, useful change on its own.
2. **Phase 2 — Passenger pipeline scaffolding.** Prompt builder, pluggable CLI
   contract, caching, `--portraits` step, `overlays/generated/` tree, precedence
   wiring — all behind the dormant passenger loader. Testable end-to-end with a
   fake CLI before real passenger data exists.
3. **Phase 3 — Real passenger data** when the game feature lands (just the
   loader's data source).
4. **Phase 4 (follow-up)** — players-index silhouette thumbnails.

## Key Files (reference)

| File | Role |
|------|------|
| `cmd/generate-factions-kb/main.go` | Generator entry; overlay attach + image copy; add `--portraits` step |
| `cmd/generate-factions-kb/render.go` | Player/passenger detail templates; swap no-overlay banner for silhouette |
| `cmd/generate-factions-kb/types.go` | `Player`, `Overlay` structs; add `Passenger` |
| `cmd/generate-factions-kb/players.go` | `loadPlayers`; add `loadPassengers` sibling |
| `cmd/generate-factions-kb/overlay.go` | Overlay load, image validation, `copyOverlayImage` (reused for generated images) |
| `kb/smui.css` | `.infobox` / `.infobox-image` styling (silhouette reuses) |
| `overlays/README.md` | Document the new `overlays/generated/` tree + portrait CLI contract |
