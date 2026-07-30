# Hero-Art Footprint Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Recover a measured top-down hull footprint from each ship hero image and emit it as a canonical half-width profile that `pkg/shipglyph` can consume.

**Architecture:** A seven-stage Python pipeline under `tools/footprint/`. Each stage reads the previous stage's file from `data/footprints/<id>/` and writes its own, so any stage re-runs without repeating the ones before it. A synthetic scene generator built in Task 1 provides ground truth for every later stage, so no stage's correctness depends on trusting a hero image.

**Tech Stack:** Python 3.12, PyTorch + MoGe-2 (`Ruicheng/moge-2-vitl-normal`), OpenCV (contrib build), NumPy, SciPy, Shapely, alphashape, pytest. The vanishing-point fit is implemented on `cv2.createLineSegmentDetector` directly — see Task 3 for why lu-vp-detect is not used.

## Note on a spec detail

The spec describes MoGe's output as affine-invariant, requiring a mirror-constrained solve for depth scale and shift. That is true of **MoGe-1**. **MoGe-2 returns a metric-scale point map**, so there is no affine ambiguity to resolve.

This plan uses MoGe-2 (`Ruicheng/moge-2-vitl-normal`) because it also returns a per-pixel normal map, which makes plane fitting materially easier. The consequences:

- The scale/shift solve is retained as an **optional refinement** (Task 5), initialised at identity. On metric input it converges to a no-op; it exists so a fallback to MoGe-1 does not require a rewrite.
- The mirror solve is still required, for three things it alone provides: recovering the occluded half of the hull, locating the symmetry plane (which supplies the longitudinal axis), and producing a residual that gates reconstruction quality.

Everything else in the spec stands as written.

## Global Constraints

- MoGe runs in its own virtualenv at `~/moge-venv`, never in `~/sd-venv`. The portrait pipeline must not break because this one needed a different torch.
- The alpha radius is pinned **once per batch**, never per ship. It is chosen by sweeping a range across all recovered clouds and taking the value that maximises mean stage 5 silhouette IoU, and the chosen value is recorded with the results.
- Stage 2 is a cross-check, not a gate (REVISED 2026-07-29 after measurement; see the spec section "Stage 2 is a cross-check, not a gate"). Only 1 of 14 keyable hero images supports a trustworthy three-VP fit, because most renders are two-point-perspective or near-orthographic in one axis — the weak axes' vanishing points sit 25–57 image diagonals out with 0–2 supporting segments. A 15× search budget does not change it. The reference frame comes from recovered geometry instead: lateral axis = stage 4's symmetry-plane normal, longitudinal axis = the cloud's principal axis within that plane, up = lateral × longitudinal. No ship is skipped for low camera confidence, and no hand-clicking is required. Where stage 2 is confident, its rotation is compared to the geometric frame and the agreement logged, never averaged.
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
~/moge-venv/bin/pip install opencv-contrib-python numpy scipy shapely alphashape pytest
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

import importlib
import pathlib
import sys

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
- Modify: `tools/footprint/requirements.txt` (remove `lu-vp-detect`)

**Why this task does not use lu-vp-detect.** The plan originally wrapped that
library. It is broken on OpenCV 5.x: `vp_detection.py:179` does
`lines = lines[:, 0]` to strip a singleton dimension from LSD's `(N, 1, 4)`
return, but OpenCV 5.0 returns `(N, 4)` directly, so the slice yields a 1-D
array and the next index raises `IndexError`. It also never exposed the cluster
memberships a confidence measure needs — `get_vp_clusters` and `get_lines` do
not exist in its API. `cv2.createLineSegmentDetector` itself is healthy and
returns 1,525 usable segments on `magnate.webp`, so this task uses the detector
directly and implements the orthogonal-VP fit.

**Interfaces:**
- Consumes: `synth.Scene`, `matte.extract`, `paths.artifact_dir`
- Produces: `camera.detect_segments(image_rgb, mask, min_length) -> np.ndarray (N,4)` of `(x1,y1,x2,y2)`
- Produces: `camera.fit(image_rgb, mask) -> camera.Fit`, a dataclass with `R: np.ndarray (3,3)`, `focal: float | None` (None means orthographic), `principal: tuple[float,float]`, `confidence: float`, `source: str` in `{"auto","clicks"}`, `n_segments: int`, `inliers: tuple[int,int,int]`
- Produces: `camera.fit_from_clicks(clicks, principal=None) -> camera.Fit`
- Produces: `camera.clicks_from_scene(scene) -> dict`
- Produces: `camera.run(ship_id, image_rgb, mask, clicks=None) -> Fit`, writing `camera.json`
- Produces: `camera.CONFIDENCE_FLOOR = 0.35`

**Fallback click format** — `data/footprints/<id>/clicks.json`, unchanged:

```json
{"axis_x": [[[x1,y1],[x2,y2]], [[x3,y3],[x4,y4]]],
 "axis_y": [[[x1,y1],[x2,y2]], [[x3,y3],[x4,y4]]],
 "axis_z": [[[x1,y1],[x2,y2]], [[x3,y3],[x4,y4]]]}
```

- [ ] **Step 1: Write the failing test**

```python
"""Checks for the vanishing-point camera fit.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_camera.py
"""

import importlib
import pathlib
import sys

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


def _axis_angle(a, b):
    """Angle in degrees between two directions, ignoring sign."""
    c = np.clip(abs(float(np.dot(a, b))) / (np.linalg.norm(a) * np.linalg.norm(b)), -1, 1)
    return float(np.degrees(np.arccos(c)))


def _best_permutation_error(R_fit, R_true):
    """Largest axis error under the best matching of fitted axes to true axes.

    The fit recovers three orthogonal directions but not which is which, so a
    row-by-row comparison would fail on a correct answer that came back in a
    different order.
    """
    import itertools
    best = 180.0
    for perm in itertools.permutations(range(3)):
        worst = max(_axis_angle(R_fit[perm[i]], R_true[i]) for i in range(3))
        best = min(best, worst)
    return best


def test_detect_segments_finds_the_box_edges():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    seg = camera.detect_segments(s.image, mask, min_length=30.0)

    assert seg.ndim == 2 and seg.shape[1] == 4, seg.shape
    # A shaded box shows at least its silhouette and the three interior edges
    # meeting at the near corner. Fewer than six means the detector or the
    # masking is broken, not that the scene is simple.
    assert len(seg) >= 6, len(seg)


def test_fit_recovers_the_synthetic_rotation_within_five_degrees():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    fit = camera.fit(s.image, mask)

    assert fit.confidence > camera.CONFIDENCE_FLOOR, fit.confidence
    err = _best_permutation_error(fit.R, s.R)
    assert err < 5.0, f"worst axis off by {err:.1f} deg"


def test_fit_recovers_the_synthetic_focal_within_ten_percent():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    fit = camera.fit(s.image, mask)

    assert fit.focal is not None, "a perspective render must not be called ortho"
    assert abs(fit.focal - s.K[0, 0]) / s.K[0, 0] < 0.10, (fit.focal, s.K[0, 0])


def test_fit_is_deterministic():
    # The RANSAC is seeded. Two fits of the same image must agree exactly, or
    # every downstream artifact becomes irreproducible.
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    a, b = camera.fit(s.image, mask), camera.fit(s.image, mask)

    assert np.allclose(a.R, b.R), "rotation differs between identical fits"
    assert a.focal == b.focal
    assert a.confidence == b.confidence


def test_clicks_fallback_matches_the_known_camera():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    fit = camera.fit_from_clicks(camera.clicks_from_scene(s),
                                 principal=(s.K[0, 2], s.K[1, 2]))

    assert _best_permutation_error(fit.R, s.R) < 5.0
    assert fit.source == "clicks"
    assert abs(fit.focal - s.K[0, 0]) / s.K[0, 0] < 0.10


def test_clicks_reject_parallel_lines():
    # Two parallel clicked lines never intersect, so the vanishing point is
    # undefined. That must raise, not silently return a huge coordinate.
    clicks = {"axis_x": [[[0, 0], [100, 0]], [[0, 50], [100, 50]]],
              "axis_y": [[[0, 0], [0, 100]], [[50, 0], [50, 100]]],
              "axis_z": [[[0, 0], [100, 100]], [[10, 0], [110, 100]]]}
    try:
        camera.fit_from_clicks(clicks)
    except ValueError as e:
        assert "parallel" in str(e).lower(), str(e)
    else:
        raise AssertionError("parallel clicked lines must raise")


def test_featureless_subject_reports_low_confidence():
    # A smooth ellipsoid has no straight structural edges, so there is no
    # Manhattan frame to recover. The fit must say so rather than invent one.
    s = synth.cylinder_scene(radius=2.0, height=0.2, azimuth_deg=0, elevation_deg=89)
    mask, _ = matte.extract(s.image)
    fit = camera.fit(s.image, mask)

    assert fit.confidence <= camera.CONFIDENCE_FLOOR, fit.confidence
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_camera.py -v`
Expected: FAIL — `camera.py` does not exist.

- [ ] **Step 3: Write `camera.py`**

