# Ship Hangar Lineup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A KB page `kb/ships/hangar.html`: every catalog ship floating above a light studio floor in size order, with a video-style camera pan you can take over, click-to-orbit, filters, and ghost placeholders for ships without models.

**Architecture:** A Python exporter (`data/mesh_bakeoff/export_hangar.py`, pymeshlab venv) writes `kb/ships/hangar/lineup.json` and decimated `kb/ships/hangar/models/<id>.glb`. The page is plain ES modules with three.js from jsDelivr: `hangar/layout.js` (pure layout/rail/filter maths, unit-tested with `node --test`) and `hangar/app.js` (scene, loading, UI).

**Tech Stack:** Python 3.12 (`~/hy3d-venv`: numpy, pymeshlab, trimesh), stdlib unittest; three.js 0.185.1 (CDN importmap: GLTFLoader, OrbitControls), Node 22 `node --test`; Go (one template line in `cmd/generate-items-kb/ships.go`).

**Spec:** `docs/superpowers/specs/2026-09-26-ship-hangar-lineup-design.md`

## Global Constraints

- Sizes: window-measured `loa_m` from `data/footprints/scale/ship_scale_est.json`; unmodeled ships use its `ladder_group_median["<scale>/<role_group>"]`, else `ladder_scale_geomean["<scale>"]`, flagged `lengthSource: "estimate"`.
- Contents: all current catalog ships (`ships` table in `~/spacemolt/spacemolt-knowledge.db`, 361 today); modeled = ids in `~/spacemolt-www/public/cinema-hulls/manifest.json` without `__lod` (275 today); the rest are placeholders.
- Models: cinema-frame meshes (+X bow, +Y dorsal, length 1) decimated to `HANGAR_FACES = 6000` (the default until the LOD review picks a value), uint16 indices.
- Colour: near-white studio material with a thin empire accent band. Empire accents: solarian `#c9a227`, crimson `#e63946`, nebula `#2f9e6a`, outerrim `#2fb6c4`, voidborn `#9b6bff`, pirate `#ff6540`, independent `#8bd7ff`.
- Metres only. Legacy art-only ships excluded.
- The kb working tree has unrelated uncommitted changes: `git add` only the named files. Commit trailer: `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`.
- Any Go change: `go build ./...`, `go test ./cmd/generate-items-kb/... ./internal/kbnav/...`, `golangci-lint run ./cmd/generate-items-kb/...` with no new findings.
- Don't regenerate the whole KB site; patch the one generated line to match the template change.

## File Structure

- Modify `data/mesh_bakeoff/cinema_frame.py`: `glb_bytes` writes uint16 indices when vertices < 65536.
- Create `data/mesh_bakeoff/export_hangar.py`, and `data/mesh_bakeoff/test_export_hangar.py`.
- Create `kb/ships/hangar.html`, `kb/ships/hangar/layout.js`, `kb/ships/hangar/app.js`, `kb/ships/hangar/hangar.css`.
- Generated: `kb/ships/hangar/lineup.json`, `kb/ships/hangar/models/*.glb`.
- Create `data/hangar/layout.test.mjs` (kept out of the deployed `kb/` tree).
- Modify `cmd/generate-items-kb/ships.go` (index link) and `kb/ships/index.html` (same line).

---

### Task 1: uint16 GLB indices

**Files:** Modify `data/mesh_bakeoff/cinema_frame.py` (`glb_bytes`); Test `data/mesh_bakeoff/test_cinema_export.py`.

- [ ] **Step 1: Failing test** — append to `GlbTest`:

```python
    def test_small_meshes_use_uint16_indices(self):
        v, f = box((-.5, -.1, -.2), (.5, .1, .2))
        data = cf.glb_bytes(v.astype(np.float32), cf.vertex_normals(v, f), f.astype(np.uint32))
        jlen, _ = struct.unpack_from("<II", data, 12)
        gltf = json.loads(data[20:20 + jlen])
        self.assertEqual(gltf["accessors"][2]["componentType"], 5123)
        rv, rf = cf.read_glb(data)
        np.testing.assert_array_equal(rf, f)
        self.assertEqual(len(data) % 4, 0)
```

Run `cd data/mesh_bakeoff && ~/sf3d-venv/bin/python -m unittest test_cinema_export -v`. Expect a FAIL (componentType is 5125).

- [ ] **Step 2: Implement.** In `glb_bytes`, choose the index type:

```python
    small = len(v) < 65536
    idx = np.ascontiguousarray(faces, dtype="<u2" if small else "<u4").ravel()
```

