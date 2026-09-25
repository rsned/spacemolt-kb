#!/usr/bin/env python3
"""Tests for the cinema hull export. Run from data/mesh_bakeoff:
    ~/sf3d-venv/bin/python -m unittest test_cinema_export -v
"""
import json
import struct
import unittest

import numpy as np

import cinema_frame as cf


def box(lo, hi):
    """Closed axis-aligned box, outward CCW winding."""
    x0, y0, z0 = lo
    x1, y1, z1 = hi
    v = np.array([[x0, y0, z0], [x1, y0, z0], [x1, y1, z0], [x0, y1, z0],
                  [x0, y0, z1], [x1, y0, z1], [x1, y1, z1], [x0, y1, z1]], float)
    f = np.array([[0, 2, 1], [0, 3, 2], [4, 5, 6], [4, 6, 7], [0, 1, 5], [0, 5, 4],
                  [3, 6, 2], [3, 7, 6], [0, 4, 7], [0, 7, 3], [1, 2, 6], [1, 6, 5]])
    return v, f


def signed_volume(v, f):
    return float(np.einsum("ij,ij->i", v[f[:, 0]], np.cross(v[f[:, 1]], v[f[:, 2]])).sum() / 6)


class OrientTest(unittest.TestCase):
    def setUp(self):
        # Hy3D-style world frame: long axis along world Z, up = world Y
        self.v, self.f = box((-0.3, -0.1, -1.0), (0.3, 0.1, 1.0))

    def test_length_normalized_and_centred(self):
        p, _ = cf.orient(self.v, self.f, {}, bow_flipped=False)
        self.assertAlmostEqual(float(np.ptp(p[:, 0])), 1.0, places=5)
        np.testing.assert_allclose((p.min(0) + p.max(0)) / 2, 0, atol=1e-6)

    def test_long_axis_becomes_x_up_stays_y(self):
        p, _ = cf.orient(self.v, self.f, {}, bow_flipped=False)
        ext = np.ptp(p, axis=0)
        self.assertAlmostEqual(float(ext[0]), 1.0, places=5)      # 2.0 long
        self.assertAlmostEqual(float(ext[1]), 0.1, places=5)      # 0.2 tall
        self.assertAlmostEqual(float(ext[2]), 0.3, places=5)      # 0.6 beam

    def test_bow_flip_negates_x(self):
        v = self.v.copy(); v[v[:, 2] > 0, 0] *= 0.2               # narrow +Z end
        a, _ = cf.orient(v, self.f, {}, bow_flipped=False)
        b, _ = cf.orient(v, self.f, {}, bow_flipped=True)
        np.testing.assert_allclose(a[:, 0], -b[:, 0], atol=1e-6)

    def test_vflip_negates_y(self):
        a, _ = cf.orient(self.v, self.f, {}, False)
        b, _ = cf.orient(self.v, self.f, {"vflip": True}, False)
        np.testing.assert_allclose(a[:, 1], -b[:, 1], atol=1e-6)

    def test_stretch_does_not_change_normalized_length(self):
        p, _ = cf.orient(self.v, self.f, {"stretch": 1.25}, False)
        self.assertAlmostEqual(float(np.ptp(p[:, 0])), 1.0, places=5)
        self.assertAlmostEqual(float(np.ptp(p[:, 2])), 0.3 / 1.25, places=5)

    def test_winding_outward_even_when_mirrored(self):
        for adj in ({}, {"mirror": True}):
            p, f = cf.orient(self.v, self.f, adj, False)
            self.assertGreater(signed_volume(p, f), 0)


class NormalsTest(unittest.TestCase):
    def test_unit_and_outward(self):
        v, f = box((-1, -1, -1), (1, 1, 1))
        n = cf.vertex_normals(v, f)
        np.testing.assert_allclose(np.linalg.norm(n, axis=1), 1, atol=1e-5)
        self.assertTrue(np.all(np.einsum("ij,ij->i", n, v) > 0))