```python
#!/usr/bin/env python3
"""Stage 2: fit the camera from vanishing points.

These hulls are Manhattan-world objects with long parallel structural lines, so
three orthogonal vanishing points give rotation and focal length outright and
reveal whether a render is orthographic or mildly perspective. Never assume the
3/4 view: a low-confidence fit routes the ship to the hand-clicked fallback.

Implemented directly on cv2's line segment detector. lu-vp-detect is not used:
it breaks on OpenCV 5.x (it strips a singleton dimension LSD no longer emits)
and never exposed the cluster memberships a confidence measure needs.

    ~/moge-venv/bin/python -m tools.footprint.camera <image>
"""

import dataclasses
import json

import cv2
import numpy as np

from . import paths

CONFIDENCE_FLOOR = 0.35
MIN_LENGTH_FRACTION = 0.04     # of the smaller image side
INLIER_ANGLE_TOL_DEG = 2.0
RANSAC_ITERATIONS = 2000
RANSAC_SEED = 0
ORTHO_FOCAL = 1e5              # beyond this the projection is orthographic


@dataclasses.dataclass
class Fit:
    R: np.ndarray
    focal: float | None
    principal: tuple
    confidence: float
    source: str
    n_segments: int
    inliers: tuple

    def to_json(self) -> dict:
        return {"R": self.R.tolist(), "focal": self.focal,
                "principal": list(self.principal), "confidence": self.confidence,
                "source": self.source, "n_segments": self.n_segments,
                "inliers": list(self.inliers)}


def detect_segments(image_rgb, mask, min_length: float) -> np.ndarray:
    """Line segments inside the subject, as (N, 4) rows of (x1, y1, x2, y2).

    OpenCV 5 returns (N, 4) from LSD; OpenCV 4 returned (N, 1, 4). Both are
    accepted so a future pin change does not silently produce zero segments.
    """
    grey = cv2.cvtColor(image_rgb, cv2.COLOR_RGB2GRAY)
    grey = np.where(mask > 0, grey, 0).astype(np.uint8)

    detected = cv2.createLineSegmentDetector(0).detect(grey)[0]
    if detected is None or len(detected) == 0:
        return np.zeros((0, 4), np.float32)
    seg = np.asarray(detected, dtype=np.float64)
    if seg.ndim == 3:
        seg = seg[:, 0]

    dx, dy = seg[:, 2] - seg[:, 0], seg[:, 3] - seg[:, 1]
    return seg[np.hypot(dx, dy) >= min_length]


def _homogeneous_lines(seg: np.ndarray) -> np.ndarray:
    p1 = np.column_stack([seg[:, 0], seg[:, 1], np.ones(len(seg))])
    p2 = np.column_stack([seg[:, 2], seg[:, 3], np.ones(len(seg))])
    return np.cross(p1, p2)


def _vp_from(lines, i, j):
    v = np.cross(lines[i], lines[j])
    if abs(v[2]) < 1e-9:          # the two lines are parallel in the image
        return None
    return v[:2] / v[2]


def _focal_from_two_vps(v1, v2, pp):
    """Orthocentre relation: for orthogonal directions, f^2 = -(v1-pp).(v2-pp)."""
    d = -float(np.dot(np.asarray(v1) - pp, np.asarray(v2) - pp))
    return np.sqrt(d) if d > 0 else None


def _directions(vps, focal, pp):
    d = np.array([[v[0] - pp[0], v[1] - pp[1], focal] for v in vps], dtype=float)
    return d / np.linalg.norm(d, axis=1, keepdims=True)


def _third_vp(v1, v2, focal, pp):
    d = _directions([v1, v2], focal, pp)
    d3 = np.cross(d[0], d[1])
    if abs(d3[2]) < 1e-9:
        return None
    return np.array([pp[0] + focal * d3[0] / d3[2], pp[1] + focal * d3[1] / d3[2]])


def _score(seg, vps, tol_deg):
    """Assign each segment to the vanishing point it points at, or to none.

    A segment votes for a vp when the line from its midpoint to that vp is
    parallel to the segment itself. Returns (labels, per-vp counts).
    """
    mid = np.column_stack([(seg[:, 0] + seg[:, 2]) / 2, (seg[:, 1] + seg[:, 3]) / 2])
    d = np.column_stack([seg[:, 2] - seg[:, 0], seg[:, 3] - seg[:, 1]])
    d = d / np.maximum(np.linalg.norm(d, axis=1, keepdims=True), 1e-12)

    best = np.full(len(seg), -1)
    best_err = np.full(len(seg), np.inf)
    for k, v in enumerate(vps):
        to_vp = np.asarray(v)[None, :] - mid
        to_vp = to_vp / np.maximum(np.linalg.norm(to_vp, axis=1, keepdims=True), 1e-12)
        cos = np.abs(np.sum(to_vp * d, axis=1))
        err = np.degrees(np.arccos(np.clip(cos, -1, 1)))
        take = err < np.minimum(best_err, tol_deg)
        best[take], best_err[take] = k, err[take]

    counts = tuple(int((best == k).sum()) for k in range(3))
    return best, counts


def _confidence(counts, n_segments):
    """Explained fraction, penalised when one direction dominates.

    A fit that assigns every segment to one vanishing point has found a single
    edge direction, not a Manhattan frame, so the balance term must pull it
    down. Both terms are needed: coverage alone rewards a degenerate fit, and
    balance alone rewards a fit that explains three segments out of a thousand.
    """
    if n_segments == 0 or max(counts) == 0:
        return 0.0
    coverage = sum(counts) / n_segments
    balance = min(counts) / max(counts)
    return float(coverage * balance)


def fit(image_rgb: np.ndarray, mask: np.ndarray) -> Fit:
    h, w = mask.shape
    pp = np.array([w / 2.0, h / 2.0])
    seg = detect_segments(image_rgb, mask, min(h, w) * MIN_LENGTH_FRACTION)
    if len(seg) < 6:
        return Fit(np.eye(3), None, tuple(pp), 0.0, "auto", len(seg), (0, 0, 0))

    lines = _homogeneous_lines(seg)
    rng = np.random.default_rng(RANSAC_SEED)
    best = None

    for _ in range(RANSAC_ITERATIONS):
        i, j, k, m = rng.choice(len(seg), 4, replace=False)
        v1, v2 = _vp_from(lines, i, j), _vp_from(lines, k, m)
        if v1 is None or v2 is None:
            continue
        focal = _focal_from_two_vps(v1, v2, pp)
        if focal is None or focal < 1e-6:
            continue
        v3 = _third_vp(v1, v2, focal, pp)
        if v3 is None:
            continue
        _, counts = _score(seg, [v1, v2, v3], INLIER_ANGLE_TOL_DEG)
        conf = _confidence(counts, len(seg))
        if best is None or conf > best[0]:
            best = (conf, [v1, v2, v3], focal, counts)

    if best is None:
        return Fit(np.eye(3), None, tuple(pp), 0.0, "auto", len(seg), (0, 0, 0))

    conf, vps, focal, counts = best
    if conf <= CONFIDENCE_FLOOR:
        return Fit(np.eye(3), None, tuple(pp), conf, "auto", len(seg), counts)

    R = _orthonormalise(_directions(vps, focal, pp))
    return Fit(R, None if focal > ORTHO_FOCAL else float(focal), tuple(pp),
               conf, "auto", len(seg), counts)


def _orthonormalise(d: np.ndarray) -> np.ndarray:
    """Nearest rotation matrix to three approximately orthogonal directions."""
    u, _, vt = np.linalg.svd(d)
    R = u @ vt
    if np.linalg.det(R) < 0:
        R[-1] *= -1
    return R


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
    pp = np.asarray(principal if principal is not None else np.mean(vps, axis=0), dtype=float)

    focals = [f for f in (_focal_from_two_vps(vps[i], vps[j], pp)
                          for i, j in ((0, 1), (0, 2), (1, 2))) if f is not None]
    if not focals:
        raise ValueError("clicked vanishing points are not mutually orthogonal")
    focal = float(np.median(focals))

    return Fit(_orthonormalise(_directions(vps, focal, pp)),
               None if focal > ORTHO_FOCAL else focal, tuple(pp),
               1.0, "clicks", 0, (0, 0, 0))


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
    (paths.artifact_dir(ship_id) / "camera.json").write_text(
        json.dumps(f.to_json(), indent=2))
    return f
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_camera.py -v`
Expected: PASS, 7 tests.

If the rotation test fails, print `fit.inliers` and `fit.n_segments` first. Few
segments means `MIN_LENGTH_FRACTION` is culling the box edges — lower it and
say so in the report. Balanced inlier counts with a bad rotation means the
orthocentre focal is wrong, not the clustering.

- [ ] **Step 5: Prove the confidence gate can fail**

The gate is the whole point of this stage, so demonstrate it rather than assume
it. Temporarily replace `_confidence`'s return with `return 1.0`, run
`test_featureless_subject_reports_low_confidence`, and confirm it goes RED.
Restore, confirm GREEN. Paste both outputs into the report.

- [ ] **Step 6: Report the fit on the 14 keyable hero images**

```bash
~/moge-venv/bin/python - <<'PY'
import cv2, glob, json, os, re, sys
sys.path.insert(0, ".")
from tools.footprint import camera, matte
ids = {s["id"] for s in json.load(open(
    "/home/robert/spacemolt/spacemolt/data/game-api/latest/catalog_ships.json"))["items"]}
pre = re.compile(r"^(crimson|nebula|solarian|outerrim|voidborn|pirate)_")
for p in sorted(glob.glob(os.path.expanduser("~/Downloads/*.webp"))):
    key = os.path.basename(p)[:-5]
    if key not in ids:
        key = pre.sub("", key)
    if key not in ids:
        continue
    img = cv2.cvtColor(cv2.imread(p), cv2.COLOR_BGR2RGB)
    m, _ = matte.extract(img)
    if not matte.keyability(img)[0]:
        print(f"{key:20s} SKIPPED - not keyable")
        continue
    f = camera.fit(img, m)
    kind = "ortho" if f.focal is None else f"f={f.focal:7.0f}"
    print(f"{key:20s} conf {f.confidence:.2f}  {kind}  segs={f.n_segments:4d} "
          f"inliers={f.inliers}")
PY
```

Record which of the 14 fall below `CONFIDENCE_FLOOR`. Do NOT lower
`CONFIDENCE_FLOOR` to make more ships pass — calibrate it on synthetic ground
truth only.

**OUTCOME, 2026-07-29 (this step has been run; kept for the record).** 1 of 14
passes (`ledger`, 0.057). This is a property of the art: on most of these
renders the weak axes' vanishing points sit 25–57 image diagonals from the
principal point with 0–2 supporting segments, so the render is
two-point-perspective or near-orthographic in one axis, and several hulls are
curved with few straight structural lines at all. Confidence is unchanged at
2000 / 8000 / 30000 RANSAC iterations, so it is not a search-budget problem.

Consequently **stage 2 was demoted from a gate to a cross-check** and the
reference frame now comes from recovered geometry — see the Global Constraints
and `ground.up_vector` in Task 7. No click files are needed, and the
hand-clicked path (`fit_from_clicks`, `clicks_from_scene`) remains available but
is no longer on the batch's critical path.

- [ ] **Step 7: Remove the dead dependency**

Delete the `lu-vp-detect` line from `tools/footprint/requirements.txt`, add a
comment recording why (broken on OpenCV 5.x, and its accessors never exposed
cluster membership), and regenerate `requirements.lock.txt`. Do not uninstall
it from the venv — leave the environment alone; the requirement file is the
statement of intent.

- [ ] **Step 8: Commit**

```bash
git add tools/footprint/camera.py tools/footprint/test_camera.py \
        tools/footprint/requirements.txt tools/footprint/requirements.lock.txt
git commit -m "feat(footprint): stage 2 vanishing-point camera fit"
```


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

import importlib
import pathlib
import sys

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

> **STATUS 2026-07-29: DONE (commit `b21ca1db7`), but the plane SEARCH it builds
> is superseded on real data.** Everything else here stands as the synthetic
> foundation — `reflect`, `complete`, the affine scale/shift solve, the
> extent-normalised residual, the two-sided fixtures — and its 8 tests pass.
> What does not survive real clouds is using self-chamfer to FIND the plane: a
> one-view cloud is a front-surface sheet, and the reflection that best matches
> a sheet folds it onto itself. See the spec section "The plane cannot be found
> by self-chamfer" for the measurements, and **Task 6b** for the replacement
> objective, which is built after Task 6 because it needs `gate.reproject`.
>
> `RESIDUAL_CEILING` below reads 0.06 for historical reasons. The shipped value
> is **0.013**: this plan's calibration numbers were computed in raw chamfer
> units while `solve()` divides by the sample's bounding-box diagonal (measured
> 4.683 on the fixture), a 4.68× unit mismatch. Use 0.013, and treat it as
> uncalibrated against real data until Task 6b re-measures it.

