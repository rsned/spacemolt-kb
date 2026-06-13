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