Then use `"componentType": 5123 if small else 5125` for accessor 2. The blob is already padded to 4 bytes.

- [ ] **Step 3:** Run the full `test_cinema_export` in both venvs; expect all to pass. Commit `cinema_frame.py` and `test_cinema_export.py`: "feat(mesh): uint16 GLB indices for small meshes".

---

### Task 2: Lineup builder (pure) + model writer

**Files:** Create `data/mesh_bakeoff/export_hangar.py` and `data/mesh_bakeoff/test_export_hangar.py`.

**Interfaces produced:**
- `build_lineup(catalog, estimates, ladder, aspects, modeled) -> dict`
  - `catalog`: `{id: {"name", "faction", "tier", "scale", "category", "class"}}`
  - `estimates`: `{id: {"loa_m", "beam_m", "source"}}`
  - `ladder`: `{"ladder_group_median": {...}, "ladder_scale_geomean": {...}}`
  - `aspects`: `{id: {"beam": b/L, "height": h/L}}` for modeled ships
  - `modeled`: a set of ids
- `EMPIRE_OF(faction) -> str`: `''`, `None` or `'legacy'` → `'independent'`.
- `HANGAR_FACES = 6000`

- [ ] **Step 1: Failing tests**

```python
#!/usr/bin/env python3
"""cd data/mesh_bakeoff && ~/hy3d-venv/bin/python -m unittest test_export_hangar -v"""
import unittest

import export_hangar as eh

CATALOG = {
    "big": {"name": "Big One", "faction": "solarian", "tier": 5, "scale": 5, "category": "Combat", "class": "Dreadnought"},
    "small": {"name": "Small One", "faction": "crimson", "tier": 1, "scale": 1, "category": "Combat", "class": "Fighter"},
    "ghost": {"name": "Ghost", "faction": "", "tier": 2, "scale": 2, "category": "Commercial", "class": "Freighter"},
    "legacy": {"name": "Old", "faction": "legacy", "tier": 1, "scale": 1, "category": "Discontinued", "class": "Mystery"},
}
ESTIMATES = {"big": {"loa_m": 142.0, "beam_m": 40.0, "source": "window"},
             "small": {"loa_m": 18.0, "beam_m": 9.0, "source": "ladder:combat"}}
LADDER = {"ladder_group_median": {"2/hauler": 83.7}, "ladder_scale_geomean": {"1": 24.2, "2": 66.6}}
ASPECTS = {"big": {"beam": .28, "height": .2}, "small": {"beam": .5, "height": .25}}


class LineupTest(unittest.TestCase):
    def setUp(self):
        self.lineup = eh.build_lineup(CATALOG, ESTIMATES, LADDER, ASPECTS, {"big", "small"})
        self.by_id = {s["id"]: s for s in self.lineup["ships"]}

    def test_every_catalog_ship_in_length_order(self):
        self.assertEqual([s["id"] for s in self.lineup["ships"]], ["small", "legacy", "ghost", "big"])
        self.assertEqual(self.lineup["version"], 1)

    def test_lengths_sources_and_models(self):
        big, small, ghost = self.by_id["big"], self.by_id["small"], self.by_id["ghost"]
        self.assertEqual((big["lengthM"], big["lengthSource"], big["model"]), (142.0, "window", "models/big.glb"))
        self.assertEqual(small["lengthSource"], "estimate")        # ladder-estimated even though modeled
        self.assertEqual((ghost["lengthM"], ghost["lengthSource"], ghost["model"]), (83.7, "estimate", None))
        self.assertAlmostEqual(big["heightM"], 142 * .2)
        self.assertGreater(ghost["beamM"], 0)
        self.assertGreater(ghost["heightM"], 0)

    def test_empire_page_and_scale_fallback(self):
        self.assertEqual(self.by_id["ghost"]["empire"], "independent")
        self.assertEqual(self.by_id["legacy"]["empire"], "independent")
        self.assertEqual(self.by_id["legacy"]["lengthM"], 24.2)     # no group median -> scale geomean
        self.assertEqual(self.by_id["big"]["page"], "Combat/big.html")
        self.assertEqual(self.by_id["small"]["tier"], 1)
```

Run it and expect `ModuleNotFoundError`.

- [ ] **Step 2: Implement `export_hangar.py`**