**Files:**
- Create: `tools/footprint/mirror.py`
- Test: `tools/footprint/test_mirror.py`

**Interfaces:**
- Consumes: `pointmap.Cloud`
- Produces: `mirror.solve(points: np.ndarray, init_scale=1.0, init_shift=0.0, refine_affine=False) -> mirror.Symmetry` where `Symmetry` is a dataclass with `normal: np.ndarray (3,)`, `offset: float`, `scale: float`, `shift: float`, `residual: float`
- Produces: `mirror.reflect(points, normal, offset) -> np.ndarray`
- Produces: `mirror.complete(points, sym) -> np.ndarray` returning the union of the cloud and its reflection
- Produces: `mirror.run(ship_id, cloud, refine_affine=False) -> Symmetry`, writing `cloud_resolved.npz`. **Task 6b changes this signature to `run(ship_id, cloud, mask, refine_affine=False)`** and routes it to `solve_from_view`, since real clouds are one-sided. Implement the two-argument form here; Task 6b adds `mask`.
- Produces: `mirror.RESIDUAL_CEILING = 0.06`

- [ ] **Step 1: Write the failing test**

```python
"""Checks for the mirror-constrained symmetry solve.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_mirror.py
"""

import importlib
import pathlib
import sys

import dataclasses

import numpy as np
from scipy.spatial import cKDTree

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

mirror = _load("mirror")


def _chamfer(a, b):
    """Symmetric mean nearest-neighbour distance between two clouds."""
    return 0.5 * (cKDTree(b).query(a)[0].mean() + cKDTree(a).query(b)[0].mean())


def _symmetric_hull(seed=0, n=4000):
    """A tapered, bilaterally symmetric hull: mirror plane normal +Y, offset 0.

    Deliberately NOT a uniform box. A uniform box is symmetric about all three
    coordinate planes, so the solver could return +X or +Z and be right. Worse,
    an "asymmetric" variant built by breaking only Y would leave Z an exact
    symmetry plane and the solver would correctly find it: measured on the box
    version, Z residual 0.069 vs Y 0.183 at n=4000, and the Z figure is pure
    sampling noise that falls monotonically with density (0.135 at n=500 ->
    0.025 at n=80000). A residual-above-ceiling assertion on that fixture would
    be testing cloud density, not asymmetry.

    The x-dependent taper breaks X symmetry and the x-dependent keel offset
    breaks Z symmetry, leaving Y the unique answer. Measured: Y=0.0000 exactly,
    X=0.119, Z=0.117 at n=4000, and Y stays exact as density changes.
    """
    rng = np.random.default_rng(seed)
    x = rng.uniform(-2.0, 2.0, n // 2)
    halfwidth = 0.15 + 0.85 * (1.0 - (x + 2.0) / 4.0)   # wide at the nose
    y = 0.05 + halfwidth * rng.uniform(0.0, 1.0, n // 2)
    z = rng.uniform(-0.5, 0.5, n // 2) + 0.4 * (x / 2.0) ** 2   # keel
    half = np.column_stack([x, y, z])
    return np.vstack([half, half * [1, -1, 1]])


def test_solve_finds_the_known_symmetry_plane():
    pts = _symmetric_hull()
    sym = mirror.solve(pts)
    axis_err = np.degrees(np.arccos(min(1.0, abs(sym.normal @ [0, 1, 0]))))
    assert axis_err < 5.0, axis_err
    assert abs(sym.offset) < 0.05, sym.offset
    assert sym.residual < mirror.RESIDUAL_CEILING, sym.residual


def _lopsided_hull(seed=0, n=4000):
    """The symmetric hull plus a sponson attached to the +Y side only.

    Do NOT build this by shoving one side along an axis: `lop[lop[:,1]>0, 0] +=
    1.2` leaves Z an exact symmetry plane, so the solver correctly returns Z
    with a LOW residual and the asymmetry assertion fails on correct code.
    Adding mass to one side leaves no candidate plane: measured best residual
    over a grid of arbitrary planes is 0.094 / 0.094 / 0.085 at n =
    2000 / 4000 / 20000, i.e. driven by shape rather than sampling density.
    """
    pts = _symmetric_hull(seed, n)
    rng = np.random.default_rng(seed + 99)
    m = n // 8
    sponson = np.column_stack([rng.uniform(-0.5, 0.9, m),
                               rng.uniform(1.0, 1.8, m),
                               rng.uniform(-0.3, 0.3, m)])
    return np.vstack([pts, sponson])


def test_solve_reports_a_high_residual_on_an_asymmetric_hull():
    # Scrap-built hulls are deliberately lopsided; the residual is how we know
    # not to trust the mirrored half.
    sym = mirror.solve(_lopsided_hull())
    assert sym.residual > mirror.RESIDUAL_CEILING, sym.residual


def test_residual_ceiling_tolerates_a_noisy_symmetric_hull():
    """A symmetric hull with measurement noise must stay UNDER the ceiling.

    Without this, RESIDUAL_CEILING is only ever shown to separate 0.0 from
    0.085 — and the exact-zero case is an artifact of the fixture mirroring
    points exactly, which no real point map does. Measured Y residual on the
    symmetric hull at noise sigma 0.005 / 0.01 / 0.02 / 0.04 is 0.011 / 0.022 /
    0.038 / 0.054, against a lopsided hull's 0.094. So the ceiling at 0.06
    separates the two only while cloud noise stays below roughly sigma 0.04.
    CARRY THIS INTO TASK 9: the noise level of real MoGe clouds is unmeasured,
    so re-validate the ceiling against real clouds there. This test proves the
    mechanism, not that 0.06 is the right number for real art.
    """
    pts = _symmetric_hull()
    noisy = pts + np.random.default_rng(7).normal(0, 0.02, pts.shape)
    sym = mirror.solve(noisy)
    assert sym.residual < mirror.RESIDUAL_CEILING, sym.residual


def test_reflect_is_an_involution():
    pts = _symmetric_hull()
    n, d = np.array([0.0, 1.0, 0.0]), 0.3
    once = mirror.reflect(pts, n, d)
    twice = mirror.reflect(once, n, d)
    assert np.allclose(twice, pts, atol=1e-9)


def test_complete_fills_the_occluded_half():
    """Keep one side, as a single view would; completion must restore the other.

    The assertion compares the completed cloud against the KNOWN full hull.
    Counting how many points land at y < 0 is not enough: `complete` could
    ignore `sym` entirely and return
    `np.vstack([visible, visible * [1, -1, 1]])`, which satisfies any count-
    based assertion while reflecting across a hardcoded plane rather than the
    solved one. Chamfer against ground truth fails for a wrong plane, so this
    test actually exercises `sym`.
    """
    pts = _symmetric_hull()
    visible = pts[pts[:, 1] > 0]
    sym = mirror.solve(visible)
    full = mirror.complete(visible, sym)
    assert (full[:, 1] < 0).sum() >= len(visible) - 1
    assert _chamfer(full, pts) < 0.05, _chamfer(full, pts)


def test_complete_uses_the_solved_plane_not_a_hardcoded_one():
    """A deliberately wrong plane must produce a visibly wrong completion.

    This is the red half of the test above: it pins down that `complete`
    reflects across `sym.normal`/`sym.offset`. If it did not, both this and the
    previous test would pass on an implementation that always mirrors in Y.
    """
    pts = _symmetric_hull()
    visible = pts[pts[:, 1] > 0]
    good = mirror.solve(visible)
    wrong = dataclasses.replace(good, normal=np.array([1.0, 0.0, 0.0]), offset=0.0)
    assert _chamfer(mirror.complete(visible, wrong), pts) > 0.2


def test_affine_refinement_is_a_noop_on_metric_input():
    """MoGe-2 is already metric, so the scale/shift solve must stay at identity.

    On its own this assertion is satisfied by `scale = 1.0` hardcoded and no
    refinement at all, so it is paired with the recovery test below. Keep both:
    this one pins "does not wander", that one pins "actually solves".
    """
    pts = _symmetric_hull()
    sym = mirror.solve(pts, refine_affine=True)
    assert abs(sym.scale - 1.0) < 0.05, sym.scale
    assert abs(sym.shift) < 0.05, sym.shift


def test_affine_refinement_recovers_a_known_depth_scaling():
    """Distort depth by a known factor; the refinement must undo it.

    This is the falsifiable half of the pair above. A stub that returns
    scale=1.0 passes the no-op test and fails this one, which is the whole
    point: the affine solve is what makes an affine-invariant point map usable,
    so it must be shown to do arithmetic rather than return its initial value.
    """
    pts = _symmetric_hull()
    squashed = pts * [1.0, 1.0, 0.6]
    sym = mirror.solve(squashed, refine_affine=True)
    assert abs(sym.scale - 1.0 / 0.6) < 0.1 * (1.0 / 0.6), sym.scale
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
- Produces: `gate.project(points, intrinsics, shape) -> tuple[np.ndarray, np.ndarray]` returning per-point `(uv, ok)`, both length N and aligned with `points`
- Produces: `gate.reproject(points, intrinsics, shape) -> np.ndarray` returning a `(H,W)` uint8 mask, built from `project`
- Produces: `gate.inside_fraction(points, intrinsics, mask) -> float` — fraction of points landing inside the matte. Density-invariant; this is what stage 4 and the gate verdict use
- Produces: `gate.score(points, intrinsics, mask) -> float` returning union IoU in `[0,1]` — **a recorded diagnostic that decides nothing**
- Produces: `gate.run(ship_id, points, mirrored, intrinsics, mask) -> dict`, appending to `data/footprints/<id>/quality.json`
- Produces: `gate.MIR_FLOOR` (provisional; calibrated in Task 6b) and `gate.IOU_FLOOR = 0.70` (diagnostic only)

**Amended 2026-07-29 after review.** The original design gated on union IoU. That
cannot work, and the reason is algebra, not calibration — see the spec's "Stage 5
gates on mirrored-half spill". In short: `pointmap.infer` keeps exactly the
matte's points, so the visible half reprojects onto the pixels it was read from
and union IoU is ≈0.993 by construction; and `uv = K·p/p_z` is exactly invariant
under `p → λ(p)·p`, so IoU cannot see along-ray motion at all, including global
scale and depth flattening. Measured: 9 of 12 deliberately wrong symmetry planes
pass the 0.70 floor, and a cloud flattened to 10% of its depth extent scores
0.9910 — identical to four decimals to the unflattened one. The gate therefore
scores the **mirrored half alone**, combined with stage 4's `depth_separation`.

- [ ] **Step 1: Write the failing test**

```python
"""Checks for the reprojection silhouette gate.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_gate.py
"""

import importlib
import pathlib
import sys

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


