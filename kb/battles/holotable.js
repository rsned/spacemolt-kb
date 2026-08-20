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

// CANONICAL_ZONE_ORDER is the game's own tactical band order, innermost to
// outermost. zone is the authoritative tactical space; x/y is presentation
// layout only (design spec Finding 4: "where they disagree, trust zone").
// This is not theoretical: a real four-side battle (Kitalpha) measured "mid"
// with a SMALLER mean x/y radius than "engaged" — sorting rings by measured
// radius would draw "mid" inside "engaged", actively misleading. Order here
// is fixed and never derived from measurement.
const CANONICAL_ZONE_ORDER = ['engaged', 'inner', 'mid', 'outer'];

// MIN_RING_GAP_RATIO / MIN_RING_GAP_FLOOR set the minimum growth enforced
// between one ring's effective radius and the next when computing boundaries,
// so a measured inversion (see CANONICAL_ZONE_ORDER) can never collapse a
// ring to zero width or push a boundary back inside the previous band. The
// ratio scales the gap with the battle's own size; the floor covers the
// innermost ring, whose effective radius can be 0 (ships sitting on the
// centre), where a purely multiplicative gap would also be 0.
const MIN_RING_GAP_RATIO = 0.15;
const MIN_RING_GAP_FLOOR = 0.01;

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

// zoneRings measures each zone band's radius from the data, but orders the
// bands by CANONICAL_ZONE_ORDER, never by what it measures: zone is the
// game's authoritative tactical space, x/y is layout that can and does
// invert (see CANONICAL_ZONE_ORDER). Boundaries sit at the midpoints between
// adjacent EFFECTIVE means — a monotonic sequence derived from the measured
// means — so a boundary can never land inside the previous band even when
// the raw measurement disagrees with canonical order.
//
// The returned array carries an extra `agreesWithMeasurement` property (not
// a per-ring field, so `rings` stays a plain array for callers that only
// want `{zone, meanRadius, rInner, rOuter}`): false whenever the raw means
// did not already increase in canonical order, so a battle where geometry
// and zone semantics disagree stays visible instead of being silently
// smoothed away by the enforcement below.
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
  for (const zone of CANONICAL_ZONE_ORDER) {
    const acc = sums.get(zone);
    if (acc) rings.push({zone, meanRadius: acc.sum / acc.n});
  }
  // A zone name outside the four known bands (unexpected data) is appended
  // rather than dropped, so it stays visible instead of vanishing silently.
  // Their relative order among themselves follows Map insertion order, which
  // is not deterministic across differently-ordered input; harmless today
  // since no real zone name falls outside CANONICAL_ZONE_ORDER.
  for (const [zone, acc] of sums) {
    if (!CANONICAL_ZONE_ORDER.includes(zone)) rings.push({zone, meanRadius: acc.sum / acc.n});
  }

  let agreesWithMeasurement = true;
  for (let i = 1; i < rings.length; i++) {
    if (rings[i].meanRadius < rings[i - 1].meanRadius) {
      agreesWithMeasurement = false;
      break;
    }
  }

  // effective[i] is a running maximum of the measured means with an enforced
  // minimum gap over effective[i-1], so it is always non-decreasing by at
  // least that gap regardless of what the raw means do. meanRadius itself is
  // left untouched for diagnostics; effective only feeds the boundaries.
  const effective = [];
  for (let i = 0; i < rings.length; i++) {
    if (i === 0) {
      effective.push(rings[i].meanRadius);
      continue;
    }
    const gap = effective[i - 1] * MIN_RING_GAP_RATIO + MIN_RING_GAP_FLOOR;
    effective.push(Math.max(rings[i].meanRadius, effective[i - 1] + gap));
  }

  for (let i = 0; i < rings.length; i++) {
    rings[i].rInner = i === 0 ? 0 : (effective[i - 1] + effective[i]) / 2;
    rings[i].rOuter = i === rings.length - 1
      ? effective[i] * outerMargin
      : (effective[i] + effective[i + 1]) / 2;
  }

  // A single zone whose ships sit exactly on the centre (meanRadius 0) — a
  // one-participant frame, or every ship stacked at one point — makes the
  // only ring's rOuter equal its rInner (both 0, since outerMargin scales a
  // zero). centre is the position bounds' midpoint, so a one-ship frame lands
  // exactly there, not merely close to it: this is reachable, not academic.
  // Same treatment fitView already gives zero-span bounds via DEGENERATE_SPAN.
  const last = rings[rings.length - 1];
  if (last && !(last.rOuter > last.rInner)) last.rOuter = last.rInner + MIN_RING_GAP_FLOOR;

  rings.agreesWithMeasurement = agreesWithMeasurement;
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