```python
#!/usr/bin/env python3
"""Export the KB ship hangar: kb/ships/hangar/lineup.json + decimated models.

    ~/hy3d-venv/bin/python export_hangar.py [--faces 6000]
See docs/superpowers/specs/2026-09-26-ship-hangar-lineup-design.md.
"""
import argparse
import json
import sqlite3
import statistics
import sys
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(HERE.parent / "footprints" / "scale"))
import cinema_frame as cf                    # noqa: E402
from compute_scale import role_group         # noqa: E402

KB_DB = Path.home() / "spacemolt" / "spacemolt-knowledge.db"
HULLS = Path.home() / "spacemolt-www" / "public" / "cinema-hulls"
SCALE = HERE.parent / "footprints" / "scale" / "ship_scale_est.json"
OUT = HERE.parent.parent / "kb" / "ships" / "hangar"
HANGAR_FACES = 6000


def EMPIRE_OF(faction):
    return faction if faction and faction != "legacy" else "independent"


def build_lineup(catalog, estimates, ladder, aspects, modeled):
    group_of = {sid: role_group(c.get("class")) for sid, c in catalog.items()}
    by_group = {}
    for sid, a in aspects.items():
        by_group.setdefault(group_of.get(sid, "other"), []).append(a)
    every = list(aspects.values()) or [{"beam": .35, "height": .22}]

    def typical(sid, key):
        pool = by_group.get(group_of[sid]) or every
        return statistics.median(a[key] for a in pool)

    ships = []
    for sid, c in catalog.items():
        est = estimates.get(sid)
        if est and sid in modeled:
            length, source = float(est["loa_m"]), "window" if est.get("source") == "window" else "estimate"
            beam = float(est.get("beam_m") or length * aspects[sid]["beam"])
            height = length * aspects[sid]["height"]
        else:
            key = f"{c.get('scale')}/{group_of[sid]}"
            length = ladder["ladder_group_median"].get(key) or ladder["ladder_scale_geomean"].get(str(c.get("scale"))) or 30.0
            length, source = float(length), "estimate"
            beam, height = length * typical(sid, "beam"), length * typical(sid, "height")
        ships.append({"id": sid, "name": c["name"], "empire": EMPIRE_OF(c.get("faction")), "tier": c.get("tier") or 0,
                      "category": c.get("category") or "", "lengthM": round(length, 1), "lengthSource": source,
                      "beamM": round(beam, 1), "heightM": round(height, 1),
                      "model": f"models/{sid}.glb" if sid in modeled else None,
                      "page": f"{c.get('category')}/{sid}.html"})
    ships.sort(key=lambda s: (s["lengthM"], s["name"]))
    return {"version": 1, "ships": ships}


def load_catalog(db_path):
    db = sqlite3.connect(db_path)
    try:
        rows = db.execute("SELECT id, name, faction, tier, scale, category, class FROM ships").fetchall()
    finally:
        db.close()
    return {r[0]: {"name": r[1], "faction": r[2], "tier": r[3], "scale": r[4], "category": r[5], "class": r[6]} for r in rows}


def write_models(ids, faces_budget, out_dir):
    from make_lod_variants import decimate
    out_dir.mkdir(parents=True, exist_ok=True)
    aspects = {}
    for sid in ids:
        verts, faces = cf.read_glb((HULLS / f"{sid}.glb").read_bytes())
        if len(faces) > faces_budget:
            verts, faces = decimate(verts, faces, faces_budget)
        ext = np.ptp(verts, axis=0)
        aspects[sid] = {"beam": float(ext[2] / ext[0]), "height": float(ext[1] / ext[0])}
        (out_dir / f"{sid}.glb").write_bytes(cf.glb_bytes(verts.astype(np.float32), cf.vertex_normals(verts, faces), faces.astype(np.uint32)))
    return aspects


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--faces", type=int, default=HANGAR_FACES)
    args = parser.parse_args()
    catalog = load_catalog(KB_DB)
    scale = json.loads(SCALE.read_text())
    modeled = {sid for sid in json.loads((HULLS / "manifest.json").read_text())["ships"] if "__lod" not in sid} & set(catalog)
    aspects = write_models(sorted(modeled), args.faces, OUT / "models")
    lineup = build_lineup(catalog, scale["ships"], scale, aspects, modeled)
    (OUT / "lineup.json").write_text(json.dumps(lineup, separators=(",", ":")))
    size = sum(p.stat().st_size for p in (OUT / "models").glob("*.glb"))
    print(f"{len(lineup['ships'])} ships ({len(modeled)} modeled), models {size / 1e6:.1f} MB -> {OUT}")


if __name__ == "__main__":
    main()
```

