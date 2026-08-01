"""Checks for the chroma-key matte.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_matte.py
"""

import importlib.util
import os
import pathlib
import sys

import cv2
import numpy as np
import pytest

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

synth = _load("synth")
matte = _load("matte")
paths = _load("paths")


# The 19 files under HERO_DIR that match a real catalog ship, classified by
# keyability against the actual art (measured 2026-07-29; see task-2-report.md
# for the full corner_spread/border_std table). The other 18 files that also
# match HERO_GLOB are unrelated downloads (banners, maps, portraits, fleet
# views) and are not part of this catalog.
KEYABLE_HERO_IDS = [
    "comet", "excessive_force", "ledger", "liquidity_event", "magnate",
    "outerrim_prayer", "premiere", "rapid_smelter", "reliquary", "smelter",
    "superposition", "thermodynamic_end", "war_wagon", "yard_sale",
]
NOT_KEYABLE_HERO_IDS = ["bonanza", "crowbar", "last_warning", "paradox", "principia"]


def _load_hero(stem):
    """Load a real hero image by filename stem, skipping cleanly if the art
    drop isn't present in this environment (paths.HERO_DIR defaults to
    ~/Downloads, which won't exist in CI)."""
    p = paths.HERO_DIR / f"{stem}.webp"
    if not p.exists():
        pytest.skip(f"hero art not present: {p}")
    img = cv2.imread(str(p))
    if img is None:
        pytest.skip(f"hero art unreadable: {p}")
    return cv2.cvtColor(img, cv2.COLOR_BGR2RGB)


def test_matte_recovers_the_rendered_subject():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, frac = matte.extract(s.image)

    assert mask.shape == s.image.shape[:2]
    assert set(np.unique(mask)).issubset({0, 1})
    assert 0.05 < frac < 0.85, frac
    # Corners are background in every hero image.
    for y, x in [(0, 0), (0, -1), (-1, 0), (-1, -1)]:
        assert mask[y, x] == 0


def test_matte_drops_disconnected_specks():
    # The speck must survive the 5x5 opening on its own merit, or this test
    # passes regardless of whether the largest-connected-component filter
    # exists: opening alone erodes anything narrower than the kernel to
    # nothing (verified empirically: a 4x4 speck is erased by opening alone;
    # a 6x6 one survives opening+closing intact and is only removed by the
    # CC filter). A 4x4 speck doesn't discriminate between the two
    # implementations; 6x6 does.
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    img = s.image.copy()
    img[5:11, 5:11] = (20, 20, 20)  # an isolated non-magenta speck
    mask, _ = matte.extract(img)
    assert mask[5:11, 5:11].sum() == 0


def test_matte_is_not_fooled_by_a_magenta_subject_pixel():
    # A block that matches the background exactly, but sits inside the hull,
    # must still be foreground: the mask is filled, not merely thresholded.
    #
    # The block must be a genuine hole, not a single pixel: the 5x5 closing
    # kernel in extract() bridges a 1px gap on its own, so a single corrupted
    # pixel passes whether or not the explicit flood-fill hole-fill exists —
    # it doesn't discriminate between the two implementations. A 7x7 block is
    # small enough to sit inside the subject but wide enough that closing
    # alone cannot bridge it (verified empirically: closing alone leaves a
    # >=5x5 hole unrecovered; only the flood-fill step recovers it).
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    ys, xs = np.nonzero(mask)
    cy, cx = int(ys.mean()), int(xs.mean())
    half = 3
    img = s.image.copy()
    img[cy - half:cy + half + 1, cx - half:cx + half + 1] = matte.background_color(s.image)
    mask2, _ = matte.extract(img)
    assert mask2[cy, cx] == 1


def test_keyability_true_for_a_flat_background_hero_image():
    img = _load_hero("outerrim_prayer")
    is_keyable, corner_spread, border_std, border_key_fraction = matte.keyability(img)
    assert is_keyable
    assert abs(corner_spread - 7.8) < 0.5, corner_spread
    assert abs(border_std - 3.6) < 0.5, border_std


