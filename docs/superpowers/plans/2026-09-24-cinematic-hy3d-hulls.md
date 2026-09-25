# Cinematic Hy3D Hulls Implementation Plan (Part A)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show our Hunyuan3D ship meshes, with engines, turrets, hull-shaped shields and breakup, in a local fork of the spacemolt.com battle cinematic, behind the URL flag `?hulls=hy3d`.

**Architecture:** A Python exporter in the kb repo turns each Hy3D mesh into a normalized cinema-frame GLB (+X bow, +Y dorsal, length 1) plus a JSON sidecar (engines, candidate mounts, weapon slots) and a manifest, written into the fork's gitignored `public/cinema-hulls/`. In the fork, a new `hull-models.ts` loads them before the scene mounts, and `createShip` gets one extra hull branch that feeds our geometry through the existing `add()` / `engine()` / `fitHardware()` machinery, so turrets, engine glow, damage, cloak and wreck breakup keep working unchanged.

**Tech Stack:** Python 3.12 (`~/sf3d-venv`: numpy 2, scipy, trimesh 4.0.5, shapely), stdlib `unittest`; TypeScript, three.js 0.185 (`three/addons/loaders/GLTFLoader.js`), Next 16, `bun test`.

**Spec:** `docs/superpowers/specs/2026-09-24-cinematic-hy3d-hulls-design.md` (Part A only; Part B backdrop and Part C hero colours get their own plans).

## Global Constraints

