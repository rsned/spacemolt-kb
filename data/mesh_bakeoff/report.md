# Image-to-3D bake-off: TripoSR vs the footprint pipeline
(Agent-authored; saved by controller — the agent's Write of report files is hook-blocked.)

Model: TripoSR (stabilityai/TripoSR, ungated), VAST-AI-Research/TripoSR @ 107cefd. SF3D skipped (HF-gated, no token on box). torchmcubes replaced by a skimage marching-cubes shim (no nvcc on box); axis convention irrelevant (frame discovered from mesh). rembg on CPU onnxruntime (CUDA13 vs bundled-12 mismatch).

32 images processed (18 batch ships — 4 sourced from the original webp drop — + 7 dup pairs + comet), 0 outright failures. TripoSR: 168s total warm; mean 3.57s/image. mesh_footprint.py: mean 10.7s/image.

Dimensional bounds on mesh profiles: 31/32 pass; comet fails at 12.27 vs 12.0 ceiling. 6/32 frame_ambiguous.

## Headline: two-view self-consistency mostly FAILS
Only archive/solarian_archive agree tightly (0.8% aspect delta, r 0.83). The other 6 pairs disagree 10-57% on aspect; 4/7 have w-profile r < 0.6. Root cause (eyeballed on precept, the worst pair): TripoSR's reconstructed aspect tracks the photograph's apparent foreshortening — top-down shot reads stubby (1.22), side-on shot reads elongated (2.42) — a per-view-plausible, angle-VARIANT reconstruction. This is the hallucination the test exists to catch.

Eyeball: bonanza recognizable (nacelle split matches concave stations); solarian_archive clean with a hazy hallucinated trailing edge that didn't corrupt the footprint; magenta edge-fringing from rembg's matte noted, no visible distortion yet — watch at scale.
