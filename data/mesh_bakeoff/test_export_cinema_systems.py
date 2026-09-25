#!/usr/bin/env python3
"""cd data/mesh_bakeoff && python3 -m unittest test_export_cinema_systems -v"""
import unittest

import export_cinema_systems as ecs

PLANETS = [  # (system_id, poi_id, planet_class, radius_km, orbital_distance_au)
    ("sol", "mars", "terran", 7510.0, 1.5),
    ("sol", "earth", "terran", 7042.0, 1.0),
    ("sol", "jupiter", "jovian", 46874.0, 5.11),
    ("gliese_1", "gliese_1_i", "arid", 5000.0, .4),
]
STARS = [  # (system_id, star_class, color_hex, size_multiplier, render_size)
    ("sol", "G2V", "#fffad0", 1.0, 10.0),
    ("node_beta", "K2III", "#ffe58c", 1.6, 16.0),
]
TEXTURES = {"sol_earth.png", "sol_jupiter.png"}


class BuildTest(unittest.TestCase):
    def setUp(self):
        self.out = ecs.build_systems(PLANETS, STARS, TEXTURES)

    def test_planets_in_orbit_order_with_texture_paths_only_when_present(self):
        sol = self.out["systems"]["sol"]["planets"]
        self.assertEqual([p["id"] for p in sol], ["earth", "mars", "jupiter"])
        self.assertEqual(sol[0]["texture"], "planets/sol_earth.png")
        self.assertNotIn("texture", sol[1])
        self.assertEqual((sol[2]["class"], sol[2]["radiusKm"], sol[2]["orbitAu"]), ("jovian", 46874.0, 5.11))

    def test_star_fields_and_systems_with_only_a_star_or_only_planets(self):
        self.assertEqual(self.out["systems"]["sol"]["star"],
                         {"class": "G2V", "color": "#fffad0", "sizeMultiplier": 1.0, "renderSize": 10.0})
        self.assertEqual(self.out["systems"]["node_beta"], {"star": {"class": "K2III", "color": "#ffe58c", "sizeMultiplier": 1.6, "renderSize": 16.0}, "planets": []})
        self.assertIsNone(self.out["systems"]["gliese_1"]["star"])
        self.assertEqual(self.out["version"], 1)


if __name__ == "__main__":
    unittest.main()
