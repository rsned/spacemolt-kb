# Hero-Art Footprint Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Recover a measured top-down hull footprint from each ship hero image and emit it as a canonical half-width profile that `pkg/shipglyph` can consume.

**Architecture:** A seven-stage Python pipeline under `tools/footprint/`. Each stage reads the previous stage's file from `data/footprints/<id>/` and writes its own, so any stage re-runs without repeating the ones before it. A synthetic scene generator built in Task 1 provides ground truth for every later stage, so no stage's correctness depends on trusting a hero image.

**Tech Stack:** Python 3.12, PyTorch + MoGe-2 (`Ruicheng/moge-2-vitl-normal`), OpenCV, NumPy, SciPy, Shapely, alphashape, lu-vp-detect, pytest.

## Note on a spec detail

The spec describes MoGe's output as affine-invariant, requiring a mirror-constrained solve for depth scale and shift. That is true of **MoGe-1**. **MoGe-2 returns a metric-scale point map**, so there is no affine ambiguity to resolve.

This plan uses MoGe-2 (`Ruicheng/moge-2-vitl-normal`) because it also returns a per-pixel normal map, which makes plane fitting materially easier. The consequences:

- The scale/shift solve is retained as an **optional refinement** (Task 5), initialised at identity. On metric input it converges to a no-op; it exists so a fallback to MoGe-1 does not require a rewrite.
- The mirror solve is still required, for three things it alone provides: recovering the occluded half of the hull, locating the symmetry plane (which supplies the longitudinal axis), and producing a residual that gates reconstruction quality.

Everything else in the spec stands as written.

## Global Constraints

- MoGe runs in its own virtualenv at `~/moge-venv`, never in `~/sd-venv`. The portrait pipeline must not break because this one needed a different torch.
- The alpha radius is pinned **once per batch**, never per ship. It is chosen by sweeping a range across all recovered clouds and taking the value that maximises mean stage 5 silhouette IoU, and the chosen value is recorded with the results.
- Stage 2 gates stage 3. A low-confidence vanishing-point fit sends that ship to the hand-clicked fallback. It never silently inherits an assumed camera.
- Stage 5 is an exclusion gate. A ship that fails it is listed as a reconstruction failure and excluded from every aggregate. It is never quietly folded into an average.
- Stage 6 uses an alpha shape, never a convex hull. A convex hull erases the concavities that distinguish one hull from another.
- The pipeline is batch-shaped: pointing it at a full 335-ship art drop is a path change, not a rewrite. No ship ID is hardcoded outside test fixtures.
- Artifacts under `data/footprints/` are regenerable; only `profile.json` is committed. Add `data/footprints/*/*.npz` and `data/footprints/*/matte.png` to `.gitignore`.
- Python style follows the existing `tools/` convention: module docstring opening with a one-line summary and a usage line, tests in `test_<module>.py` beside the module, run under pytest.

---

## File Structure

| Path | Responsibility |
| --- | --- |
| `tools/footprint/requirements.txt` | Pinned dependencies for `~/moge-venv` |
| `tools/footprint/README.md` | venv setup and how to run the batch |
| `tools/footprint/paths.py` | The only module that knows the artifact directory layout |
| `tools/footprint/synth.py` | Synthetic scene generator: known geometry, known camera, known footprint |
| `tools/footprint/matte.py` | Stage 1 — chroma-key matte |
| `tools/footprint/camera.py` | Stage 2 — vanishing-point camera fit |
| `tools/footprint/pointmap.py` | Stage 3 — MoGe-2 wrapper |
| `tools/footprint/mirror.py` | Stage 4 — symmetry plane, occluded-half recovery, residual |
| `tools/footprint/gate.py` | Stage 5 — reprojection silhouette IoU |
| `tools/footprint/ground.py` | Stage 6 — ground projection and alpha shape |
| `tools/footprint/profile.py` | Stage 7 — canonicalise and sample 96 stations |
| `tools/footprint/run.py` | Batch driver and CLI |
| `tools/footprint/test_*.py` | pytest module beside each of the above |

`data/footprints/<id>/` holds `matte.png`, `camera.json`, `cloud.npz`, `cloud_resolved.npz`, `footprint.json`, `profile.json`.

---

### Task 1: Scaffold and synthetic ground truth

Everything downstream is tested against this task's output, so it comes first.

**Files:**
- Create: `tools/footprint/requirements.txt`
- Create: `tools/footprint/README.md`
- Create: `tools/footprint/paths.py`
- Create: `tools/footprint/synth.py`
- Test: `tools/footprint/test_synth.py`
- Modify: `.gitignore`

**Interfaces:**
- Produces: `paths.artifact_dir(ship_id: str) -> pathlib.Path`, `paths.HERO_GLOB: str`, `paths.FOOTPRINT_ROOT: pathlib.Path`
- Produces: `synth.Scene` dataclass with fields `image: np.ndarray (H,W,3) uint8`, `K: np.ndarray (3,3)`, `R: np.ndarray (3,3)`, `t: np.ndarray (3,)`, `points: np.ndarray (N,3)` world-space surface samples, `footprint_xz: np.ndarray (M,2)` ground-truth footprint polygon, `up: np.ndarray (3,)`
- Produces: `synth.box_scene(length, width, height, azimuth_deg, elevation_deg, ortho=False) -> Scene`
- Produces: `synth.cylinder_scene(radius, height, azimuth_deg, elevation_deg, ortho=False) -> Scene`

- [ ] **Step 1: Create the venv and pin dependencies**

```bash
python3 -m venv ~/moge-venv
~/moge-venv/bin/pip install --upgrade pip
~/moge-venv/bin/pip install torch --index-url https://download.pytorch.org/whl/cu121
~/moge-venv/bin/pip install git+https://github.com/microsoft/MoGe.git
~/moge-venv/bin/pip install opencv-python-headless numpy scipy shapely alphashape lu-vp-detect pytest
~/moge-venv/bin/pip freeze > tools/footprint/requirements.lock.txt
```

Then hand-write `tools/footprint/requirements.txt` with the direct dependencies only. A full `pip freeze` records torch's CUDA-specific build, which plain `pip install -r` cannot resolve from PyPI — so the freeze is a lockfile for exact reproduction, and this file is what the README's setup instructions actually use:

```
# Direct dependencies. torch must come from the CUDA index first:
#   pip install torch --index-url https://download.pytorch.org/whl/cu121
# Exact versions that produced a given result: requirements.lock.txt
torch>=2.5
moge @ git+https://github.com/microsoft/MoGe.git
opencv-python-headless>=4.10
numpy>=1.26
scipy>=1.14
shapely>=2.0
alphashape>=1.3
lu-vp-detect>=1.0
pytest>=8.0
```

Take the lower bounds from what actually installed — read them out of `requirements.lock.txt` rather than copying the numbers above if they differ.

Verify CUDA is visible before continuing:

```bash
~/moge-venv/bin/python -c "import torch; print(torch.__version__, torch.cuda.is_available())"
```

Expected: prints a torch version and `True`. If it prints `False`, stop and report — the rest of the pipeline is unusable on CPU at this scale.

- [ ] **Step 2: Write `paths.py`**

```python
#!/usr/bin/env python3
"""Artifact layout for the footprint recovery pipeline.

The single place that knows where per-ship intermediates live, so pointing the
batch at a different art drop is a path change here and nowhere else.

    from tools.footprint import paths
"""

import os
import pathlib

REPO_ROOT = pathlib.Path(__file__).resolve().parents[2]
FOOTPRINT_ROOT = REPO_ROOT / "data" / "footprints"

# Hero art location. Override with SMKB_HERO_DIR when a full art drop arrives.
HERO_DIR = pathlib.Path(os.environ.get("SMKB_HERO_DIR", pathlib.Path.home() / "Downloads"))
HERO_GLOB = "*.webp"


def artifact_dir(ship_id: str) -> pathlib.Path:
    """Return the per-ship artifact directory, creating it if absent."""
    d = FOOTPRINT_ROOT / ship_id
    d.mkdir(parents=True, exist_ok=True)
    return d
```

- [ ] **Step 3: Write the failing test for the synthetic box**

```python
"""Ground-truth checks for the synthetic scene generator.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_synth.py
"""

import importlib
import pathlib
import sys

import numpy as np

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
synth = importlib.import_module("tools.footprint.synth")


def test_box_scene_footprint_is_a_rectangle_of_the_right_aspect():
    s = synth.box_scene(length=4.0, width=2.0, height=1.5,
                        azimuth_deg=35.0, elevation_deg=30.0)
    fp = s.footprint_xz
    extent_x = fp[:, 0].max() - fp[:, 0].min()
    extent_z = fp[:, 1].max() - fp[:, 1].min()
    assert abs(extent_x - 4.0) < 1e-6, extent_x
    assert abs(extent_z - 2.0) < 1e-6, extent_z


def test_box_scene_renders_a_subject_on_magenta():
    s = synth.box_scene(length=4.0, width=2.0, height=1.5,
                        azimuth_deg=35.0, elevation_deg=30.0)
    assert s.image.shape[2] == 3
    corner = s.image[0, 0]
    assert corner[0] > 200 and corner[2] > 200 and corner[1] < 60, corner
    # The subject must actually cover a sensible slice of frame.
    bg = np.all(np.abs(s.image.astype(int) - corner.astype(int)) < 30, axis=-1)
    covered = 1.0 - bg.mean()
    assert 0.05 < covered < 0.85, covered


def test_cylinder_scene_footprint_is_circular():
    s = synth.cylinder_scene(radius=1.0, height=3.0,
                             azimuth_deg=20.0, elevation_deg=35.0)
    fp = s.footprint_xz
    r = np.linalg.norm(fp - fp.mean(axis=0), axis=1)
    assert abs(r.mean() - 1.0) < 0.02, r.mean()
    assert r.std() < 0.02, r.std()
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_synth.py -v`
Expected: FAIL — `synth.py` does not exist, collection error.

- [ ] **Step 5: Write `synth.py`**

The renderer is a painter's-algorithm rasteriser: build a mesh, transform to camera space, project, depth-sort faces, `cv2.fillPoly` back to front with per-face shading. No extra dependencies.

