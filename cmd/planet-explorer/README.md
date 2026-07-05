# planet-explorer

Web-based parameter explorer for the planet generator. Compiles
`pkg/planetgen` to Wasm and renders live in a browser canvas.

## Build

The Wasm binary is a build artifact, not committed:

```bash
GOOS=js GOARCH=wasm go build \
    -o cmd/planet-explorer/web/planet-explorer.wasm \
    ./cmd/planet-explorer/wasm
```

For smaller binaries:

```bash
GOOS=js GOARCH=wasm go build -ldflags="-s -w" \
    -o cmd/planet-explorer/web/planet-explorer.wasm \
    ./cmd/planet-explorer/wasm
```

## Run (dev)

```bash
go run ./cmd/planet-explorer
# default: serves cmd/planet-explorer/web on http://localhost:8080
```

Flags:
- `-addr` (default `:8080`) — listen address
- `-web` (default `cmd/planet-explorer/web`) — static asset directory
- `-wasm` (default `cmd/planet-explorer/web/planet-explorer.wasm`) — Wasm binary path

## Workflow

1. Pick a planet type from the dropdown. The default profile JSON
   loads into the textarea.
2. Adjust slider panels in the left sidebar (each Tier-S algorithm
   adds its own panel).
3. Click **Regenerate** (or press Enter while focused on the sliders)
   to render at the selected face size.
4. The cube-map cross renders into the upper canvas; the equirect
   bake renders below.
5. Click **Export JSON** to copy the current profile into the
   clipboard. Paste back into `pkg/planetgen/profile.go` to
   commit a tuned default.

## Wasm-callable API

The Wasm binary exposes three functions on `js.Global()`:

| JS name | Args | Returns |
|---|---|---|
| `planetExplorerGenerate` | `profileJSON: string, seed: string, faceSize: int` | `Uint8Array` (cube-map cross PNG bytes) or error JSON string |
| `planetExplorerBakeEquirect` | `cubePNG: Uint8Array, width: int, height: int` | `Uint8Array` (equirect PNG bytes) or error JSON string |
| `planetExplorerDefaultProfile` | `planetType: string` | JSON string of the in-code default profile |

## Performance

Re-render runs on click, not per-frame. At face size 256 the full
Phase 1 pipeline finishes in under 1 s on a modern laptop. Face
size 1024 is the production size used by `cmd/generate-planet-maps`
batch mode and takes ~10–20 s in the browser.

## Patch Lab (Phase 13)

Full-sphere renders at production size (S_prod=1024) take too long to
tune interactively, and lowering `-face` to keep things fast bypasses
the sphere-global tectonic stages (plates, cratons, JFA coastal
distance), so a coarse preview isn't faithful either. **Patch Lab**
resolves this by computing tectonics once on the sphere at a modest
face size (S_tect, default 256 — plates + cratons + FX classification)
and then extracting a single **512×512 flat window** on one cube face,
addressed at virtual production resolution S_prod=1024. All 13
downstream layers (tectonic base/FX, control noise, smoothing,
normalize, coastal noise, erosion, craters, rivers, climate, biome
color, waterlines, civ) run only on that 512² patch, each with a
dirty-tracked per-layer cache — a slider change re-renders only the
affected layer suffix onward, not the whole stack. Every layer's
output is byte-exact-golden-tested against a fixed seed, so drift in
any single stage is caught precisely.

When you're happy with the tuned profile, **Go!** hands off to the
existing, unchanged full production render at S_prod and switches back
to the normal sphere/cube-map/equirect views.

### Using it

- **Patch Lab** button (next to Regenerate) enters the mode; it
  requires a crust-enabled archetype (the legacy non-crust path has no
  tectonic fields to extract) and swaps out the rotating-sphere/
  cube-map/equirect canvases for the patch canvas + minimap.
- The **layer rail** lists all 13 layers in pipeline order; picking a
  row renders the stack up to (and including) that layer, using cached
  output for everything upstream.
- The **View** selector shows the current layer's output as **Color**
  (biome/palette colorization), **Height** (grayscale heightmap), or
  **Tectonic** (plate/craton/FX debug overlay).
- **Next window** cycles through the ranked candidate windows returned
  by the sphere-side picker (scored on FX-class diversity, boundary
  activity, craton edges, and land/ocean mix) — the first candidate is
  the smart-picked default.
- The **Sea level** slider live-overrides the waterline layer's ocean
  gate without recomputing anything upstream.
- Every other slider panel in the sidebar (Tectonic FX, control noise,
  erosion, coastal, climate, civ, …) also drives the patch view while
  Patch Lab is open — editing a param marks its owning layer (and
  everything downstream of it) dirty and debounces a re-render.
- **Go! (full render)** exits Patch Lab and runs the normal production
  pipeline with the current profile.

### Known divergences from production

Patch Lab is a faithful crop of the production render modulo four
documented, intentional approximations — summarized here; see
[`docs/superpowers/specs/2026-07-02-patch-lab-design.md`](../../docs/superpowers/specs/2026-07-02-patch-lab-design.md)
§7 for the canonical list:

1. **Coastal distance** is computed patch-locally (a two-pass chamfer
   over the 512² window), not via production's sphere-global JFA — the
   coast distance near the window edge can differ slightly from a real
   crop.
2. **Rain-shadow wind walks** clamp at the patch boundary instead of
   continuing as a great-circle walk across the sphere, so upwind
   terrain outside the window has no effect.
3. **Tectonic fields are upsampled** from the S_tect sphere (bilinear/
   nearest crop) rather than computed natively at S_prod, which smooths
   boundary detail slightly.
4. **Sea level** is the S_tect sphere's post-flow quantile, passed in
   as a scalar (a patch-local quantile would not be representative of
   the globe) — absent a slider override this is the same value
   production computes, just sourced from a coarser sphere.

## Planet picker and per-planet save (Phase 5)

The header bar exposes a **Planet** dropdown alongside the existing **Type** dropdown. It lists every JSON file under `data/planet-profiles/` (configurable via `-profiles-dir`).

- Selecting a planet loads its envelope from the server, swaps the slider state to the envelope's `profile`, and enables **Save**.
- **Save** PUTs the current slider state back to the selected slug as `handTuned: true`. The file is overwritten on disk; check `git diff` to review.
- **Save as new…** prompts for a slug (`[a-z0-9_]+`); if the slug already exists, you'll be asked to confirm overwrite.
- Changing the **Type** dropdown clears the Planet selection — you're back to the in-code defaults until you reselect a planet.

`data/planet-profiles/` is normal git-tracked content. Commit hand-tunes alongside any code changes that motivated them.

The `-readonly` flag turns the server into a viewer: PUTs return 405. Useful for demos or shared dev servers.

To bake or refresh the canonical (non-hand-tuned) envelopes, use `cmd/tools/seed-planet-profiles`.