- [ ] **Step 3:** Run the tests; expect 3 to pass. Then run the export: `free -h`, then `systemd-run --user --scope -p MemoryMax=8G -p MemorySwapMax=0 ~/hy3d-venv/bin/python export_hangar.py`. Expect "361 ships (275 modeled), models ~30–40 MB".
- [ ] **Step 4:** Commit `export_hangar.py`, `test_export_hangar.py`, `kb/ships/hangar/lineup.json` and `kb/ships/hangar/models/`: "feat(kb): hangar lineup data and decimated models".

---

### Task 3: `layout.js` (pure maths) + node tests

**Files:** Create `kb/ships/hangar/layout.js`; Test `data/hangar/layout.test.mjs`.

**Interfaces produced:**
- `filterShips(ships, filters)`: `filters = {empires?: Set, tiers?: Set, categories?: Set}`; an empty or absent set means all.
- `layoutLineup(ships) -> [{id, x}]`: centres along +X, with gap = `max(4, 0.35 × max(neighbour lengths))`.
- `railAt(layout, ships, u) -> {x, length}`: `u` is a fractional ship index, clamped; interpolates between ships.
- `railPose(x, length) -> {position: [x, y, z], target: [x, y, z]}`
- `nearestIndex(layout, x)`
- `parseFilters(search)` and `formatFilters(filters)`: query keys `empire`, `tier`, `cat` as comma lists.
- `FLOAT_M = 3`

- [ ] **Step 1: Failing tests**

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { filterShips, layoutLineup, railAt, railPose, nearestIndex, parseFilters, formatFilters, FLOAT_M } from '../../kb/ships/hangar/layout.js'

const ships = [
  { id: 'a', lengthM: 10, empire: 'crimson', tier: 1, category: 'Combat' },
  { id: 'b', lengthM: 20, empire: 'solarian', tier: 2, category: 'Commercial' },
  { id: 'c', lengthM: 100, empire: 'crimson', tier: 5, category: 'Combat' },
]

test('layout packs centres with size-proportional gaps', () => {
  const l = layoutLineup(ships)
  assert.equal(l[0].x, 0)
  assert.equal(l[1].x, 5 + 7 + 10)          // half a + gap max(4, .35*20)=7 + half b
  assert.equal(l[2].x, l[1].x + 10 + 35 + 50)
})

test('filters combine and empty sets mean all', () => {
  assert.deepEqual(filterShips(ships, {}).map(s => s.id), ['a', 'b', 'c'])
  assert.deepEqual(filterShips(ships, { empires: new Set(['crimson']) }).map(s => s.id), ['a', 'c'])
  assert.deepEqual(filterShips(ships, { empires: new Set(['crimson']), tiers: new Set([5]) }).map(s => s.id), ['c'])
})

test('rail interpolates between ships and clamps', () => {
  const l = layoutLineup(ships)
  assert.deepEqual(railAt(l, ships, 0), { x: 0, length: 10 })
  assert.deepEqual(railAt(l, ships, 1.5), { x: (l[1].x + l[2].x) / 2, length: 60 })
  assert.deepEqual(railAt(l, ships, 99), { x: l[2].x, length: 100 })
})

test('rail pose frames bigger ships from further away, looking at float height', () => {
  const small = railPose(0, 10), big = railPose(0, 100)
  assert.equal(small.target[1], FLOAT_M)
  assert.ok(big.position[2] > small.position[2] * 5)
  assert.ok(big.position[1] > small.position[1])
})

test('nearest index and filter query round trip', () => {
  const l = layoutLineup(ships)
  assert.equal(nearestIndex(l, l[1].x + 1), 1)
  const f = parseFilters('?empire=crimson,solarian&tier=1,5&cat=Combat')
  assert.deepEqual([...f.empires], ['crimson', 'solarian'])
  assert.deepEqual([...f.tiers], [1, 5])
  assert.equal(formatFilters(f), 'empire=crimson,solarian&tier=1,5&cat=Combat')
  assert.equal(formatFilters({}), '')
})
```

Run `node --test data/hangar/` and expect a failure: module not found.

- [ ] **Step 2: Implement `layout.js`**

```js
// Pure layout / rail / filter maths for the hangar lineup (no three.js).
export const FLOAT_M = 3

const inSet = (set, value) => !set || set.size === 0 || set.has(value)

export function filterShips(ships, { empires, tiers, categories } = {}) {
  return ships.filter(s => inSet(empires, s.empire) && inSet(tiers, s.tier) && inSet(categories, s.category))
}

export function layoutLineup(ships) {
  let x = 0
  return ships.map((s, i) => {
    if (i > 0) {
      const prev = ships[i - 1]
      x += prev.lengthM / 2 + Math.max(4, .35 * Math.max(prev.lengthM, s.lengthM)) + s.lengthM / 2
    }
    return { id: s.id, x }
  })
}