```python
#!/usr/bin/env python3
"""Synthetic scenes with known geometry, camera and ground-truth footprint.

Every later pipeline stage is tested against these, so no stage's correctness
depends on trusting a hero image. Renders a magenta-backed 3/4 view matching
the hero art convention.

    from tools.footprint import synth
    scene = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
"""

import dataclasses

import cv2
import numpy as np

MAGENTA = (255, 0, 255)
IMAGE_SIZE = (900, 1200)  # (H, W), the hero art aspect
UP = np.array([0.0, 1.0, 0.0])


@dataclasses.dataclass
class Scene:
    image: np.ndarray
    K: np.ndarray
    R: np.ndarray
    t: np.ndarray
    points: np.ndarray
    footprint_xz: np.ndarray
    up: np.ndarray
    ortho: bool


def _look_at(azimuth_deg, elevation_deg, distance):
    """Camera rotation and translation for an orbit position around the origin."""
    az, el = np.radians(azimuth_deg), np.radians(elevation_deg)
    eye = distance * np.array([np.cos(el) * np.sin(az), np.sin(el), np.cos(el) * np.cos(az)])
    fwd = -eye / np.linalg.norm(eye)
    right = np.cross(fwd, UP)
    right /= np.linalg.norm(right)
    down = np.cross(fwd, right)
    R = np.stack([right, down, fwd])  # world -> camera, OpenCV axes
    return R, -R @ eye


def _box_mesh(length, width, height):
    hx, hy, hz = length / 2, height / 2, width / 2
    v = np.array([[sx * hx, sy * hy, sz * hz]
                  for sx in (-1, 1) for sy in (-1, 1) for sz in (-1, 1)], dtype=float)
    f = [(0, 1, 3, 2), (4, 6, 7, 5), (0, 4, 5, 1),
         (2, 3, 7, 6), (0, 2, 6, 4), (1, 5, 7, 3)]
    return v, f


def _cylinder_mesh(radius, height, segments=48):
    ang = np.linspace(0, 2 * np.pi, segments, endpoint=False)
    ring = np.stack([radius * np.cos(ang), np.zeros_like(ang), radius * np.sin(ang)], axis=1)
    bot, top = ring - [0, height / 2, 0], ring + [0, height / 2, 0]
    v = np.vstack([bot, top])
    f = [(i, (i + 1) % segments, segments + (i + 1) % segments, segments + i)
         for i in range(segments)]
    f.append(tuple(range(segments, 2 * segments)))
    f.append(tuple(reversed(range(segments))))
    return v, f


def _render(v, f, K, R, t, ortho):
    cam = v @ R.T + t
    if ortho:
        scale = K[0, 0] / abs(t[2])
        uv = cam[:, :2] * scale + np.array([K[0, 2], K[1, 2]])
    else:
        uv = (cam @ K.T)[:, :2] / cam[:, 2:3]
    img = np.full((*IMAGE_SIZE, 3), MAGENTA, dtype=np.uint8)
    order = sorted(range(len(f)), key=lambda i: -cam[list(f[i])][:, 2].mean())
    for rank, i in enumerate(order):
        poly = uv[list(f[i])].astype(np.int32)
        shade = 90 + int(120 * (rank + 1) / len(f))
        cv2.fillPoly(img, [poly], (shade, shade, shade))
    return img


def _surface_samples(v, f, per_face=400, seed=0):
    """Sample points across every face, standing in for a dense surface scan."""
    rng = np.random.default_rng(seed)
    out = []
    for face in f:
        p = v[list(face)]
        for _ in range(per_face):
            w = rng.random(len(p))
            out.append((p * (w / w.sum())[:, None]).sum(axis=0))
    return np.array(out)


def _scene(v, f, footprint_xz, azimuth_deg, elevation_deg, ortho):
    H, W = IMAGE_SIZE
    focal = 1400.0
    K = np.array([[focal, 0, W / 2], [0, focal, H / 2], [0, 0, 1]])
    radius = 4.0 * max(v.max(axis=0) - v.min(axis=0))
    R, t = _look_at(azimuth_deg, elevation_deg, radius)
    return Scene(image=_render(v, f, K, R, t, ortho), K=K, R=R, t=t,
                 points=_surface_samples(v, f), footprint_xz=footprint_xz,
                 up=UP.copy(), ortho=ortho)


def box_scene(length, width, height, azimuth_deg, elevation_deg, ortho=False):
    v, f = _box_mesh(length, width, height)
    hx, hz = length / 2, width / 2
    fp = np.array([[-hx, -hz], [hx, -hz], [hx, hz], [-hx, hz]])
    return _scene(v, f, fp, azimuth_deg, elevation_deg, ortho)


def cylinder_scene(radius, height, azimuth_deg, elevation_deg, ortho=False):
    v, f = _cylinder_mesh(radius, height)
    ang = np.linspace(0, 2 * np.pi, 128, endpoint=False)
    fp = np.stack([radius * np.cos(ang), radius * np.sin(ang)], axis=1)
    return _scene(v, f, fp, azimuth_deg, elevation_deg, ortho)
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_synth.py -v`
Expected: PASS, 3 tests.

If `test_box_scene_renders_a_subject_on_magenta` fails on coverage, adjust the `radius` multiplier in `_scene` — the subject must fill a reasonable share of frame for later stages to have anything to work with.

- [ ] **Step 7: Write `README.md`**

```markdown
# Footprint recovery

Recovers a measured top-down hull footprint from a ship hero image.
Design: `docs/superpowers/specs/2026-07-29-hero-art-footprint-recovery-design.md`

## Setup

    python3 -m venv ~/moge-venv
    ~/moge-venv/bin/pip install torch --index-url https://download.pytorch.org/whl/cu121
    ~/moge-venv/bin/pip install -r tools/footprint/requirements.txt

torch comes from the CUDA index first; the rest resolve from PyPI. To
reproduce an exact environment instead, use `requirements.lock.txt`.

This venv is deliberately separate from `~/sd-venv`; do not merge them.

## Run

    ~/moge-venv/bin/python tools/footprint/run.py --all

Artifacts land in `data/footprints/<id>/`. Only `profile.json` is committed.
Point at a different art drop with `SMKB_HERO_DIR=/path/to/art`.

## Tests

    ~/moge-venv/bin/python -m pytest tools/footprint/
```

- [ ] **Step 8: Ignore regenerable artifacts**

Append to `.gitignore`:

```
data/footprints/*/*.npz
data/footprints/*/matte.png
```

- [ ] **Step 9: Commit**

```bash
git add tools/footprint .gitignore
git commit -m "feat(footprint): scaffold pipeline with synthetic ground truth"
```

---

### Task 2: Stage 1 — chroma-key matte

**Files:**
- Create: `tools/footprint/matte.py`
- Test: `tools/footprint/test_matte.py`

**Interfaces:**
- Consumes: `synth.box_scene`, `paths.artifact_dir`
- Produces: `matte.extract(image_rgb: np.ndarray) -> tuple[np.ndarray, float]` returning a `(H,W)` uint8 mask in `{0,1}` and the foreground fraction
- Produces: `matte.run(ship_id: str, image_rgb: np.ndarray) -> float` which writes `matte.png` and returns the foreground fraction

- [ ] **Step 1: Write the failing test**

```python
"""Checks for the chroma-key matte.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_matte.py
"""

import importlib.util
import os

import numpy as np

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

synth = _load("synth")
matte = _load("matte")


def test_matte_recovers_the_rendered_subject():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, frac = matte.extract(s.image)

    assert mask.shape == s.image.shape[:2]
    assert set(np.unique(mask)).issubset({0, 1})
    assert 0.05 < frac < 0.85, frac
    # Corners are background in every hero image.
    for y, x in [(0, 0), (0, -1), (-1, 0), (-1, -1)]:
        assert mask[y, x] == 0


def test_matte_drops_disconnected_specks():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    img = s.image.copy()
    img[5:9, 5:9] = (20, 20, 20)  # an isolated non-magenta speck
    mask, _ = matte.extract(img)
    assert mask[5:9, 5:9].sum() == 0


def test_matte_is_not_fooled_by_a_magenta_subject_pixel():
    # A pixel that matches the background exactly, but sits inside the hull,
    # must still be foreground: the mask is filled, not merely thresholded.
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    ys, xs = np.nonzero(mask)
    cy, cx = int(ys.mean()), int(xs.mean())
    img = s.image.copy()
    img[cy, cx] = matte.background_color(s.image)
    mask2, _ = matte.extract(img)
    assert mask2[cy, cx] == 1
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_matte.py -v`
Expected: FAIL — `matte.py` does not exist.

- [ ] **Step 3: Write `matte.py`**

```python
#!/usr/bin/env python3
"""Stage 1: chroma-key the magenta hero background into a binary matte.

The hero backgrounds are a flat key, so a colour-distance threshold gives an
exact alpha without a segmentation model. The matte doubles as the view-plane
silhouette that stage 5 checks the reconstruction against.

    ~/moge-venv/bin/python -m tools.footprint.matte <image>
"""

import cv2
import numpy as np

from . import paths

TOLERANCE = 60.0
MIN_COMPONENT_FRACTION = 0.01


def background_color(image_rgb: np.ndarray) -> np.ndarray:
    """Median of the four corner patches — the flat key colour."""
    k = 8
    patches = np.concatenate([
        image_rgb[:k, :k].reshape(-1, 3), image_rgb[:k, -k:].reshape(-1, 3),
        image_rgb[-k:, :k].reshape(-1, 3), image_rgb[-k:, -k:].reshape(-1, 3)])
    return np.median(patches, axis=0).astype(image_rgb.dtype)


def extract(image_rgb: np.ndarray, tolerance: float = TOLERANCE):
    """Return a (H,W) uint8 mask in {0,1} and the foreground fraction."""
    bg = background_color(image_rgb).astype(np.float32)
    dist = np.linalg.norm(image_rgb.astype(np.float32) - bg, axis=-1)
    mask = (dist > tolerance).astype(np.uint8)

    kernel = np.ones((5, 5), np.uint8)
    mask = cv2.morphologyEx(mask, cv2.MORPH_OPEN, kernel)
    mask = cv2.morphologyEx(mask, cv2.MORPH_CLOSE, kernel)

    # Keep only the largest connected component: hero art has one subject, and
    # stray specks would otherwise widen the footprint.
    n, labels, stats, _ = cv2.connectedComponentsWithStats(mask, connectivity=8)
    if n > 1:
        areas = stats[1:, cv2.CC_STAT_AREA]
        mask = (labels == 1 + int(np.argmax(areas))).astype(np.uint8)

    # Fill interior holes so subject pixels that happen to match the key colour
    # are not punched out of the middle of the hull.
    filled = mask.copy()
    h, w = mask.shape
    flood = np.zeros((h + 2, w + 2), np.uint8)
    cv2.floodFill(filled, flood, (0, 0), 1)
    mask = (mask | (1 - filled)).astype(np.uint8)

    return mask, float(mask.mean())


def run(ship_id: str, image_rgb: np.ndarray) -> float:
    mask, frac = extract(image_rgb)
    cv2.imwrite(str(paths.artifact_dir(ship_id) / "matte.png"), mask * 255)
    return frac
```

