#!/usr/bin/env python3
"""python3 -m unittest scripts/test_publish_planet_textures.py -v"""
import sys
import tempfile
import unittest
from pathlib import Path

from PIL import Image

sys.path.insert(0, str(Path(__file__).resolve().parent))
import publish_planet_textures as ppt  # noqa: E402


def png(path, colour):
    path.parent.mkdir(parents=True, exist_ok=True)
    Image.new("RGB", (40, 20), colour).save(path)


def colour_of(path):
    return Image.open(path).convert("RGB").getpixel((5, 5))


class PublishTest(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())
        self.src, self.out, self.over = self.tmp / "render", self.tmp / "out", self.tmp / "overlays"
        png(self.src / "alpha_alpha_i.png", (200, 0, 0))
        png(self.src / "sol_earth.png", (0, 200, 0))          # procedural render, overlay must win
        png(self.over / "sol_earth.png", (0, 0, 200))
        png(self.over / "sol_mars.png", (200, 200, 0))        # overlay-only planet still published
        png(self.out / "alpha_alpha_i.png", (9, 9, 9))        # stale PNG output gets retired
        self.report = ppt.publish(self.src, self.out, self.over, quality=90)

    def test_renders_become_webp_and_stale_pngs_are_removed(self):
        self.assertEqual(sorted(p.name for p in self.out.iterdir()),
                         ["alpha_alpha_i.webp", "sol_earth.webp", "sol_mars.webp"])
        r, g, b = colour_of(self.out / "alpha_alpha_i.webp")
        self.assertGreater(r, 180)

    def test_overlay_wins_over_the_render(self):
        r, g, b = colour_of(self.out / "sol_earth.webp")
        self.assertGreater(b, 180)
        self.assertLess(g, 30)

    def test_report_counts(self):
        self.assertEqual(self.report, {"rendered": 1, "overlaid": 2, "removed_png": 1})


if __name__ == "__main__":
    unittest.main()