def _dense_surface_cloud(scene, target=60_000, seed=0):
    """Barycentric samples over the scene mesh, at realistic cloud density.

    `Scene.points` is far too sparse to stand in for a MoGe cloud: 2400 samples
    across a mask covering 75192 px is 0.032 points per pixel, where MoGe
    returns roughly ONE point per valid pixel. Reprojecting the sparse set and
    closing it with a 5x5 kernel cannot produce a filled region, so a PERFECT
    cloud scores IoU 0.127 — below IOU_FLOOR. Measured: 0.053 / 0.127 / 0.319 /
    0.499 at dilation 3 / 5 / 9 / 15, i.e. no dilation rescues it.

    That would have made `test_perfect_cloud_scores_near_one` fail on correct
    code, and worse, `test_displaced_cloud_scores_low` would have passed for the
    wrong reason — everything scores low when nothing fills.

    Sampling the mesh at realistic density fixes it at the fixture, not by
    loosening the assertion. Measured perfect / displaced IoU: 0.957 / 0.122 at
    20k samples, 0.991 / 0.123 at 60k, 0.993 / 0.123 at 250k. 60k is the knee.
    """
    verts = scene.vertices
    tris = []
    for face in scene.faces:
        idx = np.asarray(face)
        for i in range(1, len(idx) - 1):          # fan-triangulate each polygon
            tris.append([idx[0], idx[i], idx[i + 1]])
    tris = np.array(tris)

    a, b, c = verts[tris[:, 0]], verts[tris[:, 1]], verts[tris[:, 2]]
    area = 0.5 * np.linalg.norm(np.cross(b - a, c - a), axis=1)

    rng = np.random.default_rng(seed)
    pick = rng.choice(len(tris), size=target, p=area / area.sum())
    u, v = rng.random((target, 1)), rng.random((target, 1))
    fold = (u + v) > 1.0                          # reflect into the triangle
    u[fold], v[fold] = 1.0 - u[fold], 1.0 - v[fold]
    return a[pick] + u * (b[pick] - a[pick]) + v * (c[pick] - a[pick])


def _project_scene(s):
    """A realistically dense perfect cloud, in camera coordinates."""
    return _dense_surface_cloud(s) @ s.R.T + s.t


def test_perfect_cloud_scores_near_one():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    iou = gate.score(_project_scene(s), s.K, mask)
    assert iou > 0.90, iou


def test_displaced_cloud_scores_low():
    """A misaligned reconstruction must fail the gate.

    This assertion is only meaningful BECAUSE test_perfect_cloud_scores_near_one
    passes on the same fixture: without that pairing, "scores low" is satisfied
    by a gate that scores everything low, which is exactly what the original
    sparse fixture did (perfect cloud 0.127, displaced 0.024 — both under the
    floor, test green, gate useless). Measured here: 0.991 perfect vs 0.123
    displaced.
    """
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

### Task 6b: Stage 4 rewrite — silhouette-and-depth plane search

**Why this task exists.** Task 5's `solve()` finds the plane by minimising
chamfer between the cloud and its own reflection. On two-sided synthetic
fixtures that is correct and its tests pass. On real one-view clouds it fails
structurally, because a one-view point map is a front-surface **sheet** and the
reflection that best matches a sheet is the one that folds the sheet onto
itself; the true bilateral plane scores worse, since it maps visible points onto
the occluded half where there is nothing to match.

Measured on `outerrim_prayer` / `ledger` / `smelter` with the Task 5 solver: the
plane cuts through the cloud (41% / 54% / 64% of points on the + side) within
2.3% / 0.5% / 7.8% of the centroid; recovered normals are mutually inconsistent
(`[0.06,0.94,0.33]`, `[0.35,0.93,-0.10]`, `[-0.77,0.35,-0.53]`); `complete()`
raises extent by only 1.035× / 1.035× / 1.059×; and the supposed occluded half
sits `+0.18` behind a hull 6.3 deep (2.8%) and `+0.007` behind one 5 deep
(0.15%), where a real half-hull sits roughly a beam's width back. Cloud noise is
ruled out: measured local surface roughness is 0.00036 absolute / 0.00003
relative, and a synthetic hull at matched relative noise gives residual 0.00007,
230× below the observed 0.01625.

This matters more than it would have before the stage 2 demotion, because
`sym.normal` is now the lateral axis of the reference frame — a wrong normal
rotates every footprint, not just the mirrored half.

**Files:**
- Modify: `tools/footprint/mirror.py`
- Test: `tools/footprint/test_mirror.py`

**Interfaces:**
- Consumes: `pointmap.Cloud`, `gate.reproject` (Task 6), the stage 1 matte
- Produces: `mirror.solve_from_view(points, mask, intrinsics, refine_affine=False) -> mirror.Symmetry`. The existing `solve(points, ...)` is KEPT unchanged for two-sided input and remains what the synthetic tests exercise; the new entry point is what `run.process` calls on real clouds.
- Produces: `mirror.Symmetry` gains `depth_separation: float` — the mean signed depth of mirrored points behind the visible surface at shared pixels, in cloud units. This is the discriminating term and must be recorded in `quality.json`, not merely thresholded.
- Produces: `mirror.Symmetry` gains `obliquity: float` — `abs(normal @ [0,0,1])`, how much the recovered lateral axis points toward the camera. **This is not decoration: the achievable depth separation is a steep function of it** (see below), so `depth_separation` is uninterpretable without it. Record both in `quality.json`.
- Produces: `mirror.MIN_DEPTH_SEPARATION_FRACTION = 0.01` — of the cloud's z-extent. Below this the plane is a fold and the solve must report failure rather than return it.

**The 0.10 in the first draft of this task was wrong and would have rejected
correct solves.** Measured on a clean synthetic front-surface sheet (ellipsoid
2.0 x 1.0 x 0.6, back-face culled, ~27k visible points), sweeping the angle
between the lateral axis and the view direction, `obliquity = |n·ẑ|`:

| obliquity | true-plane sep / z-extent | best fold sep / z-extent | ratio |
|-----------|---------------------------|--------------------------|-------|
| 0.500     | 0.028                     | 0.0001                   | 343x  |
| 0.707     | 0.098                     | 0.0001                   | 947x  |
| 0.866     | 0.292                     | 0.00003                  | 12314x |
| 0.966     | 0.938                     | 0.00002                  | 27715x |
| 1.000     | 1.399                     | 0.0001                   | 19050x |

Two conclusions, and they point opposite ways:

- **The objective works.** A fold's depth separation is 343x to 27000x smaller
  than the true plane's at every angle tested. The term breaks the degeneracy
  exactly as intended, which is why this task proceeds as designed.
- **A fixed 0.10 floor is a bug.** It is above the true-plane separation at
  obliquity 0.500 and only marginally below it at 0.707, so it would reject the
  correct answer for any ship whose lateral axis is more than about 45 degrees
  from the view direction — i.e. anything rendered closer to bow-on than a
  three-quarter view. 0.01 clears the worst measured true plane by 2.8x and the
  worst measured fold by 100x.

Because separation collapses to **exactly zero** at obliquity 0 (a bow-on view,
where the mirror plane contains the view direction and reflection moves nothing
in depth), the depth term carries no signal there at all. Report `obliquity`
per ship in Step 5. If any real hero image comes back below ~0.3, say so
plainly rather than tuning the floor down to admit it: at that angle this
objective cannot distinguish a fold, and that is a finding about which ships
this pipeline can handle, not a constant to adjust.

**The objective.** For a candidate plane, reflect the cloud, then score two
terms that a fold cannot satisfy together:

1. **Silhouette agreement** — the fraction of mirrored points whose reprojection
   lands inside the matte. The occluded half is behind the ship, so it must
   project within the same silhouette. Use **`gate.project`** (per-point `(uv, ok)`)
   and **`gate.inside_fraction`**, not `gate.reproject`. `reproject` returns a
   closed mask and discards the per-point information both of these terms need —
   the fraction is not recoverable from a mask, and term 2 needs `uv` *and* `z`
   per point to pair mirrored against visible at shared pixels. Task 6's fix
   round extracts `project` for exactly this reason, so that stage 4 and stage 5
   share one projection and one intrinsics-denormalisation rule rather than
   diverging.
2. **Depth separation** — at pixels where both a visible and a mirrored point
   land, the mean of (mirrored z − visible z). A genuine occluded half is
   *behind* the visible surface; a fold puts it at the same depth. This is the
   term that breaks the degeneracy.

Maximise silhouette agreement subject to depth separation clearing
`MIN_DEPTH_SEPARATION_FRACTION` of the z-extent. Keep the chamfer value as the
reported `residual` — chamfer is still the right measure of *how symmetric the
hull is* once the plane is known. It is only the search it cannot drive.

**Budget — and a correction.** An earlier draft of this task said `gate.score`
costs 90.7ms per call at 400k points, so the search would take 10–30s per ship,
and told you to subsample for speed. **That was priced against the wrong
function.** The Task 6 review profiled the operation this search actually
performs — the inside-matte *fraction* at `mirror._SUBSAMPLE = 4000`, the
subsample the existing solver already uses — at **0.333ms**, so 300 evaluations
is about 0.1s per ship. Speed is not a concern here and you should not contort
the design for it.

**But do not call `gate.score` in the loop, for a correctness reason.** `score`
builds a mask and closes it morphologically, which makes it density-coupled: the
same cloud scores 0.204 at 4000 subsampled points and 0.991 at 60k. A subsampled
IoU is comparable neither to `IOU_FLOOR` nor across candidate planes with
differing point counts. Use the per-point `gate.inside_fraction`, which involves
no closing and is density-invariant.

**Fix the bound-discard guard, which is currently defeated and untested.**
Task 5 added a guard that discards any optimiser result terminating on a scale
bound, then falls back to `best_any` — the best result of any kind — if every
candidate was discarded. That fallback returns exactly the value the guard exists
to reject. Measured on the recovery fixture, 3 axes x scale0 in {0.5, 1.0, 2.0},
9 candidates:

```
axis=0 scale0=0.5/1.0/2.0 -> scale=0.200000  residual 1.21e-02  ON BOUND (x3)
axis=1 scale0=0.5         -> scale=0.200000  residual 7.88e-03  ON BOUND
axis=1 scale0=1.0         -> scale=1.666667  residual 4.87e-09
axis=1 scale0=2.0         -> scale=1.666667  residual 2.68e-09
axis=2 scale0=0.5/1.0     -> scale=0.200000  residual 7.29e-03  ON BOUND (x2)
axis=2 scale0=2.0         -> scale=1.473616  residual 1.75e-02
```

Two separate problems:

- **The guard is not outcome-determining, so no test pins it.** `axis=1` at
  `scale0=1.0/2.0` already wins on raw residual (2.68e-9 against ≥7.29e-03), so
  deleting the guard entirely leaves the final answer identical — verified by
  mutation. It is currently unfalsifiable code.