Note the flood-fill runs from `(0,0)`, which is background in every hero image. If a future art drop crops the subject to the frame edge, that assumption breaks and the fill must seed from a known-background pixel instead.

- [ ] **Step 4: Run the test to verify it passes**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_matte.py -v`
Expected: PASS, 3 tests.

- [ ] **Step 5: Eyeball the mattes on the real art**

```bash
~/moge-venv/bin/python - <<'PY'
import cv2, glob, os, sys
sys.path.insert(0, "tools")
from footprint import matte
for p in sorted(glob.glob(os.path.expanduser("~/Downloads/*.webp")))[:5]:
    img = cv2.cvtColor(cv2.imread(p), cv2.COLOR_BGR2RGB)
    if img is None: continue
    m, frac = matte.extract(img)
    print(f"{os.path.basename(p):28s} foreground {frac:.3f}")
PY
```

Expected: foreground fractions roughly in 0.15–0.6. A value near 0 or near 1 means the key colour was misdetected on that image; note which and raise it rather than adjusting `TOLERANCE` until the number looks nice.

- [ ] **Step 6: Commit**

```bash
git add tools/footprint/matte.py tools/footprint/test_matte.py
git commit -m "feat(footprint): stage 1 chroma-key matte"
```

---

### Task 3: Stage 2 — vanishing-point camera fit

**Files:**
- Create: `tools/footprint/camera.py`
- Test: `tools/footprint/test_camera.py`

**Interfaces:**
- Consumes: `synth.Scene`, `matte.extract`, `paths.artifact_dir`
- Produces: `camera.fit(image_rgb, mask) -> camera.Fit` where `Fit` is a dataclass with `R: np.ndarray (3,3)`, `focal: float | None` (None means orthographic), `principal: tuple[float,float]`, `confidence: float`, `source: str` in `{"auto","clicks"}`
- Produces: `camera.fit_from_clicks(clicks: dict) -> camera.Fit` reading the fallback format
- Produces: `camera.run(ship_id, image_rgb, mask, clicks=None) -> camera.Fit`, writing `camera.json`
- Produces: `camera.CONFIDENCE_FLOOR = 0.35`

**Fallback click format** — `data/footprints/<id>/clicks.json`:

```json
{"axis_x": [[[x1,y1],[x2,y2]], [[x3,y3],[x4,y4]]],
 "axis_y": [[[x1,y1],[x2,y2]], [[x3,y3],[x4,y4]]],
 "axis_z": [[[x1,y1],[x2,y2]], [[x3,y3],[x4,y4]]]}
```

Two line segments per axis, each two clicked endpoints in pixels. A vanishing point is the intersection of its axis's two lines.

- [ ] **Step 1: Write the failing test**

```python
"""Checks for the vanishing-point camera fit.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_camera.py
"""

import importlib.util
import os

import numpy as np

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

synth = _load("synth")
matte = _load("matte")
camera = _load("camera")


def _angle_between(a, b):
    c = np.clip((a * b).sum() / (np.linalg.norm(a) * np.linalg.norm(b)), -1, 1)
    return np.degrees(np.arccos(abs(c)))


def test_fit_recovers_the_synthetic_rotation_within_five_degrees():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    fit = camera.fit(s.image, mask)

    assert fit.confidence > camera.CONFIDENCE_FLOOR, fit.confidence
    for axis in range(3):
        err = _angle_between(fit.R[axis], s.R[axis])
        assert err < 5.0, f"axis {axis} off by {err:.1f} deg"


def test_fit_recovers_the_synthetic_focal_within_ten_percent():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    fit = camera.fit(s.image, mask)

    assert fit.focal is not None
    assert abs(fit.focal - s.K[0, 0]) / s.K[0, 0] < 0.10, fit.focal


def test_clicks_fallback_matches_the_auto_fit():
    # Two lines per axis taken from the known projection must produce
    # essentially the same rotation as the automatic fit.
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    clicks = camera.clicks_from_scene(s)
    fit = camera.fit_from_clicks(clicks)
    for axis in range(3):
        assert _angle_between(fit.R[axis], s.R[axis]) < 5.0


def test_low_confidence_is_reported_not_hidden():
    # A featureless disc has no parallel structural lines, so the fit must
    # report low confidence rather than inventing a camera.
    s = synth.cylinder_scene(radius=2.0, height=0.2, azimuth_deg=0, elevation_deg=89)
    mask, _ = matte.extract(s.image)
    fit = camera.fit(s.image, mask)
    assert fit.confidence <= camera.CONFIDENCE_FLOOR, fit.confidence
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_camera.py -v`
Expected: FAIL — `camera.py` does not exist.

- [ ] **Step 3: Write `camera.py`**

The focal length is unknown but `lu_vp_detect` needs one, so iterate: fit vanishing points at a seed focal, recompute the focal from the orthocentre of the VP triangle, refit. Three iterations converge on these images.

```python
#!/usr/bin/env python3
"""Stage 2: fit the camera from vanishing points.

These hulls are Manhattan-world objects with long parallel structural lines, so
three orthogonal vanishing points give rotation and focal length outright and
reveal whether a render is orthographic or mildly perspective. Never assume the
3/4 view: a low-confidence fit routes the ship to the hand-clicked fallback.

    ~/moge-venv/bin/python -m tools.footprint.camera <image>
"""

import dataclasses
import json

import cv2
import numpy as np
from lu_vp_detect import VPDetection

from . import paths

CONFIDENCE_FLOOR = 0.35
_SEED_FOCAL = 1500.0
_ITERATIONS = 3
_ORTHO_FOCAL = 1e5  # beyond this the projection is orthographic for our purposes


@dataclasses.dataclass
class Fit:
    R: np.ndarray
    focal: float | None
    principal: tuple
    confidence: float
    source: str

    def to_json(self) -> dict:
        return {"R": self.R.tolist(), "focal": self.focal,
                "principal": list(self.principal),
                "confidence": self.confidence, "source": self.source}


def _focal_from_vps(vps_2d, principal):
    """Orthocentre relation: for orthogonal VPs, f^2 = -(v1-pp).(v2-pp)."""
    pp = np.asarray(principal, dtype=float)
    vals = []
    for i in range(3):
        for j in range(i + 1, 3):
            d = -np.dot(vps_2d[i] - pp, vps_2d[j] - pp)
            if d > 0:
                vals.append(np.sqrt(d))
    return float(np.median(vals)) if vals else None


def _rotation_from_vps(vps_2d, focal, principal):
    pp = np.asarray(principal, dtype=float)
    dirs = []
    for v in vps_2d:
        d = np.array([v[0] - pp[0], v[1] - pp[1], focal])
        dirs.append(d / np.linalg.norm(d))
    R = np.stack(dirs)
    # Re-orthonormalise: the fitted directions are only approximately orthogonal.
    u, _, vt = np.linalg.svd(R)
    return u @ vt


def fit(image_rgb: np.ndarray, mask: np.ndarray) -> Fit:
    h, w = mask.shape
    principal = (w / 2.0, h / 2.0)
    masked = image_rgb.copy()
    masked[mask == 0] = 0

    focal = _SEED_FOCAL
    vpd = None
    for _ in range(_ITERATIONS):
        vpd = VPDetection(length_thresh=min(h, w) * 0.04,
                          principal_point=principal, focal_length=focal, seed=0)
        vpd.find_vps(cv2.cvtColor(masked, cv2.COLOR_RGB2BGR))
        new_focal = _focal_from_vps(vpd.vps_2D, principal)
        if new_focal is None:
            break
        focal = new_focal

    clusters = vpd.get_vp_clusters() if vpd is not None else None
    confidence = _confidence(clusters, vpd)
    if focal is None or confidence <= CONFIDENCE_FLOOR:
        return Fit(R=np.eye(3), focal=None, principal=principal,
                   confidence=confidence, source="auto")

    R = _rotation_from_vps(vpd.vps_2D, focal, principal)
    return Fit(R=R, focal=None if focal > _ORTHO_FOCAL else float(focal),
               principal=principal, confidence=confidence, source="auto")


def _confidence(clusters, vpd):
    """Fraction of detected segments assigned to one of the three VP clusters."""
    if vpd is None or clusters is None:
        return 0.0
    assigned = sum(len(c) for c in clusters)
    total = len(vpd.get_lines()) if hasattr(vpd, "get_lines") else assigned
    if not total:
        return 0.0
    balance = min(len(c) for c in clusters) / max(1, max(len(c) for c in clusters))
    return float(assigned / total) * float(balance)


def _intersect(l1, l2):
    (x1, y1), (x2, y2) = l1
    (x3, y3), (x4, y4) = l2
    d = (x1 - x2) * (y3 - y4) - (y1 - y2) * (x3 - x4)
    if abs(d) < 1e-9:
        return None
    a, b = x1 * y2 - y1 * x2, x3 * y4 - y3 * x4
    return np.array([(a * (x3 - x4) - (x1 - x2) * b) / d,
                     (a * (y3 - y4) - (y1 - y2) * b) / d])