export function railAt(layout, ships, u) {
  const last = layout.length - 1
  const c = Math.max(0, Math.min(last, u)), i = Math.min(last, Math.floor(c)), f = c - i
  const j = Math.min(last, i + 1)
  return { x: layout[i].x + (layout[j].x - layout[i].x) * f, length: ships[i].lengthM + (ships[j].lengthM - ships[i].lengthM) * f }
}

export function railPose(x, length) {
  const d = Math.max(14, length * 1.7)
  return { position: [x - d * .18, FLOAT_M + d * .42, d], target: [x, FLOAT_M, 0] }
}

export function nearestIndex(layout, x) {
  let best = 0
  for (let i = 1; i < layout.length; i++) if (Math.abs(layout[i].x - x) < Math.abs(layout[best].x - x)) best = i
  return best
}

export function parseFilters(search) {
  const q = new URLSearchParams(search)
  const list = key => new Set((q.get(key) || '').split(',').filter(Boolean))
  return { empires: list('empire'), tiers: new Set([...list('tier')].map(Number)), categories: list('cat') }
}

export function formatFilters({ empires, tiers, categories } = {}) {
  const parts = []
  if (empires?.size) parts.push(`empire=${[...empires].join(',')}`)
  if (tiers?.size) parts.push(`tier=${[...tiers].join(',')}`)
  if (categories?.size) parts.push(`cat=${[...categories].join(',')}`)
  return parts.join('&')
}
```

- [ ] **Step 3:** Run `node --test data/hangar/` and expect 5 to pass. Commit `layout.js` and `layout.test.mjs`.

---

### Task 4: Page shell, studio scene, placeholders, labels, lazy models

**Files:** Create `kb/ships/hangar.html`, `kb/ships/hangar/hangar.css` and `kb/ships/hangar/app.js`.

- [ ] **Step 1: `hangar.html`**

```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Ship Hangar — SpaceMolt KB</title>
  <link rel="stylesheet" href="hangar/hangar.css">
  <script type="importmap">{"imports":{"three":"https://cdn.jsdelivr.net/npm/three@0.185.1/build/three.module.js","three/addons/":"https://cdn.jsdelivr.net/npm/three@0.185.1/examples/jsm/"}}</script>
</head>
<body>
  <canvas id="stage"></canvas>
  <header id="topbar">
    <a href="index.html" class="back">&larr; Ships</a>
    <h1>Ship Hangar <span id="count"></span></h1>
    <div id="filters"></div>
  </header>
  <aside id="card" hidden></aside>
  <footer id="rail">
    <button id="play" aria-label="Pause">&#10074;&#10074;</button>
    <input id="scrub" type="range" min="0" value="0" step="0.01" aria-label="Position along the lineup">
    <span id="now"></span>
  </footer>
  <p id="note">Lengths from cockpit-window measurements; <em>est.</em> = estimated. Ghost hulls: model pending. Models are shape scans (colour coming).</p>
  <script type="module" src="hangar/app.js"></script>
