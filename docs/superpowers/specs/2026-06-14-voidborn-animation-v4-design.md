# Voidborn Animation v4 — Form Animations (shell-growth + gas-swirl)

**Date:** 2026-06-14
**Status:** Approved, ready for implementation plan
**Builds on:** the procedural animation pipeline (`tools/animate.py`) shipped in v1–v3. Adds two new loop-seamless effects for the non-humanoid "Beyond the Human Form" concept renders.

## Context

After rendering 10 non-humanoid Voidborn forms, the user selected favorites and identified which would animate well. The current pipeline is **image-space only** (displacement warps via `remap`, masking, compositing, and real vector-geometry overlay like `polytope_overlay`). It is **not** a particle simulator and has **no kinematics**. Sorting the user's animation ideas against that constraint:

- **Buildable now (geometry/coverage effects):** the dimensional-projection shell-growth and the gas containment-cage side-cycling.
- **Approximate-able now:** the gas hurricane swirl, via curl-noise advection — a convincing approximation, not a true fluid sim.
- **Deferred to the generative img2vid lane:** shard-accurate swarm dynamics, articulated tentacles (tentacle fabricator, medusa pulse), and any walk cycle (centauroid). These need particles/kinematics that image-warping cannot fake.

The user already wrote curl-noise advection for Jovian gas-giant clouds in the `kb-phase-0-cube-map` worktree (`pkg/planetgen/noise/curl.go`): a semi-Lagrangian **backward-trace** that subtracts `dt·(jet + amp·curlNoise)` per step and renormalizes. That is Go (sphere/3D) and cannot be imported into the Python pipeline, but the **technique ports directly**: the pipeline's `remap` already does inverse (backward) sampling, so a 2D backward-trace + curl flow drops straight in.

## Goals

1. `shell-growth` effect: a rotating mandala core plus a continuous rotating growth-wave shell with a bright fractal leading edge, over `form_projection_c.png`.
2. `gas-swirl` effect: curl-noise + vortex advection of the green gas (top-down-hurricane approximation), with an optional containment polygon cage whose side count cycles min→max→min one side at a time.
3. Both effects are **loop-seamless** (frame 0 == frame N) like every existing effect.
4. Wire two new clips into `anims.json` under a new group; extend the test suite.

## Non-goals

- No particle system, fluid solver, or kinematics. The gas swirl is explicitly an approximation.
- No animation of swarm/tentacle/medusa/centauroid in this round (generative lane).
- No changes to existing effects beyond reusing their helpers (`remap`, `_value_noise`, the polytope line rasterizer).
- No ML/generative model (pure numpy/Pillow + imageio-ffmpeg, consistent with v1–v3).

## Architecture

All changes in `tools/animate.py`; tests in `tools/test_animate.py`. Both effects keep the `fn(imgs, params, t) -> uint8 (H,W,3)` contract and register in `EFFECTS`.

### 1. `_curl_flow(h, w, freq, seed, phase) -> (fx, fy)`

A 2D divergence-free flow field from the curl of a scalar potential. Build a low-frequency scalar potential `Φ(x,y)` by sampling/upscaling `_value_noise` (reuse the existing value-noise; no new noise backend). The 2D curl of a scalar potential is `(∂Φ/∂y, −∂Φ/∂x)`, computed by central differences (`np.gradient`). `phase` shifts the potential's sample coordinates so the field evolves over the loop. Returns two `(H,W)` float arrays normalized to roughly unit magnitude. t-independent except through `phase` — callers pass a per-frame phase; cache the base potential keyed `(h, w, freq, seed)` in a module dict (`_FLOW_CACHE`), cleared in `render` like the others.

### 2. `_advect(img, fx, fy, vortex, turns, amp, iters, t) -> (H,W,3)`

Semi-Lagrangian backward-trace (2D port of `BackwardTrace`). For every pixel start at its coordinate and step backward `iters` times, each step subtracting `dt·(vortex_term + amp·curl_term)`:
- **Vortex term:** tangential (rotational) velocity around the image center, magnitude falling off with radius (e.g. `∝ 1/(r+r0)`), rotating `turns` full turns over the loop. This is the hurricane core.
- **Curl term:** sample `(fx, fy)` at the current traced position (via `remap` of the flow components, or nearest-index lookup).
After tracing, sample `img` at the final traced position with `remap`. Loop-seamlessness: the total angular advance of the vortex over `t∈[0,1)` is `2π·turns` (integer `turns`), and the curl `phase` advances `2π` over the loop, so the field returns to its start — frame 0 == frame N.

### 3. `gas_swirl(imgs, params, t)`

- `img = imgs[0]`.
- `fx, fy = _curl_flow(h, w, freq, seed, phase=2πt)`.
- `out = _advect(img, fx, fy, vortex, turns, amp, iters, t)`.
- If `params["cage"]`: overlay a cycling polygon cage via `_draw_cycling_cage` (below), screen/additive-blended like `polytope_overlay`.
- Params: `amp` (0.5), `vortex` (0.6), `turns` (1), `freq` (3.0), `iters` (3), `seed` (0), `cage` (False), `cage_min` (8), `cage_max` (10), `cage_color` ([0.6,1.0,0.7]), `cage_width` (3), `cage_glow` (8), `cage_size` (0.78). 1 input.

### 4. `_draw_cycling_cage(h, w, n_min, n_max, t, size, color, width, glow) -> (H,W,3) float`

