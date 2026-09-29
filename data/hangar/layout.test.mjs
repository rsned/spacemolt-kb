import { test } from 'node:test'
import assert from 'node:assert/strict'
import { filterShips, layoutLineup, railAt, railPose, nearestIndex, parseFilters, formatFilters, FLOAT_M, panStep, smoothFactor, nowText, PAN_SHIPS_PER_SEC, easeInOut, tweenX, repackMoves, isClick, CLICK_SLOP_PX, focusPose, indexAtLength, uAtX, nearestFirst, FIT_USABLE } from '../../kb/ships/hangar/layout.js'

const ships = [
  { id: 'a', lengthM: 10, empire: 'crimson', tier: 1, category: 'Combat' },
  { id: 'b', lengthM: 20, empire: 'solarian', tier: 2, category: 'Commercial' },
  { id: 'c', lengthM: 100, empire: 'crimson', tier: 5, category: 'Combat' },
]

test('layout packs centres with size-proportional gaps', () => {
  const l = layoutLineup(ships)
  assert.equal(l[0].x, 0)
  assert.equal(l[1].x, 5 + 7 + 10)          // half a + gap max(4, .35*20)=7 + half b
  assert.equal(l[2].x, l[1].x + 10 + 35 + 50)
})

test('filters combine and empty sets mean all', () => {
  assert.deepEqual(filterShips(ships, {}).map(s => s.id), ['a', 'b', 'c'])
  assert.deepEqual(filterShips(ships, { empires: new Set(['crimson']) }).map(s => s.id), ['a', 'c'])
  assert.deepEqual(filterShips(ships, { empires: new Set(['crimson']), tiers: new Set([5]) }).map(s => s.id), ['c'])
})

test('rail interpolates between ships and clamps', () => {
  const l = layoutLineup(ships)
  // Ships without heightM fall back to .3 x length for framing.
  assert.deepEqual(railAt(l, ships, 0), { x: 0, length: 10, height: 3 })
  assert.deepEqual(railAt(l, ships, 1.5), { x: (l[1].x + l[2].x) / 2, length: 60, height: 18 })
  assert.deepEqual(railAt(l, ships, 99), { x: l[2].x, length: 100, height: 30 })
  assert.equal(railAt(layoutLineup([{ id: 'h', lengthM: 50, heightM: 20 }]), [{ id: 'h', lengthM: 50, heightM: 20 }], 0).height, 20)
})

test('rail pose frames bigger ships from further away, looking at the ship centre', () => {
  const small = railPose(0, 10), big = railPose(0, 100)
  assert.equal(small.target[1], FLOAT_M + 10 * .3 / 2)
  assert.ok(big.position[2] > small.position[2] * 5)
  assert.ok(big.position[1] > small.position[1])
})

test('nearest index and filter query round trip', () => {
  const l = layoutLineup(ships)
  assert.equal(nearestIndex(l, l[1].x + 1), 1)
  const f = parseFilters('?empire=crimson&empire=solarian&tier=1&tier=5&cat=Combat')
  assert.deepEqual([...f.empires], ['crimson', 'solarian'])
  assert.deepEqual([...f.tiers], [1, 5])
  assert.equal(formatFilters(f), 'empire=crimson&empire=solarian&tier=1&tier=5&cat=Combat')
  assert.equal(formatFilters({}), '')
})

const KNOWN = { empires: new Set(['crimson', 'solarian']), tiers: new Set([1, 5]), categories: new Set(['Combat', 'Salvage, Heavy']) }

test('filter values with commas and spaces survive a round trip', () => {
  const f = { empires: new Set(), tiers: new Set(), categories: new Set(['Salvage, Heavy', 'Combat']) }
  const q = formatFilters(f)
  assert.deepEqual([...parseFilters('?' + q, KNOWN).categories], ['Salvage, Heavy', 'Combat'])
})

test('parseFilters drops values with no chip when given the known sets', () => {
  const f = parseFilters('?empire=crimson&empire=klingon&tier=9&tier=5&tier=x&cat=Nope', KNOWN)
  assert.deepEqual([...f.empires], ['crimson'])
  assert.deepEqual([...f.tiers], [5])
  assert.deepEqual([...f.categories], [])
})

test('parseFilters still reads old comma-joined links against the known sets', () => {
  const f = parseFilters('?empire=crimson,solarian&tier=1,5', KNOWN)
  assert.deepEqual([...f.empires], ['crimson', 'solarian'])
  assert.deepEqual([...f.tiers], [1, 5])
})

test('indexAtLength finds the first ship at least that long, else the last', () => {
  const l = [{ lengthM: 10 }, { lengthM: 20 }, { lengthM: 20 }, { lengthM: 100 }]
  assert.equal(indexAtLength(l, 0), 0)
  assert.equal(indexAtLength(l, 10), 0)
  assert.equal(indexAtLength(l, 15), 1)
  assert.equal(indexAtLength(l, 20), 1)
  assert.equal(indexAtLength(l, 99.5), 3)
  assert.equal(indexAtLength(l, 500), 3)
  assert.equal(indexAtLength([], 5), 0)
})

test('uAtX inverts the rail: fractional index for a world x, clamped', () => {
  const l = layoutLineup(ships)
  assert.equal(uAtX(l, 0), 0)
  assert.equal(uAtX(l, l[1].x), 1)
  assert.equal(uAtX(l, (l[1].x + l[2].x) / 2), 1.5)
  assert.equal(uAtX(l, -50), 0)
  assert.equal(uAtX(l, 1e6), 2)
  for (const u of [.25, 1.3]) assert.ok(Math.abs(uAtX(l, railAt(l, ships, u).x) - u) < 1e-9)
  assert.equal(uAtX([], 5), 0)
})

