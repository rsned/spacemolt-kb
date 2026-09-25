# Cinematic battle viewer: Hy3D hulls + real system backdrop

Date: 2026-09-24
Status: approved design, pre-plan

## Goal

See our Hunyuan3D ship meshes (and our planet textures / star metadata) inside
the spacemolt.com battle cinematic (`/battles/<id>/cinematic`), running
locally. Improved visuals first; upstream merge is a later, separate decision
(the devs invited a PR "if it doesn't break things").

Non-goals for this phase: mesh decimation/compression, the assets.spacemolt.com
publishing workflow, committing binaries to www (its `check:binaries` rejects
anything > 500 KiB), the distant instanced fleet, hull name decals, the
interactive mount-placement tool, and the new terrain generator's formats.

## Context: how the cinematic builds ships

- Code: `SpaceMolt/www` → `src/lib/cinema/` (three.js 0.185, Next 16, bun test).
- `createShip(appearance, seed, detail, hardware)` in `ships.ts` returns a
  `THREE.Group`: +X bow, +Y dorsal, geometry normalized to length 1; the scene
  scales it by `cinemaHullWorldSize(appearance)`.
- Everything is procedural at runtime — there is no stored model or sidecar.
  The group carries the contracts the rest of the scene depends on:
  - meshes named `hull|armor|dark|metal` → contact bounds, boarding, damage
    darkening and cloak (`scene.ts` ~254–290, ~670–681);
  - `userData.weaponRig` + a per-vertex `cinemaMount` attribute → turrets are
    rotated in the vertex shader; beams leave real muzzles (`ship-weapons.ts`,
    `MAX_WEAPON_MOUNTS = 12`, mount = `{family, pivot, muzzle, normal}`);
  - engine meshes with `userData.engine` / `baseIntensity` → thrust glow;
  - `userData.wreckPartRoles` / `wreckSurface` → `createShipWreckage` breakup
    (≤ 12 fragments, ≤ 30k vertices);
  - `addHullMarkings` name decals.
- Only the first 28 featured ships get `createShip(..., 'hero')`; the rest are
  instanced `'distant'` templates.
- `CinemaHardware.weapons` = per-family counts of weapons actually fired in the
  battle. Beam colour already follows damage type (`getWeaponColor(family,
  damageType)` in `weaponVisuals.ts`).
- Backdrop (`scene.ts` ~190–233): nebula shader, starfield, one fixed
  shader-banded planet + atmosphere, a warm sun sprite, and a directional sun
  light. Battle summaries carry `system_id`, `system_name`, optional
  `origin_poi`.

## Workspace

- Fork `SpaceMolt/www` to the user's GitHub; local clone `~/spacemolt-www`,
  branch `hy3d-hulls`. Run `pnpm dev` with `NEXT_PUBLIC_DEV_MODE=true` (no Clerk
  keys) against the live public API.
- Every new behaviour is gated behind the URL flag `?hulls=hy3d`. Without the
  flag the page must be byte-for-byte the current behaviour.
- Generated assets go to `public/cinema-hulls/` in the fork, gitignored there.

## Part A — Hy3D hulls

### A1. Export (kb repo, Python)

New `kb/data/mesh_bakeoff/export_cinema_hulls.py` (run with `~/sf3d-venv`;
note that venv's trimesh calls the numpy-2-removed `ndarray.ptp()` — avoid
`.extents`/`.bounds.ptp`, `trimesh.load` of GLB scenes; load OBJ with
`process=False` and use numpy).

For every art stem with a current KB id in `ship_id_map.json` (275 ships):

1. Load `mesh_adjusted.obj` if present (stretch/solo applied), else `mesh.obj`.
2. Transform into the cinema frame: bow +X, dorsal +Y, using the stored
   footprint frame (the same frame the SVG footprints/views use — they are
   bow-right and agree; apply `flip` from `adjustments-final.json`). Centre on
   the bbox (not vertex mean). Scale so bow–stern length = 1.
3. Write `<kb_id>.glb` (full 40k faces, no compression for this phase).
4. Write `<kb_id>.json` sidecar (schema below).
5. Write `manifest.json`: `{version, ships: {<kb_id>: {glb, sidecar}}}`.

`--out <dir>` (default `~/spacemolt-www/public/cinema-hulls`), `--only a,b,c`
for the first slice (~8 ships from battle `242b5fd8676d27c997f9dcd6b76a8cb7`),
then all.

### A2. Sidecar schema (also the future placement tool's output)