Draw a regular polygon centered in the frame whose side count cycles `n_min → n_max → n_min` over the loop via a triangle wave on `t` (seamless). Smooth side emergence uses a **fractional-vertex polygon**: for fractional count `n = n_min + frac`, place `ceil(n)` vertices but interpolate the newest vertex outward from an edge midpoint by `frac` (so a side grows in/out continuously rather than snapping). Rasterize edges with the same 2×-supersampled AA-line + glow approach as `polytope_overlay` (factor out a shared `_draw_glow_lines(points, edges, ...)` helper if clean; otherwise mirror it). Returns an additive RGB layer.

### 5. `shell_growth(imgs, params, t)`

Over the crisp keyframe (`form_projection_c.png`):
- **Rotating mandala core:** rotate an inner disc (radius `core_r`) about the center by `2π·turns·t` via a polar remap (sample source at angle `θ − 2π·turns·t`), feathered into the static outer region by a radial mask so the edge is seamless. Integer `turns` → loop-seamless.
- **Rotating growth-wave shell:** define an angular coordinate `θ∈[0,1)` around center. The pink shell layer is present where `frac(θ − t) < coverage`, i.e. an arc of angular width `coverage` that rotates once per loop. Radial extent of the layer is the shell band `[core_r, 1.0]`, perturbed by `_value_noise` for a fractal edge.
- **Bright leading edge:** an additive glow band at the wave front (where `frac(θ − t)` is just under `coverage` / near 0), tinted brighter than the fill.
- **Composite:** `out = base*(1−a) + tint*a` for the pink fill `a`, plus the additive edge glow; the rotated core is blended in first.
- Params: `turns` (1), `coverage` (0.55), `tint` ([0.95,0.45,0.7]), `edge_glow` (1.4), `core_r` (0.32), `softness` (0.06), `grain` (3), `seed` (0). 1 input.

### 6. Wiring (`anims.json`)

New group `anim_forms` "Animations · Beyond the Human Form":
- `anim_form_projection.mp4` — src `form_projection_c.png`, effect `shell-growth`, params `{turns:1, coverage:0.55, edge_glow:1.4}`, duration 6, fps 24.
- `anim_form_gas.mp4` — src `form_gas_c.png` (a starred gas still), effect `gas-swirl`, params `{amp:0.5, vortex:0.7, turns:1, cage:true, cage_min:8, cage_max:10}`, duration 6, fps 24.

`EFFECTS` registry gains `"shell-growth"` and `"gas-swirl"`. `anims.json` stays tracked via `git add -f`; the `.mp4`s remain gitignored.

### 7. Gallery

The gallery builder (`tools/build_voidborn_gallery.py`) already renders any `anims.json` group as `<video>` tiles and the sticky nav auto-includes new groups — no builder change needed. Rebuild after rendering.

## Error handling

- Empty/single-color source → effects still run (advection of a flat field is a no-op; cage/shell draw over it).
- `n_min == n_max` → cage is a fixed polygon (no cycling), still valid.
- Odd dims handled by the existing `ensure_even` in the encode path.
- `iters == 0` → advection returns the source unchanged (still valid, just static).
- Flow/potential caching keyed on `(h, w, freq, seed)`; `render` clears `_FLOW_CACHE` at start alongside the other caches.

## Testing (extend `tools/test_animate.py`)

- **`_curl_flow`:** returns two `(H,W)` arrays; the field is approximately divergence-free (mean |divergence| small vs mean |field|); different seeds differ.
- **`_advect` loop:** advecting a test image at `t=0` ≈ source; at mid-`t` it differs (pixels moved); frame at `t→1` returns to `t=0` within ≤1 uint8.
- **`gas_swirl`:** output shape/dtype correct; loop-seamless (frame0≈frameN ≤1); with `cage:true` the output differs from `cage:false` (cage drawn); advection moved pixels at mid-loop.
- **`_draw_cycling_cage`:** side count at `t=0` is `n_min`, at `t=0.5` is near `n_max`, back to `n_min` at `t→1` (assert via counting bright vertices or comparing to reference polygon coverage); additive layer is non-negative.
- **`shell_growth`:** output shape/dtype; loop-seamless; pink-channel coverage over the shell band varies across t (grows/rotates) and returns at the loop point; the rotating core region changes between t=0 and t=0.25.
- **Registry:** both effects resolvable via `EFFECTS` and runnable end-to-end through `render` to a small mp4 (frame count == duration·fps).

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -v`.

## Verification (whole feature)

- All tests pass.
- `./tools/animate --batch voidborn-concepts/anims.json` → renders the 2 new clips with `0 skipped`.
- Visual review: projection shell-growth shows a rotating core + a pink wave sweeping the shell with a bright fractal leading edge; gas-swirl shows the green gas churning in a top-down rotation with the containment cage cycling its side count.
- `python3 tools/build_voidborn_gallery.py` → the new `anim_forms` group appears with 2 `<video>` tiles and a nav chip.
- `git status`: tracked changes only `tools/animate.py`, `tools/test_animate.py`, `voidborn-concepts/anims.json`, and this spec/plan; `.mp4`s remain gitignored.

## Notes

- Loop-seamlessness is the hard constraint for both effects; the design pins it via integer-turn rotation + full-cycle phase advance (advection) and `frac(θ − t)` angular waves (shell), the same periodic-in-t discipline used throughout v1–v3.
- `gas-swirl` is a deliberate approximation; the spec records that true volumetric churn belongs to the generative lane so the limitation is not mistaken for a bug.
- Expect a likely second tuning pass on `coverage`/`edge_glow` (shell) and `amp`/`vortex`/`freq` (gas) via `anims.json` after the first visual review.
