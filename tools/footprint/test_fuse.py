"""Tests for the fusion selector (tools/footprint/fuse.py).

Unit tests run on synthetic fixtures; the final integration test reads the
real data tree and asserts headline invariants only.
"""
import json
import pathlib
import sys
import warnings

import numpy as np

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
from tools.footprint import fuse  # noqa: E402

LABELS = json.loads(
    (pathlib.Path(__file__).resolve().parents[2]
     / "data/footprints/eyeball_labels_2026-08-01.json").read_text())


def test_load_rosters_reads_the_session_rulings():
    r = fuse.load_rosters(LABELS)
    assert "apeiron" in r.wing_filled
    assert {"voidborn_singularity", "qualia", "vigil"} <= r.wing_crescent
    assert "frankenhauler" in r.prong_confirmed
    assert "no_refunds" in r.receding and "long_haul" in r.receding
    assert any(a == "aether" and b == "close_enough"
               for a, b, _, _ in r.family_pairs)


def _mini_tree(tmp_path):
    """Two-ship synthetic data tree: 'plain' (full candidates) and 'bare'
    (report record only)."""
    foot = tmp_path / "footprints"
    bakeoff = tmp_path / "bakeoff"
    w = [0.2] * 96
    report = {"alpha": 8.0, "background": "x", "results": [
        {"id": "plain", "status": "ok_asymmetric", "aspect": 2.5, "w": w,
         "concave": [False] * 96, "orientation": "bow_t0",
         "orientation_source": {"flipped_from_stored": False},
         "quality": {"silhouette_iou": 0.99}},
        {"id": "bare", "status": "failed_dimensional_check", "aspect": 0.5,
         "w": w, "concave": [False] * 96, "orientation": "unknown",
         "quality": {"silhouette_iou": 0.98}},
    ]}
    foot.mkdir()
    (foot / "report.json").write_text(json.dumps(report))
    (foot / "plain").mkdir()
    (foot / "plain" / "footprint.json").write_text(json.dumps({
        "alpha": 8.0,
        "polygon": {"type": "Polygon", "coordinates":
                    [[[0, -0.2], [1, -0.2], [1, 0.2], [0, 0.2], [0, -0.2]]]}}))
    # mesh stored under a faction prefix — exercises the prefix-stripped pass
    (bakeoff / "outerrim_plain").mkdir(parents=True)
    (bakeoff / "outerrim_plain" / "profile.json").write_text(json.dumps(
        {"w": [0.3] * 96, "aspect": 1.8}))
    return foot, bakeoff


def test_load_candidates_assembles_all_sources(tmp_path):
    foot, bakeoff = _mini_tree(tmp_path)
    cands = fuse.load_candidates(foot, bakeoff)
    c = cands["plain"]
    assert c.pipe["aspect"] == 2.5
    assert c.polygon["polygon"]["type"] == "Polygon"
    assert c.mesh_w is not None and len(c.mesh_w) == 96
    assert c.mesh_aspect == 1.8


def test_load_candidates_tolerates_missing_sources(tmp_path):
    foot, bakeoff = _mini_tree(tmp_path)
    c = fuse.load_candidates(foot, bakeoff)["bare"]
    assert c.pipe["status"] == "failed_dimensional_check"
    assert c.polygon is None and c.mesh_w is None and c.mesh_aspect is None


def test_mesh_resolution_prefers_the_exact_stem(tmp_path):
    foot, bakeoff = _mini_tree(tmp_path)
    # an exact-stem dir must beat the prefixed one, matching the contact sheet
    (bakeoff / "plain").mkdir()
    (bakeoff / "plain" / "profile.json").write_text(json.dumps(
        {"w": [0.5] * 96, "aspect": 9.9}))
    c = fuse.load_candidates(foot, bakeoff)["plain"]
    assert c.mesh_aspect == 9.9


