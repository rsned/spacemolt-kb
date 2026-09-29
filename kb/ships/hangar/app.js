import * as THREE from 'three'
import { GLTFLoader } from 'three/addons/loaders/GLTFLoader.js'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { filterShips, layoutLineup, railAt, railPose, parseFilters, formatFilters, FLOAT_M, panStep, smoothFactor, nowText, easeInOut, tweenX, repackMoves, isClick, focusPose, indexAtLength, uAtX, nearestFirst } from './layout.js'

const ACCENT = { solarian: '#c9a227', crimson: '#e63946', nebula: '#2f9e6a', outerrim: '#2fb6c4', voidborn: '#9b6bff', pirate: '#ff6540', independent: '#8bd7ff' }
const LOAD_WINDOW = 14
const MAX_LOADS = 4          // concurrent model downloads
const IDLE_RESUME_MS = 6000
const WHEEL_SHIPS_PER_NOTCH = .25
const FOCUS_FLY_S = .8
const REPACK_S = .6
const accentOf = empire => ACCENT[empire] || ACCENT.independent
const EMPIRE_NAME = { solarian: 'Solarian', crimson: 'Crimson', nebula: 'Nebula', outerrim: 'Outer Rim', voidborn: 'Voidborn', pirate: 'Pirate', independent: 'Independent' }
const canvas = document.getElementById('stage'), noteEl = document.getElementById('note')
function fail(message) { noteEl.textContent = message; noteEl.classList.add('error') }
let renderer
try { renderer = new THREE.WebGLRenderer({ canvas, antialias: true }) } catch (err) {
  fail('The hangar needs WebGL, which this browser or device has turned off or does not support.')
  throw err
}
renderer.setPixelRatio(Math.min(2, devicePixelRatio))
renderer.shadowMap.enabled = true
renderer.shadowMap.type = THREE.PCFSoftShadowMap
renderer.toneMapping = THREE.ACESFilmicToneMapping
const scene = new THREE.Scene()
scene.background = new THREE.Color(0x000000)
scene.fog = new THREE.Fog(0x000000, 200, 900)
const camera = new THREE.PerspectiveCamera(38, 1, .5, 20000)

// Studio: light floor fading into black space, key light with soft shadows.
// Untextured, so it simply follows the camera in x (like the stars) to cover the whole line.
const floor = new THREE.Mesh(new THREE.PlaneGeometry(40000, 6000), new THREE.MeshStandardMaterial({ color: 0x9fb1c2, roughness: .38, metalness: .08 }))
floor.rotation.x = -Math.PI / 2; floor.receiveShadow = true; scene.add(floor)
scene.add(new THREE.HemisphereLight(0xdfe8f2, 0x3a4550, 1.1))
const key = new THREE.DirectionalLight(0xffffff, 2.4)
key.castShadow = true; key.shadow.mapSize.set(2048, 2048); key.shadow.bias = -.0004
scene.add(key, key.target)
const fill = new THREE.DirectionalLight(0xcfe0ff, .6); fill.position.set(-1, .4, 1); scene.add(fill)
const starGeo = new THREE.BufferGeometry()
const starPos = new Float32Array(4000 * 3)
for (let i = 0; i < 4000; i++) { const a = Math.random() * Math.PI * 2, y = .15 + Math.random() * .85, r = Math.sqrt(1 - y * y), R = 12000; starPos.set([Math.cos(a) * r * R, y * R, Math.sin(a) * r * R], i * 3) }
starGeo.setAttribute('position', new THREE.BufferAttribute(starPos, 3))
const stars = new THREE.Points(starGeo, new THREE.PointsMaterial({ color: 0xffffff, size: 18, fog: false, sizeAttenuation: true }))
scene.add(stars)

const white = new THREE.MeshStandardMaterial({ color: 0xf1f3f5, roughness: .45, metalness: .2 })
// Three-quarter profile: each hull turns this far about its vertical axis so the
// bow (+X, screen right) swings toward the camera. The line itself stays square.
const SHIP_YAW_DEG = 30, SHIP_YAW = -SHIP_YAW_DEG * Math.PI / 180