- **`best_any` silently defeats it.** With `scale0=0.5` alone, all three axes land
  on the bound, `best` stays `None`, and the fallback returns a bound result at
  residual 0.0073 — *under* `RESIDUAL_CEILING`, i.e. a 5x depth collapse reported
  as trustworthy. That is precisely the false-trust failure the guard was added
  for.

Required: an all-bounds outcome must be a reported **failure**, not a silent
fallback. Add a field to `Symmetry` (you are already adding `depth_separation` and
`obliquity`) or raise — your call, but it must be visible to `run.process` and
recorded in `quality.json`. Pin it with a test that constructs the all-bounds case
and asserts the failure is surfaced; a guard whose removal changes no test is not
a guard.

**Also solve `shift` here.** Self-chamfer cannot identify it: with the plane
offset free the objective is translation-equivariant, so `shift` and `offset`
form a flat valley, and Task 5's measured shifts of 0.000000 / 0.561740 /
−1.205369 from initial values 0.0 / 0.7 / −1.5 all sat below 1e-7 cost on one
cloud. Reprojection under perspective *is* sensitive to a z-translation, so this
objective can identify what chamfer cannot. Pin it with a test that recovers a
known injected shift, and confirm it reddens when the solve returns its initial
value.

**Calibrate `RESIDUAL_CEILING` on ONE-VIEW clouds, never two-sided ones.** The
old 0.013 came from a two-sided synthetic hull, which production never has: on a
one-view fixture that is exactly symmetric with zero noise the residual is
already 0.0123, and 0.0133 at sigma 0.005 — over the ceiling before any
asymmetry exists. Because the residual divides by the *observed* bounding box it
is partly a function of how much of the hull the camera saw, which is why no
synthetic constant transfers. Derive the new value from the real clouds measured
in Step 5, state the margin, and say plainly what it separates.

- [ ] **Step 1: Write the failing test — a one-sided synthetic cloud**

The fixture must be a **front-facing SURFACE SHEET**, not a subsample of a solid
volume. This is the single most important thing in this task, and the first draft
of it was wrong in a way that would have failed on correct code.

**Do NOT build the fixture from `_symmetric_hull()`.** That fixture samples a
solid volume, and reflecting a bilaterally symmetric *solid* across its true
plane maps the solid onto itself — so the depth separation of the CORRECT answer
is zero by construction. Measured directly: sweeping azimuth 0–70 degrees at
elevation 25, the true plane's `depth_separation / z-extent` came out
−0.004, −0.000, −0.003, +0.002, +0.005 — indistinguishable from zero, sometimes
negative, against a floor of 0.01. `assert good.depth_separation > 0.0` fails
about half the time on a perfect solver. Depth separation only exists for a
one-view sheet, where the reflected front surface becomes the genuinely occluded
back surface.

```python
def _hull_surface(n=60000, a=2.0, b=1.0, c=0.6, seed=0):
    """Points ON a hull surface, with outward normals. Symmetry plane: y=0.

    An ellipsoid rather than the tapered `_symmetric_hull`, because we need
    analytic outward normals to cull back faces, and because the taper is not
    what this test is about — the plane search is. Its symmetry about x and z is
    harmless here: this test asserts the recovered axis against a known answer
    rather than relying on the axis being unique.
    """
    rng = np.random.default_rng(seed)
    v = rng.normal(size=(n, 3))
    v /= np.linalg.norm(v, axis=1, keepdims=True)
    pts = v * np.array([a, b, c])
    nrm = v / np.array([a, b, c])
    nrm /= np.linalg.norm(nrm, axis=1, keepdims=True)
    return pts, nrm


def _one_sided_view(obliquity_deg=60.0, dist=8.0, shape=(480, 640), focal=600.0):
    """Render a hull as a single view: back-face culled, in camera coordinates.

    `obliquity_deg` is the angle that rotates the symmetry-plane normal toward
    the camera. It is the parameter that controls how much depth separation the
    TRUE plane can possibly show, so it is explicit rather than buried: at 0 the
    plane contains the view direction and separation is exactly zero; measured
    true-plane separation / z-extent is 0.028 at 30 deg, 0.098 at 45, 0.292 at
    60, 0.938 at 75. 60 is chosen to sit well clear of the 0.01 floor while
    staying a plausible three-quarter view.

    Returns (points_cam, mask, K, expected_normal_cam).
    """
    b = np.radians(obliquity_deg)
    R = np.array([[1.0, 0.0, 0.0],
                  [0.0, np.cos(b), np.sin(b)],
                  [0.0, -np.sin(b), np.cos(b)]])
    pts_w, nrm_w = _hull_surface()
    t = np.array([0.0, 0.0, dist])
    cam = pts_w @ R.T + t
    nrm_c = nrm_w @ R.T
    viewdir = cam / np.linalg.norm(cam, axis=1, keepdims=True)
    view = cam[(nrm_c * viewdir).sum(1) < 0]     # back-face cull

    h, w = shape
    K = np.array([[focal, 0.0, w / 2.0], [0.0, focal, h / 2.0], [0.0, 0.0, 1.0]])
    mask = gate.reproject(view, K, shape)        # the view's own silhouette
    return view, mask, K, R @ np.array([0.0, 1.0, 0.0])


def test_solve_from_view_recovers_the_plane_a_self_chamfer_solve_folds():
    """The regression test for the whole task: same input, both solvers.

    `solve` (self-chamfer) must fold and `solve_from_view` must not. Asserting
    only that the new solver works would leave the test passing if someone
    quietly routed it back to the old objective.
    """
    view, mask, K, n_true = _one_sided_view(obliquity_deg=60.0)
    zext = float(view[:, 2].max() - view[:, 2].min())

    good = mirror.solve_from_view(view, mask, K)
    axis_err = np.degrees(np.arccos(min(1.0, abs(good.normal @ n_true))))
    assert axis_err < 8.0, axis_err
    # Measured 0.292 * zext for the true plane at this obliquity, versus
    # 0.00003 * zext for the best fold — a 12000x gap. Assert well inside it.
    assert good.depth_separation > 0.05 * zext, good.depth_separation
    assert good.obliquity > 0.8, good.obliquity     # cos(60 deg) rotation -> 0.866

    folded = mirror.solve(view)                    # the superseded objective
    fold_err = np.degrees(np.arccos(min(1.0, abs(folded.normal @ n_true))))
    assert fold_err > 20.0, (
        "the self-chamfer solve is expected to fold on one-sided input; if it "
        "no longer does, this task's premise changed and the fixture is wrong")
```

Note `gate.reproject(view, K, shape)` is used to build the fixture's own mask, so
the test's silhouette and the solver's scoring share one projection. `reproject`
takes `(points, intrinsics, shape)` and returns a closed `(H,W)` uint8 mask —
verified against the delivered Task 6 signature. It treats intrinsics with
`K[0,2] <= 2.0` as unit-normalised, so the explicit pixel-unit `K` above is
passed through unscaled, which is what we want.

`test_mirror.py` does not currently import `gate`; add `gate = _load("gate")`
alongside the existing `_load` calls at the top of the file.

- [ ] **Step 2: Run it and confirm both halves behave as stated**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_mirror.py -k from_view -v`
Expected: FAIL on `solve_from_view` not existing. Before implementing, confirm
the second half of the assertion independently — that `solve` really does fold
on this fixture — and record the measured `fold_err`. If it does not fold, the
fixture is not one-sided enough and must be rebuilt before continuing.

- [ ] **Step 3: Implement `solve_from_view`**

Search plane orientation and offset as in `solve`, but score with the two terms
above instead of chamfer. Reuse `_chamfer` only to fill `residual`.

- [ ] **Step 4: Prove the depth term is load-bearing**

Delete the depth-separation constraint, leaving silhouette agreement alone, and
confirm the fold returns and the test reddens. A fold reprojects *inside* the
matte perfectly well — measured silhouette agreement was 0.55 for the true plane
and comparable for folds on the synthetic sheet — so silhouette agreement alone
is not sufficient. If the test still passes without the depth term, it is not
testing what it claims.

Record the measured numbers both ways in a comment. For reference, the numbers
this step should reproduce on the `obliquity_deg=60` fixture: true plane
`sep/zext = 0.292`, best fold found by a 400-plane random search
`sep/zext = 0.00003`.

- [ ] **Step 5: Re-measure the real clouds and re-derive the ceiling**

```bash
~/moge-venv/bin/python - <<'PY'
import sys; sys.path.insert(0, ".")
from tools.footprint import matte, pointmap, mirror, paths
import cv2, os
for name in ("outerrim_prayer", "ledger", "smelter", "magnate", "comet"):
    p = os.path.join(str(paths.HERO_DIR), f"{name}.webp")
    img = cv2.cvtColor(cv2.imread(p), cv2.COLOR_BGR2RGB)
    m, _ = matte.extract(img)
    c = pointmap.infer(img, m)
    s = mirror.solve_from_view(c.points, m, c.intrinsics)
    full = mirror.complete(c.points, s)
    import numpy as np
    ext = np.linalg.norm(c.points.max(0) - c.points.min(0))
    zext = c.points[:, 2].max() - c.points[:, 2].min()
    gain = np.linalg.norm(full.max(0) - full.min(0)) / ext
    print(f"{name:18s} residual={s.residual:.5f} depth_sep={s.depth_separation:.4f} "
          f"sep/zext={s.depth_separation/zext:.4f} obliquity={s.obliquity:.3f} "
          f"extent_gain={gain:.3f} normal={np.round(s.normal,3)}")
PY
```

Report `sep/zext` and `obliquity` per ship, not just the raw separation — the
synthetic sweep above shows the achievable separation spans a factor of 50 across
the obliquity range, so a raw number without its obliquity says nothing about
whether the solve succeeded.

**Also calibrate `gate.MIR_FLOOR` here, because this is the first point at which
a correct plane exists to calibrate against.** Task 6's fix round adds
`gate.inside_fraction` and a provisional floor but cannot pin it: the mirrored
fraction is only meaningful once stage 4 returns a plane that is not a fold. Add
the mirrored fraction to the table above, and report it for the solved plane
alongside at least three deliberately wrong planes per ship (`n = +x`, `n = +y`,
`n = +z`, and the solved plane with `offset + 1`). The review measured this
separation on the old folding solve: mirrored fraction spans 0.078–0.933 across
those cases, and `outerrim_prayer` with `offset + 1` reads 0.267 where union IoU
passed it at 0.734. Propose a floor with the margin stated on both sides, and
say plainly if the populations overlap — if they do, that is a finding, not a
number to split.

Report the table. A working solve should show `extent_gain` clearly above the
1.035× the folding solver produced, and mutually consistent normals across
ships. **Do not retune `RESIDUAL_CEILING` to make these pass** — report the
measured residuals and propose a value with its justification, noting that the
old 0.013 was derived from a synthetic hull with Gaussian noise and that real
clouds are the better calibration source.

- [ ] **Step 6: Commit**

```bash
git add tools/footprint/mirror.py tools/footprint/test_mirror.py
git commit -m "feat(footprint): find the symmetry plane by silhouette and depth, not self-chamfer"
```

---

### Task 7: Stage 6 — ground projection and alpha shape

**Files:**
- Create: `tools/footprint/ground.py`
- Test: `tools/footprint/test_ground.py`

**Interfaces:**
- Consumes: `mirror.Symmetry`, `camera.Fit`
- Produces: `ground.up_vector(sym: mirror.Symmetry, points: np.ndarray, normals=None, fit: camera.Fit | None = None) -> np.ndarray`. Derives the hull frame from geometry; `fit` is accepted only for logging agreement and never overrides it. NOTE the argument order changed on 2026-07-29 — `sym` and `points` are now first and `fit` is last and optional.
- Produces: `ground.project(points, up) -> np.ndarray (N,2)`
- Produces: `ground.hull(xy: np.ndarray, alpha: float) -> shapely.geometry.Polygon`
- Produces: `ground.sweep_alpha(clouds_xy: list[np.ndarray], candidates: list[float]) -> float`
- Produces: `ground.run(ship_id, points, up, alpha) -> shapely.geometry.Polygon`, writing `footprint.json`

- [ ] **Step 1: Write the failing test**

```python
"""Checks for ground projection and the alpha-shape footprint.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_ground.py
"""