- Two repos: **kb** = `/home/robert/spacemolt/kb` (Python exporter, this plan); **fork** = `/home/robert/spacemolt-www` (branch `hy3d-hulls`).
- Every fork change is behind `?hulls=hy3d`. Without the flag, behaviour must be identical to upstream (no loader call, no geometry change).
- Never commit binaries to the fork (`check:binaries` limit 500 KiB). `public/cinema-hulls/` is gitignored there.
- The kb working tree has unrelated uncommitted changes: `git add` only the files named in each task, never `-A`.
- `~/sf3d-venv` trimesh calls the numpy-2-removed `ndarray.ptp()`: never use `Trimesh.extents`, `.bounds.ptp`, `Trimesh.export(...glb)`, or `trimesh.load` of GLB scenes. Load OBJ with `process=False`, then work in numpy. (`np.ptp(array)` the function is fine.)
- Mesh memory caps from the fleet sweep: long Python jobs run under `systemd-run --user --scope -p MemoryMax=8G -p MemorySwapMax=0`, after checking `free -h`.
- Cinema frame: +X bow, +Y dorsal, +Z starboard (right-handed), bbox-centred, bow-to-stern length = 1.
- pnpm isn't installed globally: use `npx -y pnpm <cmd>`.
- Test battle: `242b5fd8676d27c997f9dcd6b76a8cb7`. 20 of its 21 ship classes have art (only `magnate` doesn't). First export slice: `axiom,dirk,paradox,hells_bells,last_warning,prospect,theoria,absolute_entropy`.

## Deliberate deviations from the spec (found while planning)

1. **Turret placement:** `createShip`'s `fitHardware()` already raycasts turrets onto whatever geometry was `add`ed as hull (`deckAt`/`hullSurface` in `ships.ts`), using `CinemaHardware` counts. We reuse that instead of placing turrets at sidecar mounts. The sidecar still exports ranked `mounts`, but only for the future placement tool; nothing consumes them yet.
2. **Wreckage:** we always add the hull in 8 bow-to-stern slices. Each `add()` call gets its own `cinemaStructuralPart`, so `createShipWreckage` breaks the hull apart with no special code. This replaces the spec's "check first, then maybe split".
3. **No de-roll:** the export uses the footprint frame and bow/vflip/mirror flags but not `make_views`' 2D de-roll. Revisit only if hulls visibly list.
4. **Source mesh:** the export loads `mesh.obj` and re-applies solo + stretch itself (via `frame_for`/the footprint pipeline), rather than reading a precomputed `mesh_adjusted.obj`. Equivalent geometry, but one code path for all 402 stems instead of depending on a second per-stem artifact.

## File Structure

kb repo (`data/mesh_bakeoff/`):
- `cinema_frame.py`: mesh → cinema frame, vertex normals, minimal GLB writer. Pure functions plus `load_stem`.
- `hardpoints.py`: guesses engines and ranked mount candidates from a cinema-frame mesh.
- `export_cinema_hulls.py`: CLI that writes GLB, sidecar and manifest.
- `test_cinema_export.py`: unittest for all three.

fork (`src/lib/cinema/`):
- `hull-models.ts`: sidecar types and parsing, the URL flag, the loader, shield shell.
- `hull-models.test.ts`
- `ship-model-hull.ts`: `sliceHullGeometry`, `buildModelHull`, `addMountTints`.
- `ship-model-hull.test.ts`
- Modify `ships.ts` (createShip signature and hull branch), `scene.ts` (options, actor wiring, shield shell), `src/components/cinema/CinemaPlayer.tsx` (load before mount), `.gitignore`.

---

### Task 1: Fork, local dev baseline

**Files:**
- Modify: `/home/robert/spacemolt-www/.gitignore`
- Create: `/home/robert/spacemolt-www/.env.local` (gitignored already; verify)

**Interfaces:**
- Produces: a running `http://localhost:3000/battles/242b5fd8676d27c997f9dcd6b76a8cb7/cinematic` on branch `hy3d-hulls`, and baseline screenshots.

- [ ] **Step 1: Fork (outward-facing: confirm with the user before running)**

```bash
cd /home/robert/spacemolt-www
git fetch --unshallow origin
gh repo fork --remote --remote-name origin   # renames SpaceMolt/www to 'upstream'
git remote -v                                 # expect origin = <user>/www, upstream = SpaceMolt/www
git switch -c hy3d-hulls
```

- [ ] **Step 2: Ignore generated assets**

Append to `.gitignore`:

```
# Hy3D cinema hulls exported from the kb repo (binaries; never commit)
/public/cinema-hulls/
```

- [ ] **Step 3: Env + install**

```bash
cd /home/robert/spacemolt-www
git check-ignore -v .env.local            # must print a rule; if not, STOP
cat > .env.local <<'EOF'
NEXT_PUBLIC_DEV_MODE=true
EOF
npx -y pnpm install
node scripts/fetch-catalog.mjs
```

- [ ] **Step 4: Run and verify the baseline**

Run `npx -y pnpm dev` in the background, then open
`http://localhost:3000/battles/242b5fd8676d27c997f9dcd6b76a8cb7/cinematic`.
Expected: the cinematic plays, matching production. If Clerk refuses to start without keys, STOP and report the exact error. Don't invent keys.
Add `cinema-shots/` to `/home/robert/spacemolt/kb/data/mesh_bakeoff/.gitignore`, then save 3 screenshots (opening, a mid-battle broadside, an explosion) to `data/mesh_bakeoff/cinema-shots/baseline-{1,2,3}.png`.

- [ ] **Step 5: Baseline tests and types pass**

```bash
cd /home/robert/spacemolt-www && bun test src/lib/cinema && npx -y pnpm lint
```

Expected: all pass. Record the test count to compare against later.

- [ ] **Step 6: Commit**

```bash
cd /home/robert/spacemolt-www && git add .gitignore && git commit -m "chore: ignore exported Hy3D cinema hulls"
cd /home/robert/spacemolt/kb && git add data/mesh_bakeoff/.gitignore && git commit -m "chore(mesh): ignore cinema screenshot scratch dir"
```

---

### Task 2: Cinema-frame transform + GLB writer (kb)

**Files:**
- Create: `/home/robert/spacemolt/kb/data/mesh_bakeoff/cinema_frame.py`
- Test: `/home/robert/spacemolt/kb/data/mesh_bakeoff/test_cinema_export.py`

**Interfaces:**
- Consumes: `make_views.frame_for(verts, adj) -> (lateral, longitudinal, up)`, `apply_adjustments.solo_hull(mesh)`, `make_svg_footprints.bow_flip(rings, user_flip) -> bool`, `make_svg_footprints.rings_of(geojson)`.
- Produces:
  - `orient(verts: np.ndarray, faces: np.ndarray, adj: dict, bow_flipped: bool) -> tuple[np.ndarray(float32, N×3), np.ndarray(uint32, M×3)]`
  - `vertex_normals(verts, faces) -> np.ndarray(float32, N×3)`
  - `glb_bytes(verts, normals, faces) -> bytes`
  - `load_stem(stem: str, adj: dict) -> tuple[verts, faces, bow_flipped: bool]`

- [ ] **Step 1: Write failing tests**

```python
#!/usr/bin/env python3
"""Tests for the cinema hull export. Run from data/mesh_bakeoff:
    ~/sf3d-venv/bin/python -m unittest test_cinema_export -v
"""
import json
import struct
import unittest

import numpy as np

import cinema_frame as cf


def box(lo, hi):
    """Closed axis-aligned box, outward CCW winding."""
    x0, y0, z0 = lo
    x1, y1, z1 = hi
    v = np.array([[x0, y0, z0], [x1, y0, z0], [x1, y1, z0], [x0, y1, z0],
                  [x0, y0, z1], [x1, y0, z1], [x1, y1, z1], [x0, y1, z1]], float)
    f = np.array([[0, 2, 1], [0, 3, 2], [4, 5, 6], [4, 6, 7], [0, 1, 5], [0, 5, 4],
                  [3, 6, 2], [3, 7, 6], [0, 4, 7], [0, 7, 3], [1, 2, 6], [1, 6, 5]])
    return v, f


def signed_volume(v, f):
    return float(np.einsum("ij,ij->i", v[f[:, 0]], np.cross(v[f[:, 1]], v[f[:, 2]])).sum() / 6)


class OrientTest(unittest.TestCase):
    def setUp(self):
        # Hy3D-style world frame: long axis along world Z, up = world Y
        self.v, self.f = box((-0.3, -0.1, -1.0), (0.3, 0.1, 1.0))

    def test_length_normalized_and_centred(self):
        p, _ = cf.orient(self.v, self.f, {}, bow_flipped=False)
        self.assertAlmostEqual(float(np.ptp(p[:, 0])), 1.0, places=5)
        np.testing.assert_allclose((p.min(0) + p.max(0)) / 2, 0, atol=1e-6)

    def test_long_axis_becomes_x_up_stays_y(self):
        p, _ = cf.orient(self.v, self.f, {}, bow_flipped=False)
        ext = np.ptp(p, axis=0)
        self.assertAlmostEqual(float(ext[0]), 1.0, places=5)      # 2.0 long
        self.assertAlmostEqual(float(ext[1]), 0.1, places=5)      # 0.2 tall
        self.assertAlmostEqual(float(ext[2]), 0.3, places=5)      # 0.6 beam

    def test_bow_flip_negates_x(self):
        v = self.v.copy(); v[v[:, 2] > 0, 0] *= 0.2               # narrow +Z end
        a, _ = cf.orient(v, self.f, {}, bow_flipped=False)
        b, _ = cf.orient(v, self.f, {}, bow_flipped=True)
        np.testing.assert_allclose(a[:, 0], -b[:, 0], atol=1e-6)

    def test_vflip_negates_y(self):
        a, _ = cf.orient(self.v, self.f, {}, False)
        b, _ = cf.orient(self.v, self.f, {"vflip": True}, False)
        np.testing.assert_allclose(a[:, 1], -b[:, 1], atol=1e-6)

    def test_stretch_does_not_change_normalized_length(self):
        p, _ = cf.orient(self.v, self.f, {"stretch": 1.25}, False)
        self.assertAlmostEqual(float(np.ptp(p[:, 0])), 1.0, places=5)
        self.assertAlmostEqual(float(np.ptp(p[:, 2])), 0.3 / 1.25, places=5)

    def test_winding_outward_even_when_mirrored(self):
        for adj in ({}, {"mirror": True}):
            p, f = cf.orient(self.v, self.f, adj, False)
            self.assertGreater(signed_volume(p, f), 0)


class NormalsTest(unittest.TestCase):
    def test_unit_and_outward(self):
        v, f = box((-1, -1, -1), (1, 1, 1))
        n = cf.vertex_normals(v, f)
        np.testing.assert_allclose(np.linalg.norm(n, axis=1), 1, atol=1e-5)
        self.assertTrue(np.all(np.einsum("ij,ij->i", n, v) > 0))


class GlbTest(unittest.TestCase):
    def test_header_chunks_and_accessors(self):
        v, f = box((-.5, -.1, -.2), (.5, .1, .2))
        data = cf.glb_bytes(v.astype(np.float32), cf.vertex_normals(v, f), f.astype(np.uint32))
        magic, version, total = struct.unpack_from("<III", data, 0)
        self.assertEqual((magic, version, total), (0x46546C67, 2, len(data)))
        jlen, jtype = struct.unpack_from("<II", data, 12)
        self.assertEqual(jtype, 0x4E4F534A)
        gltf = json.loads(data[20:20 + jlen])
        acc = gltf["accessors"]
        self.assertEqual(acc[0]["count"], 8)
        self.assertEqual(acc[2]["count"], 36)
        self.assertEqual(gltf["meshes"][0]["primitives"][0]["attributes"], {"POSITION": 0, "NORMAL": 1})
        self.assertEqual(len(data) % 4, 0)


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run tests, verify they fail**

Run: `cd /home/robert/spacemolt/kb/data/mesh_bakeoff && ~/sf3d-venv/bin/python -m unittest test_cinema_export -v`
Expected: `ModuleNotFoundError: No module named 'cinema_frame'`

- [ ] **Step 3: Implement `cinema_frame.py`**

```python
#!/usr/bin/env python3
"""Hy3D mesh -> the battle cinematic's hull frame.

Cinema frame (SpaceMolt/www src/lib/cinema/ships.ts): +X bow, +Y dorsal,
+Z starboard (right-handed), bbox-centred, bow-to-stern length 1.

Uses the same footprint frame, stretch and bow verdict as the shipped
SVG footprints / side views (make_views.make_one), so every exported hull
points the way the KB drawings do.
"""
import json
import struct
import sys
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import apply_adjustments as aa                         # noqa: E402  (solo_hull)
from make_svg_footprints import bow_flip, rings_of     # noqa: E402
from make_views import frame_for                       # noqa: E402

SWEEP = HERE / "out-hy3d-full"


def _signed_volume(v: np.ndarray, f: np.ndarray) -> float:
    return float(np.einsum("ij,ij->i", v[f[:, 0]], np.cross(v[f[:, 1]], v[f[:, 2]])).sum() / 6)


def orient(verts: np.ndarray, faces: np.ndarray, adj: dict, bow_flipped: bool):
    """Pure transform into the cinema frame. Returns (float32 verts, uint32 faces)."""
    verts = np.asarray(verts, dtype=float)
    faces = np.asarray(faces, dtype=np.int64)
    centroid = verts.mean(axis=0)
    _lateral, longitudinal, up = frame_for(verts, adj)
    c = verts - centroid
    stretch = float(adj.get("stretch", 1.0))
    if abs(stretch - 1.0) > 1e-3:
        c = c + (stretch - 1.0) * np.outer(c @ longitudinal, longitudinal)
    xdir = -longitudinal if bow_flipped else longitudinal
    ydir = -up if adj.get("vflip") else up
    zdir = np.cross(xdir, ydir)          # proper rotation: keeps true chirality
    if adj.get("mirror"):
        zdir = -zdir                     # deliberate port/starboard swap
    p = np.column_stack([c @ xdir, c @ ydir, c @ zdir])
    lo, hi = p.min(axis=0), p.max(axis=0)
    p = (p - (lo + hi) / 2) / (hi[0] - lo[0])
    if _signed_volume(p, faces) < 0:     # mirror (or an inside-out source) flips winding
        faces = faces[:, ::-1]
    return p.astype(np.float32), np.ascontiguousarray(faces, dtype=np.uint32)


def vertex_normals(verts: np.ndarray, faces: np.ndarray) -> np.ndarray:
    """Area-weighted smooth vertex normals (numpy only: no trimesh .ptp paths)."""
    v = np.asarray(verts, dtype=float)
    f = np.asarray(faces, dtype=np.int64)
    fn = np.cross(v[f[:, 1]] - v[f[:, 0]], v[f[:, 2]] - v[f[:, 0]])
    n = np.zeros_like(v)
    for k in range(3):
        np.add.at(n, f[:, k], fn)
    n /= np.maximum(np.linalg.norm(n, axis=1, keepdims=True), 1e-12)
    return n.astype(np.float32)


def glb_bytes(verts: np.ndarray, normals: np.ndarray, faces: np.ndarray) -> bytes:
    """Minimal glTF 2.0 binary: one mesh, POSITION + NORMAL + uint32 indices."""
    v = np.ascontiguousarray(verts, dtype="<f4")
    n = np.ascontiguousarray(normals, dtype="<f4")
    idx = np.ascontiguousarray(faces, dtype="<u4").ravel()
    blob = v.tobytes() + n.tobytes() + idx.tobytes()
    blob += b"\0" * (-len(blob) % 4)
    lv, ln = v.nbytes, n.nbytes
    gltf = {
        "asset": {"version": "2.0", "generator": "kb export_cinema_hulls"},
        "scene": 0, "scenes": [{"nodes": [0]}], "nodes": [{"mesh": 0, "name": "hull"}],
        "meshes": [{"primitives": [{"attributes": {"POSITION": 0, "NORMAL": 1}, "indices": 2}]}],
        "buffers": [{"byteLength": len(blob)}],
        "bufferViews": [
            {"buffer": 0, "byteOffset": 0, "byteLength": lv, "target": 34962},
            {"buffer": 0, "byteOffset": lv, "byteLength": ln, "target": 34962},
            {"buffer": 0, "byteOffset": lv + ln, "byteLength": idx.nbytes, "target": 34963},
        ],
        "accessors": [
            {"bufferView": 0, "componentType": 5126, "count": len(v), "type": "VEC3",
             "min": v.min(axis=0).tolist(), "max": v.max(axis=0).tolist()},
            {"bufferView": 1, "componentType": 5126, "count": len(n), "type": "VEC3"},
            {"bufferView": 2, "componentType": 5125, "count": int(idx.size), "type": "SCALAR"},
        ],
    }
    js = json.dumps(gltf, separators=(",", ":")).encode()
    js += b" " * (-len(js) % 4)
    total = 12 + 8 + len(js) + 8 + len(blob)
    return (struct.pack("<III", 0x46546C67, 2, total)
            + struct.pack("<II", len(js), 0x4E4F534A) + js
            + struct.pack("<II", len(blob), 0x004E4942) + blob)


def load_stem(stem: str, adj: dict):
    """Raw Hy3D mesh (solo applied) + the committed bow-right verdict."""
    import trimesh
    d = SWEEP / stem
    mesh = trimesh.load(d / "mesh.obj", force="mesh", process=False)
    if adj.get("solo"):
        mesh = aa.solo_hull(mesh)
    fp = json.loads((d / "footprint.json").read_text())
    flipped = bow_flip(rings_of(fp["polygon"]), bool(adj.get("flip")))
    return np.asarray(mesh.vertices, dtype=float), np.asarray(mesh.faces, dtype=np.int64), flipped
```

- [ ] **Step 4: Run tests, verify they pass**

Run: `cd /home/robert/spacemolt/kb/data/mesh_bakeoff && ~/sf3d-venv/bin/python -m unittest test_cinema_export -v`
Expected: 8 tests, OK.

- [ ] **Step 5: Real-mesh smoke check (bow direction)**

```bash
cd /home/robert/spacemolt/kb/data/mesh_bakeoff && ~/sf3d-venv/bin/python - <<'EOF'
import json, numpy as np, cinema_frame as cf
adj = json.load(open("adjustments-final.json"))
for stem in ["crimson_dirk", "solarian_axiom", "voidborn_paradox"]:
    a = adj.get(stem, {})
    v, f, fl = cf.load_stem(stem, a)
    p, f2 = cf.orient(v, f, a, fl)
    ext = np.ptp(p, axis=0)
    # bow is the narrow end: beam near +X should be smaller than near -X
    fore = np.ptp(p[p[:, 0] > .3, 2]); aft = np.ptp(p[p[:, 0] < -.3, 2])
    print(stem, ext.round(3), "fore/aft beam", round(float(fore), 3), round(float(aft), 3))
EOF
```

Expected: x extent 1.0 for each. For wedge-shaped hulls, fore beam ≤ aft beam. (The committed SVGs use the same bow verdict, so a disagreement here means a bug in `orient`, not in the data.)

- [ ] **Step 6: Commit**

```bash
cd /home/robert/spacemolt/kb && git add data/mesh_bakeoff/cinema_frame.py data/mesh_bakeoff/test_cinema_export.py
git commit -m "feat(mesh): cinema-frame transform and minimal GLB writer for Hy3D hulls"
```

---

### Task 3: Engine and mount guessing (kb)

**Files:**
- Create: `/home/robert/spacemolt/kb/data/mesh_bakeoff/hardpoints.py`
- Modify: `/home/robert/spacemolt/kb/data/mesh_bakeoff/test_cinema_export.py` (append test classes)

**Interfaces:**
- Consumes: cinema-frame `verts` (float, N×3) and `faces` (M×3) from Task 2.
- Produces:
  - `guess_engines(verts, faces, max_engines=6) -> list[{"pos": [x, y, z], "radius": r}]`
  - `guess_mounts(verts, faces, max_mounts=12) -> list[{"pivot": [x, y, z], "normal": [x, y, z]}]`

- [ ] **Step 1: Append failing tests to `test_cinema_export.py`** (before the `if __name__` block)

```python
import hardpoints as hp


def merge(*parts):
    vs, fs, off = [], [], 0
    for v, f in parts:
        vs.append(v); fs.append(f + off); off += len(v)
    return np.vstack(vs), np.vstack(fs)


def subdivide(v, f, times):
    """Midpoint 4-split, enough to give flat faces many candidate triangles."""
    for _ in range(times):
        mids = {}
        verts = list(map(tuple, v))
        def mid(a, b):
            key = (min(a, b), max(a, b))
            if key not in mids:
                mids[key] = len(verts)
                verts.append(tuple((np.asarray(verts[a]) + np.asarray(verts[b])) / 2))
            return mids[key]
        nf = []
        for a, b, c in f:
            ab, bc, ca = mid(a, b), mid(b, c), mid(c, a)
            nf += [[a, ab, ca], [ab, b, bc], [ca, bc, c], [ab, bc, ca]]
        v, f = np.array(verts), np.array(nf)
    return v, f


class EnginesTest(unittest.TestCase):
    def test_two_nozzles_found_and_rear_wall_rejected(self):
        hull = box((-.5, -.1, -.15), (.5, .1, .15))
        n1 = box((-.55, -.02, .08), (-.5, .02, .12))
        n2 = box((-.55, -.02, -.12), (-.5, .02, -.08))
        v, f = merge(hull, n1, n2)
        engines = hp.guess_engines(v, f)
        self.assertEqual(len(engines), 2)
        zs = sorted(e["pos"][2] for e in engines)
        self.assertAlmostEqual(zs[0], -.1, places=2)
        self.assertAlmostEqual(zs[1], .1, places=2)
        for e in engines:
            self.assertAlmostEqual(e["pos"][0], -.55, places=2)
            self.assertTrue(.015 <= e["radius"] <= .05)

    def test_fallback_single_stern_engine(self):
        v, f = box((-.5, -.1, -.15), (.5, .1, .15))
        engines = hp.guess_engines(v, f)
        self.assertEqual(len(engines), 1)
        self.assertAlmostEqual(engines[0]["pos"][0], -.5, places=3)
        self.assertAlmostEqual(engines[0]["pos"][2], 0, places=3)


class MountsTest(unittest.TestCase):
    def setUp(self):
        self.v, self.f = subdivide(*box((-.5, -.1, -.15), (.5, .1, .15)), 3)

    def test_mounts_forward_unit_normals_capped(self):
        mounts = hp.guess_mounts(self.v, self.f)
        self.assertTrue(0 < len(mounts) <= 12)
        for m in mounts:
            self.assertGreater(m["pivot"][0], 0)
            self.assertAlmostEqual(float(np.linalg.norm(m["normal"])), 1, places=4)

    def test_mirror_pairs_on_symmetric_hull(self):
        mounts = hp.guess_mounts(self.v, self.f)
        for m in mounts:
            x, y, z = m["pivot"]
            if abs(z) > .02:
                self.assertTrue(any(abs(o["pivot"][0] - x) < .04 and abs(o["pivot"][2] + z) < .04
                                    for o in mounts), f"no mirror for {m}")
```

- [ ] **Step 2: Run, verify fail**

Run: `~/sf3d-venv/bin/python -m unittest test_cinema_export -v`
Expected: `ModuleNotFoundError: No module named 'hardpoints'`

- [ ] **Step 3: Implement `hardpoints.py`**

```python
#!/usr/bin/env python3
"""Heuristic engine + weapon-mount guesses for cinema-frame hulls
(+X bow, +Y dorsal, length 1). Deliberately rough: the future placement
tool overwrites the sidecar with source "placed"."""
import numpy as np
from scipy.cluster.hierarchy import fclusterdata

REAR_SLAB = 0.10         # rear fraction of length searched for nozzles
FACING = 0.7             # normal . (-X) for a nozzle face
LINK = 0.03              # single-linkage distance (length units) in YZ
MIN_AREA_FRAC = 0.02     # of all rear-facing area
WALL_FRAC = 0.4          # cluster spread > this * hull beam/height = a wall, not a nozzle
MAX_CLUSTER_PTS = 4000
R_MIN, R_MAX = 0.015, 0.12


def _faces(v, f):
    tri = v[f]
    cross = np.cross(tri[:, 1] - tri[:, 0], tri[:, 2] - tri[:, 0])
    area = np.linalg.norm(cross, axis=1) / 2
    normal = cross / np.maximum(2 * area[:, None], 1e-12)
    return tri.mean(axis=1), normal, area


def _r(x):
    return [round(float(c), 4) for c in x]


def guess_engines(verts, faces, max_engines=6):
    v, f = np.asarray(verts, float), np.asarray(faces, np.int64)
    cent, normal, area = _faces(v, f)
    xmin = v[:, 0].min()
    span_yz = np.ptp(v[:, 1:], axis=0).max()
    sel = (cent[:, 0] < xmin + REAR_SLAB) & (normal[:, 0] < -FACING) & (area > 0)
    engines = []
    if sel.sum() >= 2:
        idx = np.flatnonzero(sel)
        if len(idx) > MAX_CLUSTER_PTS:
            idx = np.random.default_rng(0).choice(idx, MAX_CLUSTER_PTS, replace=False)
        labels = fclusterdata(cent[idx][:, 1:], t=LINK, criterion="distance", method="single")
        total = area[idx].sum()
        for lab in np.unique(labels):
            m = idx[labels == lab]
            a = area[m]
            if a.sum() < MIN_AREA_FRAC * total:
                continue
            spread = np.ptp(v[f[m]].reshape(-1, 3)[:, 1:], axis=0).max()
            if spread > WALL_FRAC * span_yz:
                continue
            yz = np.average(cent[m][:, 1:], axis=0, weights=a)
            x = v[f[m]].reshape(-1, 3)[:, 0].min()
            engines.append((a.sum(), {"pos": _r([x, yz[0], yz[1]]),
                                      "radius": round(float(np.clip(spread / 2, R_MIN, R_MAX)), 4)}))
    engines = [e for _, e in sorted(engines, key=lambda t: -t[0])][:max_engines]
    if not engines:
        stern = v[v[:, 0] < xmin + 0.02]
        engines = [{"pos": _r([xmin, float(np.median(stern[:, 1])), 0.0]), "radius": 0.05}]
    return engines


def guess_mounts(verts, faces, max_mounts=12, pair_tol=0.04):
    v, f = np.asarray(verts, float), np.asarray(faces, np.int64)
    cent, normal, area = _faces(v, f)
    ok = (cent[:, 0] > 0) & ((normal[:, 0] > 0.5) | (normal[:, 1] > 0.6)) & (area > 0)
    cand = np.flatnonzero(ok)
    if not len(cand):
        return []
    seeds_pool = cand[cent[cand, 2] >= -1e-6]         # starboard + centreline seeds
    if not len(seeds_pool):
        seeds_pool = cand
    first = seeds_pool[np.argmax(cent[seeds_pool, 1])]  # highest dorsal point
    chosen = [first]
    dist = np.linalg.norm(cent[seeds_pool] - cent[first], axis=1)
    out = []

    def emit(i):
        out.append({"pivot": _r(cent[i]), "normal": _r(normal[i] / np.linalg.norm(normal[i]))})

    def mirror_of(i):
        target = cent[i] * np.array([1, 1, -1])
        d = np.linalg.norm(cent[cand] - target, axis=1)
        j = cand[np.argmin(d)]
        return j if d.min() < pair_tol else None

    while len(out) < max_mounts:
        i = chosen[-1]
        emit(i)
        if abs(cent[i, 2]) > 0.02 and len(out) < max_mounts:
            j = mirror_of(i)
            if j is not None:
                emit(j)
        k = int(np.argmax(dist))
        if dist[k] < 1e-6:
            break
        chosen.append(seeds_pool[k])
        dist = np.minimum(dist, np.linalg.norm(cent[seeds_pool] - cent[seeds_pool[k]], axis=1))
    return out[:max_mounts]
```

- [ ] **Step 4: Run, verify pass**

Run: `~/sf3d-venv/bin/python -m unittest test_cinema_export -v`
Expected: 12 tests, OK. If `test_mirror_pairs_on_symmetric_hull` fails because the 12-mount cap cuts a pair, the fix is to stop emitting a lone seed when only one slot remains (`if len(out) == max_mounts - 1 and abs(cent[i, 2]) > .02: break`), not to loosen the test.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb && git add data/mesh_bakeoff/hardpoints.py data/mesh_bakeoff/test_cinema_export.py
git commit -m "feat(mesh): heuristic engine and weapon-mount guesses for cinema hulls"
```

---

### Task 4: Export CLI (kb) + first slice

**Files:**
- Create: `/home/robert/spacemolt/kb/data/mesh_bakeoff/export_cinema_hulls.py`
- Modify: `test_cinema_export.py` (append `SidecarTest`)

**Interfaces:**
- Consumes: Tasks 2–3; `ship_id_map.json` (`{"mapping": {stem: {"id": kb_id, ...}}}`), `adjustments-final.json` (`{stem: adj}`), sqlite `ships.weapon_slots` in `~/spacemolt/spacemolt-knowledge.db`.
- Produces, in `--out` (default `~/spacemolt-www/public/cinema-hulls`):
  - `<kb_id>.glb`
  - `<kb_id>.json` = `{"version":1,"id","source":"auto","engines":[{pos,radius}],"mounts":[{pivot,normal}],"weaponSlots":int}`
  - `manifest.json` = `{"version":1,"ships":{kb_id:{"glb":"<kb_id>.glb","sidecar":"<kb_id>.json"}}}`. Merged, so `--only` runs accumulate.
  - `build_sidecar(kb_id: str, verts, faces, weapon_slots: int) -> dict`

- [ ] **Step 1: Append failing test**

```python
import export_cinema_hulls as ech


class SidecarTest(unittest.TestCase):
    def test_schema(self):
        v, f = box((-.5, -.1, -.15), (.5, .1, .15))
        s = ech.build_sidecar("dirk", v, f, 3)
        self.assertEqual(set(s), {"version", "id", "source", "engines", "mounts", "weaponSlots"})
        self.assertEqual((s["version"], s["id"], s["source"], s["weaponSlots"]), (1, "dirk", "auto", 3))
        json.dumps(s)   # must be plain JSON types
```

- [ ] **Step 2: Run, verify fail** (`No module named 'export_cinema_hulls'`)

- [ ] **Step 3: Implement `export_cinema_hulls.py`**

```python
#!/usr/bin/env python3
"""Export Hy3D hulls for the battle cinematic (SpaceMolt/www fork).

    ~/sf3d-venv/bin/python export_cinema_hulls.py [--only dirk,axiom] [--out DIR]

Writes <kb_id>.glb + <kb_id>.json sidecar + manifest.json. See
docs/superpowers/specs/2026-09-24-cinematic-hy3d-hulls-design.md.
"""
import argparse
import json
import sqlite3
import sys
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import cinema_frame as cf   # noqa: E402
import hardpoints as hp     # noqa: E402

DEFAULT_OUT = Path.home() / "spacemolt-www" / "public" / "cinema-hulls"
DEFAULT_DB = Path.home() / "spacemolt" / "spacemolt-knowledge.db"


def build_sidecar(kb_id, verts, faces, weapon_slots):
    return {"version": 1, "id": kb_id, "source": "auto",
            "engines": hp.guess_engines(verts, faces),
            "mounts": hp.guess_mounts(verts, faces),
            "weaponSlots": int(weapon_slots)}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", type=Path, default=DEFAULT_OUT)
    ap.add_argument("--db", type=Path, default=DEFAULT_DB)
    ap.add_argument("--only", default="", help="comma-separated KB ship ids")
    args = ap.parse_args()

    mapping = json.loads((HERE / "ship_id_map.json").read_text())["mapping"]
    adjustments = json.loads((HERE / "adjustments-final.json").read_text())
    stem_of = {m["id"]: stem for stem, m in mapping.items()}
    wanted = [s for s in args.only.split(",") if s] or sorted(stem_of)
    slots = dict(sqlite3.connect(args.db).execute("SELECT id, weapon_slots FROM ships").fetchall())

    args.out.mkdir(parents=True, exist_ok=True)
    manifest_path = args.out / "manifest.json"
    manifest = json.loads(manifest_path.read_text()) if manifest_path.exists() else {"version": 1, "ships": {}}

    failures = 0
    for kb_id in wanted:
        stem = stem_of.get(kb_id)
        if not stem or not (cf.SWEEP / stem / "mesh.obj").exists():
            print(f"{kb_id:32} SKIP (no mesh)")
            continue
        try:
            adj = adjustments.get(stem, {})
            v, f, flipped = cf.load_stem(stem, adj)
            p, f2 = cf.orient(v, f, adj, flipped)
            (args.out / f"{kb_id}.glb").write_bytes(cf.glb_bytes(p, cf.vertex_normals(p, f2), f2))
            sidecar = build_sidecar(kb_id, p, f2, slots.get(kb_id) or 0)
            (args.out / f"{kb_id}.json").write_text(json.dumps(sidecar, indent=1))
            manifest["ships"][kb_id] = {"glb": f"{kb_id}.glb", "sidecar": f"{kb_id}.json"}
            ext = np.ptp(p, axis=0)
            print(f"{kb_id:32} ok  beam={ext[2]:.3f} height={ext[1]:.3f} "
                  f"engines={len(sidecar['engines'])} mounts={len(sidecar['mounts'])} slots={sidecar['weaponSlots']}")
        except Exception as exc:   # one bad hull must not stop the fleet
            failures += 1
            print(f"{kb_id:32} FAIL {exc!r}")
    manifest["ships"] = dict(sorted(manifest["ships"].items()))
    manifest_path.write_text(json.dumps(manifest, indent=1))
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
```

- [ ] **Step 4: Run tests, verify pass** (13 tests OK)

- [ ] **Step 5: Export the first slice**

```bash
cd /home/robert/spacemolt/kb/data/mesh_bakeoff && free -h
systemd-run --user --scope -p MemoryMax=8G -p MemorySwapMax=0 \
  ~/sf3d-venv/bin/python export_cinema_hulls.py --only axiom,dirk,paradox,hells_bells,last_warning,prospect,theoria,absolute_entropy
ls -la ~/spacemolt-www/public/cinema-hulls/
```

Expected: 8 `ok` lines, each ~0.9–1.0 MB GLB, `manifest.json` with 8 ships. Every line should show `engines≥1`.

- [ ] **Step 6: Commit (kb only; the outputs are gitignored in the fork)**

```bash
cd /home/robert/spacemolt/kb && git add data/mesh_bakeoff/export_cinema_hulls.py data/mesh_bakeoff/test_cinema_export.py
git commit -m "feat(mesh): export Hy3D hulls + sidecars + manifest for the battle cinematic"
```

---

### Task 5: `hull-models.ts`: flag, sidecar parsing, loader, shield shell (fork)

**Files:**
- Create: `/home/robert/spacemolt-www/src/lib/cinema/hull-models.ts`
- Test: `/home/robert/spacemolt-www/src/lib/cinema/hull-models.test.ts`

**Interfaces:**
- Produces (exact exports):
  - `type Vec3 = [number, number, number]`
  - `interface HullEngine { pos: Vec3; radius: number }`
  - `interface HullMount { pivot: Vec3; normal: Vec3 }`
  - `interface HullSidecar { version: 1; id: string; source: 'auto' | 'placed'; engines: HullEngine[]; mounts: HullMount[]; weaponSlots: number }`
  - `interface HullModel { id: string; geometry: THREE.BufferGeometry; shell: THREE.BufferGeometry; beam: number; height: number; engines: HullEngine[]; mounts: HullMount[]; weaponSlots: number }`
  - `type HullModelMap = ReadonlyMap<string, HullModel>`
  - `interface HullModelIO { fetchJson(url: string): Promise<unknown>; loadGeometry(url: string): Promise<THREE.BufferGeometry> }`
  - `hullModelsEnabled(search: string): boolean`
  - `parseHullSidecar(raw: unknown): HullSidecar | undefined`
  - `createShieldShell(geometry: THREE.BufferGeometry, offset?: number): THREE.BufferGeometry`
  - `hullModelFromGeometry(sidecar: HullSidecar, geometry: THREE.BufferGeometry): HullModel`
  - `loadHullModels(classes: Iterable<string>, base?: string, io?: HullModelIO): Promise<HullModelMap>`
  - `disposeHullModels(models: HullModelMap): void`

- [ ] **Step 1: Write failing tests**

```ts
import { describe, expect, test } from 'bun:test'
import * as THREE from 'three'
import { createShieldShell, hullModelFromGeometry, hullModelsEnabled, loadHullModels, parseHullSidecar, type HullModelIO, type HullSidecar } from './hull-models'

const sidecar: HullSidecar = { version: 1, id: 'dirk', source: 'auto', engines: [{ pos: [-.5, 0, .1], radius: .03 }], mounts: [{ pivot: [.2, .1, 0], normal: [0, 1, 0] }], weaponSlots: 2 }

describe('hullModelsEnabled', () => {
  test('only the exact hy3d flag enables models', () => {
    expect(hullModelsEnabled('?hulls=hy3d')).toBe(true)
    expect(hullModelsEnabled('?q=1&hulls=hy3d')).toBe(true)
    expect(hullModelsEnabled('')).toBe(false)
    expect(hullModelsEnabled('?hulls=other')).toBe(false)
  })
})

describe('parseHullSidecar', () => {
  test('accepts the exported schema', () => {
    expect(parseHullSidecar(JSON.parse(JSON.stringify(sidecar)))).toEqual(sidecar)
  })
  test('rejects wrong version, bad vectors and bad radii', () => {
    expect(parseHullSidecar({ ...sidecar, version: 2 })).toBeUndefined()
    expect(parseHullSidecar({ ...sidecar, engines: [{ pos: [0, 0], radius: .03 }] })).toBeUndefined()
    expect(parseHullSidecar({ ...sidecar, engines: [{ pos: [0, 0, 0], radius: -1 }] })).toBeUndefined()
    expect(parseHullSidecar({ ...sidecar, mounts: [{ pivot: [0, 0, NaN], normal: [0, 1, 0] }] })).toBeUndefined()
    expect(parseHullSidecar(null)).toBeUndefined()
  })
})

describe('hullModelFromGeometry', () => {
  test('beam and height come from the normalized geometry', () => {
    const model = hullModelFromGeometry(sidecar, new THREE.BoxGeometry(1, .2, .3))
    expect(model.beam).toBeCloseTo(.3)
    expect(model.height).toBeCloseTo(.2)
    expect(model.geometry.getAttribute('normal')).toBeDefined()
  })
})

describe('createShieldShell', () => {
  test('pushes the surface outward along normals', () => {
    const shell = createShieldShell(new THREE.BoxGeometry(1, .2, .3), .05)
    shell.computeBoundingBox()
    const size = shell.boundingBox!.getSize(new THREE.Vector3())
    expect(size.x).toBeCloseTo(1.1)
    expect(size.y).toBeCloseTo(.3)
    expect(size.z).toBeCloseTo(.4)
  })
})

describe('loadHullModels', () => {
  const io = (files: Record<string, unknown>, loads: string[] = []): HullModelIO => ({
    fetchJson: async url => { if (!(url in files)) throw new Error(`404 ${url}`); return files[url] },
    loadGeometry: async url => { loads.push(url); return new THREE.BoxGeometry(1, .2, .3) },
  })
  const manifest = { version: 1, ships: { dirk: { glb: 'dirk.glb', sidecar: 'dirk.json' }, axiom: { glb: 'axiom.glb', sidecar: 'axiom.json' } } }

  test('missing manifest yields an empty map, not an error', async () => {
    expect((await loadHullModels(['dirk'], '/h', io({}))).size).toBe(0)
  })
  test('loads only requested classes that the manifest knows, once each', async () => {
    const loads: string[] = []
    const models = await loadHullModels(['dirk', 'dirk', 'magnate', ''], '/h', io({ '/h/manifest.json': manifest, '/h/dirk.json': sidecar }, loads))
    expect([...models.keys()]).toEqual(['dirk'])
    expect(loads).toEqual(['/h/dirk.glb'])
  })
  test('one broken ship does not drop the others', async () => {
    const models = await loadHullModels(['dirk', 'axiom'], '/h', io({ '/h/manifest.json': manifest, '/h/dirk.json': sidecar, '/h/axiom.json': { version: 9 } }))
    expect([...models.keys()]).toEqual(['dirk'])
  })
})
```

- [ ] **Step 2: Run, verify fail**

Run: `cd /home/robert/spacemolt-www && bun test src/lib/cinema/hull-models.test.ts`
Expected: fails with `Cannot find module './hull-models'`.

- [ ] **Step 3: Implement `hull-models.ts`**

```ts
import * as THREE from 'three'

/** Hy3D hull models exported by the kb repo (export_cinema_hulls.py). Frame
 * matches createShip: +X bow, +Y dorsal, +Z starboard, bow-to-stern length 1. */
export type Vec3 = [number, number, number]
export interface HullEngine { pos: Vec3; radius: number }
export interface HullMount { pivot: Vec3; normal: Vec3 }
export interface HullSidecar { version: 1; id: string; source: 'auto' | 'placed'; engines: HullEngine[]; mounts: HullMount[]; weaponSlots: number }
export interface HullModel { id: string; geometry: THREE.BufferGeometry; shell: THREE.BufferGeometry; beam: number; height: number; engines: HullEngine[]; mounts: HullMount[]; weaponSlots: number }
export type HullModelMap = ReadonlyMap<string, HullModel>
export interface HullModelIO { fetchJson(url: string): Promise<unknown>; loadGeometry(url: string): Promise<THREE.BufferGeometry> }

const SHIELD_OFFSET = .05

export function hullModelsEnabled(search: string): boolean {
  return new URLSearchParams(search).get('hulls') === 'hy3d'
}

const isVec3 = (value: unknown): value is Vec3 => Array.isArray(value) && value.length === 3 && value.every(n => typeof n === 'number' && Number.isFinite(n))
const record = (value: unknown): Record<string, unknown> | undefined => value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined

export function parseHullSidecar(raw: unknown): HullSidecar | undefined {
  const s = record(raw)
  if (!s || s.version !== 1 || typeof s.id !== 'string' || (s.source !== 'auto' && s.source !== 'placed')) return undefined
  if (!Array.isArray(s.engines) || !Array.isArray(s.mounts) || typeof s.weaponSlots !== 'number' || !Number.isInteger(s.weaponSlots) || s.weaponSlots < 0) return undefined
  const engines: HullEngine[] = []
  for (const item of s.engines) {
    const e = record(item)
    if (!e || !isVec3(e.pos) || typeof e.radius !== 'number' || !(e.radius > 0) || !Number.isFinite(e.radius)) return undefined
    engines.push({ pos: e.pos, radius: e.radius })
  }
  const mounts: HullMount[] = []
  for (const item of s.mounts) {
    const m = record(item)
    if (!m || !isVec3(m.pivot) || !isVec3(m.normal)) return undefined
    mounts.push({ pivot: m.pivot, normal: m.normal })
  }
  return { version: 1, id: s.id, source: s.source, engines, mounts, weaponSlots: s.weaponSlots }
}

/** Inflates along vertex normals: a hull-shaped bubble that stays a shell on
 * long or outrigged hulls, unlike uniform scaling about the centre. */
export function createShieldShell(geometry: THREE.BufferGeometry, offset = SHIELD_OFFSET): THREE.BufferGeometry {
  const shell = geometry.clone()
  if (!shell.getAttribute('normal')) shell.computeVertexNormals()
  const position = shell.getAttribute('position'), normal = shell.getAttribute('normal')
  for (let i = 0; i < position.count; i++) {
    position.setXYZ(i, position.getX(i) + normal.getX(i) * offset, position.getY(i) + normal.getY(i) * offset, position.getZ(i) + normal.getZ(i) * offset)
  }
  position.needsUpdate = true
  shell.computeBoundingSphere()
  return shell
}

export function hullModelFromGeometry(sidecar: HullSidecar, geometry: THREE.BufferGeometry): HullModel {
  if (!geometry.getAttribute('normal')) geometry.computeVertexNormals()
  geometry.computeBoundingBox()
  const size = geometry.boundingBox!.getSize(new THREE.Vector3())
  return { id: sidecar.id, geometry, shell: createShieldShell(geometry), beam: size.z, height: size.y,
    engines: sidecar.engines, mounts: sidecar.mounts, weaponSlots: sidecar.weaponSlots }
}

const browserIO: HullModelIO = {
  fetchJson: async url => {
    const response = await fetch(url)
    if (!response.ok) throw new Error(`${response.status} ${url}`)
    return response.json()
  },
  loadGeometry: async url => {
    const { GLTFLoader } = await import('three/addons/loaders/GLTFLoader.js')
    const gltf = await new GLTFLoader().loadAsync(url)
    let geometry: THREE.BufferGeometry | undefined
    gltf.scene.traverse(object => { if (!geometry && object instanceof THREE.Mesh) geometry = object.geometry })
    if (!geometry) throw new Error(`no mesh in ${url}`)
    return geometry
  },
}

export async function loadHullModels(classes: Iterable<string>, base = '/cinema-hulls', io: HullModelIO = browserIO): Promise<HullModelMap> {
  const models = new Map<string, HullModel>()
  let manifest: Record<string, unknown> | undefined
  try { manifest = record(record(await io.fetchJson(`${base}/manifest.json`))?.ships) } catch { return models }
  if (!manifest) return models
  const wanted = [...new Set(classes)].filter(id => id && record(manifest![id]))
  await Promise.all(wanted.map(async id => {
    const entry = record(manifest![id])!
    try {
      const sidecar = parseHullSidecar(await io.fetchJson(`${base}/${entry.sidecar}`))
      if (!sidecar) throw new Error('invalid sidecar')
      models.set(id, hullModelFromGeometry(sidecar, await io.loadGeometry(`${base}/${entry.glb}`)))
    } catch (error) {
      console.warn(`[cinema] hull model ${id} skipped:`, error)
    }
  }))
  return models
}

export function disposeHullModels(models: HullModelMap): void {
  for (const model of models.values()) { model.geometry.dispose(); model.shell.dispose() }
}
```

Note: in the "one broken ship" test the invalid sidecar's GLB is never loaded, because the sidecar is parsed first.

- [ ] **Step 4: Run, verify pass**

Run: `bun test src/lib/cinema/hull-models.test.ts`, expect all pass. Then run `npx -y pnpm lint`; expect no type errors. If `three/addons/loaders/GLTFLoader.js` lacks types, use `three/examples/jsm/loaders/GLTFLoader.js`, the same path style `hull-contact.ts` uses for ConvexHull.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt-www && git add src/lib/cinema/hull-models.ts src/lib/cinema/hull-models.test.ts
git commit -m "feat(cinema): opt-in Hy3D hull model loader, sidecar parsing and shield shell"
```

---

### Task 6: Model hull branch in `createShip` (fork)

**Files:**
- Create: `/home/robert/spacemolt-www/src/lib/cinema/ship-model-hull.ts`
- Test: `/home/robert/spacemolt-www/src/lib/cinema/ship-model-hull.test.ts`
- Modify: `/home/robert/spacemolt-www/src/lib/cinema/ships.ts`: the signature at line 102; the hull choice at ~line 353 (`const bespoke = buildSpecialHull(...) || buildEmpireHull(...)`); after `const { deckAt, detailSurfaces } = fitHardware()` at ~line 614.

**Interfaces:**
- Consumes: `HullModel` (Task 5); inside `createShip`, `context.add(geometry, material)`, `context.engine(x, y, z, radius)`, `rig: WeaponRig`, `getWeaponColor(family, damageType?)` from `./weapons`.
- Produces:
  - `sliceHullGeometry(geometry: THREE.BufferGeometry, slices?: number): THREE.BufferGeometry[]`
  - `buildModelHull(model: HullModel, c: { add: (g: THREE.BufferGeometry, material: 'hull') => void; engine: (x: number, y: number, z: number, radius: number) => void }): true`
  - `addMountTints(group: THREE.Group, rig: WeaponRig): void`
  - `createShip(appearance, seed, detail = 'hero', hardware?, model?: HullModel)`

- [ ] **Step 1: Write failing tests**

```ts
import { describe, expect, test } from 'bun:test'
import * as THREE from 'three'
import { resolveAppearance } from './appearance'
import { hullModelFromGeometry } from './hull-models'
import { createShip } from './ships'
import { sliceHullGeometry, MODEL_HULL_SLICES } from './ship-model-hull'
import type { WeaponRig } from './ship-weapons'

const fit = { source: 'modules' as const, weapons: { laser: 2 }, cargo: 0, mining: 0, salvage: 0, sensor: 0, defense: 0, utility: 0 }
const model = () => hullModelFromGeometry({ version: 1, id: 'box', source: 'auto', engines: [{ pos: [-.5, 0, .08], radius: .03 }, { pos: [-.5, 0, -.08], radius: .03 }], mounts: [], weaponSlots: 2 },
  new THREE.BoxGeometry(1, .2, .3, 16, 4, 4))

function dispose(group: THREE.Group) {
  group.traverse(object => { if (object instanceof THREE.Mesh) { object.geometry.dispose(); for (const m of [object.material].flat()) m.dispose() } })
}

describe('sliceHullGeometry', () => {
  test('keeps every triangle and splits bow to stern', () => {
    const source = new THREE.BoxGeometry(1, .2, .3, 16, 4, 4)
    const triangles = source.index!.count / 3
    const slices = sliceHullGeometry(source)
    expect(slices.length).toBe(MODEL_HULL_SLICES)
    expect(slices.reduce((n, g) => n + g.getAttribute('position').count / 3, 0)).toBe(triangles)
    for (const g of slices) expect(g.getAttribute('normal')).toBeDefined()
    expect(source.index).not.toBeNull()   // source untouched: shared by every actor using this class
  })
})

describe('createShip with a model hull', () => {
  test('uses the model surface, sidecar engines and fitted turrets', () => {
    const group = createShip(resolveAppearance('Cruiser', 'solarian', 2, 'Combat', 2), 7, 'hero', fit, model())
    try {
      const hull = group.getObjectByName('hull') as THREE.Mesh
      hull.geometry.computeBoundingBox()
      const size = hull.geometry.boundingBox!.getSize(new THREE.Vector3())
      expect(size.x).toBeGreaterThan(.95)
      expect(size.x).toBeLessThan(1.2)
      const cores = group.children.filter(o => o.userData.engine && !o.userData.plume && !o.userData.retrothruster)
      expect(cores.length).toBe(2)
      const rig = group.userData.weaponRig as WeaponRig
      expect(rig.mounts.length).toBeGreaterThan(0)
      for (const mount of rig.mounts) {
        expect(Math.abs(mount.pivot.x)).toBeLessThan(.65)
        expect(Math.abs(mount.pivot.y)).toBeLessThan(.3)
        expect(Math.abs(mount.pivot.z)).toBeLessThan(.3)
      }
      expect(group.children.filter(o => o.name === 'mount-tint').length).toBe(rig.mounts.length)
      const roles = Object.values(group.userData.wreckPartRoles as Record<number, string>)
      expect(roles.filter(role => role === 'hull').length).toBeGreaterThanOrEqual(2)
    } finally { dispose(group) }
  })

  test('without a model the ship is built exactly as before', () => {
    const appearance = resolveAppearance('Cruiser', 'solarian', 2, 'Combat', 2)
    const a = createShip(appearance, 7, 'hero', fit), b = createShip(appearance, 7, 'hero', fit, undefined)
    try {
      const count = (g: THREE.Group) => (g.getObjectByName('hull') as THREE.Mesh).geometry.getAttribute('position').count
      expect(count(a)).toBe(count(b))
      expect(a.children.some(o => o.name === 'mount-tint')).toBe(false)
    } finally { dispose(a); dispose(b) }
  })
})
```

- [ ] **Step 2: Run, verify fail** (`Cannot find module './ship-model-hull'`)

Run: `bun test src/lib/cinema/ship-model-hull.test.ts`

- [ ] **Step 3: Implement `ship-model-hull.ts`**

```ts
import * as THREE from 'three'
import type { HullModel } from './hull-models'
import type { WeaponRig } from './ship-weapons'
import { getWeaponColor } from './weapons'

/** Each slice is added separately, so createShipWreckage sees distinct
 * structural parts and breaks a scanned hull apart like a built one. */
export const MODEL_HULL_SLICES = 8
const TINT_RADIUS = .018

export function sliceHullGeometry(geometry: THREE.BufferGeometry, slices = MODEL_HULL_SLICES): THREE.BufferGeometry[] {
  const source = geometry.index ? geometry.toNonIndexed() : geometry.clone()
  if (!source.getAttribute('normal')) source.computeVertexNormals()
  const position = source.getAttribute('position'), normal = source.getAttribute('normal')
  source.computeBoundingBox()
  const { min, max } = source.boundingBox!
  const span = max.x - min.x || 1
  const buckets = Array.from({ length: slices }, () => ({ p: [] as number[], n: [] as number[] }))
  for (let i = 0; i < position.count; i += 3) {
    const cx = (position.getX(i) + position.getX(i + 1) + position.getX(i + 2)) / 3
    const bucket = buckets[Math.min(slices - 1, Math.max(0, Math.floor((cx - min.x) / span * slices)))]
    for (let j = i; j < i + 3; j++) {
      bucket.p.push(position.getX(j), position.getY(j), position.getZ(j))
      bucket.n.push(normal.getX(j), normal.getY(j), normal.getZ(j))
    }
  }
  source.dispose()
  return buckets.filter(b => b.p.length).map(b => {
    const slice = new THREE.BufferGeometry()
    slice.setAttribute('position', new THREE.Float32BufferAttribute(b.p, 3))
    slice.setAttribute('normal', new THREE.Float32BufferAttribute(b.n, 3))
    return slice
  })
}

export function buildModelHull(model: HullModel, c: { add: (geometry: THREE.BufferGeometry, material: 'hull') => void; engine: (x: number, y: number, z: number, radius: number) => void }): true {
  for (const slice of sliceHullGeometry(model.geometry)) c.add(slice, 'hull')
  for (const { pos: [x, y, z], radius } of model.engines) c.engine(x, y, z, radius)
  return true
}

/** A ring around each fitted turret base, coloured by weapon family. The ring
 * is symmetric about the mount normal, so traverse rotation never displaces it.
 * Standard material, so damage darkening and cloak fade apply like the hull. */
export function addMountTints(group: THREE.Group, rig: WeaponRig): void {
  const up = new THREE.Vector3(0, 0, 1)
  for (const mount of rig.mounts) {
    const color = getWeaponColor(mount.family)
    const ring = new THREE.Mesh(new THREE.TorusGeometry(TINT_RADIUS, TINT_RADIUS * .18, 6, 20),
      new THREE.MeshStandardMaterial({ color: 0x000000, emissive: color, emissiveIntensity: 2.2, toneMapped: false }))
    ring.name = 'mount-tint'
    ring.position.copy(mount.pivot)
    ring.quaternion.setFromUnitVectors(up, (mount.normal ?? new THREE.Vector3(0, 1, 0)).clone().normalize())
    group.add(ring)
  }
}
```

- [ ] **Step 4: Wire into `ships.ts`**

Import at the top with the other local imports:

```ts
import type { HullModel } from './hull-models'
import { addMountTints, buildModelHull } from './ship-model-hull'
```

Signature (line 102):

```ts
export function createShip(appearance: ShipAppearance, seed: number, detail: 'hero' | 'distant' = 'hero', hardware?: CinemaHardware, model?: HullModel): THREE.Group {
```

Hull choice (~line 353). Replace

```ts
    const bespoke = buildSpecialHull(appearance.recipe, context) || buildEmpireHull(appearance, context)
```

with

```ts
    // Opt-in scanned hull (?hulls=hy3d): replaces only the procedural body; the
    // fitted hardware below is still seated on it by deckAt/hullSurface raycasts.
    const bespoke = (model ? buildModelHull(model, context) : false) || buildSpecialHull(appearance.recipe, context) || buildEmpireHull(appearance, context)
```

After `const { deckAt, detailSurfaces } = fitHardware()` (~line 614), add:

```ts
    if (model) addMountTints(group, rig)
```

- [ ] **Step 5: Run the new tests and the whole cinema suite**

```bash
cd /home/robert/spacemolt-www && bun test src/lib/cinema/ship-model-hull.test.ts && bun test src/lib/cinema && npx -y pnpm lint
```

Expected: new tests pass. The whole suite passes with the Task 1 count plus the new tests. Lint is clean.
If the turret-seating assertion fails (mounts floating far from the box), check how `buildFittedHardware` uses `deckAt`/`hullSurface` for this family, and report before changing anything. Don't loosen the bounds.

- [ ] **Step 6: Commit**

```bash
git add src/lib/cinema/ship-model-hull.ts src/lib/cinema/ship-model-hull.test.ts src/lib/cinema/ships.ts
git commit -m "feat(cinema): build ships on scanned hull models with fitted turrets, engines and mount tints"
```

---

### Task 7: Scene + player wiring, shaped shield (fork)

**Files:**
- Modify: `/home/robert/spacemolt-www/src/lib/cinema/scene.ts`: `CinemaOptions` (line 37); the `Actor` interface (find with `grep -n "interface Actor" scene.ts`); actor creation (lines ~247–292); the shield pool (~363–375); the shield impact use (~792–797); cleanups.
- Modify: `/home/robert/spacemolt-www/src/components/cinema/CinemaPlayer.tsx` (the mount effect, lines ~60–78)

**Interfaces:**
- Consumes: `HullModelMap`, `loadHullModels`, `hullModelsEnabled`, `disposeHullModels` (Task 5); `createShip(..., model)` (Task 6).
- Produces: `CinemaOptions.hullModels?: HullModelMap`; `Actor.hullModel?: HullModel`.

- [ ] **Step 1: `CinemaOptions` + `Actor`**

In `CinemaOptions` add:

```ts
  /** Opt-in scanned hulls keyed by ship class (?hulls=hy3d). Owned by the caller. */
  hullModels?: HullModelMap
```

Add `import type { HullModel, HullModelMap } from './hull-models'`, and add `hullModel?: HullModel` to the `Actor` interface.

- [ ] **Step 2: Actor creation**

Replace the `appearance` / `model` lines in `film.ships.map(...)` (~247–254) with:

```ts
    const known = ship.kind==='station' ? resolveStationAppearance(ship.playerId) : appearances[ship.shipClass]
    const resolved = ['station', 'creature', 'drone'].includes(ship.kind) ? { ...(known ?? resolveAppearance(ship.shipClass)), family: ship.kind as ShipAppearance['family'] } : known ?? resolveAppearance(ship.shipClass)
    const hullModel = ['station', 'creature', 'drone'].includes(ship.kind) ? undefined : options.hullModels?.get(ship.shipClass)
    // A scanned hull brings its real proportions; formation spacing uses them too.
    const appearance = hullModel ? { ...resolved, beam: hullModel.beam, height: hullModel.height } : resolved
    const size = cinemaHullWorldSize(appearance)
    const seed = hash(ship.id)
    const side = sides.indexOf(ship.sideIndex)
    const angle = side / Math.max(2, sides.length) * Math.PI * 2
    const lane = shipRanks.get(`${ship.sideIndex}:${ship.playerId}`) ?? 0
    const model = detailed.has(ship.id) ? createShip(appearance, seed, 'hero', ship.hardware, hullModel) : null
```

Guard hull markings (~line 274): change `if (!['creature','drone'].includes(ship.kind)) {` to `if (!['creature','drone'].includes(ship.kind) && !hullModel) {`.
Add `hullModel` to the returned actor object: `return { ship, appearance, model, hullModel, contactHull, ... }`.

(`cinemaHullWorldSize` in `ship-scale.ts` reads only `appearance.length`, so the beam/height override doesn't change a ship's world length.)

- [ ] **Step 3: Shaped shield**

Loosen the pool type and keep each sphere:

```ts
  const shields: THREE.Mesh<THREE.BufferGeometry, THREE.ShaderMaterial>[] = []
```

and after creating each shield: `shield.userData.sphere = shield.geometry`.

At the impact site (~794–795), replace

```ts
              const shield = shields[shieldCount++]; shield.visible = true; shield.position.copy(to.position); shield.rotation.y = to.rotation
              shield.scale.set(to.size * 0.6, to.size * 0.32, to.size * 0.43)
```

with

```ts
              const shield = shields[shieldCount++]; shield.visible = true; shield.position.copy(to.position)
              if (to.hullModel && to.model) {
                // Hull-shaped bubble: the scanned hull inflated along its normals.
                shield.geometry = to.hullModel.shell; shield.quaternion.copy(to.model.quaternion); shield.scale.setScalar(to.size)
              } else {
                shield.geometry = shield.userData.sphere; shield.rotation.set(0, to.rotation, 0)
                shield.scale.set(to.size * 0.6, to.size * 0.32, to.size * 0.43)
              }
```

At the death/knockout shield site (~819), add `shield.geometry = shield.userData.sphere; shield.rotation.set(0, destination.rotation, 0)` before its scale lines, so a pooled mesh last used as a shell resets to a sphere. Teardown: the scene's generic `scene.traverse` dispose (~line 113) disposes whatever geometry each shield holds at that moment, which may be a shell. Disposing it twice (again in `disposeHullModels`) is harmless in three.js. A sphere swapped out at that moment would leak, so add right after the pool loop:

```ts
  cleanups.push(() => { for (const shield of shields) (shield.userData.sphere as THREE.BufferGeometry).dispose() })
```

- [ ] **Step 4: Player: load before mount, dispose after**

In `CinemaPlayer.tsx`, replace the `import('@/lib/cinema/scene').then(({ mountCinema }) => { ... })` block with:

```ts
    let hullModels: import('@/lib/cinema/hull-models').HullModelMap | undefined
    Promise.all([import('@/lib/cinema/scene'), import('@/lib/cinema/hull-models')]).then(async ([{ mountCinema }, hulls]) => {
      // Opt-in scanned hulls (?hulls=hy3d) load before the synchronous scene build.
      if (hulls.hullModelsEnabled(window.location.search)) hullModels = await hulls.loadHullModels(film.ships.map(ship => ship.shipClass))
      if (disposed || !canvas.current) { if (hullModels) hulls.disposeHullModels(hullModels); return }
      player.current = mountCinema(canvas.current, film, appearances, {
        quality: mountQuality.current, muted: true, volume: 0.65,
        reducedMotion: window.matchMedia('(prefers-reduced-motion: reduce)').matches,
        hullModels,
        onTime: value => { if (!disposed) setTime(value) },
        onEnd: () => { if (!disposed) { setPlaying(false); setEnded(true) } },
        onError: () => { if (!disposed) { setFailed(true); setPlaying(false); setReady(false) } },
      })
      if (!disposed) setReady(true)
    }).catch(() => { if (!disposed) { setFailed(true); setReady(false) } })
    return () => {
      disposed = true; player.current?.dispose(); player.current = null
      if (hullModels) import('@/lib/cinema/hull-models').then(hulls => hulls.disposeHullModels(hullModels!))
    }
```

- [ ] **Step 5: Tests + types**

```bash
cd /home/robert/spacemolt-www && bun test src/lib/cinema && npx -y pnpm lint
```

Expected: everything green, same count as after Task 6.

- [ ] **Step 6: Visual check against the first slice**

With `npx -y pnpm dev` running, open
`http://localhost:3000/battles/242b5fd8676d27c997f9dcd6b76a8cb7/cinematic?hulls=hy3d`
and the same URL without the flag. Check and note in `cinema-shots/notes.md`:
- with the flag, the 8 exported classes show scanned hulls, bow leading the direction of travel, top side up;
- engine glow sits on the stern; turrets sit on the hull, and beams leave them;
- the hull-shaped shield flashes on shield hits;
- a destroyed scanned ship breaks apart into chunks;
- without the flag, the scene matches the baseline screenshots.

Save `cinema-shots/hy3d-{1,2,3}.png` at the same moments as the baseline shots. Check the browser console for `[cinema] hull model ... skipped` warnings.

- [ ] **Step 7: Commit**

```bash
git add src/lib/cinema/scene.ts src/components/cinema/CinemaPlayer.tsx
git commit -m "feat(cinema): load opt-in Hy3D hulls before mount and flash hull-shaped shields"
```

---

### Task 8: Full fleet export + review

**Files:**
- Modify: none in code. Outputs go to `~/spacemolt-www/public/cinema-hulls/`, and notes to `kb/data/mesh_bakeoff/cinema-shots/notes.md` (ignored).

- [ ] **Step 1: Export all mapped ships**

```bash
cd /home/robert/spacemolt/kb/data/mesh_bakeoff && free -h
systemd-run --user --scope -p MemoryMax=8G -p MemorySwapMax=0 \
  ~/sf3d-venv/bin/python export_cinema_hulls.py 2>&1 | tee cinema-shots/export-all.log
grep -c " ok " cinema-shots/export-all.log; grep -E "FAIL|SKIP" cinema-shots/export-all.log
```

Expected: about 275 `ok`. Every FAIL gets listed in `notes.md` with its error. A nonzero exit code only means at least one FAIL.

- [ ] **Step 2: Review the remaining classes in the test battle**

Replay `?hulls=hy3d` and confirm all 20 art-backed classes render as scanned hulls. `magnate` stays procedural. In `notes.md`, list any hull that is backwards, upside down, has engines off the stern, or has floating turrets. Those notes are the input for adjustments or the placement tool; don't fix them here.

- [ ] **Step 3: Push the fork branch** (outward-facing: confirm with the user first)

```bash
cd /home/robert/spacemolt-www && git push -u origin hy3d-hulls
```

---

## Self-review

- **Spec coverage:** workspace and flag (T1, T5, T7); A1 export (T2, T4); A2 sidecar (T4); A3 guessing (T3); A4 loader (T5), model branch (T6), engines (T6), turrets (T6, via fitHardware, deviation 1), turret tint (T6), shaped shield (T5, T7), wreckage (T6, deviation 2), distant fleet untouched (T7 passes the model only to `detailed` actors), markings skipped (T7); testing (every task); smooth normals (T2 exports area-weighted vertex normals). Part B is out of scope for this plan by design.
- **Types:** `HullModel`/`HullModelMap`/`HullSidecar` are defined in T5 and used unchanged in T6 and T7; `createShip`'s 5th parameter `model?: HullModel` in T6 matches the T7 call; `MODEL_HULL_SLICES` is exported in T6 and imported by its test.
