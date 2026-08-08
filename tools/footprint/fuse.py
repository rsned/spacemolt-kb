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
import re

import numpy as np

from tools.footprint import profile as _profile

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