def test_pick_sources_cover_every_sheet_panel():
    assert fuse.PICK_SOURCES == {
        "moge": ("pipeline_profile", False),
        "footprint": ("pipeline_polygon", False),
        "mesh": ("mesh", False),
        "mesh067": ("mesh", False),
        "mesh067sq": ("mesh_squared", True),
    }


def test_mesh_shape_applies_beam_correction_and_optional_squaring():
    w = np.full(96, 1.0)
    w[60:90] = 2.0
    w[70] = 1.8                       # dent inside the wide run
    plain = fuse.mesh_shape(w, squared=False)
    assert np.allclose(plain[0], 0.67) and np.allclose(plain[60], 1.34)
    assert np.isclose(plain[70], 1.8 * 0.67)   # dent survives unsquared
    sq = fuse.mesh_shape(w, squared=True)
    assert np.isclose(sq[70], 1.34)            # dent snapped to the plateau


def test_mesh_adjusted_aspect_inverts_the_beam_correction():
    assert np.isclose(fuse.mesh_adjusted_aspect(1.8), 1.8 / 0.67)


# --- Helper fixtures for the rule ladder tests ---

def _cand(status="ok_asymmetric", iou=0.99, aspect=2.0, mesh=True,
          polygon=True, concave=None, orientation="bow_t0", no_w=False):
    w = [0.25] * 96
    pipe = {"id": "x", "status": status, "aspect": aspect,
            "concave": concave or [False] * 96,
            "orientation": orientation,
            "orientation_source": {"flipped_from_stored": False},
            "quality": {"silhouette_iou": iou}}
    if not no_w:
        pipe["w"] = w
    return fuse.Candidate(
        pipe=pipe,
        polygon={"alpha": 8.0, "polygon": {"type": "Polygon",
                 "coordinates": [[[0, -0.25], [1, -0.25], [1, 0.25],
                                  [0, 0.25], [0, -0.25]]]}} if polygon else None,
        mesh_w=np.full(96, 0.3) if mesh else None,
        mesh_aspect=1.7 if mesh else None)


def _rosters(**kw):
    base = dict(wing_filled=frozenset(), wing_crescent=frozenset(),
                prong_confirmed=frozenset(), receding=frozenset(),
                family_pairs=())
    base.update(kw)
    return fuse.Rosters(**base)


# --- Rule ladder tests ---

def test_rule1_user_pick_wins_and_maps_the_panel():
    d = fuse.apply_rules("s", _cand(), _rosters(), "mesh067sq", {})
    assert (d.rule, d.confidence) == ("user_pick", "user_pick")
    assert d.shape_source == "mesh_squared" and d.squared
    assert d.aspect_source == "mesh"


def test_rule1_footprint_pick_takes_pipeline_scale():
    d = fuse.apply_rules("s", _cand(), _rosters(), "footprint", {})
    assert d.shape_source == "pipeline_polygon"
    assert d.aspect_source == "pipeline_profile"


def test_rule1_stale_pick_without_the_source_falls_through():
    # a 'footprint' pick with no polygon on disk (withdrawn ship) must not
    # produce a polygon winner out of thin air
    d = fuse.apply_rules("s", _cand(polygon=False, status="failed_low_obliquity",
                                    mesh=False),
                         _rosters(), "footprint", {})
    assert d.rule != "user_pick"
    assert any("stale pick" in n for n in d.notes)


def test_rule2_wings_take_pipeline_aspect_never_mesh():
    r = _rosters(wing_filled=frozenset({"s"}))
    d = fuse.apply_rules("s", _cand(status="failed_dimensional_check",
                                    aspect=0.73), r, None, {})
    assert d.rule == "wing_family"
    assert d.shape_source == "pipeline_profile"      # filled wing -> profile
    assert d.aspect_source == "pipeline_profile"
    r2 = _rosters(wing_crescent=frozenset({"s"}))
    d2 = fuse.apply_rules("s", _cand(), r2, None, {})
    assert d2.shape_source == "pipeline_polygon"     # crescent -> polygon