import importlib
import pathlib
import sys
import types

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


def _hull_cloud(n=4000, seed=0):
    """A hull whose three axes have DISTINCT extents, in a rotated frame.

    Extents must differ: measured 3.999 long, 1.599 wide, 0.800 tall. If two
    were equal the principal axis would be ambiguous and the test could pass on
    a solver that picked either. The rotation angle (1.15 rad) is chosen so a
    stub returning a fixed coordinate axis fails with margin. Measured |dot|
    against the true up, all three against the 0.98 bar:

        fixed +Z axis                       0.798
        centroid direction (the old         0.167
          normals-mean fallback's shape)
        longitudinal instead of up          0.001
        the intended derivation             0.999999

    (An earlier draft of this docstring recorded 0.798 for the centroid stub as
    well. Measured, it is 0.167 — the stub fails harder than claimed, not less
    hard, but the number was wrong and is corrected here.)
    """
    rng = np.random.default_rng(seed)
    local = rng.uniform([-2.0, -0.8, -0.4], [2.0, 0.8, 0.4], size=(n, 3))
    axis = np.array([0.3, 0.5, 0.81])
    axis /= np.linalg.norm(axis)
    c, s = np.cos(1.15), np.sin(1.15)
    K = np.array([[0, -axis[2], axis[1]], [axis[2], 0, -axis[0]], [-axis[1], axis[0], 0]])
    R = np.eye(3) * c + s * K + (1 - c) * np.outer(axis, axis)
    # local x = longitudinal, y = lateral, z = up
    return local @ R.T, R[:, 1], R[:, 2]


def test_up_vector_comes_from_the_hull_geometry():
    """Recover a known up from the symmetry plane and the cloud's own axes.

    The cloud is deliberately rotated out of axis alignment and given three
    distinct extents, so this fails for a stub returning a fixed axis, for one
    returning the mean normal, and for one that confuses the longitudinal and
    up axes. `fit=None` throughout: the geometric path is the primary one, not
    a fallback.
    """
    pts, lateral, up_true = _hull_cloud()
    sym = types.SimpleNamespace(normal=lateral, offset=0.0)
    up = ground.up_vector(sym, pts, normals=None, fit=None)
    assert abs(abs(float(up @ up_true)) - 1.0) < 0.02, up @ up_true


def test_up_vector_ignores_a_confident_but_wrong_camera_fit():
    """A confident camera fit must not override the geometric frame.

    Stage 2 is a cross-check (see Global Constraints). One of fourteen real
    images produces a confident fit at all, and on a synthetic oracle roughly
    one seed in thirty produces a confident fit that is 24 degrees wrong, so a
    code path that deferred to `fit` would import that error. Passing a
    deliberately garbage R with high confidence must change nothing.
    """
    pts, lateral, up_true = _hull_cloud()
    sym = types.SimpleNamespace(normal=lateral, offset=0.0)
    baseline = ground.up_vector(sym, pts, normals=None, fit=None)
    bogus = types.SimpleNamespace(R=np.eye(3), confidence=0.99, source="auto")
    assert np.allclose(ground.up_vector(sym, pts, normals=None, fit=bogus), baseline)


def test_up_vector_sign_follows_the_viewer():
    """Up must point toward the camera side, not away from it.

    Sign is the one thing geometry alone cannot settle — lateral x longitudinal
    is up or down depending on handedness — so it is resolved from the normals.
    Flipping every normal must flip the returned up, or the sign logic is dead
    code and the footprint could come out mirrored.
    """
    pts, lateral, _ = _hull_cloud()
    sym = types.SimpleNamespace(normal=lateral, offset=0.0)
    normals = np.tile(np.array([0.0, 0.0, -1.0]), (len(pts), 1))
    a = ground.up_vector(sym, pts, normals=normals, fit=None)
    b = ground.up_vector(sym, pts, normals=-normals, fit=None)
    assert float(a @ b) < -0.98, (a, b)


def test_alpha_shape_keeps_a_concavity_a_convex_hull_would_erase():
    """Two parallel bars with a gap: the nacelle case.

    Measured on this exact fixture (true two-bar area 4.800, convex hull 7.987):
    the alpha shape is a single gap-filled blob up to alpha 2.4 (area 7.85-7.88)
    and correctly splits into 2 parts from alpha 2.5 up. alpha=3.0 therefore
    sits above the transition with margin.

    The area assertion is what makes this test bite. Without it, the two
    `contains` checks pass on an implementation that keeps only the LARGEST
    part — that part still holds one bar's interior point, and the gap is
    trivially absent from it. Measured: max-area returns 2.336 (one bar),
    keeping both returns 4.663. So the area bound below is the assertion that
    fails for `max(geoms, key=area)`, and it is the bug this fixture exists to
    catch.
    """
    rng = np.random.default_rng(0)
    left = rng.uniform([-2, -1.0], [2, -0.4], size=(3000, 2))
    right = rng.uniform([-2, 0.4], [2, 1.0], size=(3000, 2))
    xy = np.vstack([left, right])

    poly = ground.hull(xy, alpha=3.0)
    assert not poly.contains_properly(_point(0.0, 0.0)), "gap was filled in"
    assert poly.contains(_point(0.0, -0.7)), "lost the lower bar"
    assert poly.contains(_point(0.0, 0.7)), "lost the upper bar"
    assert poly.area > 4.0, f"only kept part of the hull: area {poly.area:.3f}"
    assert poly.area < 6.0, f"gap partially filled: area {poly.area:.3f}"


def test_hull_of_two_bars_stays_multi_part():
    """The twin-nacelle footprint must survive as a MultiPolygon.

    This is the `hull` half of the pair guarded here; the `profile.canonicalise`
    half lives in Task 8's test file, because `profile.py` does not exist yet at
    Task 7 time and importing it here raises ModuleNotFoundError at collection.
    """
    rng = np.random.default_rng(0)
    xy = np.vstack([rng.uniform([-2, -1.0], [2, -0.4], size=(3000, 2)),
                    rng.uniform([-2, 0.4], [2, 1.0], size=(3000, 2))])
    poly = ground.hull(xy, alpha=3.0)
    assert poly.geom_type == "MultiPolygon", poly.geom_type


def _point(x, y):
    from shapely.geometry import Point
    return Point(x, y)


def test_alpha_shape_of_a_solid_rectangle_has_the_right_area():
    rng = np.random.default_rng(1)
    xy = rng.uniform([-2, -1], [2, 1], size=(6000, 2))
    poly = ground.hull(xy, alpha=3.0)
    assert abs(poly.area - 8.0) / 8.0 < 0.15, poly.area


def test_sweep_picks_the_tightest_alpha_that_keeps_its_points():
    """Assert the VALUE, not merely membership in the candidate list.

    `assert alpha in candidates` cannot fail: sweep_alpha returns a candidate by
    construction, so a stub returning `candidates[0]` (0.5) and a stub returning
    `max(candidates)` (20.0) both pass it. Both are wrong.

    Measured on these three clouds: alpha 0.5 keeps 2000/2000 points (area
    7.849, 1 part), alpha 3.0 keeps 2000/2000 (area 7.764, 1 part), alpha 20.0
    fragments into 3 parts and keeps only 1644/2000 = 82.2%, failing the 90%
    retention bar. So 3.0 is the tightest passing candidate and the only correct
    answer.
    """
    rng = np.random.default_rng(2)
    clouds = [rng.uniform([-2, -1], [2, 1], size=(2000, 2)) for _ in range(3)]
    alpha = ground.sweep_alpha(clouds, candidates=[0.5, 3.0, 20.0])
    assert alpha == 3.0, alpha
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
import shapely
from shapely.geometry import MultiPolygon, mapping

from . import paths

ALPHA_CANDIDATES = [0.5, 1.0, 2.0, 3.0, 5.0, 8.0, 13.0]

# alphashape's cost is superlinear and severe: measured on a uniform rectangle,
# 2k points take 0.59s, 8k take 2.66s, 20k take 7.79s and 50k take 21.54s. A
# production MoGe cloud is ~400k points, and sweep_alpha evaluates every
# candidate against every cloud. Extrapolating, a 7-candidate sweep over 335
# ships at full density is hundreds of hours. Subsample before both the sweep
# and the final hull; the footprint outline does not need every point, and using
# the same cap for both keeps the chosen alpha meaningful for the hull it will
# be applied to.
SWEEP_SAMPLE = 20_000


def subsample(xy: np.ndarray, cap: int = SWEEP_SAMPLE, seed: int = 0) -> np.ndarray:
    """Deterministically thin a cloud to at most `cap` points.

    Seeded, because an alpha swept on one random subset and applied to another
    is not the alpha that was validated.
    """
    if len(xy) <= cap:
        return xy
    idx = np.random.default_rng(seed).choice(len(xy), size=cap, replace=False)
    return xy[np.sort(idx)]