const LABEL_W = 512, LABEL_H = 96
function labelSprite(ship) {
  const cnv = document.createElement('canvas'); cnv.width = LABEL_W; cnv.height = LABEL_H
  const g = cnv.getContext('2d')
  // Empire colour lives in a thick border around the caption, not on the hull.
  const border = 10, pad = 22, detail = `${ship.lengthM} m${ship.lengthSource === 'estimate' ? ' est.' : ''}${ship.model ? '' : ' · model pending'}`
  g.fillStyle = 'rgba(244,247,250,.72)'; g.fillRect(0, 0, LABEL_W, LABEL_H)
  g.lineWidth = border; g.strokeStyle = accentOf(ship.empire); g.strokeRect(border / 2, border / 2, LABEL_W - border, LABEL_H - border)
  let size = 32; g.font = `600 ${size}px system-ui`
  while (size > 18 && g.measureText(ship.name).width > LABEL_W - 2 * pad) g.font = `600 ${--size}px system-ui`
  g.fillStyle = '#0d1b26'; g.fillText(ship.name, pad, 44)
  g.font = '400 22px system-ui'; g.fillStyle = '#223a4b'; g.fillText(detail, pad, 76)
  const tex = new THREE.CanvasTexture(cnv); tex.colorSpace = THREE.SRGBColorSpace; tex.anisotropy = 8
  // Once on the GPU the pixels live there; free the CPU-side canvas backing store.
  tex.onUpdate = () => { cnv.width = cnv.height = 0; tex.onUpdate = null }
  // Caption size: readable, but small next to the ship.
  const w = Math.max(8, ship.lengthM * .45)
  const plane = new THREE.Mesh(new THREE.PlaneGeometry(w, w * LABEL_H / LABEL_W), new THREE.MeshBasicMaterial({ map: tex, transparent: true, depthWrite: false }))
  plane.rotation.x = -Math.PI / 2
  return plane
}

function ghost(ship) {
  const r = Math.max(.5, Math.min(ship.beamM, ship.heightM) / 2)
  const geo = new THREE.CapsuleGeometry(r, Math.max(.1, ship.lengthM - 2 * r), 8, 16)
  geo.rotateZ(Math.PI / 2); geo.scale(1, ship.heightM / (2 * r), ship.beamM / (2 * r))
  return new THREE.Mesh(geo, new THREE.MeshStandardMaterial({ color: 0xffffff, transparent: true, opacity: .22, emissive: accentOf(ship.empire), emissiveIntensity: .35, depthWrite: false }))
}

const loader = new GLTFLoader()
let lineup
try {
  // ?lod: the local detail comparison (make_hangar_lod.py) instead of the fleet.
  const lodReview = new URLSearchParams(location.search).has('lod')
  if (lodReview) document.querySelector('#topbar h1').firstChild.textContent = 'Ship Hangar — detail comparison '
  const res = await fetch(lodReview ? 'hangar/lod/lineup.json' : 'hangar/lineup.json')
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  lineup = await res.json()
} catch (err) {
  fail(`Could not load the ship lineup (${err.message}). Try reloading the page.`)
  throw err
}
const all = lineup.ships
const entries = new Map(all.map(ship => {
  const group = new THREE.Group(); group.userData.ship = ship
  const body = new THREE.Group(); body.rotation.y = SHIP_YAW; body.userData.ship = ship; group.add(body); group.userData.body = body
  const g = ghost(ship); body.add(g); group.userData.ghost = g
  // Caption on the floor just in front of the hull (plane depth is w * LABEL_H/LABEL_W).
  const label = labelSprite(ship); label.userData.label = true
  const depth = label.geometry.parameters.height
  // The turned hull reaches further toward the camera: clear its rotated footprint.
  const reach = (ship.lengthM * Math.abs(Math.sin(SHIP_YAW)) + ship.beamM * Math.cos(SHIP_YAW)) / 2
  label.position.set(0, .05 - (FLOAT_M + ship.heightM / 2), reach + depth / 2 + 1); group.add(label)
  group.position.y = FLOAT_M + ship.heightM / 2
  scene.add(group)
  return [ship.id, { ship, group, state: ship.model ? 'idle' : 'none' }]
}))

