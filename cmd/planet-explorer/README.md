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

## Planet picker and per-planet save (Phase 5)

The header bar exposes a **Planet** dropdown alongside the existing **Type** dropdown. It lists every JSON file under `data/planet-profiles/` (configurable via `-profiles-dir`).

- Selecting a planet loads its envelope from the server, swaps the slider state to the envelope's `profile`, and enables **Save**.
- **Save** PUTs the current slider state back to the selected slug as `handTuned: true`. The file is overwritten on disk; check `git diff` to review.
- **Save as new…** prompts for a slug (`[a-z0-9_]+`); if the slug already exists, you'll be asked to confirm overwrite.
- Changing the **Type** dropdown clears the Planet selection — you're back to the in-code defaults until you reselect a planet.

`data/planet-profiles/` is normal git-tracked content. Commit hand-tunes alongside any code changes that motivated them.

The `-readonly` flag turns the server into a viewer: PUTs return 405. Useful for demos or shared dev servers.

To bake or refresh the canonical (non-hand-tuned) envelopes, use `cmd/tools/seed-planet-profiles`.
