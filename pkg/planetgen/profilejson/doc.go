// Package profilejson is the on-disk envelope format for
// per-planet PlanetProfile overrides. The envelope wraps the
// existing types.PlanetProfile with schema versioning and dispatch
// metadata; the wasm contract is unchanged because the envelope is
// unwrapped before the profile reaches the renderer.
//
// See docs/superpowers/specs/2026-05-07-phase-5-per-planet-profile-json-design.md.
package profilejson
