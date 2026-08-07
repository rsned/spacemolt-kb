# Footprint Fusion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `tools/footprint/fuse.py`, which selects one canonical footprint
per catalog ship (267) from the pipeline/mesh/annotation candidates via a
deterministic rule ladder, and writes the tiered `data/footprints/fused/`
dataset with provenance and built-in validation.

**Architecture:** Pure downstream consumer — reads `report.json`, per-ship
`footprint.json` polygons, the TripoSR bakeoff profiles, user annotations, and
eyeball labels; never mutates inputs. A rule ladder (user_pick → wing_family →
prong_or_pod → wrecked_solve → clean_pipeline → unresolved) picks shape and
aspect sources independently. Spec:
`docs/superpowers/specs/2026-08-07-footprint-fusion-design.md`.

**Tech Stack:** Python 3 under `~/moge-venv/bin/python` (numpy + shapely,
already in the venv). Tests with pytest, run as
`~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q` from the
repo root `/home/robert/spacemolt/kb`.

## Global Constraints

- Python for this pipeline ALWAYS runs under `~/moge-venv/bin/python`
  (NEVER `~/sd-venv`). Working dir for commands: `/home/robert/spacemolt/kb`.
- `data/footprints/report.json` EMBEDS full profiles — fuse.py must treat it
  read-only. The fuser writes ONLY under `data/footprints/fused/`.
- 96 stations everywhere (`profile.STATIONS`).
- Mesh beam correction is exactly ×0.67 (aspect ÷0.67); squaring is
  `profile.snap_plateaus` with its default thresholds.
- Follow existing tools/footprint code style: module docstrings explaining
  WHY, constants at top, small pure functions, no classes unless dataclasses.
- TDD: every behavior gets a failing test first. Commit after each task.

## File Structure

- **Modify** `data/footprints/eyeball_labels_2026-08-01.json` — add a
  machine-readable `fusion_rosters` block (prose notes stay).
- **Create** `tools/footprint/fuse.py` — all fusion logic + `main()`.
- **Create** `tools/footprint/test_fuse.py` — unit tests (synthetic fixtures)
  + one real-data integration test.
- **Generated** `data/footprints/fused/<ship>.json` + `fused/index.json`
  (committed in the final task).

---

### Task 1: fusion_rosters block + loader

**Files:**
- Modify: `data/footprints/eyeball_labels_2026-08-01.json`
- Create: `tools/footprint/fuse.py`
- Create: `tools/footprint/test_fuse.py`

**Interfaces:**
- Produces: `fuse.load_rosters(labels: dict) -> Rosters` where `Rosters` is a
  frozen dataclass with fields `wing_filled: frozenset[str]`,
  `wing_crescent: frozenset[str]`, `prong_confirmed: frozenset[str]`,
  `receding: frozenset[str]` (union of both mirror batches),
  `family_pairs: tuple[tuple[str, str, float, float], ...]` — (ship_a,
  ship_b, lo, hi) meaning `fused_aspect(a) / fused_aspect(b)` is expected in
  `[lo, hi]`.

- [ ] **Step 1: Add the `fusion_rosters` block to the labels JSON**

