// Pure layout / rail / filter maths for the hangar lineup (no three.js).
export const FLOAT_M = 3
export const PAN_SHIPS_PER_SEC = .35

const inSet = (set, value) => !set || set.size === 0 || set.has(value)

export function filterShips(ships, { empires, tiers, categories } = {}) {
  return ships.filter(s => inSet(empires, s.empire) && inSet(tiers, s.tier) && inSet(categories, s.category))
}

export function layoutLineup(ships) {
  let x = 0
  return ships.map((s, i) => {
    if (i > 0) {
      const prev = ships[i - 1]
      x += prev.lengthM / 2 + Math.max(4, .35 * Math.max(prev.lengthM, s.lengthM)) + s.lengthM / 2
    }
    return { id: s.id, x }
  })
}

export function railAt(layout, ships, u) {
  const last = layout.length - 1
  const c = Math.max(0, Math.min(last, u)), i = Math.min(last, Math.floor(c)), f = c - i
  const j = Math.min(last, i + 1)
  const h = s => s.heightM ?? s.lengthM * .3
  return { x: layout[i].x + (layout[j].x - layout[i].x) * f, length: ships[i].lengthM + (ships[j].lengthM - ships[i].lengthM) * f,
    height: h(ships[i]) + (h(ships[j]) - h(ships[i])) * f }
}

// Rail framing: the camera looks along a fixed three-quarter direction and backs
// off until the ship's bounding sphere (length x height) fits the smaller of the
// vertical and horizontal fields of view, shrunk for the top/bottom UI bars, with
// padding to spare.
export const FIT_USABLE = .72     // share of the vertical half-fov left after the UI bars
export const FIT_PAD = 1.12       // extra room around the ship
export const RAIL_MIN_DIST = 22
const RAIL_DIR = (() => { const v = [-.18, .42, 1], n = Math.hypot(...v); return v.map(c => c / n) })()
export function railPose(x, length, height = length * .3, { fovDeg = 38, aspect = 16 / 9 } = {}) {
  const halfV = fovDeg * Math.PI / 360
  const halfH = Math.atan(Math.tan(halfV) * aspect)
  const half = Math.min(halfV * FIT_USABLE, halfH * .92)
  const radius = Math.hypot(length, height) / 2
  const d = Math.max(RAIL_MIN_DIST, radius * FIT_PAD / Math.sin(half))
  const target = [x, FLOAT_M + height / 2, 0]
  return { position: target.map((c, k) => c + RAIL_DIR[k] * d), target }
}

// One auto-pan step along the rail; `ended` once u reaches the last ship.
export function panStep(u, dt, n, speed = PAN_SHIPS_PER_SEC) {
  const last = Math.max(0, n - 1)
  const next = Math.min(last, u + dt * speed)
  return { u: next, ended: next >= last }
}

// Frame-rate independent lerp factor for exponential smoothing.
export function smoothFactor(dt, rate = 3) {
  return 1 - Math.exp(-dt * rate)
}

// Scrubber readout: "<name> · <length> m (<i+1>/<n>)" for the ship nearest u.
export function nowText(ships, u) {
  if (!ships.length) return ''
  const i = Math.max(0, Math.min(ships.length - 1, Math.round(u)))
  return `${ships[i].name} · ${ships[i].lengthM} m (${i + 1}/${ships.length})`
}

export function nearestIndex(layout, x) {
  let best = 0
  for (let i = 1; i < layout.length; i++) if (Math.abs(layout[i].x - x) < Math.abs(layout[best].x - x)) best = i
  return best
}

// First index whose ship is at least `length` long (ships sorted by length), else the last.
export function indexAtLength(ships, length) {
  const i = ships.findIndex(s => s.lengthM >= length)
  return i >= 0 ? i : Math.max(0, ships.length - 1)
}

// Inverse of railAt's x: the fractional index at world x, clamped to the line.
export function uAtX(layout, x) {
  const last = layout.length - 1
  if (last <= 0 || x <= layout[0].x) return 0
  if (x >= layout[last].x) return last
  let i = 0
  while (layout[i + 1].x < x) i++
  const span = layout[i + 1].x - layout[i].x
  return i + (span > 0 ? (x - layout[i].x) / span : 0)
}

// Indices within `radius` of i (clamped to [0, n)), nearest first, ahead before behind.
export function nearestFirst(i, n, radius) {
  const out = n > 0 ? [Math.max(0, Math.min(n - 1, i))] : []
  const c = out[0]
  for (let d = 1; d <= radius; d++) {
    if (c + d < n) out.push(c + d)
    if (c - d >= 0) out.push(c - d)
  }
  return out
}

const FILTER_KEYS = [['empires', 'empire'], ['tiers', 'tier'], ['categories', 'cat']]

// ?empire=a&empire=b&tier=1&cat=X (repeated keys, so values may hold commas).
// With `known` sets, values that have no chip are dropped, and old comma-joined
// links (?empire=a,b) are still read.
export function parseFilters(search, known) {
  const q = new URLSearchParams(search), out = {}
  for (const [field, key] of FILTER_KEYS) {
    const ok = known?.[field], conv = field === 'tiers' ? Number : String
    let values = q.getAll(key).filter(Boolean)
    if (ok) values = values.flatMap(v => !ok.has(conv(v)) && v.includes(',') ? v.split(',') : [v])
    out[field] = new Set(values.map(conv).filter(v => !ok || ok.has(v)))
  }
  return out
}

export function formatFilters(filters = {}) {
  const q = new URLSearchParams()
  for (const [field, key] of FILTER_KEYS) for (const v of filters[field] || []) q.append(key, String(v))
  return q.toString()
}

// --- focus + re-pack tweens ---
export const CLICK_SLOP_PX = 5
export const FOCUS_MIN_DIST = 8

// Smoothstep-style cubic ease, clamped to [0, 1].
export function easeInOut(t) {
  const c = Math.max(0, Math.min(1, t))
  return c < .5 ? 4 * c * c * c : 1 - (-2 * c + 2) ** 3 / 2
}

export function tweenX(from, to, t) {
  return from + (to - from) * easeInOut(t)
}

// Re-pack plan: each ship in the new layout moves from its previous x (or appears in place).
export function repackMoves(prevX, layout) {
  return layout.map(({ id, x }) => ({ id, from: prevX.has(id) ? prevX.get(id) : x, to: x }))
}

// pointerdown → pointerup counts as a click only if the pointer barely moved.
export function isClick(down, up, slop = CLICK_SLOP_PX) {
  return Math.hypot(up.x - down.x, up.y - down.y) <= slop
}

// Orbit start pose: front-above-left of the hull centre, ~1.3 × length away.
export function focusPose(center, length) {
  const d = Math.max(FOCUS_MIN_DIST, length * 1.3)
  const dir = [-.3, .45, 1], n = Math.hypot(...dir)
  return { position: center.map((c, i) => c + dir[i] / n * d), target: [...center] }
}
