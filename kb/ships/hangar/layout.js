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
  return { x: layout[i].x + (layout[j].x - layout[i].x) * f, length: ships[i].lengthM + (ships[j].lengthM - ships[i].lengthM) * f }
}

export function railPose(x, length) {
  const d = Math.max(22, length * 1.7)
  return { position: [x - d * .18, FLOAT_M + d * .42, d], target: [x, FLOAT_M, 0] }
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

export function parseFilters(search) {
  const q = new URLSearchParams(search)
  const list = key => new Set((q.get(key) || '').split(',').filter(Boolean))
  return { empires: list('empire'), tiers: new Set([...list('tier')].map(Number)), categories: list('cat') }
}

export function formatFilters({ empires, tiers, categories } = {}) {
  const parts = []
  if (empires?.size) parts.push(`empire=${[...empires].join(',')}`)
  if (tiers?.size) parts.push(`tier=${[...tiers].join(',')}`)
  if (categories?.size) parts.push(`cat=${[...categories].join(',')}`)
  return parts.join('&')
}
