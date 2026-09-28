import { test } from 'node:test'
import assert from 'node:assert/strict'
import { filterShips, layoutLineup, railAt, railPose, nearestIndex, parseFilters, formatFilters, FLOAT_M } from '../../kb/ships/hangar/layout.js'

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
  assert.deepEqual(railAt(l, ships, 0), { x: 0, length: 10 })
  assert.deepEqual(railAt(l, ships, 1.5), { x: (l[1].x + l[2].x) / 2, length: 60 })
  assert.deepEqual(railAt(l, ships, 99), { x: l[2].x, length: 100 })
})

test('rail pose frames bigger ships from further away, looking at float height', () => {
  const small = railPose(0, 10), big = railPose(0, 100)
  assert.equal(small.target[1], FLOAT_M)
  assert.ok(big.position[2] > small.position[2] * 5)
  assert.ok(big.position[1] > small.position[1])
})

test('nearest index and filter query round trip', () => {
  const l = layoutLineup(ships)
  assert.equal(nearestIndex(l, l[1].x + 1), 1)
  const f = parseFilters('?empire=crimson,solarian&tier=1,5&cat=Combat')
  assert.deepEqual([...f.empires], ['crimson', 'solarian'])
  assert.deepEqual([...f.tiers], [1, 5])
  assert.equal(formatFilters(f), 'empire=crimson,solarian&tier=1,5&cat=Combat')
  assert.equal(formatFilters({}), '')
})