def fit_from_clicks(clicks: dict, principal=None) -> Fit:
    vps = []
    for key in ("axis_x", "axis_y", "axis_z"):
        v = _intersect(clicks[key][0], clicks[key][1])
        if v is None:
            raise ValueError(f"{key}: the two clicked lines are parallel in image space")
        vps.append(v)
    if principal is None:
        principal = tuple(np.mean(vps, axis=0))
    focal = _focal_from_vps(vps, principal)
    if focal is None:
        raise ValueError("clicked vanishing points are not mutually orthogonal")
    return Fit(R=_rotation_from_vps(vps, focal, principal),
               focal=None if focal > _ORTHO_FOCAL else float(focal),
               principal=principal, confidence=1.0, source="clicks")


def clicks_from_scene(scene) -> dict:
    """Synthesise a click file from a known scene, for tests."""
    out = {}
    for name, axis in (("axis_x", 0), ("axis_y", 1), ("axis_z", 2)):
        d = np.zeros(3)
        d[axis] = 1.0
        lines = []
        for offset in (np.zeros(3), np.array([0.3, 0.4, 0.5])):
            pts = np.stack([offset - 2 * d, offset + 2 * d])
            cam = pts @ scene.R.T + scene.t
            uv = (cam @ scene.K.T)[:, :2] / cam[:, 2:3]
            lines.append([uv[0].tolist(), uv[1].tolist()])
        out[name] = lines
    return out


def run(ship_id: str, image_rgb, mask, clicks=None) -> Fit:
    f = fit_from_clicks(clicks) if clicks else fit(image_rgb, mask)
    (paths.artifact_dir(ship_id) / "camera.json").write_text(json.dumps(f.to_json(), indent=2))
    return f
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_camera.py -v`
Expected: PASS, 4 tests.

If `test_fit_recovers_the_synthetic_rotation_within_five_degrees` fails, print `vpd.vps_2D` and check whether the three VPs came out in a different axis order than the scene's — the fit is order-agnostic and the test compares row by row. Sort `dirs` by which world axis each is closest to before building `R`.

- [ ] **Step 5: Report the fit on the real art**

```bash
~/moge-venv/bin/python - <<'PY'
import cv2, glob, os, sys
sys.path.insert(0, "tools")
from footprint import camera, matte
for p in sorted(glob.glob(os.path.expanduser("~/Downloads/*.webp"))):
    img = cv2.cvtColor(cv2.imread(p), cv2.COLOR_BGR2RGB)
    if img is None: continue
    m, _ = matte.extract(img)
    f = camera.fit(img, m)
    kind = "ortho" if f.focal is None else f"f={f.focal:.0f}"
    print(f"{os.path.basename(p):28s} conf {f.confidence:.2f}  {kind}")
PY
```

Record which ships fall below `CONFIDENCE_FLOOR`. Those need click files before the batch runs; that is expected, not a failure.

- [ ] **Step 6: Commit**

```bash
git add tools/footprint/camera.py tools/footprint/test_camera.py
git commit -m "feat(footprint): stage 2 vanishing-point camera fit"
```

---

### Task 4: Stage 3 — MoGe-2 point map

**Files:**
- Create: `tools/footprint/pointmap.py`
- Test: `tools/footprint/test_pointmap.py`

**Interfaces:**
- Consumes: `matte.extract`, `paths.artifact_dir`
- Produces: `pointmap.infer(image_rgb, mask, background="neutral") -> pointmap.Cloud` where `Cloud` is a dataclass with `points: np.ndarray (N,3)`, `pixels: np.ndarray (N,2)` source pixel per point, `normals: np.ndarray (N,3) | None`, `intrinsics: np.ndarray (3,3)`
- Produces: `pointmap.run(ship_id, image_rgb, mask, background) -> Cloud`, writing `cloud.npz`
- Produces: `pointmap.load(ship_id) -> Cloud`
- Produces: `pointmap.MODEL_ID = "Ruicheng/moge-2-vitl-normal"`

- [ ] **Step 1: Write the failing test**

The model is a 2 GB download and needs a GPU, so the tests skip cleanly when it is unavailable rather than failing for the wrong reason.

```python
"""Checks for the MoGe-2 point map wrapper.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_pointmap.py
"""

import importlib.util
import os

import numpy as np
import pytest

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

synth = _load("synth")
matte = _load("matte")
pointmap = _load("pointmap")

torch = pytest.importorskip("torch")
pytestmark = pytest.mark.skipif(not torch.cuda.is_available(), reason="needs CUDA")


@pytest.fixture(scope="module")
def cloud():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    return pointmap.infer(s.image, mask), s


def test_cloud_covers_only_the_subject(cloud):
    c, s = cloud
    assert len(c.points) > 1000
    assert c.points.shape[1] == 3
    assert c.pixels.shape == (len(c.points), 2)
    # Every returned point must come from a foreground pixel.
    mask, _ = matte.extract(s.image)
    px = c.pixels.astype(int)
    assert mask[px[:, 1], px[:, 0]].all()


def test_cloud_is_finite_and_in_front_of_the_camera(cloud):
    c, _ = cloud
    assert np.isfinite(c.points).all()
    assert (c.points[:, 2] > 0).all()


def test_intrinsics_are_returned_and_plausible(cloud):
    c, s = cloud
    assert c.intrinsics.shape == (3, 3)
    # MoGe returns normalised intrinsics; denormalised focal should be within
    # a factor of two of the true one. A wilder value means the FOV recovery
    # disagrees with the render and stage 2 should be believed instead.
    h, w = s.image.shape[:2]
    focal_px = c.intrinsics[0, 0] * w
    assert 0.5 < focal_px / s.K[0, 0] < 2.0, focal_px
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_pointmap.py -v`
Expected: FAIL — `pointmap.py` does not exist.

- [ ] **Step 3: Write `pointmap.py`**

```python
#!/usr/bin/env python3
"""Stage 3: recover a point map with MoGe-2.

MoGe-2 emits a metric-scale point map, a validity mask, per-pixel normals and
normalised intrinsics from a single image, so a point cloud comes out without
asserting a camera. Points outside the stage 1 matte are discarded: the
background is not part of the ship.

    ~/moge-venv/bin/python -m tools.footprint.pointmap <image>
"""

import dataclasses
import functools

import cv2
import numpy as np
import torch
from moge.model.v2 import MoGeModel

from . import paths

MODEL_ID = "Ruicheng/moge-2-vitl-normal"
NEUTRAL = (128, 128, 128)


@dataclasses.dataclass
class Cloud:
    points: np.ndarray
    pixels: np.ndarray
    normals: np.ndarray | None
    intrinsics: np.ndarray


@functools.lru_cache(maxsize=1)
def _model():
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    return MoGeModel.from_pretrained(MODEL_ID).to(device).eval(), device


def _composite(image_rgb, mask, background):
    """Replace the chroma key with a neutral field, or leave it alone."""
    if background == "raw":
        return image_rgb
    out = image_rgb.copy()
    out[mask == 0] = NEUTRAL
    return out


def infer(image_rgb: np.ndarray, mask: np.ndarray, background: str = "neutral") -> Cloud:
    model, device = _model()
    img = _composite(image_rgb, mask, background)
    t = torch.tensor(img / 255.0, dtype=torch.float32, device=device).permute(2, 0, 1)
    with torch.no_grad():
        out = model.infer(t)

    points = out["points"].cpu().numpy()
    valid = out["mask"].cpu().numpy().astype(bool) & (mask > 0)
    normals = out["normal"].cpu().numpy() if "normal" in out else None

    ys, xs = np.nonzero(valid)
    return Cloud(points=points[ys, xs],
                 pixels=np.stack([xs, ys], axis=1).astype(np.float32),
                 normals=None if normals is None else normals[ys, xs],
                 intrinsics=out["intrinsics"].cpu().numpy())


def run(ship_id: str, image_rgb, mask, background: str = "neutral") -> Cloud:
    c = infer(image_rgb, mask, background)
    np.savez_compressed(paths.artifact_dir(ship_id) / "cloud.npz",
                        points=c.points, pixels=c.pixels,
                        normals=np.array([]) if c.normals is None else c.normals,
                        intrinsics=c.intrinsics)
    return c


def load(ship_id: str) -> Cloud:
    z = np.load(paths.artifact_dir(ship_id) / "cloud.npz")
    n = z["normals"]
    return Cloud(points=z["points"], pixels=z["pixels"],
                 normals=None if n.size == 0 else n, intrinsics=z["intrinsics"])
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_pointmap.py -v`
Expected: PASS, 3 tests. First run downloads roughly 2 GB.

- [ ] **Step 5: Settle the background question**

The spec leaves one open experiment: whether MoGe does better on the raw magenta image or on the matted subject composited onto neutral. Flat saturated magenta is out of distribution for a model trained on photographs. Decide it now, on three real ships, and record the answer.

```bash
~/moge-venv/bin/python - <<'PY'
import cv2, os, sys, numpy as np
sys.path.insert(0, "tools")
from footprint import matte, pointmap
for name in ("outerrim_prayer", "magnate", "comet"):
    p = os.path.expanduser(f"~/Downloads/{name}.webp")
    img = cv2.cvtColor(cv2.imread(p), cv2.COLOR_BGR2RGB)
    m, _ = matte.extract(img)
    for bg in ("raw", "neutral"):
        c = pointmap.infer(img, m, background=bg)
        d = c.points[:, 2]
        print(f"{name:18s} {bg:8s} n={len(c.points):7d} "
              f"depth spread={d.std() / max(d.mean(), 1e-6):.3f}")
