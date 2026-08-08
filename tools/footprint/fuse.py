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
