// Package shipglyph renders top-down blueprint-style ship outlines as SVG.
//
// A ship's shape is described by a Descriptor: a list of hull parts along the
// spine plus appendages and weapon mount zones, all in a normalized coordinate
// space where t runs 0 at the nose to 1 at the tail and y is a signed offset
// from the centerline. Descriptors are produced by Infer from catalog stats and
// may be overridden field-by-field by hand-authored JSON overlays.
//
// Rendering is deterministic: all pseudo-random variation derives from an
// FNV-1a hash of the ship ID, so regenerating produces byte-identical output.
package shipglyph
