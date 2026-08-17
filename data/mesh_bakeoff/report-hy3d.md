# Image-to-3D bake-off round 2: Hunyuan3D-2 vs TripoSR

Follow-up to `report.md` (round 1, TripoSR, 2026-08-07). Round 1 concluded
that TripoSR's reconstructions were angle-VARIANT: two photographs of the
same ship produced two different hulls, because the reconstructed aspect
tracked each photograph's apparent foreshortening. This round asks whether
that was a capacity limit of a small, fast model or something intrinsic to
single-image 3D — prompted by a commercial result (meshy.ai on
`outerrim_rapid_smelter`) that looked convincingly good.

Model: Hunyuan3D-2 (`tencent/Hunyuan3D-2`, `hunyuan3d-dit-v2-0`, fp16),
shape-only — `Hunyuan3D-Paint` never loaded, since footprints need geometry
and skipping texture halves the memory and time cost. 50 steps, octree 320,
guidance 5.5, fixed seed 1234 so both views of a pair face identical
sampling noise. SPAR3D was the first choice and was dropped: still HF-gated,
still no token on this box — the same blocker that killed SF3D in round 1.

Hardware: RTX 2000 Ada, 15.58 GiB. Peak 5.26 GB VRAM — less than a third of
the card, so octree resolution was not the binding constraint. 87.6 s/image
mean, 15/15 succeeded, all watertight. (`nvidia-smi` is broken on this box:
kernel module 595.71.05 vs userspace 595.84, needs a reboot. CUDA itself is
unaffected; VRAM was read through torch.)

Background removal is a hard chroma key against the magenta drop, not rembg.
Cross-check: `outerrim_rapid_smelter` keys to foreground fraction 0.497
against the footprint pipeline's independently-recorded 0.4984. This also
retires round 1's noted magenta edge-fringing risk.

Everything downstream of the generative model — `mesh_footprint.py`, the
frame discovery, the stage-7 profile sampler — is byte-identical to round 1,
so the comparison isolates the model.

## RETRACTED 2026-08-11: the pair premise is invalid

Everything below the next heading is unsound and is kept only as a record of
what was run. The "duplicate pairs" inherited from round 1 are NOT two views
of one ship.

Checked against the ship catalog (`spacemolt-knowledge.db`), the 18
double-stem filenames in the drop split three ways:

- 7 stems where the bare name is a real ship and the prefixed one is not
  (`precept` yes, `solarian_precept` no) -- **these are the 7 the bake-off used**
- 5 where only the prefixed form is real (`crimson_devastator` yes,
  `devastator` no)
- 6 where NEITHER is real: `foundation.png` + `nebula_foundation.png` while
  the actual ship is `solarian_foundation` -- the filename prefix is wrong

The real convention is `faction_shipid.png`, confirmed by `controlled_chaos`
and `swarm_boss`: both real outerrim ships, one image each, correctly
prefixed. Double-stems are anomalies, not deliberate second views. The
footprint pipeline agrees -- it built `data/footprints/precept` and never
`solarian_precept`.

Visual confirmation: `precept.png` is a wide delta hull with a domed turret
and dish arrays; `solarian_precept.png` is a slender angular wedge with a
side gun mount. No camera angle maps one onto the other. `capacity` differs
in sphere count (3 vs 4). Several second views (`solarian_promenade`) are
cropped by the frame with the hull running off all four edges, so aspect is
unrecoverable from them for reasons unrelated to any model.

So the measured spreads are variance in the ART DROP, not evidence about
TripoSR or Hunyuan3D-2. Round 1's report.md carries the same flaw and its
headline should be treated as retracted too.

**What survives:** the environment and harness (both work, 15/15 meshes,
88 s/image, 5.26 GB peak); the meshes themselves are structurally faithful;
and the separate, still-open observation that these models embellish
geometry on the VISIBLE side of the image, which the pair test never
addressed.

**Valid replacements** (neither needs pairs): (1) reproject -- render each
mesh's silhouette over a search of camera orientation and scale, score best
achievable IoU against the source matte, residual = hallucination on
observed geometry; (2) synthetic ground truth -- take a known mesh, render
two views, reconstruct each, compare against the mesh you started with.

## [RETRACTED] Headline: NEGATIVE. Frontier quality does not fix it.

| pair | TripoSR spread | Hy3D spread |
|---|---|---|
| warranty | 131% | 66% |
| precept | 99% | **161%** |
| excessive_force | 60% | 64% |
| capacity | 40% | 16% |
| promenade | 37% | 1% |
| ordinance | 11% | 47% |
| archive | 1% | 11% |
| **median** | **40.2%** | **46.6%** |
| **within 10%** | **1/7** | **1/7** |

Hunyuan3D-2 is not better. It redistributes which pairs fail (promenade and
capacity improve, ordinance and archive regress) but the median two-view
disagreement is unchanged, and only one pair in seven lands within 10% in
either round. Control for the extractor: dropping pairs where either view
failed to resolve a lateral-vs-vertical frame leaves 4 clean pairs, and the
disagreement persists in both rounds (TripoSR 49.9% median, Hy3D 31.2%,
0/4 within 10% for both). Frame ambiguity is comparable across rounds
(3/15 vs 4/15), so it is not the driver.

The mesh quality itself is not in question — the `outerrim_rapid_smelter`
reconstruction is structurally faithful (both nacelle pairs, blocky forward
hull, angled rear), close to the meshy.ai result that motivated the round,
if slightly softer on panel edges. It is a good mesh of the wrong ship
proportions. `precept` is the clean illustration: one view yields a stubby
wide hull (aspect 1.13), the other a long slender one (2.93). Both are
internally coherent, plausible ships.

Profile correlation stays moderate while aspect swings (median r 0.67-0.73
across both rounds). That is the signature: the normalised width profile —
the shape — largely survives, while the length-to-width ratio does not.
The models recover *silhouette character* and invent *proportion*.

## What this means

Single-image 3D reconstructs a hull consistent with the photograph,
including its perspective. Absent a known camera, aspect is unrecoverable,
and a better prior produces a prettier mesh with the same bias. A generated
footprint is therefore a second artistic opinion, not evidence, and must not
be fused into `data/footprints/fused/` alongside measured picks.

This does not rule the technique out for *stylistic* uses where proportion
comes from elsewhere — e.g. taking shape character from the mesh while
holding aspect from the existing pipeline's `profile.json`. It does rule it
out as an independent source of footprint truth, which is what round 1 and
round 2 were both testing.

## Reproduce

    ~/hy3d-venv/bin/python run_hy3d.py --dry-run   # plan only, no GPU
    ~/hy3d-venv/bin/python run_hy3d.py             # 15 images, ~25 min
    ./extract_hy3d.sh                              # footprints via round-1 script
    python3 compare_hy3d.py                        # scored table above

`render_mesh.py <mesh.obj> <prefix>` writes shaded three-quarter / side /
top-down previews without needing GL.