let loading = 0             // model downloads in flight (capped at MAX_LOADS)
function loadModel(entry) {
  entry.state = 'loading'; loading++
  loader.load(`hangar/${entry.ship.model}`, gltf => {
    loading--
    let mesh; gltf.scene.traverse(o => { if (!mesh && o.isMesh) mesh = o })
    if (!mesh) { entry.state = 'failed'; return }
    const hull = new THREE.Mesh(mesh.geometry, white); hull.castShadow = true; hull.receiveShadow = true
    hull.scale.setScalar(entry.ship.lengthM)
    const g = entry.group.userData.ghost
    entry.group.userData.body.add(hull)
    if (g) { g.parent.remove(g); g.geometry.dispose(); g.material.dispose(); entry.group.userData.ghost = null }
    entry.state = 'loaded'
  }, undefined, () => { loading--; entry.state = 'failed' })
}

// --- lineup state ---
const playBtn = document.getElementById('play'), scrub = document.getElementById('scrub')
const nowEl = document.getElementById('now'), countEl = document.getElementById('count')
const categories = [...new Set(all.map(s => s.category).filter(Boolean))].sort()
const TIERS = [0, 1, 2, 3, 4, 5]
let filters = parseFilters(location.search, { empires: new Set(Object.keys(ACCENT)), tiers: new Set(TIERS), categories: new Set(categories) })
let visible = filterShips(all, filters)
let layout = layoutLineup(visible)
let u = 0
let shownNow = ''
function placeLineup() {
  const shown = new Set(visible.map(s => s.id))
  for (const [id, e] of entries) e.group.visible = shown.has(id)
  u = Math.max(0, Math.min(Math.max(0, visible.length - 1), u))
  scrub.max = String(Math.max(0, visible.length - 1)); scrub.value = String(u)
  countEl.textContent = `(${visible.length} ships)`
  if (!visible.length) nowEl.textContent = shownNow = 'No ships match these filters'
}
placeLineup()
layout.forEach((p, i) => { entries.get(visible[i].id).group.position.x = p.x })

function resize() { const w = innerWidth, h = innerHeight; renderer.setSize(w, h, false); camera.aspect = w / h; camera.updateProjectionMatrix() }
addEventListener('resize', resize); resize()

// --- rail control: auto-pan, button, scrubber, keys, wheel ---
let playing = true          // the #play toggle; false also once auto-pan reaches the end
const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)').matches
let holdUntil = 0           // user input pauses auto-pan until this time (ms)
let scrubbing = false       // pointer is dragging #scrub: don't write its value back
function setPlaying(on) {
  playing = on
  playBtn.innerHTML = on ? '&#10074;&#10074;' : '&#9654;'
  playBtn.setAttribute('aria-label', on ? 'Pause' : 'Play')
}
function takeOver(next) {
  u = Math.max(0, Math.min(Math.max(0, visible.length - 1), next))
  holdUntil = performance.now() + IDLE_RESUME_MS
}
if (reducedMotion) setPlaying(false)   // start paused; the play button still works
playBtn.addEventListener('click', () => {
  if (focused) exitFocus()
  if (!playing && u >= visible.length - 1) u = 0   // replay from the start of the line
  holdUntil = 0; setPlaying(!playing)
})
scrub.addEventListener('input', () => { if (focused) exitFocus(); takeOver(Number(scrub.value)) })
scrub.addEventListener('pointerdown', () => { scrubbing = true })
addEventListener('pointerup', () => { scrubbing = false })
addEventListener('pointercancel', () => { scrubbing = false })
addEventListener('keydown', e => {
  if (e.key === 'Escape' && focused) { exitFocus(true); return }
  if (focused) return   // arrows belong to the rail only when not focused
  const t = e.target
  if (t !== scrub && (t.isContentEditable || /^(INPUT|SELECT|TEXTAREA)$/.test(t.tagName))) return
  if (e.key === 'Enter' || e.key === ' ') {
    if (/^(BUTTON|A|SUMMARY)$/.test(t.tagName) || !visible.length) return   // let those activate themselves
    e.preventDefault()
    enterFocus(entries.get(visible[Math.round(u)].id), true)
    return
  }
  if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
  e.preventDefault()   // also stops the focused range's native step
  takeOver(Math.round(u) + (e.key === 'ArrowRight' ? 1 : -1))
})
canvas.addEventListener('wheel', e => {
  if (focused) return   // OrbitControls owns the wheel (zoom) while focused
  e.preventDefault()
  const d = Math.abs(e.deltaX) > Math.abs(e.deltaY) ? e.deltaX : e.deltaY
  const notches = e.deltaMode === 1 ? d / 3 : e.deltaMode === 2 ? d : d / 100
  takeOver(u + WHEEL_SHIPS_PER_NOTCH * Math.max(-1, Math.min(1, notches)))
}, { passive: false })

