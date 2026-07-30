# Hero-Art Footprint Recovery — Design

**Date:** 2026-07-29
**Status:** Approved, not yet planned
**Supersedes nothing.** Extends `2026-07-27-ship-record-sheets-design.md`.

## Problem

`pkg/shipglyph` infers a ship's top-down shape from its class archetype,
faction and slot counts. Across all 335 catalog ships that inference produces
**six distinct hull signatures**:

| hull signature | ships | local hero art |
| --- | --- | --- |
| `beam + engine_cone` | 91 | 2 |
| `box + engine_cone + open_frame` | 85 | 7 |
| `beam` | 62 | 5 |
| `drum + engine_cone + beam` | 59 | 2 |
| `bay_rack + engine_cone` | 24 | 3 |
| `disc + beam` | 14 | 0 |

Six shapes for 335 ships is the ceiling on how distinct the glyphs can ever
look. No amount of style tuning moves it. The shapes are also unvalidated: no
one has measured how far an inferred glyph is from the ship it depicts.

The hero images are the only ground truth that exists. They are Gemini
renders produced from each ship's own description, so they are downstream of
the same lore the descriptors should encode — but every one is a 3/4 view, so
none can be traced directly.

## Approach

Recover a measured top-down footprint from each hero image and make that
footprint the descriptor source of truth.

The comparison happens in the plane the glyph already lives in. Nothing has
to guess a camera in order to compare — the camera is resolved once, during
recovery, and the output is plan-view geometry.

Rejected alternative: extrude the existing 2D glyph into 3D and compare a
rendered 3/4 view against the art. It inverts the information flow — pushing
a guess forward through a projection and judging the result by eye — and it
returns no reusable geometry. A mesh pipeline was rejected for the same
reason: a point map answers "what is the implied shape" directly.

### Model choice

**MoGe / MoGe-2** (Microsoft). It emits an affine-invariant point map plus a
recovered FOV, so a point cloud comes out of one image without asserting a
camera. ViT-L, roughly 2 GB — comfortable on the local card.

Alternatives, not chosen: Depth Pro (Apple) and Metric3D v2 are metric-depth
models, which is more than is needed and less directly usable than a point
map. Depth Anything V2 is relative-only and would leave scale and shear to be
resolved separately.

### The mirror constraint

A single view recovers only the front surface. For a bilaterally symmetric
hull seen from roughly (1,1,1), the visible surface is most of the top, and
the rest is recoverable by mirroring across the symmetry plane.

That mirror also resolves the affine ambiguity for free: choose the depth
scale **and shift** that minimise reprojection error between the cloud and
its mirror. This is a real geometric constraint, and it is the standard
approach in single-view symmetric reconstruction — not a fudge factor.

Affine-invariant means both scale and shift are unknown. Solving only for
scale leaves the footprint sheared.

**The plane cannot be found by self-chamfer.** *(Revised 2026-07-29, after
measurement.)* The original design minimised chamfer distance between the
cloud and its own reflection. That objective is wrong for a single view, and
not marginally: a one-view point map is a front-surface **sheet**, not a
volume, and the reflection that best matches a sheet is the one that folds the
sheet onto itself. The true bilateral plane scores *worse*, because it maps
visible points onto the occluded half where there are no points to match.

Measured on `outerrim_prayer`, `ledger` and `smelter`: the recovered plane cuts
through the cloud (41%, 54%, 64% of points on one side) within 2.3%, 0.5% and
7.8% of the centroid; the recovered normals are mutually inconsistent;
completion increases extent by only 1.035×, 1.035× and 1.059×; and the supposed
occluded half sits just 2.8% and 0.15% behind the visible surface where a real
half-hull would sit roughly a beam's width back. Cloud noise is ruled out as
the cause — measured surface roughness is 0.00003 relative, and a synthetic
hull at matched noise yields a residual 230× smaller than observed. The same
degeneracy is provable synthetically: given a hull with one half removed, a
spurious interior fold scores 0.025 against the true plane's 0.095.

**The objective is silhouette agreement plus depth separation.** The occluded
half is not merely symmetric, it is *behind* the visible surface and *within*
the silhouette. Both are checkable:

- every mirrored point must reproject inside the matte, and
- at a shared pixel, the mirrored point must lie measurably behind the visible
  one.

A fold satisfies the first and fails the second, which is what makes the pair
well-posed where chamfer alone is not. Depth separation is the discriminating
term and must be reported, not just thresholded.

Two consequences for sequencing. The reprojection machinery (stage 5) is now a
dependency of stage 4, so it is built first. And because a correctly completed
cloud is a volume rather than a sheet, this also repairs stage 6's reference
frame: a principal-axis decomposition of a volume gives usable hull axes, where
the same decomposition of a sheet returns the camera direction.

Chamfer against a reflection remains correct for the *residual* — for asking
how symmetric a hull is once the plane is known — and for scoring a plane on a
two-sided synthetic fixture. It is only the plane search on one-sided real data
that it cannot do.

### Camera verification

