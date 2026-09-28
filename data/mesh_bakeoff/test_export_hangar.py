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