Run this to insert it (values transcribed from the session rulings recorded in
the same file's prose):

```bash
~/moge-venv/bin/python - <<'EOF'
import json
p = 'data/footprints/eyeball_labels_2026-08-01.json'
d = json.load(open(p))
d['fusion_rosters'] = {
    '_desc': ('Machine-readable rosters lifted from the prose notes/rulings in '
              'this file (2026-08-07) so tools/footprint/fuse.py reads data, '
              'not English. The prose remains the human record; edit BOTH when '
              'a ruling changes.'),
    'wing_filled': ['apeiron'],
    'wing_crescent': ['voidborn_singularity', 'qualia', 'vigil'],
    'prong_confirmed': ['crucible', 'annihilator', 'encyclopedia', 'apeiron',
                        'frankenhauler'],
    'receding_right_2_3': ['probable_cause', 'running_tab', 'no_refunds',
        'spit_and_prayer', 'plausible_deniability', 'off_the_books',
        'manifest_destiny', 'losers_weepers', 'insurance_fraud',
        'hold_my_beer', 'hells_bells', 'glows_in_the_dark', 'free_radical',
        'finish_line', 'finders_keepers', 'dead_reckoning', 'baling_wire',
        'bad_idea'],
    'receding_left_9_10': ['claim_jumper', 'dust_devil',
        'five_finger_discount', 'junk_convoy', 'last_call', 'long_haul'],
    'family_pairs': [
        ['aether', 'close_enough', 1.5, 3.0],
        ['prayer', 'start_praying', 0.7, 1.4],
        ['overdue', 'overhead', 0.8, 1.25],
        ['crowbar', 'crucible', 0.7, 1.4],
        ['manifest_destiny', 'no_refunds', 0.6, 1.6],
    ],
}
json.dump(d, open(p, 'w'), indent=1)
print('rosters added')
EOF
```

- [ ] **Step 2: Write the failing loader test**

Create `tools/footprint/test_fuse.py`:

```python
"""Tests for the fusion selector (tools/footprint/fuse.py).

Unit tests run on synthetic fixtures; the final integration test reads the
real data tree and asserts headline invariants only.
"""
import json
import pathlib
import sys

import numpy as np

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
from tools.footprint import fuse  # noqa: E402

LABELS = json.loads(
    (pathlib.Path(__file__).resolve().parents[2]
     / "data/footprints/eyeball_labels_2026-08-01.json").read_text())


def test_load_rosters_reads_the_session_rulings():
    r = fuse.load_rosters(LABELS)
    assert "apeiron" in r.wing_filled
    assert {"voidborn_singularity", "qualia", "vigil"} <= r.wing_crescent
    assert "frankenhauler" in r.prong_confirmed
    assert "no_refunds" in r.receding and "long_haul" in r.receding
    assert any(a == "aether" and b == "close_enough"
               for a, b, _, _ in r.family_pairs)
```

- [ ] **Step 3: Run it to make sure it fails**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q`
Expected: FAIL — `fuse` has no attribute `load_rosters` (module is empty/new).

- [ ] **Step 4: Minimal implementation**

Create `tools/footprint/fuse.py`:

```python
"""Fuse the footprint candidates into one canonical geometry per ship.

The measurement half produced multiple candidates per ship (pipeline profile,
pipeline polygon, TripoSR mesh profile) that disagree in KNOWN ways — the
rulings live in eyeball_labels_2026-08-01.json. This module applies those
rulings as a deterministic rule ladder and writes data/footprints/fused/.
Design: docs/superpowers/specs/2026-08-07-footprint-fusion-design.md.
"""
import dataclasses
import json
import pathlib

import numpy as np

REPO = pathlib.Path(__file__).resolve().parents[2]
FOOT = REPO / "data/footprints"
BAKEOFF = REPO / "data/mesh_bakeoff/out-full"
MESH_BEAM = 0.67          # TripoSR over-widens; user-measured correction
SCHEMA = 1


@dataclasses.dataclass(frozen=True)
class Rosters:
    wing_filled: frozenset
    wing_crescent: frozenset
    prong_confirmed: frozenset
    receding: frozenset
    family_pairs: tuple


def load_rosters(labels: dict) -> Rosters:
    fr = labels["fusion_rosters"]
    return Rosters(
        wing_filled=frozenset(fr["wing_filled"]),
        wing_crescent=frozenset(fr["wing_crescent"]),
        prong_confirmed=frozenset(fr["prong_confirmed"]),
        receding=frozenset(fr["receding_right_2_3"] + fr["receding_left_9_10"]),
        family_pairs=tuple(tuple(p) for p in fr["family_pairs"]),
    )
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q`
Expected: 1 passed.

- [ ] **Step 6: Commit**

```bash
git add data/footprints/eyeball_labels_2026-08-01.json tools/footprint/fuse.py tools/footprint/test_fuse.py
git commit -m "feat(footprint): fusion rosters block + loader"
```

---

### Task 2: candidate loading

**Files:**
- Modify: `tools/footprint/fuse.py`
- Test: `tools/footprint/test_fuse.py`

**Interfaces:**
- Produces:
  - `fuse.Candidate` — dataclass with fields `pipe: dict | None` (the full
    report record for the ship — keys include `status`, `aspect`, `w`,
    `concave`, `orientation`, `orientation_source`, `quality`),
    `polygon: dict | None` (parsed `footprint.json`: `{"alpha", "polygon"}`
    GeoJSON), `mesh_w: np.ndarray | None` (raw 96 half-widths, uncorrected),
    `mesh_aspect: float | None` (raw, uncorrected).
  - `fuse.load_candidates(foot: pathlib.Path, bakeoff: pathlib.Path)
    -> dict[str, Candidate]` — one entry per report ship id. Mesh stems
    resolve exact-stem-first, then faction-prefix-stripped
    (`outerrim_|solarian_|voidborn_|crimson_|nebula_|pirate_`), first match
    wins — same convention as the contact sheet.
- Consumes: nothing from other tasks.

- [ ] **Step 1: Write the failing tests**

Append to `tools/footprint/test_fuse.py`:

```python
def _mini_tree(tmp_path):
    """Two-ship synthetic data tree: 'plain' (full candidates) and 'bare'
    (report record only)."""
    foot = tmp_path / "footprints"
    bakeoff = tmp_path / "bakeoff"
    w = [0.2] * 96
    report = {"alpha": 8.0, "background": "x", "results": [
        {"id": "plain", "status": "ok_asymmetric", "aspect": 2.5, "w": w,
         "concave": [False] * 96, "orientation": "bow_t0",
         "orientation_source": {"flipped_from_stored": False},
         "quality": {"silhouette_iou": 0.99}},
        {"id": "bare", "status": "failed_dimensional_check", "aspect": 0.5,
         "w": w, "concave": [False] * 96, "orientation": "unknown",
         "quality": {"silhouette_iou": 0.98}},
    ]}
    foot.mkdir()
    (foot / "report.json").write_text(json.dumps(report))
    (foot / "plain").mkdir()
    (foot / "plain" / "footprint.json").write_text(json.dumps({
        "alpha": 8.0,
        "polygon": {"type": "Polygon", "coordinates":
                    [[[0, -0.2], [1, -0.2], [1, 0.2], [0, 0.2], [0, -0.2]]]}}))
    # mesh stored under a faction prefix — exercises the prefix-stripped pass
    (bakeoff / "outerrim_plain").mkdir(parents=True)
    (bakeoff / "outerrim_plain" / "profile.json").write_text(json.dumps(
        {"w": [0.3] * 96, "aspect": 1.8}))
    return foot, bakeoff


def test_load_candidates_assembles_all_sources(tmp_path):
    foot, bakeoff = _mini_tree(tmp_path)
    cands = fuse.load_candidates(foot, bakeoff)
    c = cands["plain"]
    assert c.pipe["aspect"] == 2.5
    assert c.polygon["polygon"]["type"] == "Polygon"
    assert c.mesh_w is not None and len(c.mesh_w) == 96
    assert c.mesh_aspect == 1.8


def test_load_candidates_tolerates_missing_sources(tmp_path):
    foot, bakeoff = _mini_tree(tmp_path)
    c = fuse.load_candidates(foot, bakeoff)["bare"]
    assert c.pipe["status"] == "failed_dimensional_check"
    assert c.polygon is None and c.mesh_w is None and c.mesh_aspect is None


def test_mesh_resolution_prefers_the_exact_stem(tmp_path):
    foot, bakeoff = _mini_tree(tmp_path)
    # an exact-stem dir must beat the prefixed one, matching the contact sheet
    (bakeoff / "plain").mkdir()
    (bakeoff / "plain" / "profile.json").write_text(json.dumps(
        {"w": [0.5] * 96, "aspect": 9.9}))
    c = fuse.load_candidates(foot, bakeoff)["plain"]
    assert c.mesh_aspect == 9.9
```

- [ ] **Step 2: Run to verify failure**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q`
Expected: 3 new FAILs — `fuse` has no attribute `load_candidates`.

- [ ] **Step 3: Implement**

Append to `tools/footprint/fuse.py`:

```python
import re

_PREFIX = re.compile(r"^(outerrim|solarian|voidborn|crimson|nebula|pirate)_")


@dataclasses.dataclass
class Candidate:
    pipe: dict | None
    polygon: dict | None
    mesh_w: "np.ndarray | None"
    mesh_aspect: float | None


def _mesh_profiles(bakeoff: pathlib.Path) -> dict:
    """ship_id -> parsed mesh profile.json, exact stem preferred."""
    out = {}
    if not bakeoff.exists():
        return out
    stems = sorted(p.name for p in bakeoff.iterdir()
                   if (p / "profile.json").is_file())
    for exact_pass in (True, False):
        for stem in stems:
            sid = stem if exact_pass else _PREFIX.sub("", stem)
            if (exact_pass or sid != stem) and sid not in out:
                out[sid] = json.loads(
                    (bakeoff / stem / "profile.json").read_text())
    return out


def load_candidates(foot: pathlib.Path = FOOT,
                    bakeoff: pathlib.Path = BAKEOFF) -> dict:
    report = json.loads((foot / "report.json").read_text())
    mesh = _mesh_profiles(bakeoff)
    out = {}
    for rec in report["results"]:
        sid = rec["id"]
        fpp = foot / sid / "footprint.json"
        m = mesh.get(sid)
        out[sid] = Candidate(
            pipe=rec,
            polygon=json.loads(fpp.read_text()) if fpp.exists() else None,
            mesh_w=np.array(m["w"], dtype=float) if m else None,
            mesh_aspect=m.get("aspect") if m else None,
        )
    return out
```

- [ ] **Step 4: Run to verify pass**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add tools/footprint/fuse.py tools/footprint/test_fuse.py
git commit -m "feat(footprint): fusion candidate loading, exact-stem-first mesh resolution"
```

---

### Task 3: pick mapping + mesh corrections

**Files:**
- Modify: `tools/footprint/fuse.py`
- Test: `tools/footprint/test_fuse.py`

**Interfaces:**
- Produces:
  - `fuse.PICK_SOURCES: dict[str, tuple[str, bool]]` — panel id →
    (shape_source, squared): `{"moge": ("pipeline_profile", False),
    "footprint": ("pipeline_polygon", False), "mesh": ("mesh", False),
    "mesh067": ("mesh", False), "mesh067sq": ("mesh_squared", True)}`.
  - `fuse.mesh_shape(mesh_w: np.ndarray, squared: bool) -> np.ndarray` —
    applies ×0.67 beam, then `profile.snap_plateaus` when `squared`.
  - `fuse.mesh_adjusted_aspect(mesh_aspect: float) -> float` — `÷ 0.67`.
- Consumes: `profile.snap_plateaus` from `tools/footprint/profile.py`
  (exists; signature `snap_plateaus(w, wide_frac=0.80, snap_frac=0.85)`).

- [ ] **Step 1: Write the failing tests**

```python
def test_pick_sources_cover_every_sheet_panel():
    assert fuse.PICK_SOURCES == {
        "moge": ("pipeline_profile", False),
        "footprint": ("pipeline_polygon", False),
        "mesh": ("mesh", False),
        "mesh067": ("mesh", False),
        "mesh067sq": ("mesh_squared", True),
    }


def test_mesh_shape_applies_beam_correction_and_optional_squaring():
    w = np.full(96, 1.0)
    w[60:90] = 2.0
    w[70] = 1.8                       # dent inside the wide run
    plain = fuse.mesh_shape(w, squared=False)
    assert np.allclose(plain[0], 0.67) and np.allclose(plain[60], 1.34)
    assert np.isclose(plain[70], 1.8 * 0.67)   # dent survives unsquared
    sq = fuse.mesh_shape(w, squared=True)
    assert np.isclose(sq[70], 1.34)            # dent snapped to the plateau


def test_mesh_adjusted_aspect_inverts_the_beam_correction():
    assert np.isclose(fuse.mesh_adjusted_aspect(1.8), 1.8 / 0.67)
```

- [ ] **Step 2: Run to verify failure**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q`
Expected: FAILs — no attribute `PICK_SOURCES`.

- [ ] **Step 3: Implement**

```python
from tools.footprint import profile as _profile

# contact-sheet panel id -> (shape_source, squared). mesh and mesh067 are the
# same underlying source: the sheet's "mesh" panel is raw display, but a mesh
# pick always publishes beam-corrected geometry.
PICK_SOURCES = {
    "moge": ("pipeline_profile", False),
    "footprint": ("pipeline_polygon", False),
    "mesh": ("mesh", False),
    "mesh067": ("mesh", False),
    "mesh067sq": ("mesh_squared", True),
}


def mesh_shape(mesh_w, squared: bool):
    w = np.asarray(mesh_w, dtype=float) * MESH_BEAM
    return _profile.snap_plateaus(w) if squared else w


def mesh_adjusted_aspect(mesh_aspect: float) -> float:
    return mesh_aspect / MESH_BEAM
```

- [ ] **Step 4: Run to verify pass, then run the whole footprint suite**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add tools/footprint/fuse.py tools/footprint/test_fuse.py
git commit -m "feat(footprint): pick->source mapping and mesh beam/squaring corrections"
```

---

### Task 4: the rule ladder

**Files:**
- Modify: `tools/footprint/fuse.py`
- Test: `tools/footprint/test_fuse.py`

**Interfaces:**
- Produces:
  - `fuse.Decision` — dataclass: `rule: str`, `confidence: str`,
    `shape_source: str | None`, `aspect_source: str | None`,
    `squared: bool`, `notes: list[str]`.
  - `fuse.apply_rules(sid: str, cand: Candidate, rosters: Rosters,
    pick: str | None, picks: dict[str, str]) -> Decision`
    (`picks` is the full pick map — needed for the rule-4 sibling-squaring
    check). Rules per the spec; thresholds: bow-concave detector = ≥4 concave
    stations in `concave[:32]` AND those are ≥60% of all concave stations
    (requires `orientation == "bow_t0"`); pod-blob = ≥24 concave stations
    total; clean pipeline needs status `ok_asymmetric` (or any `ok*`) AND
    `quality.silhouette_iou >= 0.97`.
  - Aspect sources: rule 2 → `pipeline_profile`; rule 3 → `pipeline_profile`
    (polygon shares the pipeline's scale); rule 4 → `mesh`; rule 5 →
    `pipeline_profile`; rule 1 → same source as the shape (except
    `footprint` picks, whose aspect_source is `pipeline_profile`).
  - `fuse._sibling_squared(sid, picks, family_pairs) -> bool` — True if any
    family partner of `sid` has a `mesh067sq` pick.
- Consumes: Task 1 `Rosters`, Task 2 `Candidate`, Task 3 `PICK_SOURCES`.

- [ ] **Step 1: Write the failing tests** (one per rung + fall-throughs)

```python
def _cand(status="ok_asymmetric", iou=0.99, aspect=2.0, mesh=True,
          polygon=True, concave=None, orientation="bow_t0"):
    w = [0.25] * 96
    return fuse.Candidate(
        pipe={"id": "x", "status": status, "aspect": aspect, "w": w,
              "concave": concave or [False] * 96,
              "orientation": orientation,
              "orientation_source": {"flipped_from_stored": False},
              "quality": {"silhouette_iou": iou}},
        polygon={"alpha": 8.0, "polygon": {"type": "Polygon",
                 "coordinates": [[[0, -0.25], [1, -0.25], [1, 0.25],
                                  [0, 0.25], [0, -0.25]]]}} if polygon else None,
        mesh_w=np.full(96, 0.3) if mesh else None,
        mesh_aspect=1.7 if mesh else None)


def _rosters(**kw):
    base = dict(wing_filled=frozenset(), wing_crescent=frozenset(),
                prong_confirmed=frozenset(), receding=frozenset(),
                family_pairs=())
    base.update(kw)
    return fuse.Rosters(**base)


def test_rule1_user_pick_wins_and_maps_the_panel():
    d = fuse.apply_rules("s", _cand(), _rosters(), "mesh067sq", {})
    assert (d.rule, d.confidence) == ("user_pick", "user_pick")
    assert d.shape_source == "mesh_squared" and d.squared
    assert d.aspect_source == "mesh"


def test_rule1_footprint_pick_takes_pipeline_scale():
    d = fuse.apply_rules("s", _cand(), _rosters(), "footprint", {})
    assert d.shape_source == "pipeline_polygon"
    assert d.aspect_source == "pipeline_profile"


def test_rule1_stale_pick_without_the_source_falls_through():
    # a 'footprint' pick with no polygon on disk (withdrawn ship) must not
    # produce a polygon winner out of thin air
    d = fuse.apply_rules("s", _cand(polygon=False, status="failed_low_obliquity",
                                    mesh=False),
                         _rosters(), "footprint", {})
    assert d.rule != "user_pick"
    assert any("stale pick" in n for n in d.notes)


def test_rule2_wings_take_pipeline_aspect_never_mesh():
    r = _rosters(wing_filled=frozenset({"s"}))
    d = fuse.apply_rules("s", _cand(status="failed_dimensional_check",
                                    aspect=0.73), r, None, {})
    assert d.rule == "wing_family"
    assert d.shape_source == "pipeline_profile"      # filled wing -> profile
    assert d.aspect_source == "pipeline_profile"
    r2 = _rosters(wing_crescent=frozenset({"s"}))
    d2 = fuse.apply_rules("s", _cand(), r2, None, {})
    assert d2.shape_source == "pipeline_polygon"     # crescent -> polygon


def test_rule3_prong_ships_take_the_polygon():
    r = _rosters(prong_confirmed=frozenset({"s"}))
    d = fuse.apply_rules("s", _cand(), r, None, {})
    assert (d.rule, d.shape_source) == ("prong_or_pod", "pipeline_polygon")
    # bow-concave detector route (no roster membership)
    concave = [True] * 8 + [False] * 88
    d2 = fuse.apply_rules("t", _cand(concave=concave), _rosters(), None, {})
    assert d2.rule == "prong_or_pod"


def test_rule4_wrecked_solves_take_corrected_mesh():
    d = fuse.apply_rules("s", _cand(status="failed_dimensional_check",
                                    aspect=0.5), _rosters(), None, {})
    assert (d.rule, d.shape_source, d.aspect_source) == (
        "wrecked_solve", "mesh", "mesh")
    assert not d.squared


def test_rule4_sibling_mesh067sq_pick_turns_on_squaring():
    r = _rosters(family_pairs=(("s", "sib", 0.5, 2.0),))
    d = fuse.apply_rules("s", _cand(status="failed_dimensional_check"),
                         r, None, {"sib": "mesh067sq"})
    assert d.rule == "wrecked_solve" and d.squared


def test_rule5_clean_pipeline_and_receding_lower_bound_note():
    d = fuse.apply_rules("s", _cand(), _rosters(), None, {})
    assert (d.rule, d.shape_source) == ("clean_pipeline", "pipeline_profile")
    r = _rosters(receding=frozenset({"s"}))
    d2 = fuse.apply_rules("s", _cand(), r, None, {})
    assert any("lower bound" in n for n in d2.notes)


def test_rule6_no_candidates_is_unresolved():
    d = fuse.apply_rules("s", _cand(status="failed_symmetry_solve",
                                    polygon=False, mesh=False, aspect=None),
                         _rosters(), None, {})
    assert (d.rule, d.confidence) == ("unresolved", "unresolved")
    assert d.shape_source is None


def test_rule4_without_mesh_falls_through_to_unresolved():
    d = fuse.apply_rules("s", _cand(status="failed_dimensional_check",
                                    mesh=False, polygon=False),
                         _rosters(), None, {})
    assert d.rule == "unresolved"
    assert any("no mesh" in n for n in d.notes)
```

- [ ] **Step 2: Run to verify failure**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q`
Expected: FAILs — no attribute `Decision` / `apply_rules`.

- [ ] **Step 3: Implement**

```python
@dataclasses.dataclass
class Decision:
    rule: str
    confidence: str
    shape_source: "str | None"
    aspect_source: "str | None"
    squared: bool = False
    notes: list = dataclasses.field(default_factory=list)


def _sibling_squared(sid, picks, family_pairs):
    for a, b, _, _ in family_pairs:
        other = b if sid == a else a if sid == b else None
        if other and picks.get(other) == "mesh067sq":
            return True
    return False


def _has(cand, source):
    return {"pipeline_profile": cand.pipe is not None and cand.pipe.get("w"),
            "pipeline_polygon": cand.polygon is not None,
            "mesh": cand.mesh_w is not None,
            "mesh_squared": cand.mesh_w is not None}[source]


def _bow_concave(cand):
    pipe = cand.pipe or {}
    if pipe.get("orientation") != "bow_t0" or not pipe.get("concave"):
        return False
    c = np.array(pipe["concave"], dtype=bool)
    front, total = int(c[:32].sum()), int(c.sum())
    return front >= 4 and total > 0 and front >= 0.6 * total


def _pod_blob(cand):
    pipe = cand.pipe or {}
    return pipe.get("concave") and sum(pipe["concave"]) >= 24


def apply_rules(sid, cand, rosters, pick, picks):
    notes = []
    ok = (cand.pipe or {}).get("status", "").startswith("ok")

    if pick in PICK_SOURCES:
        shape, squared = PICK_SOURCES[pick]
        if _has(cand, shape):
            aspect = "pipeline_profile" if shape == "pipeline_polygon" else \
                     ("mesh" if shape.startswith("mesh") else shape)
            return Decision("user_pick", "user_pick", shape, aspect, squared)
        notes.append(f"stale pick {pick!r}: source unavailable, fell through")

    if sid in rosters.wing_filled or sid in rosters.wing_crescent:
        shape = ("pipeline_polygon" if sid in rosters.wing_crescent
                 else "pipeline_profile")
        if not _has(cand, shape):        # crescent without a polygon on disk
            shape = "pipeline_profile"
            notes.append("wing: polygon missing, used profile")
        if _has(cand, shape):
            return Decision("wing_family", "rules", shape,
                            "pipeline_profile", notes=notes)
        notes.append("wing: no pipeline shape at all")

    if (sid in rosters.prong_confirmed or _bow_concave(cand)
            or _pod_blob(cand)) and _has(cand, "pipeline_polygon"):
        return Decision("prong_or_pod", "rules", "pipeline_polygon",
                        "pipeline_profile", notes=notes)

    wrecked = (cand.pipe or {}).get("status") == "failed_dimensional_check" \
        or (not ok and sid in rosters.receding)
    if wrecked:
        if cand.mesh_w is not None:
            squared = _sibling_squared(sid, picks, rosters.family_pairs)
            return Decision("wrecked_solve", "rules", "mesh", "mesh",
                            squared, notes)
        notes.append("wrecked solve but no mesh available")

    iou = ((cand.pipe or {}).get("quality") or {}).get("silhouette_iou", 0)
    if ok and iou >= 0.97 and _has(cand, "pipeline_profile"):
        if sid in rosters.receding:
            notes.append("receding pose: pipeline aspect is a lower bound")
        return Decision("clean_pipeline", "rules", "pipeline_profile",
                        "pipeline_profile", notes=notes)

    return Decision("unresolved", "unresolved", None, None, notes=notes)
```

- [ ] **Step 4: Run to verify pass**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q`
Expected: all pass. If `test_rule4_without_mesh_falls_through_to_unresolved`
fails on the note text, the implementation's note must contain the exact
substring `no mesh`.

- [ ] **Step 5: Commit**

```bash
git add tools/footprint/fuse.py tools/footprint/test_fuse.py
git commit -m "feat(footprint): fusion rule ladder"
```

---

### Task 5: entry building (geometry, orientation, provenance)

**Files:**
- Modify: `tools/footprint/fuse.py`
- Test: `tools/footprint/test_fuse.py`

**Interfaces:**
- Produces:
  - `fuse.canonical_polygon(polygon_geojson: dict, stored_w: list | None)
    -> list` — rings (list of [x, y] lists) PCA-rotated so the long axis is
    x, translated/scaled so hull length spans x ∈ [0, 1]; when `stored_w` is
    given, x is flipped if the reversed envelope correlates better with
    `stored_w`, so the polygon frame inherits the profile's t direction
    (bow at x=0 iff profile is `bow_t0`).
  - `fuse.mesh_orientation(mesh_w, pipe_rec) -> tuple[np.ndarray, str]` —
    returns (possibly reversed mesh_w, orientation string). Correlates both
    directions of `mesh_w` against `pipe_rec["w"]` when the pipeline profile
    is `bow_t0`; adopts a direction only if best r ≥ 0.5 AND margin ≥ 0.1
    (then `"bow_t0"`, method recorded by the caller), else returns the input
    unchanged with `"unknown"`.
  - `fuse.build_entry(sid, cand, dec: Decision, rosters) -> dict` — the
    schema-1 fused entry (see spec "Entry schema"). Geometry keys:
    profile winners get `w` (list, 96) + `concave`; polygon winners get
    `polygon` (rings from `canonical_polygon`) + `w`/`concave` copied from
    the pipeline profile with `envelope_lossy: true`. `aspect` resolves from
    `aspect_source` (`mesh` → `mesh_adjusted_aspect`). `provenance` includes
    `candidate_aspects` (`pipeline`, `mesh_raw`, `mesh_adjusted`),
    `quality`, `rosters` (list of roster names containing the ship), `pick`,
    and `notes` from the decision.
- Consumes: Tasks 1–4 (`Rosters`, `Candidate`, `Decision`, `mesh_shape`,
  `mesh_adjusted_aspect`). Uses `shapely.geometry.shape` for ring extraction.

- [ ] **Step 1: Write the failing tests**

```python
def test_canonical_polygon_normalises_and_matches_profile_direction():
    gj = {"type": "Polygon", "coordinates":
          [[[0, -1], [10, -1], [10, 1], [0, 1], [0, -1]]]}
    # stored profile is wide at t=0, narrow at t=1 -> a wedge polygon must flip
    wedge = {"type": "Polygon", "coordinates":
             [[[0, 0], [10, -3], [10, 3], [0, 0]]]}
    stored = list(np.linspace(0.5, 0.05, 96))     # wide nose, narrow tail
    rings = fuse.canonical_polygon(wedge, stored)
    xs = np.array(rings[0])[:, 0]
    assert 0.0 <= xs.min() < 0.01 and 0.99 < xs.max() <= 1.0
    ring = np.array(rings[0])
    # widest section must sit near x=0 to match the stored profile
    wide_x = ring[np.argmax(np.abs(ring[:, 1])), 0]
    assert wide_x < 0.3
    # plain rectangle: no crash without a stored profile
    assert fuse.canonical_polygon(gj, None)


def test_mesh_orientation_correlates_against_an_oriented_pipeline_profile():
    t = np.linspace(0, 1, 96)
    pipe_w = 0.5 - 0.4 * t                        # wide bow, narrow stern
    rec = {"orientation": "bow_t0", "w": list(pipe_w)}
    mesh = pipe_w[::-1] + np.random.default_rng(0).normal(0, 0.01, 96)
    out, orient = fuse.mesh_orientation(mesh, rec)
    assert orient == "bow_t0"
    assert np.corrcoef(out, pipe_w)[0, 1] > 0.9   # got reversed to match
    out2, orient2 = fuse.mesh_orientation(np.full(96, 0.3), rec)
    assert orient2 == "unknown"                   # flat: no margin


def test_build_entry_profile_winner():
    c = _cand()
    d = fuse.Decision("clean_pipeline", "rules", "pipeline_profile",
                      "pipeline_profile")
    e = fuse.build_entry("s", c, d, _rosters())
    assert e["schema"] == 1 and e["rule"] == "clean_pipeline"
    assert e["shape_source"] == "pipeline_profile"
    assert len(e["w"]) == 96 and "polygon" not in e
    assert e["aspect"] == 2.0
    assert e["provenance"]["candidate_aspects"]["mesh_adjusted"] == 1.7 / 0.67


def test_build_entry_polygon_winner_carries_lossy_envelope():
    c = _cand()
    d = fuse.Decision("prong_or_pod", "rules", "pipeline_polygon",
                      "pipeline_profile")
    e = fuse.build_entry("s", c, d, _rosters())
    assert e["polygon"] and e["envelope_lossy"] is True
    assert len(e["w"]) == 96


def test_build_entry_mesh_winner_scales_shape_and_aspect():
    c = _cand(status="failed_dimensional_check")
    d = fuse.Decision("wrecked_solve", "rules", "mesh", "mesh")
    e = fuse.build_entry("s", c, d, _rosters())
    assert np.isclose(e["w"][0], 0.3 * 0.67)
    assert np.isclose(e["aspect"], 1.7 / 0.67)
    assert e["concave"] == [False] * 96           # mesh has no concave flags


def test_build_entry_unresolved_lists_candidates_only():
    c = _cand(status="failed_symmetry_solve", polygon=False, mesh=False)
    d = fuse.Decision("unresolved", "unresolved", None, None)
    e = fuse.build_entry("s", c, d, _rosters())
    assert e["shape_source"] is None and "w" not in e
    assert "candidate_aspects" in e["provenance"]
```

- [ ] **Step 2: Run to verify failure**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q`
Expected: FAILs — no attribute `canonical_polygon`.

- [ ] **Step 3: Implement**

```python
import shapely.geometry as _sg


def _envelope(rings, stations=96):
    """Max |y| per x-station over all ring segments — cheap w(t) of a
    canonical polygon, for direction matching only."""
    pts = np.concatenate([np.asarray(r) for r in rings])
    env = np.zeros(stations)
    idx = np.clip((pts[:, 0] * (stations - 1)).round().astype(int),
                  0, stations - 1)
    for i, y in zip(idx, np.abs(pts[:, 1])):
        env[i] = max(env[i], y)
    return env


def canonical_polygon(polygon_geojson, stored_w):
    poly = _sg.shape(polygon_geojson)
    rings = [np.asarray(poly.exterior.coords)] + \
            [np.asarray(i.coords) for i in poly.interiors] \
        if poly.geom_type == "Polygon" else \
        [np.asarray(r.coords) for g in poly.geoms
         for r in [g.exterior, *g.interiors]]
    pts = np.concatenate(rings)
    ctr = pts.mean(axis=0)
    _, _, vt = np.linalg.svd(pts - ctr, full_matrices=False)
    R = np.array([vt[0], vt[1]])                 # major axis -> x
    rings = [(r - ctr) @ R.T for r in rings]
    xs = np.concatenate(rings)[:, 0]
    lo, span = xs.min(), xs.max() - xs.min()
    rings = [np.column_stack(((r[:, 0] - lo) / span, r[:, 1] / span))
             for r in rings]
    if stored_w is not None:
        env = _envelope(rings)
        w = np.asarray(stored_w, dtype=float)
        if np.corrcoef(env[::-1], w)[0, 1] > np.corrcoef(env, w)[0, 1]:
            rings = [np.column_stack((1.0 - r[:, 0], r[:, 1]))
                     for r in rings]
    return [r.tolist() for r in rings]


def mesh_orientation(mesh_w, pipe_rec):
    w = np.asarray(mesh_w, dtype=float)
    if not pipe_rec or pipe_rec.get("orientation") != "bow_t0":
        return w, "unknown"
    pw = np.asarray(pipe_rec["w"], dtype=float)
    if np.std(w) < 1e-9 or np.std(pw) < 1e-9:
        return w, "unknown"
    fwd = float(np.corrcoef(w, pw)[0, 1])
    rev = float(np.corrcoef(w[::-1], pw)[0, 1])
    best, other = max(fwd, rev), min(fwd, rev)
    if best >= 0.5 and best - other >= 0.1:
        return (w if fwd >= rev else w[::-1]), "bow_t0"
    return w, "unknown"


def _roster_memberships(sid, rosters):
    return [name for name, s in [
        ("wing_filled", rosters.wing_filled),
        ("wing_crescent", rosters.wing_crescent),
        ("prong_confirmed", rosters.prong_confirmed),
        ("receding", rosters.receding)] if sid in s]


def build_entry(sid, cand, dec, rosters, pick=None):
    pipe = cand.pipe or {}
    cand_aspects = {
        "pipeline": pipe.get("aspect"),
        "mesh_raw": cand.mesh_aspect,
        "mesh_adjusted": (mesh_adjusted_aspect(cand.mesh_aspect)
                          if cand.mesh_aspect else None),
    }
    entry = {
        "schema": SCHEMA, "id": sid, "rule": dec.rule,
        "confidence": dec.confidence,
        "shape_source": dec.shape_source, "aspect_source": dec.aspect_source,
        "provenance": {
            "candidate_aspects": cand_aspects,
            "quality": pipe.get("quality"),
            "rosters": _roster_memberships(sid, rosters),
            "pick": pick,
            "notes": dec.notes,
        },
    }
    if dec.aspect_source == "mesh":
        entry["aspect"] = cand_aspects["mesh_adjusted"]
    elif dec.aspect_source == "pipeline_profile":
        entry["aspect"] = cand_aspects["pipeline"]
    else:
        entry["aspect"] = None

    if dec.shape_source == "pipeline_profile":
        entry["w"] = list(pipe["w"])
        entry["concave"] = list(pipe["concave"])
        entry["orientation"] = pipe.get("orientation", "unknown")
    elif dec.shape_source == "pipeline_polygon":
        entry["polygon"] = canonical_polygon(cand.polygon["polygon"],
                                             pipe.get("w"))
        entry["w"] = list(pipe["w"])
        entry["concave"] = list(pipe["concave"])
        entry["envelope_lossy"] = True
        entry["orientation"] = pipe.get("orientation", "unknown")
    elif dec.shape_source in ("mesh", "mesh_squared"):
        oriented, orient = mesh_orientation(cand.mesh_w, pipe)
        entry["w"] = [float(v) for v in mesh_shape(oriented, dec.squared)]
        entry["concave"] = [False] * len(entry["w"])
        entry["orientation"] = orient
        if orient == "bow_t0":
            entry["provenance"]["orientation_method"] = \
                "correlated_to_pipeline"
    return entry
```

- [ ] **Step 4: Run to verify pass**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add tools/footprint/fuse.py tools/footprint/test_fuse.py
git commit -m "feat(footprint): fused entry builder — geometry, orientation, provenance"
```

---

### Task 6: validation reports

**Files:**
- Modify: `tools/footprint/fuse.py`
- Test: `tools/footprint/test_fuse.py`

**Interfaces:**
- Produces: `fuse.validate(entries: dict[str, dict], cands, rosters, picks)
  -> dict` with keys:
  - `rules_vs_picks`: list of `{"id", "rule_answer", "pick_answer"}` — the
    ladder minus rule 1 (`apply_rules` with `pick=None`) run over picked
    ships whose rule-shape differs from the pick's mapped shape_source.
  - `family_consistency`: list of `{"pair", "ratio", "expected", "ok"}` —
    fused aspect ratios vs `family_pairs` bounds; pairs with a missing/None
    aspect get `"ratio": null, "ok": false`.
- Consumes: Tasks 1–5.

- [ ] **Step 1: Write the failing tests**

```python
def test_validate_reports_rule_vs_pick_disagreements():
    cands = {"s": _cand()}
    rosters = _rosters()
    picks = {"s": "footprint"}      # user says polygon; rules say profile
    entries = {"s": fuse.build_entry(
        "s", cands["s"],
        fuse.apply_rules("s", cands["s"], rosters, "footprint", picks),
        rosters, pick="footprint")}
    v = fuse.validate(entries, cands, rosters, picks)
    assert v["rules_vs_picks"] == [{
        "id": "s", "rule_answer": "pipeline_profile",
        "pick_answer": "pipeline_polygon"}]


def test_validate_checks_family_aspect_ratios():
    rosters = _rosters(family_pairs=(("a", "b", 1.5, 3.0),))
    ea = {"id": "a", "aspect": 2.0}
    eb = {"id": "b", "aspect": 1.0}
    v = fuse.validate({"a": ea, "b": eb}, {}, rosters, {})
    fc = v["family_consistency"][0]
    assert fc["ok"] and np.isclose(fc["ratio"], 2.0)
    v2 = fuse.validate({"a": {"id": "a", "aspect": 10.0}, "b": eb},
                       {}, rosters, {})
    assert not v2["family_consistency"][0]["ok"]
```

- [ ] **Step 2: Run to verify failure**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q`
Expected: FAILs — no attribute `validate`.

- [ ] **Step 3: Implement**

```python
def validate(entries, cands, rosters, picks):
    disagreements = []
    for sid, pick in sorted(picks.items()):
        if sid not in cands or pick not in PICK_SOURCES:
            continue
        rule_dec = apply_rules(sid, cands[sid], rosters, None, picks)
        pick_shape = PICK_SOURCES[pick][0]
        if rule_dec.shape_source != pick_shape:
            disagreements.append({"id": sid,
                                  "rule_answer": rule_dec.shape_source,
                                  "pick_answer": pick_shape})
    family = []
    for a, b, lo, hi in rosters.family_pairs:
        ea, eb = entries.get(a), entries.get(b)
        ra = ea.get("aspect") if ea else None
        rb = eb.get("aspect") if eb else None
        if ra and rb:
            ratio = ra / rb
            family.append({"pair": [a, b], "ratio": ratio,
                           "expected": [lo, hi], "ok": lo <= ratio <= hi})
        else:
            family.append({"pair": [a, b], "ratio": None,
                           "expected": [lo, hi], "ok": False})
    return {"rules_vs_picks": disagreements, "family_consistency": family}
```

- [ ] **Step 4: Run to verify pass**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add tools/footprint/fuse.py tools/footprint/test_fuse.py
git commit -m "feat(footprint): fusion validation — rules-vs-picks and family consistency"
```

---

### Task 7: runner, index, real-data integration

**Files:**
- Modify: `tools/footprint/fuse.py`
- Test: `tools/footprint/test_fuse.py`

**Interfaces:**
- Produces: `fuse.run(foot=FOOT, bakeoff=BAKEOFF, out=None) -> dict` — writes
  `out or foot/"fused"` (`<ship>.json` per ship + `index.json`), returns the
  index dict. Index: `{"schema": 1, "counts": {rule: n}, "ships": {sid:
  {"rule", "confidence", "shape_source", "aspect_source", "aspect"}},
  "validation": <validate() output>}`. Runnable:
  `~/moge-venv/bin/python -m tools.footprint.fuse` from the repo root.
- Consumes: everything above; annotations from
  `FOOT/"user_annotations_2026-08-01.json"`, labels from
  `FOOT/"eyeball_labels_2026-08-01.json"` (paths derived from the `foot`
  argument so the synthetic test can inject its own).

- [ ] **Step 1: Write the failing tests**

```python
def test_run_writes_entries_and_index(tmp_path):
    foot, bakeoff = _mini_tree(tmp_path)
    (foot / "user_annotations_2026-08-01.json").write_text(json.dumps(
        {"best_picks": {"plain": "moge"},
         "bow_directions_deg_cw_from_screen_up": {}}))
    (foot / "eyeball_labels_2026-08-01.json").write_text(json.dumps(
        {"fusion_rosters": {
            "wing_filled": [], "wing_crescent": [], "prong_confirmed": [],
            "receding_right_2_3": [], "receding_left_9_10": [],
            "family_pairs": []}}))
    index = fuse.run(foot=foot, bakeoff=bakeoff)
    fused = foot / "fused"
    assert (fused / "plain.json").exists() and (fused / "index.json").exists()
    plain = json.loads((fused / "plain.json").read_text())
    assert plain["rule"] == "user_pick" and plain["shape_source"] == \
        "pipeline_profile"
    bare = json.loads((fused / "bare.json").read_text())
    assert bare["rule"] == "unresolved"        # wrecked but no mesh
    assert index["counts"]["user_pick"] == 1
    assert index["ships"]["bare"]["confidence"] == "unresolved"


def test_run_on_the_real_data_holds_the_headline_invariants():
    """Integration: real repo data. Skipped if the mesh bakeoff is absent."""
    import pytest
    if not fuse.BAKEOFF.exists():
        pytest.skip("mesh bakeoff data not present")
    import tempfile
    with tempfile.TemporaryDirectory() as td:
        index = fuse.run(out=pathlib.Path(td))
        assert len(index["ships"]) == 267
        picks = json.loads((fuse.FOOT /
                            "user_annotations_2026-08-01.json").read_text()
                           )["best_picks"]
        for sid, entry in index["ships"].items():
            if sid in picks and entry["confidence"] == "unresolved":
                raise AssertionError(
                    f"{sid}: has a user pick but came out unresolved")
        # every pick either resolved as rule 1 or documented its staleness
        n_pick_rule = sum(1 for s in index["ships"].values()
                          if s["rule"] == "user_pick")
        assert n_pick_rule >= len([s for s in picks if s in index["ships"]]) - 5
```

- [ ] **Step 2: Run to verify failure**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/test_fuse.py -q`
Expected: 2 new FAILs — no attribute `run`.

- [ ] **Step 3: Implement**

```python
def run(foot: pathlib.Path = FOOT, bakeoff: pathlib.Path = BAKEOFF,
        out: "pathlib.Path | None" = None) -> dict:
    out = out or (foot / "fused")
    out.mkdir(parents=True, exist_ok=True)
    labels = json.loads(
        (foot / "eyeball_labels_2026-08-01.json").read_text())
    ann = json.loads(
        (foot / "user_annotations_2026-08-01.json").read_text())
    picks = ann["best_picks"]
    rosters = load_rosters(labels)
    cands = load_candidates(foot, bakeoff)

    entries = {}
    for sid, cand in sorted(cands.items()):
        dec = apply_rules(sid, cand, rosters, picks.get(sid), picks)
        entries[sid] = build_entry(sid, cand, dec, rosters,
                                   pick=picks.get(sid))
        (out / f"{sid}.json").write_text(json.dumps(entries[sid], indent=1))

    counts = {}
    for e in entries.values():
        counts[e["rule"]] = counts.get(e["rule"], 0) + 1
    index = {
        "schema": SCHEMA,
        "counts": counts,
        "ships": {sid: {k: e.get(k) for k in
                        ("rule", "confidence", "shape_source",
                         "aspect_source", "aspect")}
                  for sid, e in entries.items()},
        "validation": validate(entries, cands, rosters, picks),
    }
    (out / "index.json").write_text(json.dumps(index, indent=1))
    return index


def main():
    index = run()
    print(json.dumps(index["counts"], indent=1))
    v = index["validation"]
    print(f"rules-vs-picks disagreements: {len(v['rules_vs_picks'])}")
    bad = [f for f in v["family_consistency"] if not f["ok"]]
    print(f"family checks failing: {len(bad)}/{len(v['family_consistency'])}")


if __name__ == "__main__":
    main()
```

- [ ] **Step 4: Run the full footprint suite**

Run: `~/moge-venv/bin/python -m pytest tools/footprint/ -q`
Expected: all pass (the pre-existing 109 + the new fuse tests). The
integration test may take ~30s (267 polygon canonicalisations).

- [ ] **Step 5: Commit**

```bash
git add tools/footprint/fuse.py tools/footprint/test_fuse.py
git commit -m "feat(footprint): fusion runner + index with built-in validation"
```

---

### Task 8: generate, review, commit the fused dataset

**Files:**
- Create (generated): `data/footprints/fused/*.json` (268 files)
- Commit: `data/mesh_bakeoff/` (fusion's mesh input, currently untracked,
  ~31MB)

- [ ] **Step 1: Track the mesh bakeoff data**

The fused dataset is only reproducible if its mesh input is in the repo:

```bash
git add data/mesh_bakeoff
git commit -m "data(mesh): commit the TripoSR bakeoff profiles consumed by fusion"
```

- [ ] **Step 2: Run fusion for real**

```bash
~/moge-venv/bin/python -m tools.footprint.fuse
```

Expected output: rule counts (user_pick should be ~170; unresolved should be
well under 95 since rules 2–5 absorb many unpicked ships), disagreement and
family-check counts.

- [ ] **Step 3: Review the index before committing**

Read `data/footprints/fused/index.json`. Check:
- every rule-1 count matches the live pick count minus stale picks;
- `rules_vs_picks` disagreements: skim the list — each should be explainable
  (user preferred mesh on a clean-pipeline ship, etc.). Surface the list to
  the user; interesting disagreements may prompt new label notes.
- `family_consistency`: aether/close_enough must be `ok`; investigate any
  failure before committing.

- [ ] **Step 4: Spot-check three known ships**

```bash
~/moge-venv/bin/python - <<'EOF'
import json
for sid, expect in [("close_enough", "pipeline shape"),
                    ("apeiron", "wing: pipeline aspect ~0.73"),
                    ("syndicate", "mesh shape, aspect ~5.6")]:
    e = json.load(open(f"data/footprints/fused/{sid}.json"))
    print(sid, e["rule"], e["shape_source"], e["aspect_source"],
          round(e["aspect"], 2) if e["aspect"] else None, "|", expect)
EOF
```

Expected: close_enough → its user pick if present else clean/wrecked per
ladder; apeiron → `wing_family`, aspect ≈ 0.73 (pipeline); syndicate →
`wrecked_solve` (or user pick), aspect ≈ 5.63 (mesh-adjusted).

- [ ] **Step 5: Commit the dataset**

```bash
git add data/footprints/fused
git commit -m "data(footprints): first fused dataset — 267 ships, rule-ladder selection"
```

---

## Self-Review Notes

- Spec coverage: rosters (T1), inputs (T2), pick mapping + corrections (T3),
  ladder rules 1–6 incl. fall-throughs and scale decoupling (T4), schema +
  polygon canonicalisation + mesh orientation-by-correlation (T5, per the
  amended spec), validation (T6), runner/index/idempotence (T7), generation
  + mesh-data tracking + human review (T8). Out-of-scope items untouched.
- The spec's `fused/<ship>.json` + `index.json` naming and all schema keys
  match between Tasks 5 and 7.
- Type consistency: `Decision.shape_source` values are exactly the
  `PICK_SOURCES` shape strings plus `None`; `build_entry` branches on the
  same strings; `validate` compares them.