PY
```

Prefer the setting with the larger relative depth spread on the same subject: a collapsed depth range means the model read the image as flat. Record the winner in `tools/footprint/README.md` under a "Background handling" heading, with the numbers, and set the `run.py` default accordingly in Task 9. If the two are within 5%, keep `neutral` — it is the safer default for a full art drop.

- [ ] **Step 6: Commit**

```bash
git add tools/footprint/pointmap.py tools/footprint/test_pointmap.py tools/footprint/README.md
git commit -m "feat(footprint): stage 3 MoGe-2 point map"
```

---

### Task 5: Stage 4 — mirror-constrained symmetry solve

**Files:**
- Create: `tools/footprint/mirror.py`
- Test: `tools/footprint/test_mirror.py`

**Interfaces:**
- Consumes: `pointmap.Cloud`
- Produces: `mirror.solve(points: np.ndarray, init_scale=1.0, init_shift=0.0, refine_affine=False) -> mirror.Symmetry` where `Symmetry` is a dataclass with `normal: np.ndarray (3,)`, `offset: float`, `scale: float`, `shift: float`, `residual: float`
- Produces: `mirror.reflect(points, normal, offset) -> np.ndarray`
- Produces: `mirror.complete(points, sym) -> np.ndarray` returning the union of the cloud and its reflection
- Produces: `mirror.run(ship_id, cloud, refine_affine=False) -> Symmetry`, writing `cloud_resolved.npz`
- Produces: `mirror.RESIDUAL_CEILING = 0.06`

- [ ] **Step 1: Write the failing test**

```python
"""Checks for the mirror-constrained symmetry solve.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_mirror.py
"""

import importlib.util
import os

import numpy as np

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

mirror = _load("mirror")