```json
{
  "version": 1,
  "id": "apocalypse",
  "source": "auto",
  "engines": [{"pos": [-0.49, -0.02, 0.18], "radius": 0.045}],
  "mounts": [{"pivot": [0.21, 0.09, 0.07], "normal": [0.3, 0.95, 0.0]}],
  "weaponSlots": 4
}
```

Coordinates are in the normalized cinema frame. `mounts` is a ranked candidate
list (≤ 12). `weaponSlots` comes from the KB `ships.weapon_slots` column.
`source` is `"auto"` now, `"placed"` when the tool writes it.

### A3. Guessing mounts and engines

- Engines: faces in the rear 10 % slab along X whose normal · (−X) > 0.7;
  cluster in YZ (DBSCAN); each cluster → engine at its centroid, radius from its
  YZ spread; keep ≤ 6, largest first.
- Weapons: faces in the front half whose normal points forward (+X) or dorsal
  (+Y) above a threshold; farthest-point sampling for spread; prefer
  port/starboard mirror pairs (±Z); rank; keep ≤ 12.
- Deterministic (fixed seed). Heuristic by design — the placement tool will
  overwrite it.

### A4. Renderer integration (www fork, TypeScript)

- New `src/lib/cinema/hull-models.ts`: when `?hulls=hy3d`, fetch the manifest
  and the GLBs for the ship classes in the film via three's `GLTFLoader`,
  parse sidecars, and resolve **before** the scene is built (scene construction
  is synchronous today). Map film `shipClass` → KB id; unknown → no model.
- `createShip` gains an optional `model` argument. When present:
  - the model's geometry becomes the mesh named `hull`, with the empire's
    existing `materialIdentity` hull material (meshes are uncoloured);
  - engines are created with the existing `engine()` helper at sidecar
    positions, keeping the engine glow contract;
  - N turrets are added at the first N sidecar mounts using the existing turret
    geometry + `tagWeaponGeometry` + `rig.mounts.push`, so aiming/firing work
    unchanged. N = weapons fired (sum of `hardware.weapons`), capped by
    `weaponSlots`; fall back to `weaponSlots` when nothing fired;
  - hull markings are skipped for model hulls.
- Turret tint: add a small emissive element on each turret coloured by
  `getWeaponColor(family, damageType)`.
- Shaped shield: for model hulls, build a shell by offsetting vertices along
  their normals by a fixed fraction of length (≈ 5 %), rendered with the
  existing shield `ShaderMaterial` and timing, in place of the ellipsoid.
- Wreckage: first check `createShipWreckage` on a single-part hull. If the
  breakup is poor, split the hull into ~8 chunks along X and record
  `wreckPartRoles`.
- Distant instanced fleet stays procedural.
- If the model hull's normals look faceted, compute smooth normals at export.
  (The speckle in `render_*.png` is the point-cloud preview renderer, not the
  mesh.)

## Part B — Real system backdrop (after A)

- Export adds `systems.json`: per system, its planets (`poi_metadata_planets`:
  `poi_id`, `planet_class`, `radius_km`, and texture path if one exists in
  `kb/images/planets/<system>_<poi>.png`, 2000×1000 equirectangular) and its
  star (`poi_metadata_stars`: `star_class`, `color_hex`, `size_multiplier`,
  `render_size`). 411 planet textures exist today; 905 planets have class data.
- Planet: pick the `origin_poi` planet if it is one, else the system's first
  planet. With a texture → equirect texture on the existing sphere; without →
  the current banded shader tinted by `planet_class`. Keep the atmosphere
  glow, tinted per class. Keyed by poi id so the new terrain generator's output
  later replaces textures with no code change.
- Star: sun sprite colour and directional-light colour ← `color_hex`; sprite
  scale ← `size_multiplier`/`render_size`. Unknown → the current warm sun.

## Testing

- `bun test`: axis transform, sidecar parsing/validation, mount-count rule
  (fired vs slots), manifest miss → procedural fallback, flag-off → no loader
  call.
- Python: engine/mount guessing on a synthetic box-with-nozzles mesh.
- Visual: fixed list of battles (starting with
  `242b5fd8676d27c997f9dcd6b76a8cb7`), before/after screenshots.
- `pnpm lint` (`tsc --noEmit`) passes in the fork.

## Open risks

- Some art stems lack a KB id (120 legacy) and 67 catalog ships have no art —
  those stay procedural.
- `mesh_adjusted.obj` exists for 219 ships; stretch was tuned for footprints,
  verify it looks right in 3D.
- 28 hero ships × 40k triangles ≈ 1.1M triangles plus shadows — fine on the
  workstation, but not representative of production budgets.
