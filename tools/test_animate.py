import os
import sys

import numpy as np

sys.path.insert(0, os.path.dirname(__file__))
import animate  # noqa: E402


def _synth(h=64, w=48):
    rng = np.random.default_rng(7)
    return rng.random((h, w, 3), dtype=np.float32)


def _loops(fn, imgs, params=None):
    """A frame fn loops if its t=0 and t=1 frames match within rounding."""
    params = params or {}
    f0 = fn(imgs, params, 0.0).astype(np.int16)
    f1 = fn(imgs, params, 1.0).astype(np.int16)
    return int(np.abs(f0 - f1).max())


def test_to_uint8_clamps_and_rounds():
    arr = np.array([[[-0.5, 0.0, 1.0]]], dtype=np.float32)
    out = animate.to_uint8(arr)
    assert out.dtype == np.uint8
    assert out.tolist() == [[[0, 0, 255]]]


def test_ensure_even_pads_odd_dims():
    odd = np.zeros((63, 47, 3), dtype=np.float32)
    even = animate.ensure_even(odd)
    h, w = even.shape[:2]
    assert h % 2 == 0 and w % 2 == 0
    assert (h, w) == (64, 48)


def test_remap_identity_returns_same_image():
    img = _synth()
    h, w = img.shape[:2]
    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    out = animate.remap(img, xs, ys)
    assert np.allclose(out, img, atol=1e-4)


def test_remap_integer_shift():
    img = _synth()
    h, w = img.shape[:2]
    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    # sample from one column to the right -> output shifted left by 1
    out = animate.remap(img, xs + 1.0, ys)
    assert np.allclose(out[:, :-1], img[:, 1:], atol=1e-4)


def test_fold_churn_shape_and_loop():
    img = _synth()
    frame = animate.fold_churn([img], {}, 0.3)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.fold_churn, [img]) <= 1


def test_chromatic_split_shape_and_loop():
    img = _synth()
    frame = animate.chromatic_split([img], {}, 0.5)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.chromatic_split, [img]) <= 1


def test_noise_dissolve_shape_and_loop():
    img = _synth()
    frame = animate.noise_dissolve([img], {"seed": 3}, 0.5)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.noise_dissolve, [img], {"seed": 3}) <= 1


def test_noise_dissolve_midpoint_differs_from_source():
    img = _synth()
    f_mid = animate.noise_dissolve([img], {"seed": 3, "max_noise": 0.85}, 0.5)
    f_start = animate.noise_dissolve([img], {"seed": 3}, 0.0)
    # at the dissolve peak the frame should differ substantially from the source
    assert np.abs(f_mid.astype(np.int16) - f_start.astype(np.int16)).mean() > 5


def test_crossfade_drift_shape_and_loop():
    a = _synth()
    b = _synth() * 0.5  # a visibly different second frame
    frame = animate.crossfade_drift([a, b], {}, 0.5)
    assert frame.shape == a.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.crossfade_drift, [a, b]) <= 1


def test_crossfade_midpoint_is_mostly_b():
    a = np.zeros((64, 48, 3), dtype=np.float32)
    b = np.ones((64, 48, 3), dtype=np.float32)
    mid = animate.crossfade_drift([a, b], {"drift_px": 0.0}, 0.5)
    # at t=0.5 alpha=1 -> should be image b (white)
    assert mid.min() >= 254


import imageio_ffmpeg  # noqa: E402
from PIL import Image  # noqa: E402


def _count_mp4_frames(path):
    reader = imageio_ffmpeg.read_frames(path)
    next(reader)  # first item is the meta dict
    return sum(1 for _ in reader)


def test_render_writes_mp4_with_expected_frame_count(tmp_path):
    src = tmp_path / "src.png"
    Image.fromarray(animate.to_uint8(_synth(66, 50))).save(src)  # odd dims
    out = tmp_path / "clip.mp4"
    written, n = animate.render(
        [str(src)], "fold-churn", {}, duration=1.0, fps=12, out=str(out)
    )
    assert os.path.exists(written)
    assert os.path.getsize(written) > 0
    assert n == 12
    assert _count_mp4_frames(str(out)) == 12


def test_render_rejects_unknown_effect(tmp_path):
    src = tmp_path / "src.png"
    Image.fromarray(animate.to_uint8(_synth())).save(src)
    try:
        animate.render([str(src)], "nope", {}, 1.0, 12, str(tmp_path / "x.mp4"))
        assert False, "expected ValueError"
    except ValueError as e:
        assert "nope" in str(e)


