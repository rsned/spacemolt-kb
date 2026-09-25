#!/usr/bin/env python3
"""Round-trip the one battle we hold both ways (raw API log + our export).

    cd data/battles/replay && python3 -m unittest test_export_to_api -v
"""
import json
import unittest
from pathlib import Path

import export_to_api as e2a

HERE = Path(__file__).resolve().parent
BATTLE = "509e1ef4a76fc90d7ce4e33c85336a68"


def load():
    raw = json.loads((HERE.parent / f"{BATTLE}.raw.json").read_text())[0]["entries"]
    export = json.loads((HERE.parent / f"{BATTLE}.json").read_text())
    return raw, export


class RoundTripTest(unittest.TestCase):
    def setUp(self):
        self.raw, export = load()
        self.summary, self.entries = e2a.convert(export)

    def test_ticks_and_snapshots_match(self):
        self.assertEqual([e["tick"] for e in self.entries], [e["tick"] for e in self.raw])
        for mine, real in zip(self.entries, self.raw):
            key = lambda s: (s["player_id"], s["ship_class"], s["side_id"], s["hull"], s["shield"], s["max_hull"], s["zone"], s["stance"])
            self.assertEqual(sorted(map(key, mine["snapshots"])), sorted(map(key, real["snapshots"])))

    def test_attacks_regroup_per_attacker_target(self):
        for mine, real in zip(self.entries, self.raw):
            key = lambda a: (a["attacker_id"], a["target_id"], a["hit_success"], a["shield_damage"], a["hull_damage"],
                             a["zone_distance"], tuple(w["name"] for w in a["weapons"]), tuple(w["damage_type"] for w in a["weapons"]))
            self.assertEqual(list(map(key, mine["attacks"] or [])), list(map(key, real["attacks"] or [])))

    def test_moves_autopilot_kills(self):
        for mine, real in zip(self.entries, self.raw):
            self.assertEqual(mine["zone_moves"], real["zone_moves"])
            self.assertEqual(mine["autopilot"], real["autopilot"])
            self.assertEqual(mine["kills"], real["kills"])

    def test_weapon_instance_ids_stable_and_distinct(self):
        ids = [w["instance_id"] for a in self.entries[1]["attacks"] for w in a["weapons"]]
        again = [w["instance_id"] for a in e2a.convert(load()[1])[1][1]["attacks"] for w in a["weapons"]]
        self.assertEqual(ids, again)
        first = self.entries[1]["attacks"][0]["weapons"]
        self.assertEqual(len({w["instance_id"] for w in first}), len(first))

    def test_battle_ended_only_on_last_entry(self):
        self.assertTrue(all("battle_ended" not in e for e in self.entries[:-1]))
        mine, real = self.entries[-1]["battle_ended"], self.raw[-1]["battle_ended"]
        for key in ("outcome", "winning_side", "duration", "total_damage", "ships_destroyed", "category"):
            self.assertEqual(mine[key], real[key], key)
        self.assertEqual(sorted(p["player_id"] for p in mine["participants"]), sorted(p["player_id"] for p in real["participants"]))

    def test_summary(self):
        s = self.summary
        self.assertEqual((s["battle_id"], s["system_id"], s["status"], s["category"]), (BATTLE, "tau_bootis", "completed", "pvp"))
        self.assertEqual(s["ships_destroyed"], 1)
        self.assertEqual(s["destroyed_names"], ["MoltenOne"])
        self.assertEqual(sorted(side["side_id"] for side in s["sides"]), [1, 2])

    def test_ended_at_passes_through_so_the_loader_settles(self):
        # The cinematic only treats a finished battle as complete at once when
        # summary.ended_at is in the past; the export itself doesn't carry it.
        summary, _ = e2a.convert(load()[1], ended_at="2026-09-01T00:00:00Z")
        self.assertEqual(summary["ended_at"], "2026-09-01T00:00:00Z")
        self.assertNotIn("ended_at", self.summary)


class StatusTest(unittest.TestCase):
    def test_synthetic_complete_status_still_ends_the_battle(self):
        export = load()[1]
        export["status"] = "complete"   # the synthetic stress test's spelling
        summary, entries = e2a.convert(export)
        self.assertEqual(summary["status"], "completed")
        self.assertIn("battle_ended", entries[-1])


class PagingTest(unittest.TestCase):
    def test_pages_by_tick_cursor(self):
        entries = [{"tick": t} for t in range(10, 15)]
        page = e2a.log_page(entries, "b", 10, 2)
        self.assertEqual(([e["tick"] for e in page["entries"]], page["has_more"], page["total_ticks"]), ([10, 11], True, 5))
        page = e2a.log_page(entries, "b", 14, 2)
        self.assertEqual(([e["tick"] for e in page["entries"]], page["has_more"]), ([14], False))


if __name__ == "__main__":
    unittest.main()