class GlbTest(unittest.TestCase):
    def test_header_chunks_and_accessors(self):
        v, f = box((-.5, -.1, -.2), (.5, .1, .2))
        data = cf.glb_bytes(v.astype(np.float32), cf.vertex_normals(v, f), f.astype(np.uint32))
        magic, version, total = struct.unpack_from("<III", data, 0)
        self.assertEqual((magic, version, total), (0x46546C67, 2, len(data)))
        jlen, jtype = struct.unpack_from("<II", data, 12)
        self.assertEqual(jtype, 0x4E4F534A)
        gltf = json.loads(data[20:20 + jlen])
        acc = gltf["accessors"]
        self.assertEqual(acc[0]["count"], 8)
        self.assertEqual(acc[2]["count"], 36)
        self.assertEqual(gltf["meshes"][0]["primitives"][0]["attributes"], {"POSITION": 0, "NORMAL": 1})
        self.assertEqual(len(data) % 4, 0)


import hardpoints as hp


def merge(*parts):
    vs, fs, off = [], [], 0
    for v, f in parts:
        vs.append(v); fs.append(f + off); off += len(v)
    return np.vstack(vs), np.vstack(fs)


def subdivide(v, f, times):
    """Midpoint 4-split, enough to give flat faces many candidate triangles."""
    for _ in range(times):
        mids = {}
        verts = list(map(tuple, v))
        def mid(a, b):
            key = (min(a, b), max(a, b))
            if key not in mids:
                mids[key] = len(verts)
                verts.append(tuple((np.asarray(verts[a]) + np.asarray(verts[b])) / 2))
            return mids[key]
        nf = []
        for a, b, c in f:
            ab, bc, ca = mid(a, b), mid(b, c), mid(c, a)
            nf += [[a, ab, ca], [ab, b, bc], [ca, bc, c], [ab, bc, ca]]
        v, f = np.array(verts), np.array(nf)
    return v, f


class EnginesTest(unittest.TestCase):
    def test_two_nozzles_found_and_rear_wall_rejected(self):
        hull = box((-.5, -.1, -.15), (.5, .1, .15))
        n1 = box((-.55, -.02, .08), (-.5, .02, .12))
        n2 = box((-.55, -.02, -.12), (-.5, .02, -.08))
        v, f = merge(hull, n1, n2)
        engines = hp.guess_engines(v, f)
        self.assertEqual(len(engines), 2)
        zs = sorted(e["pos"][2] for e in engines)
        self.assertAlmostEqual(zs[0], -.1, places=2)
        self.assertAlmostEqual(zs[1], .1, places=2)
        for e in engines:
            self.assertAlmostEqual(e["pos"][0], -.55, places=2)
            self.assertTrue(.015 <= e["radius"] <= .05)

    def test_fallback_single_stern_engine(self):
        v, f = box((-.5, -.1, -.15), (.5, .1, .15))
        engines = hp.guess_engines(v, f)
        self.assertEqual(len(engines), 1)
        self.assertAlmostEqual(engines[0]["pos"][0], -.5, places=3)
        self.assertAlmostEqual(engines[0]["pos"][2], 0, places=3)


class MountsTest(unittest.TestCase):
    def setUp(self):
        self.v, self.f = subdivide(*box((-.5, -.1, -.15), (.5, .1, .15)), 3)

    def test_mounts_forward_unit_normals_capped(self):
        mounts = hp.guess_mounts(self.v, self.f)
        self.assertTrue(0 < len(mounts) <= 12)
        for m in mounts:
            self.assertGreater(m["pivot"][0], 0)
            self.assertAlmostEqual(float(np.linalg.norm(m["normal"])), 1, places=4)

    def test_mirror_pairs_on_symmetric_hull(self):
        mounts = hp.guess_mounts(self.v, self.f)
        for m in mounts:
            x, y, z = m["pivot"]
            if abs(z) > .02:
                self.assertTrue(any(abs(o["pivot"][0] - x) < .04 and abs(o["pivot"][2] + z) < .04
                                    for o in mounts), f"no mirror for {m}")


import export_cinema_hulls as ech


class SidecarTest(unittest.TestCase):
    def test_schema(self):
        v, f = box((-.5, -.1, -.15), (.5, .1, .15))
        s = ech.build_sidecar("dirk", v, f, 3)
        self.assertEqual(set(s), {"version", "id", "source", "engines", "mounts", "weaponSlots"})
        self.assertEqual((s["version"], s["id"], s["source"], s["weaponSlots"]), (1, "dirk", "auto", 3))
        json.dumps(s)   # must be plain JSON types


if __name__ == "__main__":
    unittest.main()