def test_render_rejects_wrong_input_count(tmp_path):
    src = tmp_path / "src.png"
    Image.fromarray(animate.to_uint8(_synth())).save(src)
    try:
        animate.render(
            [str(src)], "crossfade-drift", {}, 1.0, 12, str(tmp_path / "x.mp4")
        )
        assert False, "expected ValueError"
    except ValueError as e:
        assert "2" in str(e)


import json  # noqa: E402


def test_parse_params_typed():
    params = animate.parse_params(["amp_px=12", "freq=2.5", "seed=3"])
    assert params == {"amp_px": 12, "freq": 2.5, "seed": 3}


def test_run_batch_renders_all_and_skips_bad(tmp_path, capsys):
    good = tmp_path / "k.png"
    Image.fromarray(animate.to_uint8(_synth())).save(good)
    spec = {
        "groups": [
            {
                "title": "t",
                "items": [
                    {
                        "file": "ok.mp4",
                        "src": ["k.png"],
                        "effect": "fold-churn",
                        "params": {},
                        "duration": 1.0,
                        "fps": 8,
                    },
                    {
                        "file": "bad.mp4",
                        "src": ["missing.png"],
                        "effect": "fold-churn",
                        "duration": 1.0,
                        "fps": 8,
                    },
                ],
            }
        ]
    }
    spec_path = tmp_path / "anims.json"
    spec_path.write_text(json.dumps(spec))
    ok, fail = animate.run_batch(str(spec_path))
    assert ok == 1 and fail == 1
    assert (tmp_path / "ok.mp4").exists()
    assert not (tmp_path / "bad.mp4").exists()
    # the skipped item names itself on stderr so a batch run is auditable
    assert "missing.png" in capsys.readouterr().err


def _blob(h=80, w=80):
    """A bright disk on black — a stand-in 'subject' for mask tests."""
    img = np.zeros((h, w, 3), dtype=np.float32)
    yy, xx = np.mgrid[0:h, 0:w]
    cy, cx = h / 2.0, w / 2.0
    disk = (yy - cy) ** 2 + (xx - cx) ** 2 < (min(h, w) * 0.25) ** 2
    img[disk] = 0.9
    return img


def test_subject_mask_high_inside_low_outside():
    m = animate.subject_mask(_blob(), {})
    assert m.dtype == np.float32
    assert m.shape == (80, 80)
    assert m[40, 40] > 0.8          # center of the blob
    assert m[2, 2] < 0.2            # corner (background)


def test_subject_mask_empty_on_black():
    m = animate.subject_mask(np.zeros((40, 40, 3), dtype=np.float32), {})
    assert float(m.max()) == 0.0    # no bright blob -> all-zero mask


def test_subject_mask_uses_override_path(tmp_path):
    # a half-white grayscale override should win over auto-detection
    mp = tmp_path / "m.png"
    arr = np.zeros((40, 40), dtype=np.uint8)
    arr[:, :20] = 255
    Image.fromarray(arr).save(mp)
    m = animate.subject_mask(_blob(40, 40), {"mask_path": str(mp)})
    assert m[20, 5] > 0.9 and m[20, 35] < 0.1


def test_largest_component_picks_biggest_blob():
    a = np.zeros((20, 20), dtype=bool)
    a[2:5, 2:5] = True          # small blob (9 px)
    a[10:18, 10:18] = True      # big blob (64 px)
    big = animate._largest_component(a)
    assert big[14, 14] and not big[3, 3]


def test_hyper_warp_shape_and_loop():
    img = _synth()
    frame = animate.hyper_warp([img], {}, 0.3)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.hyper_warp, [img]) <= 1


def test_hyper_warp_identity_at_t0():
    img = _synth()
    f0 = animate.hyper_warp([img], {"amp": 0.4}, 0.0)
    assert np.abs(f0.astype(np.int16) - animate.to_uint8(img).astype(np.int16)).max() <= 1


def test_hyper_warp_protect_keeps_subject_closer():
    img = _blob()
    protected = animate.hyper_warp([img], {"protect_subject": True, "amp": 0.6}, 0.5)
    warped = animate.hyper_warp([img], {"protect_subject": False, "amp": 0.6}, 0.5)
    src = animate.to_uint8(img)
    cy, cx = 40, 40
    d_prot = abs(int(protected[cy, cx, 0]) - int(src[cy, cx, 0]))
    d_warp = abs(int(warped[cy, cx, 0]) - int(src[cy, cx, 0]))
    assert d_prot <= d_warp


