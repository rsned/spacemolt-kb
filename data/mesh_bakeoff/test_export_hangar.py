#!/usr/bin/env python3
"""cd data/mesh_bakeoff && ~/hy3d-venv/bin/python -m unittest test_export_hangar -v"""
import json
import sqlite3
import tempfile
import unittest
from pathlib import Path

import export_hangar as eh

CATALOG = {
    "big": {"name": "Big One", "faction": "solarian", "tier": 5, "scale": 5, "category": "Combat", "class": "Dreadnought"},
    "small": {"name": "Small One", "faction": "crimson", "tier": 1, "scale": 1, "category": "Combat", "class": "Fighter"},
    "ghost": {"name": "Ghost", "faction": "", "tier": 2, "scale": 2, "category": "Commercial", "class": "Freighter"},
    "legacy": {"name": "Old", "faction": "legacy", "tier": 1, "scale": 1, "category": "Discontinued", "class": "Mystery"},
    "nocat": {"name": "No Category", "faction": "krynn", "tier": 2, "scale": 2, "category": "", "class": "Unknown"},
    "orphan": {"name": "Orphan", "faction": "krynn", "tier": 3, "scale": 3, "category": "Support", "class": "Tender"},
}
ESTIMATES = {"big": {"loa_m": 142.0, "beam_m": 40.0, "source": "window"},
             "small": {"loa_m": 18.0, "beam_m": 9.0, "source": "ladder:combat"}}
LADDER = {"ladder_group_median": {"2/hauler": 83.7}, "ladder_scale_geomean": {"1": 24.2, "2": 66.6}}
ASPECTS = {"big": {"beam": .28, "height": .2}, "small": {"beam": .5, "height": .25}}
PAGES = {
    "big": "Combat/big.html",
    "small": "Combat/small.html",
    "ghost": "Commercial/ghost.html",
    "legacy": "Discontinued/legacy.html",
    "nocat": "Discontinued/nocat.html",   # empty DB category, but a real page exists
    # "orphan" deliberately absent from pages -> page should be None
}


class LineupTest(unittest.TestCase):
    def setUp(self):
        self.lineup = eh.build_lineup(CATALOG, ESTIMATES, LADDER, ASPECTS, {"big", "small"}, PAGES)
        self.by_id = {s["id"]: s for s in self.lineup["ships"]}

    def test_every_catalog_ship_in_length_order(self):
        self.assertEqual([s["id"] for s in self.lineup["ships"]],
                         ["small", "legacy", "orphan", "nocat", "ghost", "big"])
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

    def test_page_from_mapping_not_category(self):
        # empty DB category, but a real page exists under a different folder:
        # the category falls back to that page's directory
        self.assertEqual(self.by_id["nocat"]["category"], "Discontinued")
        self.assertEqual(self.by_id["nocat"]["page"], "Discontinued/nocat.html")
        # missing from the pages mapping entirely -> null, not a guessed path
        self.assertIsNone(self.by_id["orphan"]["page"])


    def test_empty_category_without_page_stays_empty(self):
        catalog = {"x": {**CATALOG["nocat"]}}
        lineup = eh.build_lineup(catalog, {}, LADDER, ASPECTS, set(), {})
        self.assertEqual(lineup["ships"][0]["category"], "")

    def test_sort_ties_break_by_name_then_id(self):
        catalog = {sid: {"name": name, "faction": "krynn", "tier": 1, "scale": 1, "category": "Combat", "class": "Fighter"}
                   for sid, name in [("zz", "Same"), ("aa", "Same"), ("mm", "Alpha")]}
        lineup = eh.build_lineup(catalog, {}, LADDER, ASPECTS, set(), {})
        self.assertEqual([s["id"] for s in lineup["ships"]], ["mm", "aa", "zz"])


