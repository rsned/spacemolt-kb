#!/usr/bin/env python3
"""cd data/battles/replay && python3 -m unittest test_make_showcase -v"""
import unittest

import export_to_api as e2a
import make_showcase as ms

SHIPS = {"a1": "solarian", "a2": "solarian", "b1": "crimson", "c1": "", "d1": "voidborn", "d2": "voidborn",
         "d3": "voidborn", "e1": "pirate", "f1": "nebula", "g1": "outerrim"}


class ShowcaseTest(unittest.TestCase):
    def setUp(self):
        self.battle = ms.build(SHIPS, per_tick=3)

    def test_one_ship_per_class_sides_by_empire_unaffiliated_independent(self):
        parts = self.battle["participants"]
        self.assertEqual(sorted(p["ship_class"] for p in parts), sorted(SHIPS))
        side_names = {s["side_id"]: s["faction_tag"] for s in self.battle["sides"]}
        by_class = {p["ship_class"]: side_names[p["side_id"]] for p in parts}
        self.assertEqual(by_class["c1"], "INDEPENDENT")
        self.assertEqual(by_class["d2"], "VOIDBORN")
        self.assertEqual(len(side_names), 7)

    def test_every_ship_fires_exactly_once_harmlessly_at_another_side(self):
        side = {p["player_id"]: p["side_id"] for p in self.battle["participants"]}
        shots = [s for f in self.battle["frames"] for s in f["shots"]]
        self.assertEqual(sorted(s["from_id"] for s in shots), sorted(side))
        for s in shots:
            self.assertNotEqual(side[s["from_id"]], side[s["to_id"]])
            self.assertFalse(s["hit"])
            self.assertEqual(s["damage"], 0)
        self.assertTrue(all(len(f["shots"]) <= 3 for f in self.battle["frames"]))

    def test_everyone_present_every_tick_and_nobody_dies(self):
        for frame in self.battle["frames"]:
            self.assertEqual(len(frame["ships"]), len(SHIPS))
        summary, entries = e2a.convert(self.battle, ended_at="2026-09-01T00:00:00Z")
        self.assertEqual(summary["ships_destroyed"], 0)
        self.assertIn("battle_ended", entries[-1])


if __name__ == "__main__":
    unittest.main()