def test_rule3_prong_ships_take_the_polygon():
    r = _rosters(prong_confirmed=frozenset({"s"}))
    d = fuse.apply_rules("s", _cand(), r, None, {})
    assert (d.rule, d.shape_source) == ("prong_or_pod", "pipeline_polygon")
    # bow-concave detector route (no roster membership)
    concave = [True] * 8 + [False] * 88
    d2 = fuse.apply_rules("t", _cand(concave=concave), _rosters(), None, {})
    assert d2.rule == "prong_or_pod"


def test_rule3_detector_routes_are_gated_on_ok_solve_status():
    # a wrecked solve's concave flags are artifacts of the same wreck — the
    # bow-concave/pod-blob detectors must not trust them and must fall
    # through to rule 4 (wrecked_solve), not claim rule 3 (prong_or_pod)
    concave = [True] * 30 + [False] * 66
    d = fuse.apply_rules("s", _cand(status="failed_dimensional_check",
                                    concave=concave), _rosters(), None, {})
    assert d.rule == "wrecked_solve"
    assert d.shape_source == "mesh"


def test_rule4_wrecked_solves_take_corrected_mesh():
    d = fuse.apply_rules("s", _cand(status="failed_dimensional_check",
                                    aspect=0.5), _rosters(), None, {})
    assert (d.rule, d.shape_source, d.aspect_source) == (
        "wrecked_solve", "mesh", "mesh")
    assert not d.squared


def test_rule4_sibling_mesh067sq_pick_turns_on_squaring():
    r = _rosters(family_pairs=(("s", "sib", 0.5, 2.0),))
    d = fuse.apply_rules("s", _cand(status="failed_dimensional_check"),
                         r, None, {"sib": "mesh067sq"})
    assert d.rule == "wrecked_solve" and d.squared


def test_rule1_polygon_pick_requires_pipe_w():
    # Stale pick scenario: polygon on disk but no w in pipe (e.g., interstice)
    # such a pick should fall through and be documented as stale
    d = fuse.apply_rules("s", _cand(polygon=True, no_w=True), _rosters(),
                         "footprint", {})
    assert d.rule != "user_pick"
    assert any("stale pick" in n for n in d.notes)


def test_rule5_clean_pipeline_and_receding_lower_bound_note():
    d = fuse.apply_rules("s", _cand(), _rosters(), None, {})
    assert (d.rule, d.shape_source) == ("clean_pipeline", "pipeline_profile")
    r = _rosters(receding=frozenset({"s"}))
    d2 = fuse.apply_rules("s", _cand(), r, None, {})
    assert any("lower bound" in n for n in d2.notes)


def test_rule6_no_candidates_is_unresolved():
    d = fuse.apply_rules("s", _cand(status="failed_symmetry_solve",
                                    polygon=False, mesh=False, aspect=None),
                         _rosters(), None, {})
    assert (d.rule, d.confidence) == ("unresolved", "unresolved")
    assert d.shape_source is None


def test_rule4_without_mesh_falls_through_to_unresolved():
    d = fuse.apply_rules("s", _cand(status="failed_dimensional_check",
                                    mesh=False, polygon=False),
                         _rosters(), None, {})
    assert d.rule == "unresolved"
    assert any("no mesh" in n for n in d.notes)


# --- Entry builder tests ---

