#!/usr/bin/env python3
"""cd data/battles/replay && python3 -m unittest test_make_barrage -v"""
import json
import unittest

import export_to_api as e2a
import make_barrage as mb


class BarrageTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.battle = mb.build(json.loads(mb.SOURCE.read_text()), 30, 7)
        cls.side = {p["player_id"]: p["side_id"] for p in cls.battle["participants"]}

    def test_every_shot_crosses_sides_with_the_shooters_theme(self):
        themed = {side: {w[0] for w in guns} for side, guns in mb.THEMES.items()}
        shots = [s for f in self.battle["frames"] for s in f["shots"]]
        self.assertGreater(len(shots), 1000)
        for s in shots:
            self.assertNotEqual(self.side[s["from_id"]], self.side[s["to_id"]])
            self.assertIn(s["weapon_name"], themed[self.side[s["from_id"]]])

    def test_kills_record_destruction_and_victims_leave(self):
        frames = self.battle["frames"]
        kills = [(i, k) for i, f in enumerate(frames) for k in f["kills"]]
        self.assertTrue(kills)
        for i, kill in kills:
            victim = next(p for p in self.battle["participants"] if p["player_id"] == kill["victim_id"])
            self.assertEqual(victim["destroyed_at_tick"], frames[i]["tick"])
            for later in frames[i + 1:]:
                self.assertNotIn(kill["victim_id"], {s["player_id"] for s in later["ships"]})

    def test_converts_to_a_finished_api_battle(self):
        summary, entries = e2a.convert(self.battle, ended_at="2026-09-01T00:00:00Z")
        self.assertEqual(summary["battle_id"], mb.BARRAGE_ID)
        self.assertIn("battle_ended", entries[-1])
        self.assertEqual(summary["ships_destroyed"], sum(len(f["kills"]) for f in self.battle["frames"]))


if __name__ == "__main__":
    unittest.main()
