'use strict';
// Holotable — a top-down tactical replay of a SpaceMolt battle.
//
// The table is RADIAL: concentric zone bands around a centre, with each side
// holding a spoke that its ships advance inward and retreat outward along.
// Sides are not limited to two.
//
// Everything above the draw layer is a pure function exported for node --test.
// This file reads only the replay model and the hull pack; it never learns a
// ship name, a catalog value, or an API field the adapter did not normalise.
//
// Design: docs/superpowers/specs/2026-08-16-battle-holotable-visualizer-design.md

// HULL_PX_PER_SCALE sets how many pixels of hull length one unit of catalog
// scale buys. It is the first knob to turn after seeing a render: too small and
// the silhouettes are unreadable, too large and an engaged ball becomes soup.
const HULL_PX_PER_SCALE = 14;

// OUTER_RING_MARGIN pushes the outermost ring past the ships sitting on it, so
// the table has a rim rather than clipping its own outer band.
const OUTER_RING_MARGIN = 1.12;

// DEGENERATE_SPAN is the model-space span assumed when every participant sits
// at effectively one point, which would otherwise divide by zero.
const DEGENERATE_SPAN = 1;

// fitView maps model coordinates onto the canvas: uniform scale, model bounds
// centred, the tighter axis winning so nothing is cropped.
function fitView(bounds, width, height, margin) {
  const m = margin || 0;
  let spanX = bounds.x_max - bounds.x_min;
  let spanY = bounds.y_max - bounds.y_min;
  if (!(spanX > 0)) spanX = DEGENERATE_SPAN;
  if (!(spanY > 0)) spanY = DEGENERATE_SPAN;

  const scale = Math.min((width - 2 * m) / spanX, (height - 2 * m) / spanY);
  const midX = (bounds.x_min + bounds.x_max) / 2;
  const midY = (bounds.y_min + bounds.y_max) / 2;

  return {scale, ox: width / 2 - midX * scale, oy: height / 2 - midY * scale};
}

// project maps one model point to canvas pixels.
//
// Y is deliberately NOT flipped. The replay model's side bearings were computed
// with atan2(y, x) in model space and canvas Y also grows downward, so leaving
// it alone keeps ship headings, side spokes and ring geometry in one
// convention. Flipping here would silently mirror every heading.
//
// This is the single seam P3's 2.5D view replaces.
function project(x, y, view) {
  return {px: x * view.scale + view.ox, py: y * view.scale + view.oy};
}

// zoneRings measures each zone band's radius from the data rather than assuming
// fixed radii, and orders the bands by what it measures rather than trusting
// the order the zones arrived in. Boundaries sit at the midpoints between
// adjacent means.
function zoneRings(frames, centre, opts) {
  const options = opts || {};
  const outerMargin = options.outerMargin || OUTER_RING_MARGIN;

  const sums = new Map();
  for (const frame of frames) {
    for (const ship of frame.ships) {
      // A carried-forward state repeats a stale position and would drag the
      // measurement toward wherever the ship was last seen.
      if (ship.stale) continue;
      const r = Math.hypot(ship.x - centre.x, ship.y - centre.y);
      const acc = sums.get(ship.zone) || {sum: 0, n: 0};
      acc.sum += r;
      acc.n += 1;
      sums.set(ship.zone, acc);
    }
  }

  const rings = [];
  for (const [zone, acc] of sums) {
    rings.push({zone, meanRadius: acc.sum / acc.n});
  }
  rings.sort((a, b) => a.meanRadius - b.meanRadius);

  for (let i = 0; i < rings.length; i++) {
    rings[i].rInner = i === 0 ? 0 : (rings[i - 1].meanRadius + rings[i].meanRadius) / 2;
    rings[i].rOuter = i === rings.length - 1
      ? rings[i].meanRadius * outerMargin
      : (rings[i].meanRadius + rings[i + 1].meanRadius) / 2;
  }

  return rings;
}

// headingOf is the rotation to draw a ship at. Bow toward its target if it has
// a live one, else inward toward the centre — the axis its advance and retreat
// run along. It is never a mirror; on a radial table facing follows the ship's
// own geometry.
function headingOf(ship, shipsById, centre) {
  const target = ship.target_id ? shipsById.get(ship.target_id) : null;
  if (target) {
    const dx = target.x - ship.x;
    const dy = target.y - ship.y;
    // A co-located target gives no direction; fall through to the centre.
    if (dx !== 0 || dy !== 0) return Math.atan2(dy, dx);
  }

  const cx = centre.x - ship.x;
  const cy = centre.y - ship.y;
  if (cx === 0 && cy === 0) return 0; // sitting on the centre; any heading is arbitrary
  return Math.atan2(cy, cx);
}

// hullPixels converts catalog hull scale to drawn length, so a scale-1 cobble
// and a scale-4 junk_convoy share a table at their real relative sizes.
function hullPixels(scale, opts) {
  const options = opts || {};
  const perScale = options.hullPxPerScale || HULL_PX_PER_SCALE;
  return perScale * Math.max(1, scale || 1);
}

// hullState turns raw hull and shield readings into drawable fractions.
//
// A null fraction means UNKNOWN and must be drawn as such. The battle log reads
// hull 0 for some live participants, including on tick 1 with full shields and
// no damage taken, so a bare zero cannot be trusted to mean destroyed. The
// adapter's destroyed_at_tick is what actually settles it: past that tick the
// ship is dead, before it a zero is missing data.
function hullState(ship, participant, tick) {
  const destroyedAt = participant.destroyed_at_tick || 0;
  const dead = destroyedAt > 0 && tick >= destroyedAt;

  return {
    hull: fractionOf(ship.hull, participant.max_hull, dead),
    shield: fractionOf(ship.shield, participant.max_shield, dead),
    dead,
  };
}

// fractionOf returns value/max, or null when there is nothing trustworthy to
// draw. A zero on a live ship is unknown; a zero on a dead one is a real zero.
function fractionOf(value, max, dead) {
  if (!(max > 0)) return null;
  if (!(value > 0)) return dead ? 0 : null;
  return Math.min(1, value / max);
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    fitView, project, zoneRings, headingOf, hullPixels, hullState,
    HULL_PX_PER_SCALE, OUTER_RING_MARGIN,
  };
}