def test_canonical_polygon_normalises_and_matches_profile_direction():
    gj = {"type": "Polygon", "coordinates":
          [[[0, -1], [10, -1], [10, 1], [0, 1], [0, -1]]]}
    # stored profile is wide at t=0, narrow at t=1 -> a wedge polygon must flip
    wedge = {"type": "Polygon", "coordinates":
             [[[0, 0], [10, -3], [10, 3], [0, 0]]]}
    stored = list(np.linspace(0.5, 0.05, 96))     # wide nose, narrow tail
    rings = fuse.canonical_polygon(wedge, stored)
    xs = np.array(rings[0])[:, 0]
    assert 0.0 <= xs.min() < 0.01 and 0.99 < xs.max() <= 1.0
    ring = np.array(rings[0])
    # widest section must sit near x=0 to match the stored profile
    wide_x = ring[np.argmax(np.abs(ring[:, 1])), 0]
    assert wide_x < 0.3
    # plain rectangle: no crash without a stored profile
    assert fuse.canonical_polygon(gj, None)


def test_canonical_polygon_tolerates_a_constant_profile():
    # a stored profile with exactly-zero variance (e.g. 0.5 repeated, which
    # sums without float rounding) makes np.corrcoef divide 0/0 -> NaN with
    # a RuntimeWarning, silently skipping the direction flip. Guard it the
    # same way mesh_orientation guards its own corrcoef calls.
    wedge = {"type": "Polygon", "coordinates":
             [[[0, 0], [10, -3], [10, 3], [0, 0]]]}
    stored = [0.5] * 96
    with warnings.catch_warnings():
        warnings.simplefilter("error", RuntimeWarning)
        rings = fuse.canonical_polygon(wedge, stored)
    assert rings


def test_mesh_orientation_correlates_against_an_oriented_pipeline_profile():
    t = np.linspace(0, 1, 96)
    pipe_w = 0.5 - 0.4 * t                        # wide bow, narrow stern
    rec = {"orientation": "bow_t0", "w": list(pipe_w)}
    mesh = pipe_w[::-1] + np.random.default_rng(0).normal(0, 0.01, 96)
    out, orient = fuse.mesh_orientation(mesh, rec)
    assert orient == "bow_t0"
    assert np.corrcoef(out, pipe_w)[0, 1] > 0.9   # got reversed to match
    out2, orient2 = fuse.mesh_orientation(np.full(96, 0.3), rec)
    assert orient2 == "unknown"                   # flat: no margin


def test_build_entry_profile_winner():
    c = _cand()
    d = fuse.Decision("clean_pipeline", "rules", "pipeline_profile",
                      "pipeline_profile")
    e = fuse.build_entry("s", c, d, _rosters())
    assert e["schema"] == 1 and e["rule"] == "clean_pipeline"
    assert e["shape_source"] == "pipeline_profile"
    assert len(e["w"]) == 96 and "polygon" not in e
    assert e["aspect"] == 2.0
    assert e["provenance"]["candidate_aspects"]["mesh_adjusted"] == 1.7 / 0.67


def test_build_entry_polygon_winner_carries_lossy_envelope():
    c = _cand()
    d = fuse.Decision("prong_or_pod", "rules", "pipeline_polygon",
                      "pipeline_profile")
    e = fuse.build_entry("s", c, d, _rosters())
    assert e["polygon"] and e["envelope_lossy"] is True
    assert len(e["w"]) == 96


def test_build_entry_mesh_winner_scales_shape_and_aspect():
    c = _cand(status="failed_dimensional_check")
    d = fuse.Decision("wrecked_solve", "rules", "mesh", "mesh")
    e = fuse.build_entry("s", c, d, _rosters())
    assert np.isclose(e["w"][0], 0.3 * 0.67)
    assert np.isclose(e["aspect"], 1.7 / 0.67)
    assert e["concave"] == [False] * 96           # mesh has no concave flags


def test_build_entry_unresolved_lists_candidates_only():
    c = _cand(status="failed_symmetry_solve", polygon=False, mesh=False)
    d = fuse.Decision("unresolved", "unresolved", None, None)
    e = fuse.build_entry("s", c, d, _rosters())
    assert e["shape_source"] is None and "w" not in e
    assert "candidate_aspects" in e["provenance"]


