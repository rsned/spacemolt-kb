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