def _symmetric_hull(seed=0, n=4000):
    """A bilaterally symmetric slab, mirror plane normal = +Y, offset 0."""
    rng = np.random.default_rng(seed)
    half = rng.uniform([-2, 0.05, -0.5], [2, 1.0, 0.5], size=(n // 2, 3))
    return np.vstack([half, half * [1, -1, 1]])


def test_solve_finds_the_known_symmetry_plane():
    pts = _symmetric_hull()
    sym = mirror.solve(pts)
    axis_err = np.degrees(np.arccos(min(1.0, abs(sym.normal @ [0, 1, 0]))))
    assert axis_err < 5.0, axis_err
    assert abs(sym.offset) < 0.05, sym.offset
    assert sym.residual < mirror.RESIDUAL_CEILING, sym.residual


def test_solve_reports_a_high_residual_on_an_asymmetric_hull():
    # Scrap-built hulls are deliberately lopsided; the residual is how we know
    # not to trust the mirrored half.
    pts = _symmetric_hull()
    lop = pts.copy()
    lop[lop[:, 1] > 0, 0] += 1.2  # shove one whole side down the long axis
    sym = mirror.solve(lop)
    assert sym.residual > mirror.RESIDUAL_CEILING, sym.residual


def test_reflect_is_an_involution():
    pts = _symmetric_hull()
    n, d = np.array([0.0, 1.0, 0.0]), 0.3
    once = mirror.reflect(pts, n, d)
    twice = mirror.reflect(once, n, d)
    assert np.allclose(twice, pts, atol=1e-9)


def test_complete_fills_the_occluded_half():
    # Keep only one side, as a single view would; completion must restore the
    # other side to within the sampling density.
    pts = _symmetric_hull()
    visible = pts[pts[:, 1] > 0]
    sym = mirror.solve(visible)
    full = mirror.complete(visible, sym)
    assert (full[:, 1] < 0).sum() >= len(visible) - 1


def test_affine_refinement_is_a_noop_on_metric_input():
    # MoGe-2 is already metric, so the optional scale/shift solve must not
    # wander away from identity when it has nothing to correct.
    pts = _symmetric_hull()
    sym = mirror.solve(pts, refine_affine=True)
    assert abs(sym.scale - 1.0) < 0.05, sym.scale
    assert abs(sym.shift) < 0.05, sym.shift
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_mirror.py -v`
Expected: FAIL — `mirror.py` does not exist.

- [ ] **Step 3: Write `mirror.py`**

```python
#!/usr/bin/env python3
"""Stage 4: solve the hull's plane of bilateral symmetry.

A single view recovers only the front surface. For a bilaterally symmetric hull
the rest is recoverable by mirroring, and the mirror is also the measurement:
the plane that minimises the distance between the cloud and its reflection is
the symmetry plane, its residual says whether the hull is symmetric at all, and
its intersection with the ground plane gives the longitudinal axis.

MoGe-2 output is metric, so the affine scale/shift solve is off by default. It
stays available for a fallback to MoGe-1, whose point map is affine-invariant
in both scale and shift; solving only for scale would leave the cloud sheared.

    ~/moge-venv/bin/python -m tools.footprint.mirror <ship_id>
"""

import dataclasses

import numpy as np
from scipy.optimize import minimize
from scipy.spatial import cKDTree

from . import paths, pointmap

RESIDUAL_CEILING = 0.06
_SUBSAMPLE = 4000


@dataclasses.dataclass
class Symmetry:
    normal: np.ndarray
    offset: float
    scale: float
    shift: float
    residual: float


def reflect(points: np.ndarray, normal: np.ndarray, offset: float) -> np.ndarray:
    n = normal / np.linalg.norm(normal)
    return points - 2.0 * ((points @ n) - offset)[:, None] * n


def _apply_affine(points, scale, shift):
    out = points.copy()
    out[:, 2] = out[:, 2] * scale + shift
    return out


def _sph(theta, phi):
    return np.array([np.sin(theta) * np.cos(phi), np.sin(theta) * np.sin(phi), np.cos(theta)])


def _chamfer(a, b, tree_a=None):
    tree_a = tree_a or cKDTree(a)
    d1, _ = tree_a.query(b, k=1)
    d2, _ = cKDTree(b).query(a, k=1)
    return float(np.mean(d1) + np.mean(d2)) / 2.0


def solve(points: np.ndarray, init_scale: float = 1.0, init_shift: float = 0.0,
          refine_affine: bool = False) -> Symmetry:
    rng = np.random.default_rng(0)
    idx = rng.choice(len(points), min(_SUBSAMPLE, len(points)), replace=False)
    sample = points[idx]
    centre = sample.mean(axis=0)
    extent = float(np.linalg.norm(sample.max(axis=0) - sample.min(axis=0)))

    # Initialise from the principal axes: for a bilaterally symmetric hull the
    # symmetry-plane normal is one of them, so try all three and keep the best.
    _, _, vt = np.linalg.svd(sample - centre, full_matrices=False)

    def cost(params):
        theta, phi, off = params[:3]
        n = _sph(theta, phi)
        pts = _apply_affine(sample, params[3], params[4]) if refine_affine else sample
        return _chamfer(pts, reflect(pts, n, off)) / max(extent, 1e-9)

    best = None
    for axis in vt:
        theta = np.arccos(np.clip(axis[2], -1, 1))
        phi = np.arctan2(axis[1], axis[0])
        x0 = [theta, phi, float(centre @ axis), init_scale, init_shift]
        bounds = [(0, np.pi), (-np.pi, np.pi), (None, None),
                  (0.2, 5.0) if refine_affine else (init_scale, init_scale),
                  (-extent, extent) if refine_affine else (init_shift, init_shift)]
        res = minimize(cost, x0, method="L-BFGS-B", bounds=bounds,
                       options={"maxiter": 120})
        if best is None or res.fun < best.fun:
            best = res

    theta, phi, off, scale, shift = best.x
    return Symmetry(normal=_sph(theta, phi), offset=float(off),
                    scale=float(scale), shift=float(shift), residual=float(best.fun))


def complete(points: np.ndarray, sym: Symmetry) -> np.ndarray:
    pts = _apply_affine(points, sym.scale, sym.shift)
    return np.vstack([pts, reflect(pts, sym.normal, sym.offset)])


def run(ship_id: str, cloud: pointmap.Cloud, refine_affine: bool = False) -> Symmetry:
    sym = solve(cloud.points, refine_affine=refine_affine)
    np.savez_compressed(paths.artifact_dir(ship_id) / "cloud_resolved.npz",
                        points=complete(cloud.points, sym),
                        normal=sym.normal, offset=sym.offset,
                        scale=sym.scale, shift=sym.shift, residual=sym.residual)
    return sym
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_mirror.py -v`
Expected: PASS, 5 tests.

If `test_solve_finds_the_known_symmetry_plane` lands on the wrong principal axis, the multi-start over `vt` is not being applied — check that all three axes are actually tried and the best kept.

- [ ] **Step 5: Commit**

```bash
git add tools/footprint/mirror.py tools/footprint/test_mirror.py
git commit -m "feat(footprint): stage 4 mirror-constrained symmetry solve"
```

---

### Task 6: Stage 5 — reprojection silhouette gate

**Files:**
- Create: `tools/footprint/gate.py`
- Test: `tools/footprint/test_gate.py`

**Interfaces:**
- Consumes: `pointmap.Cloud`, `mirror.Symmetry`, the stage 1 mask
- Produces: `gate.reproject(points, intrinsics, shape) -> np.ndarray` returning a `(H,W)` uint8 mask
- Produces: `gate.score(points, intrinsics, mask) -> float` returning IoU in `[0,1]`
- Produces: `gate.run(ship_id, points, intrinsics, mask) -> float`, appending to `data/footprints/<id>/quality.json`
- Produces: `gate.IOU_FLOOR = 0.70`

- [ ] **Step 1: Write the failing test**

```python
"""Checks for the reprojection silhouette gate.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_gate.py
"""

import importlib.util
import os

import numpy as np

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

synth = _load("synth")
matte = _load("matte")
gate = _load("gate")


def _project_scene(s):
    """World surface samples through the known camera, as a perfect cloud."""
    return s.points @ s.R.T + s.t


def test_perfect_cloud_scores_near_one():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    iou = gate.score(_project_scene(s), s.K, mask)
    assert iou > 0.90, iou


def test_displaced_cloud_scores_low():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    cam = _project_scene(s)
    cam[:, 0] += 3.0  # slide the whole reconstruction sideways
    iou = gate.score(cam, s.K, mask)
    assert iou < gate.IOU_FLOOR, iou


def test_reprojection_mask_matches_the_image_shape():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    m = gate.reproject(_project_scene(s), s.K, s.image.shape[:2])
    assert m.shape == s.image.shape[:2]
    assert set(np.unique(m)).issubset({0, 1})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_gate.py -v`
Expected: FAIL — `gate.py` does not exist.

- [ ] **Step 3: Write `gate.py`**

```python
#!/usr/bin/env python3
"""Stage 5: check the reconstruction against the view-plane silhouette.

The stage 1 matte is an exact silhouette, so reprojecting the resolved cloud
through the fitted camera and overlaying is a cheap, decisive consistency
check. A ship below the floor is a reconstruction failure: it is listed and
excluded from every aggregate, never quietly averaged in.

    ~/moge-venv/bin/python -m tools.footprint.gate <ship_id>
"""

import json

import cv2
import numpy as np

from . import paths

IOU_FLOOR = 0.70
_DILATE = 5


def reproject(points: np.ndarray, intrinsics: np.ndarray, shape) -> np.ndarray:
    h, w = shape
    K = intrinsics.copy()
    if K[0, 2] <= 2.0:  # MoGe returns intrinsics normalised to the unit square
        K[0, 0] *= w
        K[1, 1] *= h
        K[0, 2] *= w
        K[1, 2] *= h

    front = points[points[:, 2] > 1e-6]
    uv = (front @ K.T)[:, :2] / front[:, 2:3]
    uv = np.round(uv).astype(int)
    ok = (uv[:, 0] >= 0) & (uv[:, 0] < w) & (uv[:, 1] >= 0) & (uv[:, 1] < h)
    out = np.zeros((h, w), np.uint8)
    out[uv[ok, 1], uv[ok, 0]] = 1
    # The cloud is a point set; close it into a region before comparing areas.
    k = np.ones((_DILATE, _DILATE), np.uint8)
    return cv2.morphologyEx(out, cv2.MORPH_CLOSE, k)


def score(points: np.ndarray, intrinsics: np.ndarray, mask: np.ndarray) -> float:
    pred = reproject(points, intrinsics, mask.shape).astype(bool)
    truth = mask.astype(bool)
    union = (pred | truth).sum()
    return float((pred & truth).sum() / union) if union else 0.0


def run(ship_id: str, points, intrinsics, mask) -> float:
    iou = score(points, intrinsics, mask)
    p = paths.artifact_dir(ship_id) / "quality.json"
    data = json.loads(p.read_text()) if p.exists() else {}
    data["silhouette_iou"] = iou
    data["silhouette_pass"] = iou >= IOU_FLOOR
    p.write_text(json.dumps(data, indent=2))
    return iou
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_gate.py -v`
Expected: PASS, 3 tests.

- [ ] **Step 5: Commit**

```bash
git add tools/footprint/gate.py tools/footprint/test_gate.py
git commit -m "feat(footprint): stage 5 reprojection silhouette gate"
```

---

### Task 7: Stage 6 — ground projection and alpha shape

**Files:**
- Create: `tools/footprint/ground.py`
- Test: `tools/footprint/test_ground.py`

**Interfaces:**
- Consumes: `mirror.Symmetry`, `camera.Fit`
- Produces: `ground.up_vector(fit: camera.Fit, sym: mirror.Symmetry, normals=None) -> np.ndarray`
- Produces: `ground.project(points, up) -> np.ndarray (N,2)`
- Produces: `ground.hull(xy: np.ndarray, alpha: float) -> shapely.geometry.Polygon`
- Produces: `ground.sweep_alpha(clouds_xy: list[np.ndarray], candidates: list[float]) -> float`
- Produces: `ground.run(ship_id, points, up, alpha) -> shapely.geometry.Polygon`, writing `footprint.json`

- [ ] **Step 1: Write the failing test**

```python
"""Checks for ground projection and the alpha-shape footprint.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_ground.py
"""

import importlib.util
import os

import numpy as np

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

ground = _load("ground")


def test_projection_drops_the_up_axis():
    pts = np.array([[1.0, 5.0, 2.0], [3.0, -7.0, 4.0]])
    xy = ground.project(pts, up=np.array([0.0, 1.0, 0.0]))
    assert np.allclose(xy, [[1.0, 2.0], [3.0, 4.0]])


def test_alpha_shape_keeps_a_concavity_a_convex_hull_would_erase():
    # Two parallel bars with a gap: the nacelle case. A convex hull spans the
    # gap; an alpha shape must not.
    rng = np.random.default_rng(0)
    left = rng.uniform([-2, -1.0], [2, -0.4], size=(3000, 2))
    right = rng.uniform([-2, 0.4], [2, 1.0], size=(3000, 2))
    xy = np.vstack([left, right])

    poly = ground.hull(xy, alpha=3.0)
    assert not poly.contains_properly(_point(0.0, 0.0)), "gap was filled in"
    assert poly.contains(_point(0.0, -0.7))
    assert poly.contains(_point(0.0, 0.7))


def _point(x, y):
    from shapely.geometry import Point
    return Point(x, y)


def test_alpha_shape_of_a_solid_rectangle_has_the_right_area():
    rng = np.random.default_rng(1)
    xy = rng.uniform([-2, -1], [2, 1], size=(6000, 2))
    poly = ground.hull(xy, alpha=3.0)
    assert abs(poly.area - 8.0) / 8.0 < 0.15, poly.area


def test_sweep_picks_one_alpha_for_the_whole_batch():
    rng = np.random.default_rng(2)
    clouds = [rng.uniform([-2, -1], [2, 1], size=(2000, 2)) for _ in range(3)]
    alpha = ground.sweep_alpha(clouds, candidates=[0.5, 3.0, 20.0])
    assert alpha in (0.5, 3.0, 20.0)
    assert isinstance(alpha, float)
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_ground.py -v`
Expected: FAIL — `ground.py` does not exist.

- [ ] **Step 3: Write `ground.py`**

```python
#!/usr/bin/env python3
"""Stage 6: project the resolved cloud to the ground plane and outline it.

The footprint is an alpha shape, never a convex hull: these hulls have real
concavities between nacelles and a convex hull erases exactly the features that
tell one ship from another. The alpha radius is pinned once per batch, because
a per-ship alpha is a free parameter that can be turned until any footprint
looks right.

    ~/moge-venv/bin/python -m tools.footprint.ground <ship_id>
"""

import json

import alphashape
import numpy as np
from shapely.geometry import MultiPolygon, mapping

from . import paths

ALPHA_CANDIDATES = [0.5, 1.0, 2.0, 3.0, 5.0, 8.0, 13.0]


def up_vector(fit, sym, normals=None) -> np.ndarray:
    """World up in camera coordinates.

    The vanishing-point fit is authoritative when it is confident: its three
    axes are the Manhattan frame, and up is the one most nearly perpendicular
    to the symmetry-plane normal and most vertical in image space. Failing
    that, fall back to the dominant surface normal, which on a ship is the deck.
    """
    if fit is not None and fit.confidence > 0:
        candidates = list(fit.R) + [-a for a in fit.R]
        best = max(candidates, key=lambda a: -abs(float(a @ sym.normal)))
        return best / np.linalg.norm(best)
    if normals is not None and len(normals):
        mean = normals.mean(axis=0)
        return mean / np.linalg.norm(mean)
    raise ValueError("no camera fit and no normals: up vector is undetermined")


def project(points: np.ndarray, up: np.ndarray) -> np.ndarray:
    """Drop the up component, returning ground-plane coordinates."""
    u = up / np.linalg.norm(up)
    a = np.array([1.0, 0.0, 0.0])
    if abs(a @ u) > 0.9:
        a = np.array([0.0, 0.0, 1.0])
    e1 = a - (a @ u) * u
    e1 /= np.linalg.norm(e1)
    e2 = np.cross(u, e1)
    return np.stack([points @ e1, points @ e2], axis=1)


def hull(xy: np.ndarray, alpha: float):
    poly = alphashape.alphashape(xy, alpha)
    if isinstance(poly, MultiPolygon):
        poly = max(poly.geoms, key=lambda g: g.area)
    return poly


def sweep_alpha(clouds_xy, candidates=None) -> float:
    """Pick one alpha for the whole batch: the tightest that stays connected.

    Larger alpha hugs the points more closely but fragments; the batch value is
    the largest candidate for which every cloud still yields a single polygon
    holding most of its points.
    """
    candidates = candidates or ALPHA_CANDIDATES
    best = float(candidates[0])
    for a in sorted(float(c) for c in candidates):
        ok = True
        for xy in clouds_xy:
            p = hull(xy, a)
            if p.is_empty or p.area <= 0:
                ok = False
                break
            from shapely.geometry import MultiPoint
            covered = p.buffer(1e-9).intersection(MultiPoint(xy.tolist()))
            kept = len(covered.geoms) if hasattr(covered, "geoms") else int(not covered.is_empty)
            if kept < 0.9 * len(xy):
                ok = False
                break
        if ok:
            best = a
    return best


def run(ship_id: str, points, up, alpha: float):
    xy = project(points, up)
    poly = hull(xy, alpha)
    (paths.artifact_dir(ship_id) / "footprint.json").write_text(
        json.dumps({"alpha": alpha, "polygon": mapping(poly)}, indent=2))
    return poly
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_ground.py -v`
Expected: PASS, 4 tests.

If `test_alpha_shape_keeps_a_concavity_a_convex_hull_would_erase` fails, alpha is too small — a small alpha degenerates to the convex hull. Raise the test's alpha until the gap survives and record the working value in `ALPHA_CANDIDATES`.

- [ ] **Step 5: Commit**

```bash
git add tools/footprint/ground.py tools/footprint/test_ground.py
git commit -m "feat(footprint): stage 6 ground projection and alpha shape"
```

---

### Task 8: Stage 7 — canonical profile

**Files:**
- Create: `tools/footprint/profile.py`
- Test: `tools/footprint/test_profile.py`

**Interfaces:**
- Consumes: a shapely `Polygon` from `ground.run`
- Produces: `profile.canonicalise(poly, sym_normal_xy) -> shapely.geometry.Polygon` rotated so the long axis is +X and scaled to unit length
- Produces: `profile.sample(poly, stations=96) -> tuple[np.ndarray, np.ndarray]` returning half-widths and a boolean concavity flag per station
- Produces: `profile.run(ship_id, poly, sym_normal_xy, quality: dict) -> dict`, writing `profile.json`
- Produces: `profile.STATIONS = 96`

**Output format** — `data/footprints/<id>/profile.json`:

```json
{"id": "prayer", "stations": 96,
 "w": [0.0, 0.031, "... 96 floats, half-width in units where length == 1 ..."],
 "concave": [false, false, "... 96 booleans ..."],
 "aspect": 3.24,
 "quality": {"silhouette_iou": 0.93, "mirror_residual": 0.012,
             "camera_confidence": 0.81, "camera_source": "auto", "alpha": 3.0}}
```

`aspect` is `1 / (2 * max(w))`, which is exactly `Descriptor.Aspect` — length divided by maximum beam — so the consuming plan does not have to rederive it.

- [ ] **Step 1: Write the failing test**

```python
"""Checks for canonicalisation and station sampling.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_profile.py
"""

import importlib.util
import os

import numpy as np
from shapely.geometry import Polygon

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

profile = _load("profile")


def test_canonicalise_puts_the_long_axis_on_x_and_scales_to_unit_length():
    # A 4 x 1 rectangle lying along Y, so canonicalisation must rotate it.
    poly = Polygon([(-0.5, -2), (0.5, -2), (0.5, 2), (-0.5, 2)])
    out = profile.canonicalise(poly, sym_normal_xy=np.array([1.0, 0.0]))
    xs, ys = np.array(out.exterior.coords).T
    assert abs((xs.max() - xs.min()) - 1.0) < 1e-6
    assert abs((ys.max() - ys.min()) - 0.25) < 1e-6


def test_sample_returns_half_widths_of_a_unit_rectangle():
    poly = Polygon([(0, -0.1), (1, -0.1), (1, 0.1), (0, 0.1)])
    w, concave = profile.sample(poly)
    assert len(w) == profile.STATIONS
    assert len(concave) == profile.STATIONS
    interior = w[5:-5]
    assert np.allclose(interior, 0.1, atol=1e-6), interior
    assert not concave.any()


def test_sample_flags_a_split_cross_section():
    # Two bars with a gap between them: the station cuts two intervals, which
    # a single half-width per station cannot represent.
    poly = Polygon([(0, -0.3), (1, -0.3), (1, -0.1), (0, -0.1)]).union(
        Polygon([(0, 0.1), (1, 0.1), (1, 0.3), (0, 0.3)]))
    w, concave = profile.sample(poly)
    assert concave[48], "a split cross-section must be flagged"
    assert abs(w[48] - 0.3) < 1e-6, w[48]


def test_aspect_matches_length_over_maximum_beam():
    poly = Polygon([(0, -0.1), (1, -0.1), (1, 0.1), (0, 0.1)])
    w, _ = profile.sample(poly)
    assert abs(profile.aspect(w) - 5.0) < 1e-6, profile.aspect(w)
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_profile.py -v`
Expected: FAIL — `profile.py` does not exist.

- [ ] **Step 3: Write `profile.py`**

```python
#!/usr/bin/env python3
"""Stage 7: canonicalise the footprint into glyph space and sample it.

Glyph space runs X from 0 at the nose to 1 at the tail with Y a signed
half-width, so a canonicalised footprint sampled at the same 96 stations
pkg/shipglyph uses is directly comparable to an inferred profile. Stations
where the cross-section splits are flagged: a single half-width per station
cannot express the gap between two nacelles, and that limitation is a finding,
not something to average away.

    ~/moge-venv/bin/python -m tools.footprint.profile <ship_id>
"""

import json

import numpy as np
from shapely import affinity
from shapely.geometry import LineString

from . import paths

STATIONS = 96


def canonicalise(poly, sym_normal_xy: np.ndarray):
    """Rotate so the longitudinal axis is +X, translate to x=0, scale length to 1."""
    n = np.asarray(sym_normal_xy, dtype=float)
    n /= np.linalg.norm(n)
    axis = np.array([-n[1], n[0]])  # the symmetry plane's in-ground direction
    angle = np.degrees(np.arctan2(axis[1], axis[0]))
    out = affinity.rotate(poly, -angle, origin="centroid", use_radians=False)

    xs, ys = np.array(out.exterior.coords).T
    length = xs.max() - xs.min()
    if length <= 0:
        raise ValueError("degenerate footprint: zero length along the hull axis")
    out = affinity.translate(out, xoff=-xs.min(), yoff=-(ys.min() + ys.max()) / 2.0)
    return affinity.scale(out, xfact=1.0 / length, yfact=1.0 / length, origin=(0, 0))


def sample(poly, stations: int = STATIONS):
    """Half-width and split flag at each station along the hull."""
    miny, maxy = poly.bounds[1], poly.bounds[3]
    pad = max(abs(miny), abs(maxy)) + 1.0
    w = np.zeros(stations)
    concave = np.zeros(stations, dtype=bool)

    for i in range(stations):
        t = i / (stations - 1)
        cut = poly.intersection(LineString([(t, -pad), (t, pad)]))
        if cut.is_empty:
            continue
        parts = list(cut.geoms) if hasattr(cut, "geoms") else [cut]
        parts = [p for p in parts if p.length > 0]
        if not parts:
            continue
        ys = np.concatenate([np.array(p.coords)[:, 1] for p in parts])
        w[i] = float(np.abs(ys).max())
        concave[i] = len(parts) > 1
    return w, concave


def aspect(w: np.ndarray) -> float:
    """Length over maximum beam — exactly Descriptor.Aspect."""
    m = float(np.max(w))
    return float("inf") if m <= 0 else 1.0 / (2.0 * m)


def run(ship_id: str, poly, sym_normal_xy, quality: dict) -> dict:
    canon = canonicalise(poly, sym_normal_xy)
    w, concave = sample(canon)
    data = {"id": ship_id, "stations": STATIONS, "w": w.tolist(),
            "concave": concave.tolist(), "aspect": aspect(w), "quality": quality}
    (paths.artifact_dir(ship_id) / "profile.json").write_text(json.dumps(data, indent=2))
    return data
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_profile.py -v`
Expected: PASS, 4 tests.

- [ ] **Step 5: Commit**

```bash
git add tools/footprint/profile.py tools/footprint/test_profile.py
git commit -m "feat(footprint): stage 7 canonical profile"
```

---

### Task 9: Batch driver and the end-to-end test

**Files:**
- Create: `tools/footprint/run.py`
- Test: `tools/footprint/test_endtoend.py`
- Modify: `tools/footprint/README.md`

**Interfaces:**
- Consumes: every stage module
- Produces: `run.resolve_heroes() -> dict[str, pathlib.Path]` mapping ship ID to hero image path
- Produces: `run.process(ship_id, image_path, alpha, background) -> dict` returning the profile dict or a failure record
- Produces: `run.main()` behind `if __name__ == "__main__"`, with `--all`, `--ship <id>`, `--background {raw,neutral}`, `--report <path>`

**The end-to-end test has two modes, and only one of them is a hard assertion.** Stages 4 through 7 are our own arithmetic and must be exact on ground-truth input. Stage 3 is a neural network being asked about a flat-shaded synthetic render that looks nothing like its training data, so its accuracy is reported, never asserted — a hard assertion there would fail for reasons that say nothing about this code.

- [ ] **Step 1: Write the failing end-to-end test**

```python
"""End-to-end checks for the footprint pipeline.

Stages 4-7 are asserted exactly against ground truth. The MoGe stage is
reported, not asserted: it is a network being shown a synthetic render, and a
hard assertion there would fail for reasons unrelated to this code.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_endtoend.py -v -s
"""

import importlib.util
import os

import numpy as np
import pytest

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

synth = _load("synth")
mirror = _load("mirror")
ground = _load("ground")
profile = _load("profile")


def _ground_truth_chain(scene):
    """Stages 4-7 on a perfect cloud: our arithmetic, no network involved."""
    cam = scene.points @ scene.R.T + scene.t
    sym = mirror.solve(cam)
    full = mirror.complete(cam, sym)
    up = scene.up @ scene.R.T
    xy = ground.project(full, up)
    poly = ground.hull(xy, alpha=3.0)
    sym_xy = ground.project(sym.normal[None, :], up)[0]
    return profile.sample(profile.canonicalise(poly, sym_xy))


def test_box_recovers_a_rectangular_profile():
    s = synth.box_scene(length=4.0, width=2.0, height=1.5,
                        azimuth_deg=35, elevation_deg=30)
    w, concave = _ground_truth_chain(s)

    interior = w[8:-8]
    assert interior.std() / interior.mean() < 0.10, "a box must have constant beam"
    assert abs(profile.aspect(w) - 2.0) / 2.0 < 0.15, profile.aspect(w)
    assert not concave.any()


def test_vertical_cylinder_recovers_a_circular_profile():
    s = synth.cylinder_scene(radius=1.0, height=3.0,
                             azimuth_deg=20, elevation_deg=35)
    w, _ = _ground_truth_chain(s)

    # A circle of unit length has half-width sqrt(t - t^2) / 2 ... check the
    # midships beam equals the length, i.e. aspect 1.
    assert abs(profile.aspect(w) - 1.0) < 0.20, profile.aspect(w)
    assert w[48] > w[8] and w[48] > w[-8], "a circle must be widest amidships"


@pytest.mark.skipif(
    not __import__("importlib").util.find_spec("torch")
    or not __import__("torch").cuda.is_available(), reason="needs CUDA")
def test_moge_chain_is_reported_not_asserted(capsys):
    pointmap = _load("pointmap")
    matte = _load("matte")
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    cloud = pointmap.infer(s.image, mask)
    sym = mirror.solve(cloud.points)
    with capsys.disabled():
        print(f"\n  MoGe on synthetic box: n={len(cloud.points)} "
              f"mirror residual={sym.residual:.4f} "
              f"(ceiling {mirror.RESIDUAL_CEILING})")
    assert len(cloud.points) > 0


@pytest.mark.skipif(
    not __import__("importlib").util.find_spec("torch")
    or not __import__("torch").cuda.is_available(), reason="needs CUDA")
def test_moge_clears_a_floor_on_real_hero_art():
    """The gate on what we actually depend on.

    The synthetic check above is reporting-only because a flat-shaded render is
    out of distribution for MoGe. A hero image is not: it is a rendered object
    on a plain ground, which is what the model was trained on. So this one
    asserts. Skips when the art is absent rather than failing in a clean
    checkout.
    """
    import cv2
    pointmap = _load("pointmap")
    matte = _load("matte")
    paths = _load("paths")

    hero = paths.HERO_DIR / "outerrim_prayer.webp"
    if not hero.exists():
        pytest.skip(f"no hero art at {hero}")

    img = cv2.cvtColor(cv2.imread(str(hero)), cv2.COLOR_BGR2RGB)
    mask, _ = matte.extract(img)
    cloud = pointmap.infer(img, mask)

    assert len(cloud.points) > 10_000, "point map is implausibly sparse"
    assert np.isfinite(cloud.points).all()
    assert (cloud.points[:, 2] > 0).all(), "points behind the camera"

    # A hull seen in 3/4 must have real depth relief. A collapsed depth range
    # means the model read the image as a flat card, which would make every
    # downstream footprint a silhouette rather than a shape.
    d = cloud.points[:, 2]
    assert d.std() / d.mean() > 0.02, f"depth relief {d.std() / d.mean():.4f} too flat"

    sym = mirror.solve(cloud.points)
    assert sym.residual < 0.25, f"Prayer is a boxy hull; residual {sym.residual:.3f}"
```

The floors are deliberately loose. They catch the failure modes that would silently poison every footprint — a flat depth read, points behind the camera, a cloud too sparse to outline — without encoding a guess about how accurate MoGe happens to be. If a floor fails, investigate the model or the input before relaxing the number.

- [ ] **Step 2: Run the test to verify it fails**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_endtoend.py -v -s`
Expected: FAIL — the ground-truth chain exercises `ground.project` with a scene-space up vector and will surface any axis-convention mismatch between stages. Fix the mismatch in the stage modules, not by loosening the test.

- [ ] **Step 3: Make the ground-truth chain pass**

The likely defect is the up vector: `scene.up` is world-space and must be rotated into camera space by `scene.R` before `ground.project` uses it, since the cloud is in camera coordinates. That rotation is `scene.up @ scene.R.T`, which the test already does — if it still fails, check that `mirror.complete` returns camera-space points and that `profile.canonicalise` receives the symmetry normal projected into the same ground frame.

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_endtoend.py -v -s`
Expected: PASS, 3 tests (the MoGe one prints a line and asserts only that it produced points).

- [ ] **Step 4: Write `run.py`**

```python
#!/usr/bin/env python3
"""Batch driver for footprint recovery.

Runs all seven stages over every hero image that maps to a catalog ship, pins
one alpha for the batch, and writes a report separating recovered ships from
reconstruction failures. Failures are listed, never averaged in.

    ~/moge-venv/bin/python tools/footprint/run.py --all
    ~/moge-venv/bin/python tools/footprint/run.py --ship prayer
    SMKB_HERO_DIR=/path/to/drop ~/moge-venv/bin/python tools/footprint/run.py --all
"""

import argparse
import json
import re
import sys

import cv2
import numpy as np

from . import camera, gate, ground, matte, mirror, paths, pointmap, profile

CATALOG = "/home/robert/spacemolt/spacemolt/data/game-api/latest/catalog_ships.json"
_FACTION_PREFIX = re.compile(r"^(crimson|nebula|solarian|outerrim|voidborn|pirate)_")


def resolve_heroes() -> dict:
    """Map ship ID to hero image path, for images whose name matches a ship.

    The raw stem is tried first, and only then the faction-prefix-stripped
    form. Order matters: five current ship IDs legitimately begin with a
    faction name (crimson_devastator, crimson_stiletto, nebula_tender,
    solarian_foundation, voidborn_event_horizon), so stripping first would
    turn crimson_devastator.webp into "devastator", match nothing, and drop
    the image silently. The stripped form is still needed because catalog IDs
    carried a faction prefix before ~March 2026 and some art is named for the
    old scheme — outerrim_prayer.webp is the ship now called "prayer". No ID
    collides under stripping, so this order is unambiguous.
    """
    ids = {s["id"] for s in json.load(open(CATALOG))["items"]}
    out = {}
    for p in sorted(paths.HERO_DIR.glob(paths.HERO_GLOB)):
        key = p.stem
        if key not in ids:
            key = _FACTION_PREFIX.sub("", key)
        if key in ids:
            out[key] = p
    return out


def _stage_1_to_4(ship_id, image_path, background):
    img = cv2.cvtColor(cv2.imread(str(image_path)), cv2.COLOR_BGR2RGB)
    mask, frac = matte.extract(img)

    clicks_path = paths.artifact_dir(ship_id) / "clicks.json"
    clicks = json.loads(clicks_path.read_text()) if clicks_path.exists() else None
    fit = camera.run(ship_id, img, mask, clicks=clicks)

    cloud = pointmap.run(ship_id, img, mask, background=background)
    sym = mirror.run(ship_id, cloud)
    return img, mask, frac, fit, cloud, sym


def process(ship_id: str, image_path, alpha: float, background: str) -> dict:
    img, mask, frac, fit, cloud, sym = _stage_1_to_4(ship_id, image_path, background)

    full = mirror.complete(cloud.points, sym)
    iou = gate.run(ship_id, full, cloud.intrinsics, mask)
    quality = {"silhouette_iou": iou, "mirror_residual": sym.residual,
               "camera_confidence": fit.confidence, "camera_source": fit.source,
               "foreground_fraction": frac, "alpha": alpha}

    if iou < gate.IOU_FLOOR:
        return {"id": ship_id, "status": "failed_silhouette_gate", "quality": quality}
    if fit.confidence <= camera.CONFIDENCE_FLOOR and fit.source == "auto":
        return {"id": ship_id, "status": "needs_clicks", "quality": quality}

    up = ground.up_vector(fit, sym, cloud.normals)
    poly = ground.run(ship_id, full, up, alpha)
    sym_xy = ground.project(sym.normal[None, :], up)[0]
    data = profile.run(ship_id, poly, sym_xy, quality)
    data["status"] = "ok"
    if sym.residual > mirror.RESIDUAL_CEILING:
        data["status"] = "ok_asymmetric"
    return data


def _pick_alpha(heroes, background):
    """One alpha for the batch, from the clouds themselves."""
    clouds = []
    for ship_id, path in heroes.items():
        _, _, _, fit, cloud, sym = _stage_1_to_4(ship_id, path, background)
        up = ground.up_vector(fit, sym, cloud.normals)
        clouds.append(ground.project(mirror.complete(cloud.points, sym), up))
    return ground.sweep_alpha(clouds)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--ship")
    ap.add_argument("--background", choices=["raw", "neutral"], default="neutral")
    ap.add_argument("--alpha", type=float)
    ap.add_argument("--report", default="data/footprints/report.json")
    args = ap.parse_args()

    heroes = resolve_heroes()
    if args.ship:
        heroes = {args.ship: heroes[args.ship]}
    if not heroes:
        print("no hero images matched a catalog ship", file=sys.stderr)
        return 1

    alpha = args.alpha if args.alpha else _pick_alpha(heroes, args.background)
    print(f"batch alpha = {alpha}")

    results = [process(i, p, alpha, args.background) for i, p in heroes.items()]
    ok = [r for r in results if r["status"].startswith("ok")]
    print(f"\nrecovered {len(ok)} / {len(results)}")
    for r in results:
        if not r["status"].startswith("ok"):
            print(f"  {r['id']:22s} {r['status']:26s} "
                  f"iou={r['quality']['silhouette_iou']:.2f} "
                  f"conf={r['quality']['camera_confidence']:.2f}")

    with open(args.report, "w") as f:
        json.dump({"alpha": alpha, "background": args.background,
                   "results": results}, f, indent=2)
    return 0


if __name__ == "__main__":
    sys.exit(main())
```

- [ ] **Step 5: Run the batch on the 19**

```bash
~/moge-venv/bin/python -m tools.footprint.run --all
```

Expected: prints the chosen alpha, a recovered count, and a line per failure. Some ships failing the silhouette gate or needing clicks is the expected outcome, not a bug — record which and why.

- [ ] **Step 6: Record the outcome in the README**

Append to `tools/footprint/README.md` a "Results" section with: the batch alpha, the background setting chosen in Task 4, how many of the 19 recovered, and the ID and reason for each that did not. This is the input the consuming plan needs in order to decide whether to proceed.

- [ ] **Step 7: Commit**

```bash
git add tools/footprint data/footprints/*/profile.json data/footprints/report.json
git commit -m "feat(footprint): batch driver and end-to-end validation"
```

---

## Self-Review

**Spec coverage.** All seven stages have a task. The batch-pinned alpha is Task 7 `sweep_alpha` plus `run._pick_alpha`. The separate venv is Task 1 Step 1. The stage 2 gate on stage 3 is enforced in `run.process` via the `needs_clicks` status. Stage 5 exclusion is `run.process` returning `failed_silhouette_gate` and the report separating it from `ok`. Alpha shape over convex hull is Task 7, with a test that fails on a convex hull. Scale **and** shift is `mirror.solve`'s `refine_affine` path with its own no-op test. The synthetic end-to-end test is Task 9. The `disc+beam` gap needs no code — no local art matches those ships, so `resolve_heroes` will not return them.

**Two spec items are deliberately not in this plan**, because they belong to the consuming half: the `Merge` fix, and grading against inferred glyphs. This plan ends at `profile.json`.

**Known soft spots**, flagged rather than hidden. `camera._confidence` uses `lu_vp_detect` accessors (`get_vp_clusters`, `get_lines`) whose exact names should be verified against the installed version in Task 3 Step 4; if they differ, the confidence formula stays the same and only the accessor changes. `ground.sweep_alpha` counts covered points via a `MultiPoint` intersection, which is correct but slow on large clouds — subsample to 2000 points per cloud if the sweep takes more than a minute.
