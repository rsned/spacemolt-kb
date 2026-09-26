#!/usr/bin/env python3
"""Runs in the pymeshlab venv:
    cd data/mesh_bakeoff && ~/hy3d-venv/bin/python -m unittest test_lod_variants -v
"""
import json
import tempfile
import unittest
from pathlib import Path

import numpy as np

import cinema_frame as cf
import make_lod_variants as mlv
from test_cinema_export import box, subdivide


class DecimateTest(unittest.TestCase):
    def test_hits_the_target_and_keeps_the_frame(self):
        v, f = subdivide(*box((-.5, -.1, -.15), (.5, .1, .15)), 4)   # 3072 faces
        dv, df = mlv.decimate(v, f, 500)
        self.assertLessEqual(len(df), 500)
        self.assertGreater(len(df), 300)
        np.testing.assert_allclose(dv.min(0), v.min(0), atol=.02)
        np.testing.assert_allclose(dv.max(0), v.max(0), atol=.02)


class VariantsTest(unittest.TestCase):
    def test_writes_variant_glbs_sidecars_and_manifest_entries(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp)
            v, f = subdivide(*box((-.5, -.1, -.15), (.5, .1, .15)), 4)
            (out / "boat.glb").write_bytes(cf.glb_bytes(v.astype(np.float32), cf.vertex_normals(v, f), f.astype(np.uint32)))
            (out / "boat.json").write_text(json.dumps({"version": 1, "id": "boat", "source": "auto", "engines": [], "mounts": [], "weaponSlots": 1}))
            (out / "manifest.json").write_text(json.dumps({"version": 1, "ships": {"boat": {"glb": "boat.glb", "sidecar": "boat.json"}}}))
            written = mlv.make_variants(out, ["boat"], [2000, 500])
            self.assertEqual(written, ["boat__lod2k", "boat__lod500"])
            manifest = json.loads((out / "manifest.json").read_text())["ships"]
            self.assertIn("boat", manifest)
            for vid in written:
                self.assertEqual(manifest[vid], {"glb": f"{vid}.glb", "sidecar": f"{vid}.json"})
                self.assertEqual(json.loads((out / f"{vid}.json").read_text())["id"], vid)
            _, faces = cf.read_glb((out / "boat__lod500.glb").read_bytes())
            self.assertLessEqual(len(faces), 500)


if __name__ == "__main__":
    unittest.main()