class LoadCatalogTest(unittest.TestCase):
    def test_drops_legacy_rows(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "kb.db"
            db = sqlite3.connect(path)
            db.execute("CREATE TABLE ships (id, name, faction, tier, scale, category, class)")
            db.executemany("INSERT INTO ships VALUES (?,?,?,?,?,?,?)", [
                ("excavator", "Excavator", "", 2, 2, "", "Barge"),
                ("mining_barge", "Excavator", "legacy", 2, 2, "Industrial", "Barge"),
                ("shiv", "Shiv", "crimson", 1, 1, "Combat", "Fighter")])
            db.commit(); db.close()
            self.assertEqual(sorted(eh.load_catalog(path)), ["excavator", "shiv"])


class CliTest(unittest.TestCase):
    def test_defaults(self):
        a = eh.parse_args([])
        self.assertEqual((a.faces, a.db, a.hulls, a.out), (eh.HANGAR_FACES, eh.KB_DB, eh.HULLS, eh.OUT))

    def test_overrides(self):
        a = eh.parse_args(["--db", "x.db", "--hulls", "h", "--out", "o", "--faces", "100"])
        self.assertEqual((a.faces, a.db, a.hulls, a.out), (100, Path("x.db"), Path("h"), Path("o")))


class SourceInfoTest(unittest.TestCase):
    def test_records_inputs(self):
        with tempfile.TemporaryDirectory() as tmp:
            db = Path(tmp) / "kb.db"; db.write_text("")
            manifest = Path(tmp) / "manifest.json"; manifest.write_text(json.dumps({"ships": ["a", "b", "a__lod1"]}))
            import datetime as dt
            now = dt.datetime(2026, 9, 27, 23, 5, tzinfo=dt.timezone.utc)
            src = eh.source_info(6000, manifest, db, now)
            self.assertEqual(src["faces"], 6000)
            self.assertEqual(src["generated"], "2026-09-27")
            self.assertEqual(src["hulls"], {"manifest": str(manifest), "ships": 2})
            self.assertEqual(src["db"]["path"], str(db))
            self.assertRegex(src["db"]["mtime"], r"^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$")


class PruneModelsTest(unittest.TestCase):
    def test_removes_only_unmodeled_glbs(self):
        with tempfile.TemporaryDirectory() as tmp:
            d = Path(tmp)
            for n in ["keep.glb", "gone.glb", "notes.txt"]:
                (d / n).write_text("")
            removed = eh.prune_models(d, {"keep"})
            self.assertEqual(removed, ["gone.glb"])
            self.assertEqual(sorted(p.name for p in d.iterdir()), ["keep.glb", "notes.txt"])


def make_pages(tmp, layout):
    """layout: {"Dir": ["stem", ...]} -> touches tmp/Dir/stem.html for each."""
    root = Path(tmp)
    for d, stems in layout.items():
        (root / d).mkdir(parents=True, exist_ok=True)
        for stem in stems:
            (root / d / f"{stem}.html").write_text("")
    return root


class ResolvePageDirTest(unittest.TestCase):
    """Pure-function tests for the duplicate-page tiebreak (order-independent by
    construction, unlike testing raw filesystem glob order, which is not stable
    across filesystems/OSes)."""

    def test_prefers_dir_matching_catalog_category(self):
        self.assertEqual(eh.resolve_page_dir(["Discontinued", "Combat"], "Combat"), "Combat")
        self.assertEqual(eh.resolve_page_dir(["Combat", "Discontinued"], "Combat"), "Combat")

    def test_empty_category_prefers_non_discontinued_dir(self):
        self.assertEqual(eh.resolve_page_dir(["Discontinued", "Support"], ""), "Support")
        self.assertEqual(eh.resolve_page_dir(["Support", "Discontinued"], ""), "Support")

    def test_only_discontinued_available(self):
        self.assertEqual(eh.resolve_page_dir(["Discontinued"], "Discontinued"), "Discontinued")
        self.assertEqual(eh.resolve_page_dir(["Discontinued"], ""), "Discontinued")


class LoadPagesDuplicatesTest(unittest.TestCase):
    """load_pages end-to-end: real duplicate files on disk, resolved via resolve_page_dir."""

    def test_prefers_dir_matching_catalog_category(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = make_pages(tmp, {"Combat": ["shiv"], "Discontinued": ["shiv"]})
            pages = eh.load_pages(root, {"shiv": "Combat"})
            self.assertEqual(pages["shiv"], "Combat/shiv.html")

    def test_empty_category_prefers_non_discontinued_dir(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = make_pages(tmp, {"Support": ["nocat"], "Discontinued": ["nocat"]})
            pages = eh.load_pages(root, {"nocat": ""})
            self.assertEqual(pages["nocat"], "Support/nocat.html")

    def test_only_in_discontinued_resolves_there(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = make_pages(tmp, {"Discontinued": ["legacy"]})
            pages = eh.load_pages(root, {"legacy": "Discontinued"})
            self.assertEqual(pages["legacy"], "Discontinued/legacy.html")

    def test_ignores_non_catalog_stems(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = make_pages(tmp, {"Combat": ["shiv", "index"]})
            pages = eh.load_pages(root, {"shiv": "Combat"})
            self.assertNotIn("index", pages)
            self.assertEqual(pages["shiv"], "Combat/shiv.html")
