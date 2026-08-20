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
// outermost, used as the fallback when a replay carries no `zones` field of
// its own (see deriveZoneOrder). zone is the authoritative tactical space;
// x/y is presentation layout only (design spec Finding 4: "where they
// disagree, trust zone"). This is not theoretical: a real four-side battle
// (Kitalpha) measured "mid" with a SMALLER mean x/y radius than "engaged" —
// sorting rings by measured radius would draw "mid" inside "engaged",
// actively misleading. Order here is fixed and never derived from
// measurement.
const CANONICAL_ZONE_ORDER = ['engaged', 'inner', 'mid', 'outer'];

// deriveZoneOrder resolves the innermost-to-outermost ring order to draw:
// replay.zones when the replay carries one, or CANONICAL_ZONE_ORDER as a
// fallback. replay.zones is not a measurement — it is the adapter's own
// declared ordering (pkg/battlereplay/adapt.go), the same authority
// CANONICAL_ZONE_ORDER exists to reproduce by hand, and both shipped
// fixtures carry it as `["outer","mid","inner","engaged"]` — nearest-to-
// contact last — so it is reversed here to innermost-first. Deriving from it
// means a renamed or added band shows up in the right place automatically,
// instead of falling into the Map-insertion-order path in zoneRings.
function deriveZoneOrder(replay) {
  if (Array.isArray(replay.zones) && replay.zones.length) {
    return replay.zones.slice().reverse();
  }
  return CANONICAL_ZONE_ORDER;
}

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
  const zoneOrder = options.zoneOrder || CANONICAL_ZONE_ORDER;

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
  for (const zone of zoneOrder) {
    const acc = sums.get(zone);
    if (acc) rings.push({zone, meanRadius: acc.sum / acc.n});
  }
  // A zone name outside zoneOrder entirely (unexpected data — not even named
  // by the replay's own `zones` field) is appended rather than dropped, so
  // it stays visible instead of vanishing silently. Their relative order
  // among themselves follows Map insertion order, which is not deterministic
  // across differently-ordered input; this is now a second safety net, not
  // the normal path — deriveZoneOrder means any band the replay actually
  // names is already in zoneOrder and placed deterministically above.
  for (const [zone, acc] of sums) {
    if (!zoneOrder.includes(zone)) rings.push({zone, meanRadius: acc.sum / acc.n});
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
// bound and running off the end must not produce undefined. A non-finite id
// (a malformed side_id from upstream) falls back to index 0 rather than
// letting NaN reach ctx.fillStyle as undefined.
function sideColour(sideId, theme) {
  const palette = theme.sides;
  const id = Number.isFinite(sideId) ? sideId : 0;
  return palette[Math.abs(id) % palette.length];
}

// BAND_LABEL_STAGGER perpendicular-offsets alternating band labels, in
// canvas pixels, so two adjacent rings whose radii differ by only a few
// pixels — always true near the centre, where ring gaps are smallest —
// don't end up sharing a baseline. See bandLabelAngleDeg for why the axis
// itself also moves.
const BAND_LABEL_STAGGER = 7;

// RING_LABEL_NUDGE is a small radial push so a band label clears the ring's
// own stroke instead of sitting directly on top of it.
const RING_LABEL_NUDGE = 4;

// SPOKE_LABEL_OFFSET pushes a side's label further out along its own spoke.
const SPOKE_LABEL_OFFSET = 10;

// bandLabelAngleDeg picks the direction band labels are drawn along. The
// earlier approach fixed this at +X ("least likely to sit on top of a spoke
// for a two-side battle") — falsified by the primary fixture, Node Beta,
// whose side 2 sits at bearing_mean 4.7 degrees, essentially +X itself. This
// instead measures the widest angular gap between the battle's own side
// bearings and returns its midpoint, so the axis is pushed as far as
// possible from every spoke label regardless of how many sides there are or
// where they sit. Falls back to +X when there are no bearings to avoid.
function bandLabelAngleDeg(sides) {
  const bearings = (sides || [])
    .map(s => s.bearing_mean)
    .filter(Number.isFinite)
    .map(b => ((b % 360) + 360) % 360)
    .sort((a, b) => a - b);
  if (bearings.length === 0) return 0;

  let bestGap = -1;
  let bestStart = 0;
  for (let i = 0; i < bearings.length; i++) {
    const start = bearings[i];
    const next = i + 1 < bearings.length ? bearings[i + 1] : bearings[0] + 360;
    const gap = next - start;
    if (gap > bestGap) {
      bestGap = gap;
      bestStart = start;
    }
  }
  return (bestStart + bestGap / 2) % 360;
}

// bandLabelOffset perpendicular-staggers alternating band labels (by ring
// index) along an axis at angleDeg, so two labels whose ring radii place
// them close together along that axis don't collide. Pure so the placement
// rule can be tested without a canvas.
function bandLabelOffset(index, angleDeg) {
  const perpRad = (angleDeg + 90) * Math.PI / 180;
  const sign = index % 2 === 0 ? 1 : -1;
  return {
    dx: Math.cos(perpRad) * BAND_LABEL_STAGGER * sign,
    dy: Math.sin(perpRad) * BAND_LABEL_STAGGER * sign,
  };
}

// spokeLabelOffset pushes a side's label further out along its own spoke
// direction, rather than a flat "6px up" that only reads correctly for a
// spoke pointing straight up and drifts sideways for every other bearing.
function spokeLabelOffset(bearingDeg, distance) {
  const rad = bearingDeg * Math.PI / 180;
  return {dx: Math.cos(rad) * distance, dy: Math.sin(rad) * distance};
}

// outerRadius is the spoke length: the outermost ring's rOuter. Pulled out of
// drawGround because it is pure computation, not drawing, and the fallback of
// 1 (a battle with no measured zones at all) needs its own test coverage
// rather than living unverified inside an untestable canvas function.
function outerRadius(rings) {
  return rings.length ? rings[rings.length - 1].rOuter : 1;
}

// drawGround lays the table down: zone bands as true circles, one spoke per
// side, and a label per band.
function drawGround(ctx, view, centre, rings, sides, theme) {
  // save/restore brackets every style mutation below (strokeStyle, lineWidth,
  // fillStyle, font, textAlign) so the ship and targeting layers drawn right
  // after this one inherit the canvas's own defaults, not whatever the ground
  // layer left behind.
  ctx.save();
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
  const outer = outerRadius(rings);
  for (const side of sides) {
    // Real data always carries a finite bearing_mean (the adapter's own
    // unit-vector averaging guarantees it); this guards against adapter
    // breakage, not normal operation. Canvas silently no-ops on NaN
    // coordinates rather than throwing, so without this a bad bearing would
    // just drop a spoke with no visible sign of why.
    if (!Number.isFinite(side.bearing_mean)) continue;

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
    const spokeOff = spokeLabelOffset(side.bearing_mean, SPOKE_LABEL_OFFSET);
    ctx.fillText(`SIDE ${side.side_id} (${side.count})`, p.px + spokeOff.dx, p.py + spokeOff.dy);
  }

  // Band labels are drawn along whichever axis sits farthest from every
  // side's spoke (bandLabelAngleDeg), with alternating labels staggered
  // perpendicular to that axis (bandLabelOffset) — the axis choice alone
  // keeps a spoke label from landing on top of the band labels, but doesn't
  // separate adjacent band labels from each other, since ring radii near the
  // centre are close together regardless of angle.
  const labelAngle = bandLabelAngleDeg(sides);
  ctx.fillStyle = theme.ringLabel;
  ctx.font = '10px ui-monospace, monospace';
  ctx.textAlign = 'center';
  for (let i = 0; i < rings.length; i++) {
    const ring = rings[i];
    const end = spokeEnd(labelAngle, ring.rOuter, centre);
    // at, not c: they'd coincide only because project() is axis-aligned. P3
    // replaces project() with a tilted 2.5D projection — that seam is the
    // whole reason it's isolated — and once it does, a label positioned at
    // the centre instead of its own ring's point would silently drift off
    // the band it names.
    const at = project(end.x, end.y, view);
    const nudgeRad = labelAngle * Math.PI / 180;
    const stagger = bandLabelOffset(i, labelAngle);
    const px = at.px + Math.cos(nudgeRad) * RING_LABEL_NUDGE + stagger.dx;
    const py = at.py + Math.sin(nudgeRad) * RING_LABEL_NUDGE + stagger.dy;
    ctx.fillText(ring.zone.toUpperCase(), px, py);
  }

  ctx.restore();
}

// FOOTPRINT_WIDTH is the viewBox width every footprint carries: 1000 units of
// hull plus a 10-unit margin each side.
const FOOTPRINT_WIDTH = 1020;
// FOOTPRINT_HULL_LENGTH is the normalised hull length inside that viewBox.
const FOOTPRINT_HULL_LENGTH = 1000;

// FALLBACK_FOOTPRINT_HEIGHT is a can't-happen guard, not a real height: every
// hull that reaches hullTransform either carries a real footprint height
// (pkg/footprint.Check enforces height > the 20-unit margin) or is the
// synthetic {kind:'missing'} stand-in in drawFrame, which never reaches
// hullTransform. If a height is ever missing anyway, this only needs to be
// finite, not meaningful — it previously reused FOOTPRINT_WIDTH (1020, a
// *width*) for this, which happened to be finite but read as a copy-paste.
const FALLBACK_FOOTPRINT_HEIGHT = 1020;

// hullTransform gives the numbers for drawing a footprint at a table length:
// translate to the ship, rotate to its heading, scale, then shift the
// footprint's own centre to the origin.
function hullTransform(hull, lengthPx) {
  const height = hull.height > 0 ? hull.height : FALLBACK_FOOTPRINT_HEIGHT;
  return {
    scale: lengthPx / FOOTPRINT_HULL_LENGTH,
    cx: FOOTPRINT_WIDTH / 2,
    cy: height / 2,
  };
}

// pathCache avoids rebuilding a Path2D per ship per frame; a 373-participant
// battle would otherwise parse the same handful of paths hundreds of times.
const pathCache = new Map();

function hullPath(hull) {
  if (!hull.d) return null;
  let p = pathCache.get(hull.ship);
  if (!p) {
    p = new Path2D(hull.d);
    pathCache.set(hull.ship, p);
  }
  return p;
}

// drawStationGlyph reproduces the official viewer's station mark — a filled
// hexagon with a circle at each corner inside two concentric rings — so a
// reader who has seen one viewer can read the other.
function drawStationGlyph(ctx, px, py, r, colour) {
  ctx.save();
  ctx.translate(px, py);

  ctx.beginPath();
  for (let i = 0; i < 6; i++) {
    const a = i * Math.PI / 3;
    const x = Math.cos(a) * r;
    const y = Math.sin(a) * r;
    if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
  }
  ctx.closePath();
  ctx.fillStyle = colour;
  ctx.globalAlpha = 0.35;
  ctx.fill();
  ctx.globalAlpha = 1;
  ctx.strokeStyle = colour;
  ctx.lineWidth = 1.5;
  ctx.stroke();

  for (let i = 0; i < 6; i++) {
    const a = i * Math.PI / 3;
    ctx.beginPath();
    ctx.arc(Math.cos(a) * r, Math.sin(a) * r, r * 0.18, 0, Math.PI * 2);
    ctx.fillStyle = colour;
    ctx.fill();
  }

  for (const mult of [1.3, 1.55]) {
    ctx.beginPath();
    ctx.arc(0, 0, r * mult, 0, Math.PI * 2);
    ctx.strokeStyle = colour;
    ctx.globalAlpha = 0.4;
    ctx.lineWidth = 1;
    ctx.stroke();
    ctx.globalAlpha = 1;
  }

  ctx.restore();
}

// drawMissingGlyph marks a ship class with no footprint art. It is deliberately
// unlike a hull — a dashed chevron — so a coverage gap reads as a gap rather
// than as a badly drawn ship.
function drawMissingGlyph(ctx, px, py, r, colour) {
  ctx.save();
  ctx.translate(px, py);
  ctx.beginPath();
  ctx.moveTo(r, 0);
  ctx.lineTo(-r * 0.6, r * 0.6);
  ctx.lineTo(-r * 0.25, 0);
  ctx.lineTo(-r * 0.6, -r * 0.6);
  ctx.closePath();
  ctx.strokeStyle = colour;
  ctx.setLineDash([3, 2]);
  ctx.lineWidth = 1.2;
  ctx.stroke();
  ctx.setLineDash([]);
  ctx.restore();
}

// ARC_SWEEP is how much of a circle a full state bar covers, leaving a gap at
// the bow so the arcs never look like a closed ring.
const ARC_SWEEP = Math.PI * 1.6;
const ARC_START = -Math.PI * 0.8;

// drawStateArcs puts shield outside and hull inside, as fractions of maximum.
// A null fraction is UNKNOWN and draws as a dashed grey ring rather than an
// empty bar, so missing data does not read as a derelict ship.
function drawStateArcs(ctx, px, py, r, state, theme) {
  const bands = [
    {frac: state.shield, radius: r * 1.35, colour: '#6fc8e8'},
    {frac: state.hull, radius: r * 1.15, colour: '#e8b96f'},
  ];

  ctx.save();
  for (const band of bands) {
    ctx.beginPath();
    if (band.frac === null) {
      ctx.arc(px, py, band.radius, ARC_START, ARC_START + ARC_SWEEP);
      ctx.strokeStyle = theme.hullUnknown;
      ctx.setLineDash([2, 3]);
    } else {
      ctx.arc(px, py, band.radius, ARC_START, ARC_START + ARC_SWEEP * band.frac);
      ctx.strokeStyle = band.colour;
    }
    ctx.lineWidth = 1.5;
    ctx.stroke();
    ctx.setLineDash([]);
  }
  ctx.restore();
}

// drawShip draws one combatant: its silhouette at its heading, then its state.
function drawShip(ctx, view, ship, participant, hull, opts) {
  const theme = opts.theme;
  const p = project(ship.x, ship.y, view);
  const lengthPx = hullPixels(hull.scale, opts);
  const colour = opts.dead ? theme.wreck : sideColour(participant.side_id, theme);

  if (hull.kind === 'station') {
    drawStationGlyph(ctx, p.px, p.py, lengthPx * 0.6, colour);
    return;
  }

  // Unlike the station branch above, this doesn't check hull.kind ===
  // 'missing' directly: it's inferred from the absence of path geometry.
  // kind is authoritative for "station" (BuildHullPack sets it explicitly,
  // with no art to check), but a hull class can lack art for reasons kind
  // alone doesn't capture, so checking what actually got parsed is the more
  // direct test of "is there anything to draw here."
  const path = hullPath(hull);
  if (!path) {
    drawMissingGlyph(ctx, p.px, p.py, lengthPx * 0.5, theme.missingArt);
    drawStateArcs(ctx, p.px, p.py, lengthPx * 0.5, opts.state, theme);
    return;
  }

  const t = hullTransform(hull, lengthPx);
  ctx.save();
  ctx.translate(p.px, p.py);
  ctx.rotate(opts.heading);
  ctx.scale(t.scale, t.scale);
  ctx.translate(-t.cx, -t.cy);

  ctx.fillStyle = colour;
  ctx.globalAlpha = opts.dead ? 0.25 : 0.45;
  ctx.fill(path, 'evenodd');
  ctx.globalAlpha = 1;
  ctx.strokeStyle = colour;
  // Undo the scale so the outline is a constant pixel width whatever the hull.
  // t.scale is lengthPx / FOOTPRINT_HULL_LENGTH with lengthPx guarded > 0 by
  // hullPixels (Math.max(1, ...)), so this can never divide by zero.
  ctx.lineWidth = 1.2 / t.scale;
  ctx.stroke(path);
  ctx.restore();

  drawStateArcs(ctx, p.px, p.py, lengthPx * 0.5, opts.state, theme);
}

// busiestTick picks the most informative single frame: the one where the most
// ships are actively targeting something. P1a draws one frame, so it should be
// a frame where the fleet is visibly doing something rather than tick 1, where
// half the participants have not joined yet. When no ship in the replay ever
// targets anything, every frame ties at a target count of 0; the secondary
// key (most live ships present) still picks a frame where the fleet has
// actually assembled, rather than degenerating to frame 0 — the exact tick
// this function exists to avoid — on the very first `count > bestCount`.
function busiestTick(replay) {
  let best = null;
  let bestCount = -1;
  let bestShipCount = -1;
  for (const frame of replay.frames) {
    let count = 0;
    for (const ship of frame.ships) {
      if (ship.target_id) count++;
    }
    const shipCount = frame.ships.length;
    if (count > bestCount || (count === bestCount && shipCount > bestShipCount)) {
      bestCount = count;
      bestShipCount = shipCount;
      best = frame.tick;
    }
  }
  return best;
}

// pickFrame resolves the frame to draw: an explicitly requested tick if it
// exists, else the busiest.
function pickFrame(replay, want) {
  if (want !== null && want !== undefined) {
    const found = replay.frames.find(f => f.tick === want);
    if (found) return found;
  }
  const tick = busiestTick(replay);
  return replay.frames.find(f => f.tick === tick) || replay.frames[0];
}

// TABLE_MARGIN keeps the outermost ring and its labels off the canvas edge.
const TABLE_MARGIN = 60;

// tableBounds expands a battle's ship-position bounds to also encompass the
// outermost zone ring, so the ring — and the side spokes and labels that
// extend to its edge — never draw past what fitView fits on canvas.
//
// fitView only ever sees the bounds it's handed; by default that was
// replay.bounds, which covers where the ships actually are. Ring radius is
// a separate measurement (zoneRings, from mean distance per zone) and can
// exceed the ships' own spread on a small battle: Kitalpha's outer ring
// radius (0.894) is 42% bigger than the ships' vertical half-extent (0.628),
// so with ship-only bounds the ring — and everything anchored to its edge —
// clips off the fitted view. The union with the ship bounds is deliberate:
// on a battle where the ships spread wider than the rings (Node Beta), the
// rings must never shrink the fitted view below what the ships need.
function tableBounds(bounds, centre, outerR) {
  return {
    x_min: Math.min(bounds.x_min, centre.x - outerR),
    x_max: Math.max(bounds.x_max, centre.x + outerR),
    y_min: Math.min(bounds.y_min, centre.y - outerR),
    y_max: Math.max(bounds.y_max, centre.y + outerR),
  };
}

// layoutTable computes everything about a battle's table that does not change
// from tick to tick: the zone rings, the bounds that contain both the ships and
// the outermost ring, and the view that maps model space onto the canvas.
//
// It is pure and depends only on the replay and the canvas size, which is the
// whole point: P1b bakes the static layer once from this, so the whole-battle
// scan inside zoneRings stops happening per frame. It also puts the
// deriveZoneOrder threading somewhere a test can reach without a canvas.
function layoutTable(replay, width, height) {
  const rings = zoneRings(replay.frames, replay.centre, {zoneOrder: deriveZoneOrder(replay)});
  const bounds = tableBounds(replay.bounds, replay.centre, outerRadius(rings));
  const view = fitView(bounds, width, height, TABLE_MARGIN);
  return {rings, bounds, view};
}

// drawFrame renders one tick of the battle onto ctx.
function drawFrame(ctx, replay, hulls, frame, width, height) {
  // TODO(P1b): this save() (and the matching restore() below) isn't wrapped
  // in try/finally. Matches the rest of the file's convention, and today the
  // canvas is discarded on any error anyway — but P1b reuses the same
  // context every frame, and an unbalanced save from a mid-frame throw would
  // then survive into the next frame instead of vanishing with the canvas.
  ctx.save();
  const layout = layoutTable(replay, width, height);
  const rings = layout.rings;
  const view = layout.view;

  ctx.fillStyle = THEME.bg;
  ctx.fillRect(0, 0, width, height);

  drawGround(ctx, view, replay.centre, rings, replay.sides, THEME);

  const shipsById = new Map(frame.ships.map(s => [s.player_id, s]));
  const partById = new Map(replay.participants.map(p => [p.player_id, p]));

  // Targeting lines first, so hulls draw over them.
  ctx.save();
  ctx.strokeStyle = THEME.targetLine;
  ctx.lineWidth = 1;
  for (const ship of frame.ships) {
    const target = ship.target_id ? shipsById.get(ship.target_id) : null;
    if (!target) continue;
    const a = project(ship.x, ship.y, view);
    const b = project(target.x, target.y, view);
    ctx.beginPath();
    ctx.moveTo(a.px, a.py);
    ctx.lineTo(b.px, b.py);
    ctx.stroke();
  }
  ctx.restore();

  for (const ship of frame.ships) {
    const participant = partById.get(ship.player_id);
    if (!participant) continue;
    const hull = hulls[participant.ship_class] || {kind: 'missing', scale: 1};
    const state = hullState(ship, participant, frame.tick);
    drawShip(ctx, view, ship, participant, hull, {
      theme: THEME,
      heading: headingOf(ship, shipsById, replay.centre),
      state,
      dead: state.dead,
    });
  }
  ctx.restore();
}

// fetchJSON fetches and parses one data file, naming the URL and HTTP status
// on failure — without this, a missing file surfaces to initHolotable's catch
// as "Unexpected end of JSON input" from the JSON parser, not as the 404 that
// actually caused it.
async function fetchJSON(url) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${url}: HTTP ${res.status}`);
  return res.json();
}

// initHolotable wires the page: fetch both data files, size the canvas to the
// device, draw one frame.
async function initHolotable() {
  const cfg = window.HOLOTABLE;
  const status = document.getElementById('status');
  const canvas = document.getElementById('table');

  try {
    const [replay, hulls] = await Promise.all([
      fetchJSON(cfg.replayURL),
      fetchJSON(cfg.hullsURL),
    ]);

    const dpr = window.devicePixelRatio || 1;
    const width = canvas.clientWidth;
    const height = canvas.clientHeight;
    canvas.width = width * dpr;
    canvas.height = height * dpr;

    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);

    const params = new URLSearchParams(window.location.search);
    const wanted = params.has('tick') ? Number(params.get('tick')) : null;
    const frame = pickFrame(replay, wanted);

    drawFrame(ctx, replay, hulls, frame, width, height);

    document.getElementById('tick').textContent = String(frame.tick);
    status.textContent = '';
  } catch (err) {
    status.textContent = 'Could not draw the battle: ' + err.message;
  }
}

if (typeof document !== 'undefined') {
  document.addEventListener('DOMContentLoaded', initHolotable);
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    fitView, project, zoneRings, headingOf, hullPixels, hullState,
    spokeEnd, sideColour, outerRadius, drawGround, THEME,
    HULL_PX_PER_SCALE, OUTER_RING_MARGIN, CANONICAL_ZONE_ORDER, deriveZoneOrder,
    bandLabelAngleDeg, bandLabelOffset, spokeLabelOffset,
    hullTransform, drawShip, drawStationGlyph, drawMissingGlyph, drawStateArcs,
    busiestTick, pickFrame, drawFrame, tableBounds, layoutTable,
  };
}
