#!/usr/bin/env python3
"""cd data/mesh_bakeoff && python3 -m unittest test_make_hangar_lod -v"""
import unittest

import make_hangar_lod as mhl

BASE = {"ships": [
    {"id": "big", "name": "Big One", "lengthM": 142.0, "model": "models/big.glb", "page": "Combat/big.html"},
    {"id": "small", "name": "Small One", "lengthM": 18.0, "model": "models/small.glb", "page": None},
]}


class LodLineupTest(unittest.TestCase):
    def setUp(self):
        self.rows = mhl.build_lod_lineup(BASE, ["small", "big"], [40000, 16000, 6000, 2000])["ships"]

    def test_each_ship_gets_every_level_in_detail_order_grouped_by_ship(self):
        self.assertEqual([r["id"] for r in self.rows], [
            "small__40k", "small__16k", "small__6k", "small__2k",
            "big__40k", "big__16k", "big__6k", "big__2k"])

    def test_names_models_and_sizes(self):
        by = {r["id"]: r for r in self.rows}
        self.assertEqual(by["big__8k" if "big__8k" in by else "big__16k"]["name"], "Big One · 16k faces")
        self.assertEqual(by["big__40k"]["model"], "lod/big__40k.glb")
        self.assertEqual(by["big__6k"]["model"], "models/big.glb")        # the hangar's own model
        self.assertEqual(by["big__2k"]["model"], "lod/big__2k.glb")
        self.assertEqual(by["small__2k"]["lengthM"], 18.0)
        self.assertEqual(by["big__16k"]["page"], "Combat/big.html")

    def test_unknown_ship_is_an_error(self):
        with self.assertRaises(KeyError):
            mhl.build_lod_lineup(BASE, ["nope"], [2000])


class MasterLineupTest(unittest.TestCase):
    def setUp(self):
        self.rows = mhl.build_lod_lineup(BASE, ["big"], [40000, 6000], master=500000)["ships"]

    def test_master_leads_then_each_level_pairs_orig_with_master(self):
        self.assertEqual([r["id"] for r in self.rows], [
            "big__m500k", "big__40k", "big__m40k", "big__6k", "big__m6k"])

    def test_master_names_and_models(self):
        by = {r["id"]: r for r in self.rows}
        self.assertEqual(by["big__m500k"]["name"], "Big One · 500k master")
        self.assertEqual(by["big__m500k"]["model"], "lod/big__m500k.glb")
        self.assertEqual(by["big__40k"]["name"], "Big One · 40k orig")
        self.assertEqual(by["big__m40k"]["name"], "Big One · 40k from master")
        self.assertEqual(by["big__m6k"]["model"], "lod/big__m6k.glb")
        self.assertEqual(by["big__6k"]["model"], "models/big.glb")


if __name__ == "__main__":
    unittest.main()