def test_keyability_false_for_a_scene_render_that_passes_the_corner_test():
    # paradox is a lit cavern: its four corners happen to agree with each
    # other (corner_spread 1.1, well under threshold) but the border is far
    # from flat (border_std 47.3). This is the test that proves the border
    # term is load-bearing: delete it from keyability() and this reddens,
    # since corner_spread alone would call paradox keyable.
    img = _load_hero("paradox")
    is_keyable, corner_spread, border_std, border_key_fraction = matte.keyability(img)
    assert not is_keyable
    assert corner_spread < matte.CORNER_SPREAD_THRESHOLD, corner_spread
    assert border_std > matte.BORDER_STD_THRESHOLD, border_std
    # The cavern is lit blue-ish (median border hue ~103 half-degrees), so the
    # shadowed-magenta route must not rescue it either.
    assert border_key_fraction < matte.BORDER_KEY_FRACTION_THRESHOLD, border_key_fraction


def test_keyability_false_for_a_scene_render_that_fails_the_corner_test():
    # principia fails on the other axis too: corner_spread 50.9. Note this
    # does NOT prove the corner term is load-bearing the way the paradox
    # test above proves the border term is: principia's border_std (67.2)
    # already exceeds threshold on its own, and empirically that's true of
    # every one of the 5 not-keyable images in the current catalog (see
    # task-2-report.md) — border_std alone currently reproduces the exact
    # same 14/5 split as the full AND rule. Deleting the corner term from
    # keyability() would NOT redden this test, or any other test in this
    # file, against today's art. The corner_spread computation itself is
    # still worth checking directly (this test does that), and the term
    # stays in keyability() as a second line of defense for an image shape
    # neither of these 19 happens to instantiate — a flat border with
    # mismatched corners — but no current fixture proves it's load-bearing.
    img = _load_hero("principia")
    is_keyable, corner_spread, border_std, border_key_fraction = matte.keyability(img)
    assert not is_keyable
    assert corner_spread > matte.CORNER_SPREAD_THRESHOLD, corner_spread
    assert border_key_fraction < matte.BORDER_KEY_FRACTION_THRESHOLD, border_key_fraction


def test_shadowed_magenta_key_is_keyable_and_shadow_is_background():
    # A soft ground shadow keeps the key's hue and saturation but drops its
    # value, pushing corner_spread/border_std past the flat thresholds while
    # remaining background. All 35 "unkeyable" images in the 2026-08-01 full
    # drop were this case (shadowed/vignetted magenta), not scene renders —
    # this is the test that proves the border_key_fraction route is
    # load-bearing: delete it from keyability() and this reddens.
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    base_mask, _ = matte.extract(s.image)
    img = s.image.copy()
    h, w = img.shape[:2]
    shadow = np.zeros((h, w), dtype=bool)
    shadow[h - h // 3:, : w // 3] = True  # covers the bottom-left corner patch
    shadow &= base_mask == 0              # darken background only
    img[shadow] = (img[shadow] * 0.45).astype(np.uint8)

    is_keyable, corner_spread, border_std, border_key_fraction = matte.keyability(img)
    assert is_keyable
    # The flat rule alone would reject this image — the hue route carries it.
    assert corner_spread > matte.CORNER_SPREAD_THRESHOLD, corner_spread
    assert border_key_fraction >= matte.BORDER_KEY_FRACTION_THRESHOLD, border_key_fraction

    mask, _ = matte.extract(img)
    # The shadow must not weld itself onto the silhouette...
    assert mask[shadow].mean() < 0.001, mask[shadow].mean()
    # ...and the subject itself must be unaffected by the darkened background.
    inter = float((mask & base_mask).sum())
    union = float((mask | base_mask).sum())
    assert inter / union > 0.99, inter / union


def test_keyability_matches_the_hero_art_catalog():
    # Hardcoded, not derived: if a future art drop changes which of these 19
    # are keyable, this must go red and force a look, not quietly adapt.
    mismatches = []
    for stem in KEYABLE_HERO_IDS:
        img = _load_hero(stem)
        is_keyable, corner_spread, border_std, border_key_fraction = matte.keyability(img)
        if not is_keyable:
            mismatches.append(f"{stem}: expected keyable, got not-keyable "
                               f"(corner_spread={corner_spread:.1f}, border_std={border_std:.1f})")
    for stem in NOT_KEYABLE_HERO_IDS:
        img = _load_hero(stem)
        is_keyable, corner_spread, border_std, border_key_fraction = matte.keyability(img)
        if is_keyable:
            mismatches.append(f"{stem}: expected not-keyable, got keyable "
                               f"(corner_spread={corner_spread:.1f}, border_std={border_std:.1f})")
    assert not mismatches, "\n".join(mismatches)