// THEME is the holo palette, carried over from the point-cloud demo: a dark
// field with cyan structure. Side colours must cover at least four sides —
// three- and four-way battles occur and the upper bound is unknown, so the list
// is cycled rather than indexed blindly.
const THEME = {
  bg: '#05080d',
  ring: 'rgba(90, 190, 220, 0.20)',
  ringLabel: 'rgba(120, 200, 225, 0.55)',
  spoke: 'rgba(90, 190, 220, 0.14)',
  grid: 'rgba(60, 130, 160, 0.10)',
  hullUnknown: 'rgba(160, 160, 170, 0.5)',
  targetLine: 'rgba(120, 210, 235, 0.16)',
  wreck: 'rgba(140, 90, 80, 0.55)',
  missingArt: 'rgba(230, 170, 90, 0.9)',
  // Six colours: four-way battles are confirmed and the upper bound on
  // sides is unknown, so the list is cycled rather than indexed blindly.
  sides: ['#4fd0e8', '#e8734f', '#7fe08a', '#d9a0e8', '#e8d24f', '#8f9ae8'],
};

// spokeEnd converts a side's mean bearing into a model-space point at radius r.
// The bearing is degrees in the same atan2 convention the adapter used, so this
// is a plain polar conversion and inherits the unit-vector averaging the
// adapter already did.
function spokeEnd(bearingDeg, radius, centre) {
  const rad = bearingDeg * Math.PI / 180;
  return {x: centre.x + Math.cos(rad) * radius, y: centre.y + Math.sin(rad) * radius};
}

// sideColour cycles the palette, because the number of sides has no known upper
// bound and running off the end must not produce undefined.
function sideColour(sideId, theme) {
  const palette = theme.sides;
  return palette[Math.abs(sideId) % palette.length];
}

// drawGround lays the table down: zone bands as true circles, one spoke per
// side, and a label per band.
function drawGround(ctx, view, centre, rings, sides, theme) {
  const c = project(centre.x, centre.y, view);

  // Bands, outermost first so inner rings draw over the fill.
  for (let i = rings.length - 1; i >= 0; i--) {
    const r = rings[i].rOuter * view.scale;
    ctx.beginPath();
    ctx.arc(c.px, c.py, r, 0, Math.PI * 2);
    ctx.strokeStyle = theme.ring;
    ctx.lineWidth = 1;
    ctx.stroke();
  }

  // Spokes: each side's axis of advance and retreat.
  const outer = rings.length ? rings[rings.length - 1].rOuter : 1;
  for (const side of sides) {
    const end = spokeEnd(side.bearing_mean, outer, centre);
    const p = project(end.x, end.y, view);
    ctx.beginPath();
    ctx.moveTo(c.px, c.py);
    ctx.lineTo(p.px, p.py);
    ctx.strokeStyle = theme.spoke;
    ctx.lineWidth = 1;
    ctx.stroke();

    ctx.fillStyle = sideColour(side.side_id, theme);
    ctx.font = '11px ui-monospace, monospace';
    ctx.textAlign = 'center';
    ctx.fillText(`SIDE ${side.side_id} (${side.count})`, p.px, p.py - 6);
  }

  // Band labels along the +X axis, where a spoke is least likely to sit on top
  // of them for a two-side battle.
  ctx.fillStyle = theme.ringLabel;
  ctx.font = '10px ui-monospace, monospace';
  ctx.textAlign = 'left';
  for (const ring of rings) {
    const at = project(centre.x + ring.rOuter, centre.y, view);
    ctx.fillText(ring.zone.toUpperCase(), at.px + 4, c.py - 4);
  }
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    fitView, project, zoneRings, headingOf, hullPixels, hullState,
    spokeEnd, sideColour, drawGround, THEME,
    HULL_PX_PER_SCALE, OUTER_RING_MARGIN, CANONICAL_ZONE_ORDER,
  };
}