// --- focus: click a hull/ghost to orbit it, with an info card ---
const cardEl = document.getElementById('card')
const controls = new OrbitControls(camera, canvas)
controls.enabled = false; controls.enableDamping = true; controls.enablePan = false
let focused = null          // entry being orbited, or null while on the rail
let fly = null              // { from, fromLook, offset, t } camera flight into focus
let focusX = 0              // focused group's last x, to follow it through a re-pack
const raycaster = new THREE.Raycaster(), ndc = new THREE.Vector2()

function showCard(ship, fromKeyboard) {
  cardEl.replaceChildren()
  const close = document.createElement('button')
  close.type = 'button'; close.className = 'close'; close.innerHTML = '&times;'; close.setAttribute('aria-label', 'Close')
  close.addEventListener('click', () => exitFocus(true))
  const h = document.createElement('h2'); h.textContent = ship.name
  const dl = document.createElement('dl')
  const src = ship.lengthSource === 'estimate' ? 'estimated' : 'cockpit-window measurement'
  for (const [k, v] of [['Tier', `T${ship.tier}`], ['Category', ship.category || '—'], ['Empire', EMPIRE_NAME[ship.empire] || ship.empire], ['Length', `${ship.lengthM} m (${src})`]]) {
    const dt = document.createElement('dt'), dd = document.createElement('dd')
    dt.textContent = k; dd.textContent = v; dl.append(dt, dd)
  }
  cardEl.append(close, h, dl)
  let link = null
  if (ship.page) {
    const p = document.createElement('p'); link = document.createElement('a')
    link.href = ship.page; link.textContent = 'Ship page →'; p.append(link); cardEl.append(p)
  }
  cardEl.hidden = false
  if (fromKeyboard) (link || close).focus({ preventScroll: true })
}

function enterFocus(entry, fromKeyboard = false) {
  const { ship, group } = entry
  const fromLook = (focused ? controls.target : camLook).clone()   // refocus flies from the current orbit
  focused = entry; focusX = group.position.x
  const center = [group.position.x, group.position.y, 0]
  const pose = focusPose(center, ship.lengthM)
  fly = {
    from: camera.position.clone(),
    fromLook,
    offset: new THREE.Vector3(...pose.position).sub(new THREE.Vector3(...center)),
    t: 0,
  }
  controls.enabled = false
  controls.minDistance = Math.max(2, ship.lengthM * .4); controls.maxDistance = ship.lengthM * 6 + 60
  showCard(ship, fromKeyboard)
}

function exitFocus(returnFocus = false) {
  if (!focused) return
  // Back to the rail at the ship; if it was filtered out, at the first ship at least as long.
  const i = visible.indexOf(focused.ship)
  u = i >= 0 ? i : indexAtLength(visible, focused.ship.lengthM)
  // Hand back to the rail: it lerps from wherever the orbit left the camera.
  camPos.copy(camera.position); camLook.copy(controls.target)
  focused = null; fly = null
  controls.enabled = false
  if (returnFocus || cardEl.contains(document.activeElement)) canvas.focus({ preventScroll: true })
  cardEl.hidden = true
  holdUntil = performance.now() + IDLE_RESUME_MS
}