Do not assume the (1,1,1) view — measure it. These hulls are Manhattan-world
objects with long parallel structural lines, so fitting three vanishing
points yields rotation and focal length outright, and reveals whether a given
render is orthographic or mildly perspective. Different hero images differ
here: some show convergent nacelle lines, others read as near-orthographic,
and the answer changes the back-projection materially.

`lu_vp_detect` or a handful of hand-clicked line pairs will settle it.

### Matte

The magenta backgrounds are a pure chroma key. A colour-distance threshold
yields an exact alpha matte — no SAM 2, no segmentation model.

That matte is the view-plane silhouette, which doubles as a consistency check
on the reconstruction: reproject the resolved cloud through the fitted camera
and the silhouettes must overlay.

## Pipeline

Python, under `tools/footprint/`. One artifact directory per ship. Every
stage reads the previous stage's file from disk and writes its own, so any
stage is re-runnable without repeating the ones before it.

| # | Stage | Output |
| --- | --- | --- |
| 1 | Chroma-key matte | `matte.png`, foreground fraction |
| 2 | Camera fit from vanishing points | `camera.json`: rotation, focal or `ortho`, confidence |
| 3 | MoGe point map | `cloud.npz`, recovered FOV |
| 4 | Mirror-constrained solve for plane, depth scale and shift (uses stage 5's reprojection; **built after it** — see "The plane cannot be found by self-chamfer") | `cloud_resolved.npz`, symmetry plane, residual, depth separation |
| 5 | Silhouette gate: reproject, IoU against the matte | pass/fail, score |
| 6 | Orthographic projection to the ground plane, alpha shape | `footprint.json` polygon |
| 7 | Canonicalise and sample | `profile.json`: `w(t)`, per-station concavity flag |

**Stage 2 is a cross-check, not a gate.** *(Revised 2026-07-29, after
measurement. The original design made a low-confidence vanishing-point fit
route the ship to a hand-clicked fallback.)*

Measured on the 14 keyable hero images with a resolution-invariant,
null-normalised confidence, only one — `ledger` — supports a trustworthy
three-vanishing-point fit. The cause is a property of the art, not of the
estimator: on most of these renders the weak axes' vanishing points sit 25–57
image diagonals from the principal point with 0–2 supporting segments, i.e.
effectively at infinity, so the render is two-point-perspective or
near-orthographic in one axis. Several hulls are curved and carry few straight
structural lines at all. Raising the search budget 15× changes nothing (1/14 at
2000, 8000 and 30000 RANSAC iterations; the one passing ship plateaus at 0.057).
Hand-clicking three line pairs per ship does not scale to 335.

**The reference frame therefore comes from the recovered geometry, not the
camera.** The hull's own axes are already available downstream of stage 4:

- lateral axis = the symmetry-plane normal from stage 4
- longitudinal axis = the principal axis of the cloud *within* that plane
- up = lateral × longitudinal

This assumes no camera; it measures the hull frame from recovered 3D geometry,
which keeps the original intent — nothing silently inherits an assumed
viewpoint. Where stage 2 *is* confident, its rotation is compared against the
geometric frame and the agreement logged. Disagreement is recorded, never
averaged.

Two consequences worth stating plainly. First, the original constraint had no
implementation path in any case: stage 3 takes no camera input, so stage 2
could never have gated it — the gate only ever decided whether a ship was
skipped outright. Second, stage 2's focal length is consumed by nothing; the
stage 5 reprojection uses MoGe's intrinsics.

**Stage 3 input is an open question**, to be settled on two or three images
before the batch runs: whether MoGe performs better on the raw magenta image
or on the matted subject composited onto a neutral background. Flat saturated
magenta is out of distribution for a model trained on photographs. The
recovered FOV should also be cross-checked against the stage 2 focal length;
disagreement is logged, not averaged away.

**Stage 5 is the honesty gate.** A ship that fails it is excluded from the
numbers and listed as a reconstruction failure. It is never quietly folded
into an average.

**Stage 6 uses an alpha shape, not a convex hull.** These hulls have real
concavities — the gaps between nacelles — and a convex hull erases exactly
the features that distinguish one ship from another.

**Stage 7 canonicalises** into glyph space: the symmetry plane from stage 4
supplies the longitudinal axis, the footprint is rotated so that axis is X,
length is normalised to 1, and half-width is sampled perpendicular to the
axis at the same 96 stations `profileSamples` uses. Stations where the
footprint is disjoint are flagged.

### Alpha-shape parameter

The alpha radius is pinned once per batch, not tuned per ship. A per-ship
alpha is a free parameter that can be turned until any footprint looks
right, which would make the grading meaningless. It is chosen by sweeping a
small range across all recovered clouds and taking the value that maximises
mean stage 5 silhouette IoU, and the chosen value is recorded with the
results.

### Environment

MoGe runs in its own virtualenv, separate from the existing `~/sd-venv` used
for portrait generation. The two have unrelated dependency pins and the
portrait pipeline must not break because this one needed a different torch.

## Consumption

Go. `profile.json` crosses the language boundary; nothing else does. This
matches the existing `overlays/` convention.

### Grading

Per-station half-width error between the measured profile and the currently
inferred glyph, reported per ship and per hull signature.

Deliberately not a single overlap number: the per-station curve says *where*
an archetype is wrong, which is what an authoring pass needs and what a
scalar throws away.

Output also includes a contact sheet — hero image, matte, recovered
footprint, current glyph — for each ship.

### Authoring

Each measured profile becomes a descriptor overlay: one `beam` hull part
whose `Points` are the decimated measured control points.

This needs **no renderer change**. `HullPart.Points` already exists for kind
`beam` (`descriptor.go:21`) and `beamHalfWidth` (`parts.go:72`) already
interpolates it. Decimate to roughly 8–14 points so a human can still hand-
edit an overlay afterwards.

Ships with art get overlays. Ships without keep archetype inference. The
merge already supports exactly this split.

## Required fix: `Merge` cannot remove an inferred feature

`Merge` (`descriptor.go:80`) treats an empty slice as "not set", so
`"appendages": []` in an overlay cannot clear inferred wings. A measured
Prayer has no wings; the overlay must be able to say so.

This is already recorded as a Phase 2 blocker in
`2026-07-27-ship-glyphs-p1-outcomes.md`. This work makes it load-bearing, so
it is fixed here rather than deferred again. The fix must distinguish absent
from empty — a pointer slice, an explicit `"clear"` list, or unmarshalling
into a presence-tracking type. Whichever is chosen, `Merge`'s doc comment and
its tests must state the distinction, because "empty means unset" is
currently documented behaviour.

## File structure

| Path | Contents |
| --- | --- |
| `tools/footprint/` | Python pipeline, stages 1–7 |
| `data/footprints/<id>/` | Per-ship artifacts: `matte.png`, `camera.json`, `cloud.npz`, `cloud_resolved.npz`, `footprint.json`, `profile.json` |
| `cmd/grade-ship-glyphs/` | Go: reads `profile.json`, emits the grading report and contact sheet |
| `overlays/shipshapes/<id>.json` | Go: authored overlays, the existing directory `generate-ship-glyphs -overlays` already reads |

The intermediate artifacts under `data/footprints/` are regenerable and
large; only `profile.json` and the emitted overlays are worth committing.

## Suggested decomposition

This is one spec but naturally two implementation plans, and the second is
worth writing only if the first produces usable footprints:

1. **Recovery** — stages 1–7 plus the synthetic end-to-end test. Deliverable:
   `profile.json` for the ships that pass the silhouette gate, and a count of
   those that do not.
2. **Consumption** — the `Merge` fix, grading, the contact sheet and overlay
   emission.

## Scope

Run on the 19 hero images already in `~/Downloads/*.webp`, which cover 5 of
the 6 hull signatures. Build batch-shaped so that pointing the pipeline at a
full art drop is a path change, not a rewrite.

`disc + beam` — 14 ships, 4% of the catalog — has no local art and stays
ungraded until an art drop arrives. This is a stated gap in the results, not
a silent omission.

## Testing

**Synthetic end-to-end.** Render a known box and a known cylinder from a
known camera, run all seven stages, and assert the recovered footprints are a
rectangle and a circle at the right aspect. This validates the chain without
trusting any hero image, and it is the test that fails loudly if a stage
regresses.

**Per-ship.** The stage 5 silhouette gate is the per-ship test.

**Go side.** Ordinary unit tests on decimation, canonicalisation and the
grading metric, plus one golden ship whose profile is checked by hand.

## Known hazards

- **The Prayer carries a pilot on an exposed seat frame.** A human figure
  will land in that footprint. Every ship whose lore includes exposed crew or
  external cargo has the same problem.
- **The mirror assumption fails on deliberately asymmetric hulls.** Outer Rim
  and pirate scrap ships are the entire point of the `Skew` style. The stage
  4 residual is the detector: a high residual means "do not mirror this one",
  and may mean the footprint is only half recoverable.
- **The residual threshold is not yet calibrated against real data.** Measured
  stage 4 residuals on the first three real clouds were 0.0163, 0.0146 and
  0.0140 against a ceiling of 0.013 derived from synthetic fixtures — every ship
  would read as asymmetric. Those numbers came from the self-chamfer solve that
  has since been replaced, so they are not evidence about the hulls; they must be
  re-measured once the silhouette-plus-depth objective lands, and the ceiling
  set from real clouds rather than from a synthetic hull with Gaussian noise.
  Until then, treat the asymmetry label as uncalibrated. It labels, and does not
  exclude — stage 5 is the only exclusion gate.
- **A hero image may not depict the ship the catalog describes.** The art is
  generated, not authoritative. A large disagreement is evidence about the
  art as much as about the descriptor.

## The decision this produces

Two numbers:

1. **Median per-station half-width error**, measured against inferred, per
   hull signature.
2. **Fraction of footprints with concavities** the single-loop `Outline`
   representation structurally cannot express.

If (2) is high, the finding is not "the archetypes are wrong" but "the
outline representation is wrong" — `Outline` samples one half-width per
station and cannot represent a gap between two nacelles. That is a different
and larger piece of work, and it must be recognised as such rather than
absorbed into descriptor tuning.
