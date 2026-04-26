// Package noise provides simplex-noise primitives used throughout
// planet generation: fractal Brownian motion (FBM), domain warping
// (Phase 1), ridged multifractal (Phase 2), curl noise (Phase 2),
// and Worley noise (Phase 3+).
//
// All functions are pure and deterministic given a seeded
// Generator. Sampling is done in 3D (unit sphere directions)
// to keep generation seamless across cube-map faces.
package noise