def _point_with_curve(h=80, w=80):
    """A single bright star plus a bright vertical 'curve' down the middle."""
    img = np.zeros((h, w, 3), dtype=np.float32)
    img[:, w // 2 - 1:w // 2 + 1] = 0.9   # the fixed curve
    img[20, 20] = 1.0                      # a lone star to streak
    return img


def test_hyperspace_streak_shape_and_loop():
    img = _point_with_curve()
    frame = animate.hyperspace_streak([img], {"seed": 1}, 0.4)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.hyperspace_streak, [img], {"seed": 1}) <= 1


def test_hyperspace_streak_elongates_a_point():
    img = _point_with_curve()
    frame = animate.hyperspace_streak([img], {"seed": 1, "streak_len": 20}, 0.4)
    bright = (frame.max(axis=2) > 120)
    assert bright.sum() > (img.max(axis=2) > 0.5).sum()


def test_hyperspace_streak_keeps_curve():
    img = _point_with_curve()
    frame = animate.hyperspace_streak([img], {"seed": 1}, 0.4)
    assert frame[:, 40].max() > 180


def test_disintegrate_shape_and_loop():
    img = _blob()
    frame = animate.disintegrate([img], {"seed": 2}, 0.3)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.disintegrate, [img], {"seed": 2}) <= 1


def test_disintegrate_midpoint_erodes_body():
    img = _blob()
    mid = animate.disintegrate([img], {"seed": 2}, 0.5)
    src = animate.to_uint8(img)
    cy, cx = 40, 40
    assert abs(int(mid[cy, cx, 0]) - int(src[cy, cx, 0])) > 30


def test_disintegrate_leaves_background_untouched():
    img = _blob()
    src = animate.to_uint8(img)
    for tt in (0.0, 0.5, 1.0):
        frame = animate.disintegrate([img], {"seed": 2}, tt)
        assert np.array_equal(frame[2, 2], src[2, 2])


def test_render_injects_mask_path(tmp_path, monkeypatch):
    # write a keyframe and a sibling .mask.png; render() should pick up the mask
    src = tmp_path / "k.png"
    Image.fromarray(animate.to_uint8(_blob())).save(src)
    mask = tmp_path / "k.png.mask.png"
    Image.fromarray(np.full((80, 80), 200, dtype=np.uint8)).save(mask)

    seen = {}
    real = animate.subject_mask

    def spy(img, params):
        seen["mask_path"] = params.get("mask_path")
        return real(img, params)

    monkeypatch.setattr(animate, "subject_mask", spy)
    out = tmp_path / "o.mp4"
    animate.render([str(src)], "disintegrate", {}, 1.0, 6, str(out))
    assert seen["mask_path"] == str(src) + ".mask.png"
    assert out.exists()


def test_polytope_counts():
    v, e = animate._polytope("tesseract")
    assert v.shape == (16, 4) and len(e) == 32
    v, e = animate._polytope("16-cell")
    assert v.shape == (8, 4) and len(e) == 24
    v, e = animate._polytope("5-cell")
    assert v.shape == (5, 4) and len(e) == 10


def test_polytope_edges_in_range():
    for shape in ("tesseract", "16-cell", "5-cell"):
        v, e = animate._polytope(shape)
        n = v.shape[0]
        for (i, j) in e:
            assert 0 <= i < n and 0 <= j < n and i != j


def test_polytope_unknown_shape_raises():
    try:
        animate._polytope("dodecaplex")
        assert False, "expected ValueError"
    except ValueError as ex:
        assert "dodecaplex" in str(ex)


def test_polytope_overlay_shape_and_loop():
    img = _synth()
    frame = animate.polytope_overlay([img], {"shape": "tesseract"}, 0.3)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.polytope_overlay, [img], {"shape": "tesseract"}) <= 1


def test_polytope_overlay_brightens_dark_base():
    base = np.zeros((120, 120, 3), dtype=np.float32)   # dark base
    frame = animate.polytope_overlay([base], {"shape": "tesseract", "size": 0.8}, 0.25)
    # the wireframe adds light over a dark base
    assert int(frame.sum()) > 0


def test_polytope_overlay_screen_never_darkens():
    base = np.full((120, 120, 3), 0.5, dtype=np.float32)
    src = animate.to_uint8(base)
    frame = animate.polytope_overlay([base], {"shape": "5-cell"}, 0.4)
    # screen blend never reduces a pixel below the base value
    assert int(frame.min()) >= int(src.min()) - 1