def test_validate_reports_rule_vs_pick_disagreements():
    cands = {"s": _cand()}
    rosters = _rosters()
    picks = {"s": "footprint"}      # user says polygon; rules say profile
    entries = {"s": fuse.build_entry(
        "s", cands["s"],
        fuse.apply_rules("s", cands["s"], rosters, "footprint", picks),
        rosters, pick="footprint")}
    v = fuse.validate(entries, cands, rosters, picks)
    assert v["rules_vs_picks"] == [{
        "id": "s", "rule_answer": "pipeline_profile",
        "pick_answer": "pipeline_polygon"}]


def test_validate_checks_family_aspect_ratios():
    rosters = _rosters(family_pairs=(("a", "b", 1.5, 3.0),))
    ea = {"id": "a", "aspect": 2.0}
    eb = {"id": "b", "aspect": 1.0}
    v = fuse.validate({"a": ea, "b": eb}, {}, rosters, {})
    fc = v["family_consistency"][0]
    assert fc["ok"] and np.isclose(fc["ratio"], 2.0)
    v2 = fuse.validate({"a": {"id": "a", "aspect": 10.0}, "b": eb},
                       {}, rosters, {})
    assert not v2["family_consistency"][0]["ok"]


def test_run_writes_entries_and_index(tmp_path):
    foot, bakeoff = _mini_tree(tmp_path)
    (foot / "user_annotations_2026-08-01.json").write_text(json.dumps(
        {"best_picks": {"plain": "moge"},
         "bow_directions_deg_cw_from_screen_up": {}}))
    (foot / "eyeball_labels_2026-08-01.json").write_text(json.dumps(
        {"fusion_rosters": {
            "wing_filled": [], "wing_crescent": [], "prong_confirmed": [],
            "receding_right_2_3": [], "receding_left_9_10": [],
            "family_pairs": []}}))
    index = fuse.run(foot=foot, bakeoff=bakeoff)
    fused = foot / "fused"
    assert (fused / "plain.json").exists() and (fused / "index.json").exists()
    plain = json.loads((fused / "plain.json").read_text())
    assert plain["rule"] == "user_pick" and plain["shape_source"] == \
        "pipeline_profile"
    bare = json.loads((fused / "bare.json").read_text())
    assert bare["rule"] == "unresolved"        # wrecked but no mesh
    assert index["counts"]["user_pick"] == 1
    assert index["ships"]["bare"]["confidence"] == "unresolved"


def test_run_on_the_real_data_holds_the_headline_invariants():
    """Integration: real repo data. Skipped if the mesh bakeoff is absent."""
    import pytest
    if not fuse.BAKEOFF.exists():
        pytest.skip("mesh bakeoff data not present")
    import tempfile
    with tempfile.TemporaryDirectory() as td:
        # Load the entries to check staleness documentation
        index = fuse.run(out=pathlib.Path(td))
        assert len(index["ships"]) == 267
        picks = json.loads((fuse.FOOT /
                            "user_annotations_2026-08-01.json").read_text()
                           )["best_picks"]

        # Verify picks either resolved or are documented as stale
        unresolved_without_stale_note = []
        for sid in sorted(picks.keys()):
            if sid not in index["ships"]:
                continue
            entry_file = pathlib.Path(td) / f"{sid}.json"
            if entry_file.exists():
                entry = json.loads(entry_file.read_text())
                if entry["confidence"] == "unresolved":
                    notes = entry.get("provenance", {}).get("notes", [])
                    has_stale_note = any("stale pick" in n for n in notes)
                    if not has_stale_note:
                        unresolved_without_stale_note.append(sid)

        if unresolved_without_stale_note:
            raise AssertionError(
                f"picks came out unresolved without stale documentation: "
                f"{unresolved_without_stale_note}")

        # every pick either resolved as rule 1 or documented its staleness
        n_pick_rule = sum(1 for s in index["ships"].values()
                          if s["rule"] == "user_pick")
        assert n_pick_rule >= len([s for s in picks if s in index["ships"]]) - 5