function pickEntry(e) {
  const r = canvas.getBoundingClientRect()
  ndc.set((e.clientX - r.left) / r.width * 2 - 1, -((e.clientY - r.top) / r.height) * 2 + 1)
  raycaster.setFromCamera(ndc, camera)
  const targets = []
  for (const s of visible) targets.push(...entries.get(s.id).group.userData.body.children)
  const hit = raycaster.intersectObjects(targets, false)[0]
  return hit ? entries.get(hit.object.parent.userData.ship.id) : null
}

// Off focus, a horizontal drag slides the line: the ground under the pointer follows it.
let downAt = null           // { x, y, railX, perPx, dragging } for a primary-button press
canvas.addEventListener('pointerdown', e => {
  downAt = null
  if (e.button !== 0) return
  downAt = { x: e.clientX, y: e.clientY, dragging: false }
  if (!focused && visible.length) {
    const { x } = railAt(layout, visible, u), d = camera.position.distanceTo(camLook)
    downAt.railX = x
    downAt.perPx = 2 * d * Math.tan(THREE.MathUtils.degToRad(camera.fov / 2)) / canvas.clientHeight
  }
})
canvas.addEventListener('pointermove', e => {
  if (!downAt || downAt.railX === undefined || focused) return
  if (!downAt.dragging && isClick(downAt, { x: e.clientX, y: e.clientY })) return
  if (!downAt.dragging) { downAt.dragging = true; canvas.setPointerCapture(e.pointerId); canvas.classList.add('dragging') }
  takeOver(uAtX(layout, downAt.railX - (e.clientX - downAt.x) * downAt.perPx))
})
function endDrag() { canvas.classList.remove('dragging') }
canvas.addEventListener('pointercancel', () => { downAt = null; endDrag() })
canvas.addEventListener('pointerup', e => {
  const down = downAt; downAt = null; endDrag()
  if (!down || down.dragging || e.button !== 0 || !isClick(down, { x: e.clientX, y: e.clientY })) return   // a drag pans/orbits, not clicks
  const entry = pickEntry(e)
  if (entry) { if (entry !== focused) enterFocus(entry) } else exitFocus()
})

// --- filters: chips re-pack the line ---
const filtersEl = document.getElementById('filters')
let repack = null           // { moves: [{ id, from, to }], t }
function chip(label, set, value, colour) {
  const b = document.createElement('button')
  b.className = 'chip'; b.textContent = label; b.type = 'button'
  if (colour) b.style.setProperty('--c', colour)
  const sync = () => { b.classList.toggle('on', set.has(value)); b.setAttribute('aria-pressed', String(set.has(value))) }
  b.addEventListener('click', () => { if (set.has(value)) set.delete(value); else set.add(value); sync(); applyFilters() })
  sync()
  return b
}
function chipGroup(name, chips) {
  const d = document.createElement('div'); d.className = 'group'; d.setAttribute('role', 'group'); d.setAttribute('aria-label', name)
  const l = document.createElement('span'); l.className = 'group-label'; l.textContent = name
  d.append(l, ...chips)
  return d
}
filtersEl.append(
  chipGroup('Empire', Object.keys(ACCENT).map(k => chip(EMPIRE_NAME[k], filters.empires, k, ACCENT[k]))),
  chipGroup('Tier', TIERS.map(t => chip(`T${t}`, filters.tiers, t))),
  chipGroup('Category', categories.map(c => chip(c, filters.categories, c))),
)
// Narrow screens fold the chips behind a "Filters" toggle; wider ones keep them open.
const filterBox = document.getElementById('filterbox'), narrow = matchMedia('(max-width: 600px)')
const syncFilterBox = () => { filterBox.open = !narrow.matches }
narrow.addEventListener('change', syncFilterBox); syncFilterBox()

