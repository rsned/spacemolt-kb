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

    def test_every_ship_lands_exactly_one_scratch_hit_on_another_side(self):
        # The cinematic only films encounters with a recorded hit, so each shot
        # lands for a single shield point: filmable, but nobody is hurt.
        side = {p["player_id"]: p["side_id"] for p in self.battle["participants"]}
        shots = [s for f in self.battle["frames"] for s in f["shots"]]
        self.assertEqual(sorted(s["from_id"] for s in shots), sorted(side))
        for s in shots:
            self.assertNotEqual(side[s["from_id"]], side[s["to_id"]])
            self.assertTrue(s["hit"])
            self.assertEqual((s["damage"], s["shield_damage"], s.get("hull_damage", 0)), (1, 1, 0))
        self.assertTrue(all(len(f["shots"]) <= 3 for f in self.battle["frames"]))

    def test_everyone_present_every_tick_and_nobody_dies(self):
        for frame in self.battle["frames"]:
            self.assertEqual(len(frame["ships"]), len(SHIPS))
        summary, entries = e2a.convert(self.battle, ended_at="2026-09-01T00:00:00Z")
        self.assertEqual(summary["ships_destroyed"], 0)
        self.assertIn("battle_ended", entries[-1])


class LodReviewTest(unittest.TestCase):
    def test_one_side_per_level_with_the_same_ships_in_each(self):
        battle = ms.build_lod_review(["axiom", "opus_magna"], ["16k", "2k"])
        self.assertEqual(battle["battle_id"], ms.LOD_REVIEW_ID)
        tags = {s["side_id"]: s["faction_tag"] for s in battle["sides"]}
        self.assertEqual(sorted(tags.values()), ["16K", "2K", "FULL 40K"])
        by_side = {}
        for p in battle["participants"]:
            by_side.setdefault(tags[p["side_id"]], []).append(p["ship_class"])
        self.assertEqual(sorted(by_side["FULL 40K"]), ["axiom", "opus_magna"])
        self.assertEqual(sorted(by_side["2K"]), ["axiom__lod2k", "opus_magna__lod2k"])
        self.assertTrue(all(len(f["shots"]) <= 8 for f in battle["frames"]))


if __name__ == "__main__":
    unittest.main()