def up_vector(sym, points, normals=None, fit=None) -> np.ndarray:
    """World up in camera coordinates, from the hull's own recovered geometry.

    REVISED 2026-07-29. This used to treat a confident vanishing-point fit as
    authoritative and fall back to `normals.mean(axis=0)`. Both were wrong:

    - Only 1 of 14 keyable hero images produces a confident fit at all, so the
      camera path is the exception, not the rule (see the plan's Global
      Constraints).
    - The mean surface normal of a SINGLE-VIEW cloud points roughly at the
      camera, not at the deck, because every visible facet faces the viewer.
      That fallback would have returned a badly wrong up on every ship that
      used it.

    The hull frame is fully determined by geometry already recovered upstream:
    the symmetry-plane normal is the lateral axis, the cloud's principal axis
    within that plane is longitudinal, and up is their cross product.

    `fit` is accepted only to log agreement where stage 2 is confident; it
    never overrides the geometric frame.
    """
    lateral = np.asarray(sym.normal, dtype=float)
    lateral /= np.linalg.norm(lateral)

    # Longitudinal: the dominant direction of the cloud with the lateral
    # component projected out, so it is guaranteed to lie in the symmetry plane.
    centred = points - points.mean(axis=0)
    in_plane = centred - np.outer(centred @ lateral, lateral)
    longitudinal = np.linalg.svd(in_plane, full_matrices=False)[2][0]
    longitudinal /= np.linalg.norm(longitudinal)

    up = np.cross(lateral, longitudinal)
    up /= np.linalg.norm(up)

    # Sign: the hull is seen from above, so the camera lies on the +up side.
    # In camera coordinates the viewer sits at the origin looking down +Z, so
    # the visible surface's normals carry the sign even though their mean is a
    # poor direction estimate. Fall back to "points away from the cloud
    # centroid's viewing direction" when normals are absent.
    if normals is not None and len(normals):
        reference = -np.asarray(normals, dtype=float).mean(axis=0)
    else:
        reference = -points.mean(axis=0)
    if float(up @ reference) < 0:
        up = -up
    return up


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


SPECK_AREA_FRACTION = 0.01     # of the largest part; below this it is noise


def hull(xy: np.ndarray, alpha: float):
    """The alpha shape, keeping every substantial part.

    Do NOT reduce a MultiPolygon to `max(geoms, key=area)`. A ship with
    separated nacelles has a genuinely MULTI-PART footprint, and taking the
    largest part reports one nacelle as the whole ship. Measured on the
    two-bar fixture below (bars at y in [-1,-0.4] and [0.4,1.0], true area
    4.800): at alpha 3.0 the alpha shape correctly splits into 2 parts, and
    max-area returns 2.336 — one bar — while keeping both returns 4.663.

    That collapse also corrupted two things downstream, which is why this is
    not a cosmetic choice:
      - `profile.canonicalise` reads `.exterior`, which a MultiPolygon does not
        have, so the collapse was masking an AttributeError rather than
        avoiding one.
      - `sweep_alpha` requires a cloud to retain 90% of its points; a collapsed
        twin-nacelle hull retains about half, so the sweep rejected every alpha
        tight enough to resolve the gap and drove the WHOLE BATCH toward
        convex-hull behaviour.

    Specks are still dropped: parts below SPECK_AREA_FRACTION of the largest
    are outlier noise, not structure.
    """
    poly = alphashape.alphashape(xy, alpha)
    if not isinstance(poly, MultiPolygon):
        return poly
    parts = sorted(poly.geoms, key=lambda g: g.area, reverse=True)
    if not parts:
        return poly
    floor = parts[0].area * SPECK_AREA_FRACTION
    keep = [g for g in parts if g.area >= floor]
    return keep[0] if len(keep) == 1 else MultiPolygon(keep)


def sweep_alpha(clouds_xy, candidates=None) -> float:
    """Pick one alpha for the whole batch: the tightest that keeps its points.

    Larger alpha hugs the points more closely but eventually sheds them; the
    batch value is the largest candidate for which every cloud's alpha shape
    still contains most of its own points.

    The criterion is point retention, NOT connectivity. A footprint that splits
    into two parts is the correct answer for a ship with separated nacelles, so
    requiring a single polygon would reject exactly the alphas that resolve the
    feature this stage exists to capture — and, because the batch shares one
    alpha, one twin-hull ship would drag every other ship toward convex-hull
    behaviour.

    Pass ALREADY-SUBSAMPLED clouds: see SWEEP_SAMPLE and the note on `hull`.
    """
    candidates = candidates or ALPHA_CANDIDATES
    best = float(min(float(c) for c in candidates))
    for a in sorted(float(c) for c in candidates):
        ok = True
        for xy in clouds_xy:
            p = hull(xy, a)
            if p.is_empty or p.area <= 0:
                ok = False
                break
            # shapely.contains_xy, not MultiPoint(...).intersection: measured at
            # 50k points the MultiPoint route takes 1.53s and this takes 0.012s,
            # a 127x difference, and it is called once per candidate per cloud.
            # They also disagree — MultiPoint with buffer(1e-9) counts boundary
            # vertices as covered and reports 50000/50000, while contains_xy
            # reports 49747. The threshold below is calibrated against
            # contains_xy's stricter count.
            kept = int(shapely.contains_xy(p, xy[:, 0], xy[:, 1]).sum())
            if kept < 0.9 * len(xy):
                ok = False
                break
        if ok:
            best = a
    return best


def run(ship_id: str, points, up, alpha: float):
    xy = subsample(project(points, up))
    poly = hull(xy, alpha)
    (paths.artifact_dir(ship_id) / "footprint.json").write_text(
        json.dumps({"alpha": alpha, "polygon": mapping(poly)}, indent=2))
    return poly
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_ground.py -v`
Expected: PASS, 8 tests.

Do NOT adjust the test fixtures' alpha to make a test go green. The alpha values
in these tests are measured against the fixtures they use — the two-bar
concavity fixture transitions from one gap-filled blob to two parts between
alpha 2.4 and 2.5, and 3.0 is chosen to sit above that with margin. If
`test_alpha_shape_keeps_a_concavity_a_convex_hull_would_erase` fails, `hull` is
collapsing or over-merging parts; fix `hull`, and report the measurement if you
believe the recorded transition is wrong.

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

import importlib
import pathlib
import sys

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
    mid = profile.STATIONS // 2      # NOT a hardcoded 48: STATIONS is 96 today,
                                     # and a literal index silently tests the
                                     # wrong station if that constant changes.
    assert concave[mid], "a split cross-section must be flagged"
    assert abs(w[mid] - 0.3) < 1e-6, w[mid]


def test_aspect_matches_length_over_maximum_beam():
    poly = Polygon([(0, -0.1), (1, -0.1), (1, 0.1), (0, 0.1)])
    w, _ = profile.sample(poly)
    assert abs(profile.aspect(w) - 5.0) < 1e-6, profile.aspect(w)


def test_canonicalise_accepts_a_multi_part_footprint():
    """A twin-nacelle footprint is a MultiPolygon, which has no `.exterior`.

    Reading `poly.exterior` raises AttributeError on exactly the shape this
    pipeline exists to represent. The defect was previously masked by
    `ground.hull` collapsing to its largest part, which silently discarded the
    other nacelle — so this test and Task 7's `test_hull_of_two_bars_stays_multi_part`
    guard the two halves of one bug. It lives here rather than in Task 7 because
    it needs both modules, and `profile.py` does not exist at Task 7 time.
    """
    ground = _load("ground")
    rng = np.random.default_rng(0)
    xy = np.vstack([rng.uniform([-2, -1.0], [2, -0.4], size=(3000, 2)),
                    rng.uniform([-2, 0.4], [2, 1.0], size=(3000, 2))])
    poly = ground.hull(xy, alpha=3.0)
    assert poly.geom_type == "MultiPolygon", poly.geom_type

    canon = profile.canonicalise(poly, np.array([0.0, 1.0]))
    xs = np.concatenate([np.array(g.exterior.coords)[:, 0] for g in canon.geoms])
    assert abs((xs.max() - xs.min()) - 1.0) < 1e-6, xs.max() - xs.min()


def test_sample_of_a_multi_part_footprint_does_not_crash():
    """`sample` must handle the MultiPolygon that `canonicalise` now returns.

    A station cut across two nacelles yields a MultiLineString; a station in the
    gap between them (if the parts do not span the full length) yields nothing.
    Both paths must produce a finite half-width array of the right length.
    """
    ground = _load("ground")
    rng = np.random.default_rng(0)
    xy = np.vstack([rng.uniform([-2, -1.0], [2, -0.4], size=(3000, 2)),
                    rng.uniform([-2, 0.4], [2, 1.0], size=(3000, 2))])
    canon = profile.canonicalise(ground.hull(xy, alpha=3.0), np.array([0.0, 1.0]))
    w, concave = profile.sample(canon)
    assert len(w) == profile.STATIONS
    assert np.isfinite(w).all()
    assert concave[profile.STATIONS // 2], "a two-nacelle cut must flag as split"


def test_a_plausible_hull_passes_the_dimensional_check():
    """A 5:1 hull with real depth must not be rejected.

    The bounds exist to catch pancakes and slivers, so they must not fire on an
    ordinary hull — a check that rejects everything is as useless as one that
    rejects nothing.
    """
    poly = Polygon([(0, -0.1), (1, -0.1), (1, 0.1), (0, 0.1)])
    w, _ = profile.sample(poly)
    assert abs(profile.aspect(w) - 5.0) < 1e-6, profile.aspect(w)
    # beam = 0.2, so depth 0.08 is depth/beam = 0.40, well above the 0.15 floor
    assert profile.implausible(w, depth_extent=0.08) is None


def test_a_flattened_reconstruction_is_rejected_as_a_pancake():
    """The case stage 5 provably cannot catch.

    A cloud flattened along the viewing rays reprojects to the SAME silhouette —
    measured IoU 0.9910 either way — so it arrives here with silhouette_pass
    true. If this check does not fire, nothing in the pipeline does.
    """
    poly = Polygon([(0, -0.1), (1, -0.1), (1, 0.1), (0, 0.1)])
    w, _ = profile.sample(poly)
    reason = profile.implausible(w, depth_extent=0.01)   # depth/beam = 0.05
    assert reason is not None, "a flat card must not be publishable"
    assert "flat card" in reason, reason


def test_an_implausible_aspect_is_rejected_and_says_why():
    """Both ends of the aspect band, and the reason string is part of the API.

    The batch report names why each ship was excluded, so an empty or generic
    reason would make a failure unactionable.
    """
    stubby = Polygon([(0, -0.6), (1, -0.6), (1, 0.6), (0, 0.6)])     # aspect 0.83
    w, _ = profile.sample(stubby)
    r = profile.implausible(w, depth_extent=1.0)
    assert r is not None and "stubby" in r, r

    sliver = Polygon([(0, -0.02), (1, -0.02), (1, 0.02), (0, 0.02)])  # aspect 25
    w, _ = profile.sample(sliver)
    r = profile.implausible(w, depth_extent=1.0)
    assert r is not None and "sliver" in r, r
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

    # Every part's coords, not `out.exterior`: a twin-nacelle footprint is a
    # MultiPolygon, which has no `.exterior` at all (AttributeError). This was
    # previously hidden by ground.hull collapsing to its largest part, which
    # silently discarded the other nacelle.
    geoms = list(out.geoms) if hasattr(out, "geoms") else [out]
    xs, ys = np.concatenate([np.array(g.exterior.coords) for g in geoms]).T
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


# Stage 7 is the ONLY place a scale or depth error can be caught. The stage 5
# silhouette gate is provably blind to both: `uv = K.p / p_z` is exactly
# invariant under `p -> lambda(p) . p` for any positive per-point lambda, so a
# uniformly scaled cloud and a cloud flattened along the viewing rays reproject
# to the identical silhouette. Measured in the Task 6 review: flattening a cloud
# to 10% of its depth extent leaves IoU at 0.9910, unchanged to four decimals
# from the unflattened cloud, and x5 global scale likewise. A hull reconstructed
# as a billboard arrives here with `silhouette_pass: true`.
#
# So these bounds are not defensive boilerplate — they are the pipeline's only
# check on the dimension it exists to publish.
ASPECT_BOUNDS = (1.2, 12.0)      # derived from the catalog's own ship dimensions
MIN_DEPTH_TO_BEAM = 0.15         # below this the reconstruction is a pancake


def implausible(w: np.ndarray, depth_extent: float) -> str | None:
    """Why this profile must not be published, or None if it is plausible.

    Returns a reason string rather than a bool so the batch report can say what
    was wrong with each excluded ship instead of just counting failures.
    """
    a = aspect(w)
    lo, hi = ASPECT_BOUNDS
    if not np.isfinite(a):
        return "degenerate footprint: zero maximum beam"
    if a < lo:
        return f"aspect {a:.2f} below {lo}: too stubby to be a hull"
    if a > hi:
        return f"aspect {a:.2f} above {hi}: a sliver, not a hull"
    beam = 2.0 * float(np.max(w))
    if depth_extent / beam < MIN_DEPTH_TO_BEAM:
        return (f"depth/beam {depth_extent / beam:.3f} below "
                f"{MIN_DEPTH_TO_BEAM}: reconstructed as a flat card")
    return None


def run(ship_id: str, poly, sym_normal_xy, quality: dict,
        depth_extent: float | None = None) -> dict:
    canon = canonicalise(poly, sym_normal_xy)
    w, concave = sample(canon)
    reason = None if depth_extent is None else implausible(w, depth_extent)
    data = {"id": ship_id, "stations": STATIONS, "w": w.tolist(),
            "concave": concave.tolist(), "aspect": aspect(w), "quality": quality,
            "dimensional_pass": reason is None, "dimensional_reason": reason}
    (paths.artifact_dir(ship_id) / "profile.json").write_text(json.dumps(data, indent=2))
    return data
```