function applyFilters() {
  const prevX = new Map(visible.map(s => [s.id, entries.get(s.id).group.position.x]))
  // Keep the viewer's place: the length under the rail now (or of the focused ship).
  const atLength = focused ? focused.ship.lengthM : visible.length ? railAt(layout, visible, u).length : 0
  visible = filterShips(all, filters)
  layout = layoutLineup(visible)
  if (focused && !visible.includes(focused.ship)) exitFocus()   // lands at indexAtLength itself
  else if (focused) u = visible.indexOf(focused.ship)
  else u = indexAtLength(visible, atLength)
  placeLineup()
  const moves = repackMoves(prevX, layout)
  for (const m of moves) entries.get(m.id).group.position.x = m.from
  repack = { moves, t: 0 }
  const q = formatFilters(filters)
  history.replaceState(null, '', location.pathname + (q ? '?' + q : ''))
}

function stepRepack(dt) {
  if (!repack) return
  repack.t = Math.min(1, repack.t + dt / REPACK_S)
  for (const m of repack.moves) entries.get(m.id).group.position.x = tweenX(m.from, m.to, repack.t)
  if (repack.t >= 1) repack = null
}

// While focused: fly in, then OrbitControls; follow the ship if a re-pack moves it.
function focusCamera(dt) {
  const g = focused.group, dx = g.position.x - focusX; focusX = g.position.x
  const center = new THREE.Vector3(g.position.x, g.position.y, 0)
  if (fly) {
    fly.from.x += dx; fly.fromLook.x += dx
    fly.t = Math.min(1, fly.t + dt / FOCUS_FLY_S)
    const k = easeInOut(fly.t)
    camera.position.lerpVectors(fly.from, center.clone().add(fly.offset), k)
    controls.target.lerpVectors(fly.fromLook, center, k)
    camera.lookAt(controls.target)
    if (fly.t >= 1) { fly = null; controls.enabled = true; controls.update() }
  } else {
    camera.position.x += dx; controls.target.x += dx
    controls.update()
  }
  return { x: g.position.x, length: focused.ship.lengthM }
}

const camPos = new THREE.Vector3(), camLook = new THREE.Vector3(), wantPos = new THREE.Vector3(), wantLook = new THREE.Vector3()
let camReady = false
function frame(dt) {
  stepRepack(dt)
  if (!visible.length) return
  if (scrubbing) holdUntil = performance.now() + IDLE_RESUME_MS   // a held drag is not idle
  if (focused) holdUntil = performance.now() + IDLE_RESUME_MS   // auto-pan pauses while focused
  if (playing && performance.now() >= holdUntil) {
    const step = panStep(u, dt, visible.length); u = step.u
    if (step.ended) setPlaying(false)
  }
  if (!scrubbing) scrub.value = String(u)
  const text = nowText(visible, u); if (text !== shownNow) nowEl.textContent = shownNow = text

  let x, length
  if (focused) ({ x, length } = focusCamera(dt))
  else {
    let height
    ;({ x, length, height } = railAt(layout, visible, u))
    const pose = railPose(x, length, height, { fovDeg: camera.fov, aspect: camera.aspect })
    wantPos.set(...pose.position); wantLook.set(...pose.target)
    if (!camReady) { camPos.copy(wantPos); camLook.copy(wantLook); camReady = true }
    const a = smoothFactor(dt)
    camPos.lerp(wantPos, a); camLook.lerp(wantLook, a)
    camera.position.copy(camPos); camera.lookAt(camLook)
  }

  key.position.set(x - length, length * 3 + 40, length * 2 + 30); key.target.position.set(x, 0, 0)
  const s = Math.max(40, length * 3); Object.assign(key.shadow.camera, { left: -s, right: s, top: s, bottom: -s, near: 1, far: s * 6 }); key.shadow.camera.updateProjectionMatrix()
  scene.fog.near = length * 4 + 150; scene.fog.far = length * 12 + 700
  stars.position.set(camera.position.x, 0, camera.position.z)
  floor.position.x = camera.position.x
  const i = focused ? Math.max(0, visible.indexOf(focused.ship)) : Math.round(u)
  for (const k of nearestFirst(i, visible.length, LOAD_WINDOW)) {
    if (loading >= MAX_LOADS) break
    const e = entries.get(visible[k].id); if (e.state === 'idle') loadModel(e)
  }
}
let last = performance.now()
renderer.setAnimationLoop(now => { const dt = Math.min(.1, (now - last) / 1000); last = now; frame(dt); renderer.render(scene, camera) })