</body>
</html>
```

- [ ] **Step 2: `hangar.css`** — a full-bleed canvas; a dark translucent top bar, and chips styled as small rounded toggles (`.chip.on` in the accent colour); a bottom rail bar with a full-width range input; a card in the top right (name, meta rows, link); a small note bottom left in muted text.

```css
html,body{margin:0;height:100%;background:#000;color:#dfe6ee;font:14px/1.4 system-ui,sans-serif;overflow:hidden}
#stage{position:fixed;inset:0;width:100%;height:100%;display:block}
#topbar{position:fixed;top:0;left:0;right:0;display:flex;gap:16px;align-items:center;flex-wrap:wrap;padding:10px 16px;background:linear-gradient(#000c,#0000)}
#topbar h1{font-size:18px;margin:0;font-weight:600}#count{opacity:.6;font-weight:400}
.back{color:#8bd7ff;text-decoration:none}
#filters{display:flex;gap:6px;flex-wrap:wrap}
.chip{border:1px solid #fff3;border-radius:12px;padding:2px 10px;background:#0006;color:inherit;cursor:pointer;font-size:12px}
.chip.on{background:var(--c,#8bd7ff);color:#000;border-color:transparent}
.sep{width:1px;background:#fff3;margin:0 4px}
#rail{position:fixed;left:0;right:0;bottom:0;display:flex;gap:12px;align-items:center;padding:10px 16px;background:linear-gradient(#0000,#000c)}
#scrub{flex:1}#play{background:none;border:1px solid #fff4;color:inherit;border-radius:6px;padding:2px 10px;cursor:pointer}
#now{min-width:260px;text-align:right}
#card{position:fixed;top:64px;right:16px;width:260px;padding:12px 14px;background:#0a1016e6;border:1px solid #fff2;border-radius:8px}
#card h2{margin:0 0 6px;font-size:16px}#card dl{display:grid;grid-template-columns:auto 1fr;gap:2px 10px;margin:0}#card dt{opacity:.6}
#card a{color:#8bd7ff}
#note{position:fixed;left:16px;bottom:48px;margin:0;font-size:11px;opacity:.55}
```

- [ ] **Step 3: `app.js`: scene, lineup, ghosts, labels, lazy models.** The Task 5 and Task 6 code slots into the marked places.

```js
import * as THREE from 'three'
import { GLTFLoader } from 'three/addons/loaders/GLTFLoader.js'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { filterShips, layoutLineup, railAt, railPose, nearestIndex, parseFilters, formatFilters, FLOAT_M } from './layout.js'

const ACCENT = { solarian: '#c9a227', crimson: '#e63946', nebula: '#2f9e6a', outerrim: '#2fb6c4', voidborn: '#9b6bff', pirate: '#ff6540', independent: '#8bd7ff' }
const LOAD_WINDOW = 14
const canvas = document.getElementById('stage')
const renderer = new THREE.WebGLRenderer({ canvas, antialias: true })
renderer.setPixelRatio(Math.min(2, devicePixelRatio))
renderer.shadowMap.enabled = true
renderer.shadowMap.type = THREE.PCFSoftShadowMap
renderer.toneMapping = THREE.ACESFilmicToneMapping
const scene = new THREE.Scene()
scene.background = new THREE.Color(0x000000)
scene.fog = new THREE.Fog(0x000000, 200, 900)
const camera = new THREE.PerspectiveCamera(38, 1, .5, 20000)

// Studio: light floor fading into black space, key light with soft shadows.
const floor = new THREE.Mesh(new THREE.PlaneGeometry(40000, 6000), new THREE.MeshStandardMaterial({ color: 0x9fb1c2, roughness: .38, metalness: .08 }))
floor.rotation.x = -Math.PI / 2; floor.receiveShadow = true; scene.add(floor)
scene.add(new THREE.HemisphereLight(0xdfe8f2, 0x3a4550, 1.1))
const key = new THREE.DirectionalLight(0xffffff, 2.4)
key.castShadow = true; key.shadow.mapSize.set(2048, 2048); key.shadow.bias = -.0004
scene.add(key, key.target)
const fill = new THREE.DirectionalLight(0xcfe0ff, .6); fill.position.set(-1, .4, 1); scene.add(fill)
const starGeo = new THREE.BufferGeometry()
const starPos = new Float32Array(4000 * 3)
for (let i = 0; i < 4000; i++) { const a = Math.random() * Math.PI * 2, y = .15 + Math.random() * .85, r = Math.sqrt(1 - y * y), R = 12000; starPos.set([Math.cos(a) * r * R, y * R, Math.sin(a) * r * R], i * 3) }
starGeo.setAttribute('position', new THREE.BufferAttribute(starPos, 3))
const stars = new THREE.Points(starGeo, new THREE.PointsMaterial({ color: 0xffffff, size: 18, fog: false, sizeAttenuation: true }))
scene.add(stars)

const white = new THREE.MeshStandardMaterial({ color: 0xf1f3f5, roughness: .45, metalness: .2, vertexColors: true })

function accentBand(geometry, colour) {
  // Near-white hull with a thin empire-coloured band around the waist.
  const pos = geometry.getAttribute('position')
  geometry.computeBoundingBox()
  const { min, max } = geometry.boundingBox, mid = (min.y + max.y) / 2, half = (max.y - min.y) * .06
  const c = new THREE.Color(colour), w = new THREE.Color(0xf1f3f5), colors = new Float32Array(pos.count * 3)
  for (let i = 0; i < pos.count; i++) (Math.abs(pos.getY(i) - mid) < half ? c : w).toArray(colors, i * 3)
  geometry.setAttribute('color', new THREE.BufferAttribute(colors, 3))
}

function labelSprite(ship) {
  const cnv = document.createElement('canvas'); cnv.width = 1024; cnv.height = 192
  const g = cnv.getContext('2d')
  g.fillStyle = '#0d1b26'; g.font = '600 64px system-ui'; g.fillText(ship.name, 16, 80)
  g.font = '400 48px system-ui'; g.fillStyle = '#223a4b'
  g.fillText(`${ship.lengthM} m${ship.lengthSource === 'estimate' ? ' est.' : ''}${ship.model ? '' : ' · model pending'}`, 16, 150)
  const tex = new THREE.CanvasTexture(cnv); tex.colorSpace = THREE.SRGBColorSpace; tex.anisotropy = 8
  const w = Math.max(14, ship.lengthM * .9)
  const plane = new THREE.Mesh(new THREE.PlaneGeometry(w, w * 192 / 1024), new THREE.MeshBasicMaterial({ map: tex, transparent: true, depthWrite: false }))
  plane.rotation.x = -Math.PI / 2
  return plane
}

function ghost(ship) {
  const r = Math.max(.5, Math.min(ship.beamM, ship.heightM) / 2)
  const geo = new THREE.CapsuleGeometry(r, Math.max(.1, ship.lengthM - 2 * r), 8, 16)
  geo.rotateZ(Math.PI / 2); geo.scale(1, ship.heightM / (2 * r), ship.beamM / (2 * r))
  return new THREE.Mesh(geo, new THREE.MeshStandardMaterial({ color: 0xffffff, transparent: true, opacity: .22, emissive: ACCENT[ship.empire], emissiveIntensity: .35, depthWrite: false }))
}

const loader = new GLTFLoader()
const lineup = await (await fetch('hangar/lineup.json')).json()
const all = lineup.ships
const entries = new Map(all.map(ship => {
  const group = new THREE.Group(); group.userData.ship = ship
  const g = ghost(ship); group.add(g); group.userData.ghost = g
  const label = labelSprite(ship); label.position.set(0, .05 - (FLOAT_M + ship.heightM / 2), ship.beamM / 2 + ship.lengthM * .12 + 3); group.add(label)
  group.position.y = FLOAT_M + ship.heightM / 2
  scene.add(group)
  return [ship.id, { ship, group, state: ship.model ? 'idle' : 'none' }]
}))

function loadModel(entry) {
  entry.state = 'loading'
  loader.load(`hangar/${entry.ship.model}`, gltf => {
    let mesh; gltf.scene.traverse(o => { if (!mesh && o.isMesh) mesh = o })
    if (!mesh) { entry.state = 'failed'; return }
    accentBand(mesh.geometry, ACCENT[entry.ship.empire])
    const hull = new THREE.Mesh(mesh.geometry, white); hull.castShadow = true; hull.receiveShadow = true
    hull.scale.setScalar(entry.ship.lengthM)
    entry.group.add(hull); entry.group.remove(entry.group.userData.ghost); entry.state = 'loaded'
  }, undefined, () => { entry.state = 'failed' })
}

// --- lineup state (Task 5 adds rail + UI; Task 6 adds focus + filters) ---
let filters = parseFilters(location.search)
let visible = filterShips(all, filters)
let layout = layoutLineup(visible)
function placeLineup() {
  const shown = new Set(visible.map(s => s.id))
  for (const [id, e] of entries) e.group.visible = shown.has(id)
  layout.forEach((p, i) => { entries.get(visible[i].id).group.position.x = p.x })
}
placeLineup()

function resize() { const w = innerWidth, h = innerHeight; renderer.setSize(w, h, false); camera.aspect = w / h; camera.updateProjectionMatrix() }
addEventListener('resize', resize); resize()

let u = 0
function frame(dt) {
  // Task 5 replaces this with rail/auto-pan/focus camera logic.
  const { x, length } = railAt(layout, visible, u)
  const pose = railPose(x, length)
  camera.position.set(...pose.position); camera.lookAt(...pose.target)
  key.position.set(x - length, length * 3 + 40, length * 2 + 30); key.target.position.set(x, 0, 0)
  const s = Math.max(40, length * 3); Object.assign(key.shadow.camera, { left: -s, right: s, top: s, bottom: -s, near: 1, far: s * 6 }); key.shadow.camera.updateProjectionMatrix()
  scene.fog.near = length * 4 + 150; scene.fog.far = length * 12 + 700
  stars.position.set(camera.position.x, 0, camera.position.z)
  const i = Math.round(u)
  for (let k = Math.max(0, i - LOAD_WINDOW); k <= Math.min(visible.length - 1, i + LOAD_WINDOW); k++) {
    const e = entries.get(visible[k].id); if (e.state === 'idle') loadModel(e)
  }
}
let last = performance.now()
renderer.setAnimationLoop(now => { const dt = Math.min(.1, (now - last) / 1000); last = now; frame(dt); renderer.render(scene, camera) })
export { THREE, OrbitControls, camera, renderer, entries, all, formatFilters, nearestIndex }
```

- [ ] **Step 4: Browser check.** Serve `kb/` with `python3 -m http.server 8477 -d kb` and open `http://localhost:8477/ships/hangar.html`. Expect:
  - the studio floor and the first (smallest) ship with its label;
  - ghosts wherever a model is still loading or missing;
  - no console errors.

  Commit the page, the CSS and `app.js`.

---

### Task 5: Rail camera, auto-pan, scrubber, keys

**Files:** Modify `kb/ships/hangar/app.js`.

- [ ] Replace the `// --- lineup state` camera part with rail logic:
  - **Auto-pan:** `u` advances at `PAN_SHIPS_PER_SEC = .35`.
  - **Taking over:** `#play` toggles auto-pan. The `#scrub` `input` event sets `u` and pauses auto-pan. ←/→ step ±1 ship, and the wheel moves ±0.25 ship per notch; each pauses auto-pan.
  - **Resuming:** auto-pan resumes after 6 s idle, unless the user paused it with the button.
  - **Smoothing:** `camera.position` and a look-target vector lerp toward `railPose` with `1 - exp(-dt * 3)`.
  - **Readout:** `#now` shows "<name> · <length> m (<i+1>/<n>)".
  - **End of line:** at the end, auto-pan stops.
  - **Count:** `#count` shows "(<n> ships)".
- [ ] Browser check: pan, scrub, keys, wheel and resume all work, and the camera pulls back smoothly on big ships. Commit.

---

### Task 6: Focus/orbit + info card + filters

**Files:** Modify `kb/ships/hangar/app.js`.

- [ ] **Focus:**
  - **Click:** a raycast against visible groups. Clicking a ship enters focus: auto-pan pauses, OrbitControls turn on with target = ship centre, and the camera flies over 0.8 s to about 1.3 × length away.
  - **Card:** `#card` shows the name, tier, category, empire, length with its source, and "Ship page →" linking `ship.page`.
  - **Exit:** Esc, or a click on empty floor, leaves focus. `u` is set to that ship's index, the card hides and OrbitControls turn off.
- [ ] **Filters:**
  - **Chips:** chip rows in `#filters` for empires (the 7 in `ACCENT`, coloured with `--c`), tiers 0–5, and categories (distinct, sorted, from `all`).
  - **Toggling:** a chip toggles membership, recomputes `visible` and `layout`, and animates groups to their new x over 0.6 s (store the from/to positions).
  - **State:** `u` is clamped to the new count, and the URL updates with `history.replaceState(null, '', '?' + formatFilters(filters))`.
- [ ] Browser check: focus, orbit, the card link and Esc all work, filters close up the line, and the URL round-trips on reload. Commit.

---

### Task 7: Link from the ships index

**Files:** Modify `cmd/generate-items-kb/ships.go` (template, near line 451) and `kb/ships/index.html` (the same line).

- [ ] After the Fitting Sheet line, inside the same `{{- if .TotalBlueprints}}` block, add:

```html
      <p class="mt-1"><a href="hangar.html">&#x25A3; Ship Hangar &mdash; every ship side by side, smallest to largest &rarr;</a></p>
```

  Insert the identical line in `kb/ships/index.html` after its Fitting Sheet line.
- [ ] Run `go build ./... && go test ./cmd/generate-items-kb/... ./internal/kbnav/... && golangci-lint run ./cmd/generate-items-kb/...`; expect no new findings. Commit both files.

---

### Task 8: End-to-end verification

- [ ] Serve `kb/` locally and walk through the full page:
  - auto-pan from the first ship to the last;
  - an empire filter and a tier filter;
  - focus on a modeled ship and a ghost;
  - the ship-page link resolves;
  - no console errors.
- [ ] Record the page load time and memory (DevTools) and note them in the commit message.

## Self-review

- **Spec coverage:**
  - data and assets: T1, T2;
  - scene (floor, lighting, float, material, band, placeholders, labels): T4;
  - rail, auto-pan, scrubber and keys: T5;
  - focus, card and filters: T6;
  - links: T7;
  - tech: T4;
  - tests: T2, T3, T8.
- **Not covered, knowingly:** the spec's "soft blurred reflection" is approximated by a low-roughness floor, with no reflection pass. A real reflector (three's `Reflector`) is an easy follow-up if the floor looks flat.
- **Names:** `build_lineup`, `EMPIRE_OF` and `HANGAR_FACES` (T2); `filterShips`, `layoutLineup`, `railAt`, `railPose`, `nearestIndex`, `parseFilters`, `formatFilters` and `FLOAT_M` (T3), used in T4–T6.
