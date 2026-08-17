# Hero image → footprint SVG: the pipeline

How the SVGs in `data/footprints/hy3d-svg/` were produced, stage by stage.
One hero image in, one top-down hull-footprint SVG out. Everything runs
locally (RTX 2000 Ada 16GB); the only paid fallback is Meshy, needed for
2 of 402 ships. All scripts named below live in `data/mesh_bakeoff/`
(run them from that directory).

```
hero.png ──chroma key──► keyed RGBA ──Hunyuan3D-2──► mesh.obj ──extract──►
footprint polygon ──human triage + adjustments──► corrected polygon ──export──► ship_id.svg
```

## 1. Input: chromakey hero art

`~/Downloads/chromakeys/<stem>.png` — game hero renders, RGB on flat
magenta (nominally #FF00FF). Wildlife has no server art, so its heroes are
FLUX.1-schnell renders prompted onto the same flat magenta field
(`wildlife/gen_heroes*.sh`) — the rest of the pipeline is identical.

## 2. Chroma key → RGBA matte (`run_hy3d.py: chroma_key()`)

Four hardening layers, each added after a real failure class:

1. **Border-sampled key colour** — key against the median border colour,
   fall back to magenta when the border is close to it (osprey's hero is
   on white).
2. **Float32 distance key** (tolerance 60) — soft matte keeps antialiased
   edges. Float32, not int16: squared channel deltas overflow int16.
3. **Hue+flood shadow/backdrop removal** — drop shadows are *darkened*
   magenta (far by distance, magenta by hue). Only hue-matched pixels
   flood-connected to the image border are removed, so interior glow
   (voidborn membranes) survives while shadow puddles and painted
   backdrops go.
4. **Speckle removal** — surviving blobs <0.5% of the largest are
   compression junk; the model extrudes them into dangling sheets if left.

Plus a despill pass (clamp R,B toward G in kept fringe pixels).

## 3. Image → mesh: Hunyuan3D-2 shape-only

`run_hy3d.py` (alias `meshbake_b7.py`), `~/hy3d-venv`. Flow-matching DiT
producing an occupancy field, marched at octree 320; 50 steps, guidance
5.5, fixed seed 1234. Texture stage never loaded — footprints need
geometry only. Post: FloaterRemover, DegenerateFaceRemover, FaceReducer
→ `mesh.obj/glb/stl`, ~40k faces, watertight. ~90s/ship, peak ~5.3GB VRAM.
Run under `systemd-run --user --scope -p MemoryMax=20G` and the grep-safe
alias (the fleet-manager session pkills by over-matching patterns).

Known model behaviours: invents the hidden side (usually well — symmetric
fins/nacelles), compresses long hulls ~20% bow-stern (fixed in triage),
and occasionally hallucinates backdrop planes/L-frames — caught by
`detect_planes.py` (coplanar cluster ≥8% of area with an in-plane span
≥0.80; flat barges whitelisted), fixed by seed reroll or `cut_planes.py`.

## 4. Mesh → footprint polygon (`mesh_footprint.py`, alias `mb_fp_b7.py`)

Choose the top-down frame, project all vertices to 2D, union the
projected triangles (shapely) into the footprint polygon, then run the
stage-7 profile sampler (station widths, concavity, aspect) →
`footprint.json` + `profile.json`. Frame default is **'td'**: canonical
top-down along world Y — triage showed it right for essentially every
upright hero; PCA discovery is opt-in for the exceptions (one ship).

## 5. Human triage + adjustments (`make_triage_sheet.py` → `apply_adjustments.py`, alias `mb_adj_b7.py`)

`triage.html` shows hero / keyed / three candidate views / drawn outline
per ship, with localStorage-persisted controls. The reviewer exports an
adjustments JSON (canonical: `adjustments-final.json`, 241 ships) with
this vocabulary:

| key        | fixes                                                        |
|------------|--------------------------------------------------------------|
| `stretch`  | bow-stern compression (~120 ships, median ×1.2, max ×1.5)    |
| `sym`      | lopsided reconstruction of a symmetric hull (mirror-union, mirror axis searched for most compact union) |
| `flip`     | bow drawn facing the wrong way                               |
| `rot90`    | aspect≈1 coin-flip frames (wingspan ≈ hull length)           |
| `solo`     | companion bodies that aren't hull (launching drones)         |
| `top_view` | 'td' default; 'side'/'front'/'pca' for the exceptions        |

`apply_adjustments.py` re-runs extraction with the corrections through the
same sampler, so numbers stay comparable fleet-wide.

For creatures, pose belongs **upstream**: prompt the hero into an
overhead-readable pose ("stretched out", "gliding level") rather than
fixing it here — see the wildlife prototype (`wildlife/`).

## 6. Polygon → SVG (`make_svg_footprints.py`)

- Rotate to presentation frame (x = bow-stern), bow to the **right** via
  the wedge heuristic (narrow end forward), then apply the human `flip`
  verdicts on top.
- Name by KB catalog id via `ship_id_map.json` (verbatim > faction-prefix
  strip > fuzzy variants); duplicate art skipped, art-only legacy ships
  keep their stem + `data-kb-match="none"`.
- **Consumer contract:** corner origin — (0,0) is the viewBox TOP-LEFT =
  the stern-side/starboard-side corner; x stern→bow, y starboard→port
  (SVG y grows downward); hull length normalised to 1000 units, 10-unit
  margin all sides; single `<path>` with `fill-rule="evenodd"` (holes are
  real); scale is RELATIVE only. Provenance in `data-*` attributes:
  ship / art-stem / kb-match / aspect / frame-ambiguous / adjustments.
- `gallery.html` — filterable grid of every SVG.

## Reproducing / extending

```
# ships (full sweep):
~/hy3d-venv/bin/python meshbake_b7.py --all --out out-hy3d-full
./extract_full.sh out-hy3d-full
~/sf3d-venv/bin/python mb_adj_b7.py adjustments-final.json --dir out-hy3d-full
~/hy3d-venv/bin/python make_svg_footprints.py

# wildlife (or any new art source):
wildlife/gen_heroes.sh                                   # FLUX heroes on magenta
~/hy3d-venv/bin/python meshbake_b7.py --src wildlife/heroes --out out-wildlife <stems>
# then extract / adjust as above
```