`depth_extent` is optional so the ground-truth chain in Task 9 can call `run`
without a cloud, but `run.process` must always pass it — a profile written with
`dimensional_pass` unevaluated is exactly the silent publish this check exists to
prevent, so Task 9's driver treats a missing `depth_extent` as a failure rather
than a skip.

- [ ] **Step 4: Run the test to verify it passes**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_profile.py -v`
Expected: PASS, 9 tests.

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

**Sequencing:** Task 6b MUST be complete before this task starts. The end-to-end
test and `run.process` both call `mirror.solve_from_view`, which Task 6b creates —
running this task against the plain self-chamfer `solve` would exercise the very
plane search that was measured to fold, and the `gain > 1.15` assertion below is
precisely the check that fold fails (measured 1.035x). Execution order for the
remaining tasks is 6b, 7, 8, 9 — not the numeric order.

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

import importlib
import pathlib
import sys

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

# Match test_pointmap.py exactly. Do NOT write the guard as
# `pytest.mark.skipif(not __import__("importlib").util.find_spec("torch"), ...)`:
# `importlib.util` is a SUBMODULE and is not bound by `import importlib`, so that
# expression raises `AttributeError: module 'importlib' has no attribute 'util'`
# in plain Python. Verified — it only appears to work under pytest, because
# pytest imports importlib.util itself as a side effect. Relying on another
# library's imports to make your guard evaluable is a latent collection failure.
#
# Also per-function marks, never a module-level `pytestmark`: pytest applies a
# bare `pytestmark` to every test in the module regardless of where the line sits
# in the file, which would skip the two CPU-only ground-truth tests on a box with
# no GPU — and those are the tests that assert our own arithmetic.
torch = pytest.importorskip("torch")
needs_cuda = pytest.mark.skipif(not torch.cuda.is_available(), reason="needs CUDA")


def _ground_truth_chain(scene):
    """Stages 4-7 on a perfect cloud: our arithmetic, no network involved."""
    cam = scene.points @ scene.R.T + scene.t
    # `solve`, not `solve_from_view`: scene.points is a TWO-SIDED volume sampled
    # over the whole mesh, which is the regime self-chamfer handles correctly.
    # This chain exists to validate our arithmetic, so it must not depend on the
    # silhouette machinery the real path uses.
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
    mid, near_end = profile.STATIONS // 2, profile.STATIONS // 12
    assert w[mid] > w[near_end] and w[mid] > w[-near_end], \
        "a circle must be widest amidships"


@needs_cuda
def test_moge_chain_is_reported_not_asserted(capsys):
    pointmap = _load("pointmap")
    matte = _load("matte")
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    cloud = pointmap.infer(s.image, mask)
    # A MoGe cloud is one-sided even on a synthetic render, so this is the
    # view-based solve, not the two-sided one.
    sym = mirror.solve_from_view(cloud.points, mask, cloud.intrinsics)
    with capsys.disabled():
        print(f"\n  MoGe on synthetic box: n={len(cloud.points)} "
              f"mirror residual={sym.residual:.4f} "
              f"(ceiling {mirror.RESIDUAL_CEILING})")
    assert len(cloud.points) > 0


@needs_cuda
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
    # Documented invariant, NOT a test of our code: MoGe emits camera-space
    # points with strictly positive depth by construction (measured z in
    # [23.9, 137] even on pure random-noise input), so this cannot fail unless
    # a later change starts transforming the cloud. It is here because
    # gate.reproject relies on it when it filters points[:, 2] > 1e-6.
    assert (cloud.points[:, 2] > 0).all(), "points behind the camera"

    # A hull seen in 3/4 must have real depth relief. A collapsed depth range
    # means the model read the image as a flat card, which would make every
    # downstream footprint a silhouette rather than a shape.
    d = cloud.points[:, 2]
    assert d.std() / d.mean() > 0.02, f"depth relief {d.std() / d.mean():.4f} too flat"

    sym = mirror.solve_from_view(cloud.points, mask, cloud.intrinsics)
    assert sym.residual < 0.25, f"Prayer is a boxy hull; residual {sym.residual:.3f}"
    # The fold check, which is the failure this stage actually had: a folded
    # plane leaves the completed cloud barely larger than the visible one
    # (measured 1.035x with the superseded self-chamfer solve). A genuine
    # occluded half must add real width.
    full = mirror.complete(cloud.points, sym)
    ext = float(np.linalg.norm(cloud.points.max(0) - cloud.points.min(0)))
    gain = float(np.linalg.norm(full.max(0) - full.min(0))) / ext
    assert gain > 1.15, f"completion added almost nothing ({gain:.3f}x): plane folded"
```

The floors are deliberately loose. They catch the failure modes that would silently poison every footprint — a flat depth read, points behind the camera, a cloud too sparse to outline — without encoding a guess about how accurate MoGe happens to be. If a floor fails, investigate the model or the input before relaxing the number.

- [ ] **Step 2: Run the test to verify it fails**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_endtoend.py -v -s`
Expected: FAIL — the ground-truth chain exercises `ground.project` with a scene-space up vector and will surface any axis-convention mismatch between stages. Fix the mismatch in the stage modules, not by loosening the test.

- [ ] **Step 3: Make the ground-truth chain pass**

The likely defect is the up vector: `scene.up` is world-space and must be rotated into camera space by `scene.R` before `ground.project` uses it, since the cloud is in camera coordinates. That rotation is `scene.up @ scene.R.T`, which the test already does — if it still fails, check that `mirror.complete` returns camera-space points and that `profile.canonicalise` receives the symmetry normal projected into the same ground frame.

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_endtoend.py -v -s`
Expected on a CUDA box: PASS, 4 tests. On a CPU-only box: 2 passed, 2 skipped — and
verify that explicitly with `CUDA_VISIBLE_DEVICES="" ~/moge-venv/bin/python -m pytest
tools/footprint/test_endtoend.py -q`, because the whole point of the per-function
marks is that the two ground-truth tests still run without a GPU.

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
    # Real clouds are one-sided, so this routes to solve_from_view (Task 6b),
    # which needs the matte and the intrinsics to score silhouette agreement
    # and depth separation.
    sym = mirror.run(ship_id, cloud, mask)
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

    # No needs_clicks branch: stage 2 is a cross-check, not a gate (REVISED
    # 2026-07-29 — see Global Constraints). The frame comes from the recovered
    # geometry, so a low camera confidence no longer excludes a ship. Where the
    # fit IS confident, up_vector logs the agreement; it never overrides.
    up = ground.up_vector(sym, full, cloud.normals, fit=fit)
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
        full = mirror.complete(cloud.points, sym)
        up = ground.up_vector(sym, full, cloud.normals, fit=fit)
        clouds.append(ground.project(full, up))
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

**Spec coverage.** All seven stages have a task. The batch-pinned alpha is Task 7 `sweep_alpha` plus `run._pick_alpha`. The separate venv is Task 1 Step 1. Stage 2 is a cross-check rather than a gate (revised 2026-07-29): `run.process` has no `needs_clicks` branch, and the reference frame comes from `ground.up_vector`'s geometric derivation, which is covered by three tests in Task 7 including one asserting a confident-but-wrong camera fit does not override it. Stage 5 exclusion is `run.process` returning `failed_silhouette_gate` and the report separating it from `ok`. Alpha shape over convex hull is Task 7, with a test that fails on a convex hull. Scale **and** shift is `mirror.solve`'s `refine_affine` path with its own no-op test, paired with a recovery test on a known depth squash (the no-op assertion alone passes with `scale` hardcoded to 1.0). The stage 4 plane search on real one-view clouds is Task 6b's `solve_from_view`, built after Task 6 because it scores against `gate.reproject`; `mirror.solve` remains the two-sided path and is what the synthetic ground-truth chain exercises. The synthetic end-to-end test is Task 9. The `disc+beam` gap needs no code — no local art matches those ships, so `resolve_heroes` will not return them.

**Two spec items are deliberately not in this plan**, because they belong to the consuming half: the `Merge` fix, and grading against inferred glyphs. This plan ends at `profile.json`.

**Known soft spots**, flagged rather than hidden. `ground.sweep_alpha` counts covered points via a `MultiPoint` intersection, which is correct but slow on large clouds — subsample to 2000 points per cloud if the sweep takes more than a minute. Task 3's RANSAC is the least certain part of this plan: it samples four segments per iteration to hypothesise two vanishing points, which needs both sampled pairs to be genuinely parallel in 3D. On a hull with few long straight edges that may take many iterations to hit, and the honest outcome is a low confidence that routes the ship to the click fallback rather than a wrong camera.
