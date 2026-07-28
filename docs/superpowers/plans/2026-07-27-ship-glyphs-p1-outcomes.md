# Ship Glyphs P1 — Outcomes and Phase 2 Notes

**Completed:** 2026-07-28
**Branch:** `worktree-ship-glyphs-p1`, 16 commits from `a9c0f76ed`
**Plan:** [2026-07-27-ship-glyphs-p1.md](2026-07-27-ship-glyphs-p1.md)
**Design:** [../specs/2026-07-27-ship-record-sheets-design.md](../specs/2026-07-27-ship-record-sheets-design.md)

## What shipped

`pkg/shipglyph` renders a deterministic top-down blueprint-style SVG for any ship from its
catalog stats. `cmd/generate-ship-glyphs` writes one `.svg` per ship into `kb/ships/glyphs/`
plus a contact sheet inlining all of them grouped by faction.

- 335 glyphs, one per catalog ship. Regeneration is byte-identical.
- 55+ tests. `golangci-lint` clean. Light and dark themes both verified by rendering.
- Contact sheet at `kb/ships/glyphs/index.html` (3.6 MB, renders in ~1.9 s).

Element IDs are the consumer contract. Standalone `.svg` files use unprefixed IDs
(`region-bow`, `hp-w1`, …) so a dashboard can paint live ship state onto them; the contact
sheet passes `Options.IDPrefix` so its 335 inlined copies do not collide.

## Taste-gate findings

P1 exists so the design language can be judged before later phases build on it. Judging it
produced four items, none of them defects:

1. **Faction languages are not visually distinct.** Holding class constant, crimson, nebula,
   solarian, voidborn and pirate produce near-identical silhouettes; only outerrim is
   identifiable, via its jitter. The constants are too subtle: crimson's `Chamfer: 0.22` on a
   384-point loop cuts imperceptibly, solarian's `Flute: 0.06` is invisible, and voidborn's
   `Lobed: 0.18` does not read as organic. All three are single values in `style.go`.
2. **Within-faction sameness.** Assault, Battlecruiser, Bulk Hauler, Carrier, Command, Cruiser
   and Dreadnought all map to the `spine` archetype and collapse to similar rounded rectangles.
3. **Scale is invisible.** A Dreadnought and a Miner render the same size, because every glyph
   normalises to a common box. Changing this is one line — how `usable` is computed in `Render`.
4. **Archetype coverage gap.** 30 of 335 ships across 14 classes fall through to `spine`
   because those classes are absent from `classArchetype`. Several are visibly wrong under that
   fallback: Interceptor should be `dart`; Recon, Pathfinder, Blockade Runner and Smuggler
   should be `needle`; Assault Carrier should be `rack`. (Customs Patrol, Monitor, Hunter,
   Multirole and Command Dreadnought are fine as `spine`.)

## Phase 2 blockers

Settle these before authoring lore-derived overlays for 335 ships. The overlay path itself
works — `LoadOverlay → Merge → Render` was exercised end to end and correctly applied a
hand-authored descriptor while leaving other ships untouched — but its expressiveness is
narrower than the design assumed.

1. **Hull part kinds are width-only.** All nine kinds are half-width functions and nothing
   draws internal structure: `container_stack` returns `0.11 × grid[0]`, `box` returns
   `p.Half`, and they are indistinguishable at equal width. The design's premise — that
   authoring `{"kind":"container_stack","grid":[2,2]}` yields a visibly container-like hull —
   does not hold yet. Authoring 335 descriptors today mostly buys width profiles.
2. **An overlay cannot remove an inferred feature.** `Merge` treats a zero-length slice as
   "not set", so `"appendages": []` cannot clear inferred wings. Hand-authoring needs a way to
   say "none".
3. **No descriptor validation.** A hull part missing `"span"` renders an invisible 0.2 px
   hairline with no error; an unknown `"kind"` silently becomes a 0.15 constant-width box;
   misspelled top-level keys are ignored. One `Validate(Descriptor) error` covers all three.
4. **`safeKind` and `appendagePoly` disagree.** The former lowercases for the id/class, the
   latter switches on the raw `Kind`, so `"Wing"` gets wing styling but default geometry.
5. **`Symmetry` and `Greeble` are a second source of truth.** `Infer` computes both and nothing
   reads them; asymmetry actually comes from `Style.Jitter`. Either wire them up or drop them.
   (`Bells` and `Cells` are also unread, but those are forward-looking schema that P3's detail
   rendering will draw — worth keeping.)
6. **`Infer` never emits `lobe_cluster`.** The metaball part kind exists but no archetype uses
   it, which is why voidborn reads as conventional. It is reachable only via overlays.

## Deferred, accepted

Sub-pixel or latent today, worth revisiting when the constants above are tuned:

- `Regions` ignores `Chamfer`/`Smooth`, so region polygons diverge from the drawn outline by up
  to 0.72 px on a 200 px glyph. That divergence scales with those constants — re-measure after
  tuning.
- Appendages are excluded from aspect normalisation and the margin. Nothing overflows today;
  the tightest glyph (`theorem.svg`) has 5.1 px of its 16 px margin left, so widening dart wings
  or lowering dart aspect ~5% would clip silently.
- `placeKind` spreads by global index, so a slot type with multiple mount zones bunches unevenly.
  Unreachable while every inferred archetype supplies exactly one zone per kind.
- Region boundary filters are inclusive at both ends. No sample lands on 0.25 or 0.75 only
  because `profileSamples = 96` makes `t = i/95` non-integral there; changing that constant
  could double-assign a sample. A comment at the declaration notes this.
- `groupByFaction` uses `slices.SortFunc`, which is not stable. The catalog has no ties today.
- `Render` recomputes the profile four times per glyph. No correctness impact at this scale.

## Notes for whoever tunes this

The levers are single named constants, as intended:

| Want to change | Edit |
|---|---|
| Faction look (chamfer, jitter, flute, lobe amplitudes) | `style.go`, `StyleFor` |
| Which classes map to which shape | `infer.go`, `classArchetype` |
| Archetype proportions and hull parts | `infer.go`, `archetypeAspect` / `archetypeHull` |
| Per-part width profiles | `parts.go`, `partHalfWidth` |
| Whether size varies by ship | `render.go`, how `usable` is computed |

Regenerate with:

```
go build -o bin/generate-ship-glyphs ./cmd/generate-ship-glyphs && ./bin/generate-ship-glyphs
```

The default `-catalog` path is relative to the repo root and does not resolve from a worktree;
pass an absolute path there.