test('nearestFirst orders a clamped window around i by distance, ahead first on ties', () => {
  assert.deepEqual(nearestFirst(5, 10, 2), [5, 6, 4, 7, 3])
  assert.deepEqual(nearestFirst(0, 10, 2), [0, 1, 2])
  assert.deepEqual(nearestFirst(9, 10, 2), [9, 8, 7])
  assert.deepEqual(nearestFirst(0, 0, 2), [])
})

test('rail pose gives small ships breathing room (min distance 22 m)', () => {
  // Minimum framing distance for tiny ships.
  const dist = p => Math.hypot(p.position[0] - p.target[0], p.position[1] - p.target[1], p.position[2] - p.target[2])
  assert.ok(Math.abs(dist(railPose(0, 5)) - 22) < 1e-9)
})

test('auto-pan advances at ships/sec and stops at the end of the line', () => {
  assert.equal(PAN_SHIPS_PER_SEC, .35)
  assert.deepEqual(panStep(0, 2, 10), { u: .7, ended: false })
  assert.deepEqual(panStep(8.9, 1, 10), { u: 9, ended: true })
  assert.deepEqual(panStep(9, 1, 10), { u: 9, ended: true })
  assert.deepEqual(panStep(0, 1, 1), { u: 0, ended: true })
  assert.deepEqual(panStep(0, 1, 0), { u: 0, ended: true })
})

test('smoothing factor is 1 - exp(-dt * rate), bounded to [0, 1)', () => {
  assert.equal(smoothFactor(0), 0)
  assert.ok(Math.abs(smoothFactor(.1) - (1 - Math.exp(-.3))) < 1e-12)
  assert.ok(smoothFactor(10) > .99 && smoothFactor(10) < 1)
})

test('readout names the nearest ship with its 1-based position', () => {
  const l = [{ name: 'Alpha', lengthM: 12 }, { name: 'Beta', lengthM: 40 }]
  assert.equal(nowText(l, 0.4), 'Alpha · 12 m (1/2)')
  assert.equal(nowText(l, 0.6), 'Beta · 40 m (2/2)')
  assert.equal(nowText(l, 7), 'Beta · 40 m (2/2)')
  assert.equal(nowText([], 0), '')
})

test('easeInOut is a clamped smooth 0→1 curve', () => {
  assert.equal(easeInOut(0), 0)
  assert.equal(easeInOut(1), 1)
  assert.equal(easeInOut(.5), .5)
  assert.equal(easeInOut(-1), 0)
  assert.equal(easeInOut(2), 1)
  assert.ok(easeInOut(.1) < .1 && easeInOut(.9) > .9)
})

test('tweenX eases from old to new x and holds at the ends', () => {
  assert.equal(tweenX(10, 30, 0), 10)
  assert.equal(tweenX(10, 30, .5), 20)
  assert.equal(tweenX(10, 30, 1), 30)
  assert.equal(tweenX(10, 30, 5), 30)
})

test('repack moves each visible ship from its previous x; newcomers start in place', () => {
  const prev = new Map([['a', 0], ['c', 200]])
  const next = [{ id: 'a', x: 0 }, { id: 'b', x: 22 }, { id: 'c', x: 117 }]
  assert.deepEqual(repackMoves(prev, next), [
    { id: 'a', from: 0, to: 0 },
    { id: 'b', from: 22, to: 22 },
    { id: 'c', from: 200, to: 117 },
  ])
})

test('a pointer that moves past the slop is a drag, not a click', () => {
  assert.equal(CLICK_SLOP_PX, 5)
  assert.equal(isClick({ x: 10, y: 10 }, { x: 10, y: 10 }), true)
  assert.equal(isClick({ x: 10, y: 10 }, { x: 13, y: 14 }), true)   // distance 5
  assert.equal(isClick({ x: 10, y: 10 }, { x: 14, y: 14 }), false)
  assert.equal(isClick({ x: 0, y: 0 }, { x: 8, y: 0 }, 10), true)
})

test('focus pose looks at the ship centre from about 1.3 x length away', () => {
  const p = focusPose([100, 5, 0], 50)
  assert.deepEqual(p.target, [100, 5, 0])
  const d = Math.hypot(p.position[0] - 100, p.position[1] - 5, p.position[2])
  assert.ok(Math.abs(d - 65) < 1e-9)
  assert.ok(p.position[1] > 5 && p.position[2] > 0)
  const tiny = focusPose([0, 3, 0], 2)   // never inside a tiny hull
  assert.ok(Math.abs(Math.hypot(...tiny.position.map((v, i) => v - tiny.target[i])) - 8) < 1e-9)
})

test('railPose fits the whole ship (bounding sphere + padding) inside the usable field of view', () => {
  const dist = p => Math.hypot(p.position[0] - p.target[0], p.position[1] - p.target[1], p.position[2] - p.target[2])
  const view = { fovDeg: 38, aspect: 16 / 9 }
  const L = 300, H = 90, pose = railPose(0, L, H, view)
  const r = Math.hypot(L, H) / 2
  // the sphere's angular radius must sit inside the usable half-fov with padding to spare
  const half = Math.asin(r / dist(pose)) * 180 / Math.PI
  assert.ok(half < 19 * FIT_USABLE, `half-angle ${half}`)
  assert.equal(pose.target[1], FLOAT_M + H / 2)
  // a tall narrow window needs more distance than a wide one
  assert.ok(dist(railPose(0, L, H, { fovDeg: 38, aspect: .5 })) > dist(pose))
})
