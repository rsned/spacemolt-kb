# KB Ship Hangar — size-comparison lineup

Date: 2026-09-26
Status: approved design, pre-plan

## Goal

A KB page (`kb/ships/hangar.html`) in the style of the YouTube sci-fi ship
size-comparison videos: every SpaceMolt catalog ship floating above one light
studio floor, in size order, with the camera panning along the line. Built on
the Hy3D hull meshes already exported for the battle cinematic.

## Decisions (from brainstorming)

- **Size basis:** window-measured length `loa_m` from
  `data/footprints/scale/ship_scale_est.json` (206 measured, 69 ladder
  estimates). Reconciling with the cinematic's tier ladder, the devs' class
  canon and a recipe-cost estimate is a later, separate task (see Follow-ups).
- **Colour:** studio white / light grey, slightly glossy, with a thin empire
  accent stripe. Switch to Part C hero-projected colours when those exist.
- **Interaction:** video-style auto-pan along the rail (smallest → largest),
  user can take over (scrubber, drag, arrows, wheel); click a ship to fly in
  and orbit it; Esc / floor click returns to the rail.
- **Contents:** one line of **all current catalog ships** (361 today), sorted
  by length, with filter chips (empire, tier, category). Ships without a model
  (86 today) appear as **placeholders** in their size slot.
- Legacy art-only ships (120) are excluded (not in the catalog). Lengths shown
  in metres only.

## Data and assets

Generator: `data/mesh_bakeoff/export_hangar.py` (runs in `~/hy3d-venv`, which
has pymeshlab; reuses `cinema_frame` and `make_lod_variants.decimate`).

Outputs, committed under `kb/ships/hangar/`:

- `lineup.json` —
  `{version:1, ships:[{id, name, empire, tier, category, lengthM, lengthSource:"window"|"estimate", beamM, heightM, model: "models/<id>.glb" | null}]}`
  sorted by `lengthM` then `name`.
  - Modeled ships: length/beam from `ship_scale_est.json`; height from the
    mesh aspect (height/length of the cinema-frame GLB) × length.
  - Unmodeled ships: length from `compute_scale.py`'s ladder
    (`ladder_group_median[scale/group]`, else `ladder_scale_geomean[scale]`),
    flagged `lengthSource:"estimate"`; beam/height from median aspect of
    modeled ships in the same role group.
- `models/<id>.glb` — the cinema-frame mesh (+X bow, +Y dorsal, length 1)
  decimated to the **lineup budget** (default 6000 faces; final value chosen
  from the LOD review battle), uint16 indices. Scaled to metres at runtime.
- Budget: ~275 × 100–150 KB ≈ 30–40 MB (precedent: blueprint vignettes 23 MB).

## Scene

- Floor: large light blue-grey plane with a soft blurred reflection (or
  roughness-faked reflection) and contact shadows under each ship; fades to
  black space + starfield toward the horizon.
- Lighting: soft studio key from above-front, fill, and hemisphere; shadows
  from the key onto the floor.
- Ships float at a common height above the floor, laid along +X in size order
  at true relative length (metres), fixed gap proportional to neighbour size.
- Material: `MeshStandardMaterial` near-white, roughness ~0.45, metalness
  ~0.2; empire accent as a thin band (a second material on a slab of faces
  selected by height, or a decal strip) — the simplest approach that reads.
- Placeholder: translucent ghost lozenge (capsule scaled to length/beam/
  height), empire-tinted edge glow, "model pending" label.
- Labels: name + "142 m" (and "est." when estimated) lying on the floor in
  front of each ship (canvas-texture sprites or SDF text), readable at the rail
  camera angle.

## Camera and controls

- Rail: camera follows a spline above/in front of the line at a height and
  distance that scale with the focused ship's size (pulls back for large
  ships). Auto-pan speed constant in *ships per second* near small ships and
  slower in metres as ships grow, so each ship gets screen time.
- Scrubber: bottom slider + small thumbnail/tick strip; drag, ←/→, wheel move
  along the rail; auto-pan pauses on input, resumes after idle (toggle).
- Focus: click a ship → camera flies to it, OrbitControls around it; info card
  (name, tier, class, empire, length + source, link to its KB ship page).
  Esc / click floor → back to rail at that ship.
- Filters: chips for empire / tier / category; non-matching ships hide and the
  line re-packs with a short animation; URL query keeps filter state.
- Loading: GLBs load lazily as the camera approaches (a window of ±N ships);
  ghost shown until loaded; unload far ships only if memory becomes an issue.

## Tech

- Plain JS module page, three.js from the CDN the KB already allows
  (jsdelivr), with GLTFLoader + OrbitControls from the same version.
- No build step; page + `lineup.json` + `models/`.
- Linked from the ships index (`kb/ships/index.html`) and "Did you know".

## Testing

- Python unittest for `export_hangar.py`: ordering, estimate flagging and
  placeholder entries, face budget of written GLBs, uint16 indices, JSON shape.
- Browser check: page loads, rail pan runs, filter re-packs, focus/orbit,
  placeholders render, no console errors.

## Follow-ups (out of scope)

- Size reconciliation: window lengths vs tier ladder vs dev class canon vs a
  recipe-cost estimate (materials grow ~4.6×/tier; area-scaling ⇒ ~2.1×
  length/tier, volume-scaling ⇒ ~1.66×; across 271 ships materials ~ L^1.96).
- Part C colours; retired-ships toggle; feet/metres toggle.
