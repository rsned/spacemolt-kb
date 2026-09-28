import * as THREE from 'three'
import { GLTFLoader } from 'three/addons/loaders/GLTFLoader.js'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { filterShips, layoutLineup, railAt, railPose, nearestIndex, parseFilters, formatFilters, FLOAT_M, panStep, smoothFactor, nowText, easeInOut, tweenX, repackMoves, isClick, focusPose } from './layout.js'

const ACCENT = { solarian: '#c9a227', crimson: '#e63946', nebula: '#2f9e6a', outerrim: '#2fb6c4', voidborn: '#9b6bff', pirate: '#ff6540', independent: '#8bd7ff' }
const LOAD_WINDOW = 14
const IDLE_RESUME_MS = 6000
const WHEEL_SHIPS_PER_NOTCH = .25
const FOCUS_FLY_S = .8
const REPACK_S = .6
const EMPIRE_NAME = { solarian: 'Solarian', crimson: 'Crimson', nebula: 'Nebula', outerrim: 'Outer Rim', voidborn: 'Voidborn', pirate: 'Pirate', independent: 'Independent' }
const canvas = document.getElementById('stage')
const renderer = new THREE.WebGLRenderer({ canvas, antialias: true })
renderer.setPixelRatio(Math.min(2, devicePixelRatio))
renderer.shadowMap.enabled = true
renderer.shadowMap.type = THREE.PCFSoftShadowMap
renderer.toneMapping = THREE.ACESFilmicToneMapping
const scene = new THREE.Scene()
scene.background = new THREE.Color(0x000000)
scene.fog = new THREE.Fog(0x000000, 200, 900)
const camera = new THREE.PerspectiveCamera(38, 1, .5, 20000)

// Studio: light floor fading into black space, key light with soft shadows.
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

const white = new THREE.MeshStandardMaterial({ color: 0xf1f3f5, roughness: .45, metalness: .2, vertexColors: true })

function accentBand(geometry, colour) {
  // Near-white hull with a thin empire-coloured band around the waist.
  const pos = geometry.getAttribute('position')
  geometry.computeBoundingBox()
  const { min, max } = geometry.boundingBox, mid = (min.y + max.y) / 2, half = (max.y - min.y) * .06
  const c = new THREE.Color(colour), w = new THREE.Color(0xf1f3f5), colors = new Float32Array(pos.count * 3)
  for (let i = 0; i < pos.count; i++) (Math.abs(pos.getY(i) - mid) < half ? c : w).toArray(colors, i * 3)
  geometry.setAttribute('color', new THREE.BufferAttribute(colors, 3))
}

function labelSprite(ship) {
  const cnv = document.createElement('canvas'); cnv.width = 1024; cnv.height = 192
  const g = cnv.getContext('2d')
  g.fillStyle = '#0d1b26'; g.font = '600 64px system-ui'; g.fillText(ship.name, 16, 80)
  g.font = '400 48px system-ui'; g.fillStyle = '#223a4b'
  g.fillText(`${ship.lengthM} m${ship.lengthSource === 'estimate' ? ' est.' : ''}${ship.model ? '' : ' · model pending'}`, 16, 150)
  const tex = new THREE.CanvasTexture(cnv); tex.colorSpace = THREE.SRGBColorSpace; tex.anisotropy = 8
  // Caption size: readable, but small next to the ship.
  const w = Math.max(8, ship.lengthM * .45)
  const plane = new THREE.Mesh(new THREE.PlaneGeometry(w, w * 192 / 1024), new THREE.MeshBasicMaterial({ map: tex, transparent: true, depthWrite: false }))
  plane.rotation.x = -Math.PI / 2
  return plane
}

function ghost(ship) {
  const r = Math.max(.5, Math.min(ship.beamM, ship.heightM) / 2)
  const geo = new THREE.CapsuleGeometry(r, Math.max(.1, ship.lengthM - 2 * r), 8, 16)
  geo.rotateZ(Math.PI / 2); geo.scale(1, ship.heightM / (2 * r), ship.beamM / (2 * r))
  return new THREE.Mesh(geo, new THREE.MeshStandardMaterial({ color: 0xffffff, transparent: true, opacity: .22, emissive: ACCENT[ship.empire], emissiveIntensity: .35, depthWrite: false }))
}

const loader = new GLTFLoader()
const lineup = await (await fetch('hangar/lineup.json')).json()
const all = lineup.ships
const entries = new Map(all.map(ship => {
  const group = new THREE.Group(); group.userData.ship = ship
  const g = ghost(ship); group.add(g); group.userData.ghost = g
  // Caption on the floor just in front of the hull (plane depth is w * 192/1024).
  const label = labelSprite(ship); label.userData.label = true
  const depth = label.geometry.parameters.height
  label.position.set(0, .05 - (FLOAT_M + ship.heightM / 2), ship.beamM / 2 + depth / 2 + 1); group.add(label)
  group.position.y = FLOAT_M + ship.heightM / 2
  scene.add(group)
  return [ship.id, { ship, group, state: ship.model ? 'idle' : 'none' }]
}))

function loadModel(entry) {
  entry.state = 'loading'
  loader.load(`hangar/${entry.ship.model}`, gltf => {
    let mesh; gltf.scene.traverse(o => { if (!mesh && o.isMesh) mesh = o })
    if (!mesh) { entry.state = 'failed'; return }
    accentBand(mesh.geometry, ACCENT[entry.ship.empire])
    const hull = new THREE.Mesh(mesh.geometry, white); hull.castShadow = true; hull.receiveShadow = true
    hull.scale.setScalar(entry.ship.lengthM)
    const g = entry.group.userData.ghost
    entry.group.add(hull)
    if (g) { entry.group.remove(g); g.geometry.dispose(); g.material.dispose(); entry.group.userData.ghost = null }
    entry.state = 'loaded'
  }, undefined, () => { entry.state = 'failed' })
}

// --- lineup state ---
const playBtn = document.getElementById('play'), scrub = document.getElementById('scrub')
const nowEl = document.getElementById('now'), countEl = document.getElementById('count')
let filters = parseFilters(location.search)
let visible = filterShips(all, filters)
let layout = layoutLineup(visible)
let u = 0
function placeLineup() {
  const shown = new Set(visible.map(s => s.id))
  for (const [id, e] of entries) e.group.visible = shown.has(id)
  u = Math.max(0, Math.min(Math.max(0, visible.length - 1), u))
  scrub.max = String(Math.max(0, visible.length - 1)); scrub.value = String(u)
  countEl.textContent = `(${visible.length} ships)`
}
placeLineup()
layout.forEach((p, i) => { entries.get(visible[i].id).group.position.x = p.x })

function resize() { const w = innerWidth, h = innerHeight; renderer.setSize(w, h, false); camera.aspect = w / h; camera.updateProjectionMatrix() }
addEventListener('resize', resize); resize()

// --- rail control: auto-pan, button, scrubber, keys, wheel ---
let playing = true          // the #play toggle; false also once auto-pan reaches the end
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
  if (e.key === 'Escape' && focused) { exitFocus(); return }
  if (focused) return   // arrows belong to the rail only when not focused
  if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
  const t = e.target
  if (t !== scrub && (t.isContentEditable || /^(INPUT|SELECT|TEXTAREA)$/.test(t.tagName))) return
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

function showCard(ship) {
  cardEl.replaceChildren()
  const h = document.createElement('h2'); h.textContent = ship.name
  const dl = document.createElement('dl')
  const src = ship.lengthSource === 'estimate' ? 'estimated' : 'cockpit-window measurement'
  for (const [k, v] of [['Tier', `T${ship.tier}`], ['Category', ship.category || '—'], ['Empire', EMPIRE_NAME[ship.empire] || ship.empire], ['Length', `${ship.lengthM} m (${src})`]]) {
    const dt = document.createElement('dt'), dd = document.createElement('dd')
    dt.textContent = k; dd.textContent = v; dl.append(dt, dd)
  }
  cardEl.append(h, dl)
  if (ship.page) {
    const p = document.createElement('p'), a = document.createElement('a')
    a.href = ship.page; a.textContent = 'Ship page →'; p.append(a); cardEl.append(p)
  }
  cardEl.hidden = false
}

function enterFocus(entry) {
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
  showCard(ship)
}

function exitFocus() {
  if (!focused) return
  const i = visible.indexOf(focused.ship)
  if (i >= 0) u = i
  // Hand back to the rail: it lerps from wherever the orbit left the camera.
  camPos.copy(camera.position); camLook.copy(controls.target)
  focused = null; fly = null
  controls.enabled = false
  cardEl.hidden = true
  holdUntil = performance.now() + IDLE_RESUME_MS
}

function pickEntry(e) {
  const r = canvas.getBoundingClientRect()
  ndc.set((e.clientX - r.left) / r.width * 2 - 1, -((e.clientY - r.top) / r.height) * 2 + 1)
  raycaster.setFromCamera(ndc, camera)
  const targets = []
  for (const s of visible) for (const o of entries.get(s.id).group.children) if (!o.userData.label) targets.push(o)
  const hit = raycaster.intersectObjects(targets, false)[0]
  return hit ? entries.get(hit.object.parent.userData.ship.id) : null
}

let downAt = null
canvas.addEventListener('pointerdown', e => { downAt = e.button === 0 ? { x: e.clientX, y: e.clientY } : null })
canvas.addEventListener('pointerup', e => {
  const down = downAt; downAt = null
  if (!down || e.button !== 0 || !isClick(down, { x: e.clientX, y: e.clientY })) return   // a drag orbits, not clicks
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
function sep() { const d = document.createElement('span'); d.className = 'sep'; return d }
const categories = [...new Set(all.map(s => s.category).filter(Boolean))].sort()
filtersEl.append(
  ...Object.keys(ACCENT).map(k => chip(EMPIRE_NAME[k], filters.empires, k, ACCENT[k])), sep(),
  ...[0, 1, 2, 3, 4, 5].map(t => chip(`T${t}`, filters.tiers, t)), sep(),
  ...categories.map(c => chip(c, filters.categories, c)),
)

function applyFilters() {
  const prevX = new Map(visible.map(s => [s.id, entries.get(s.id).group.position.x]))
  visible = filterShips(all, filters)
  layout = layoutLineup(visible)
  if (focused && !visible.includes(focused.ship)) exitFocus()
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
let camReady = false, shownNow = ''
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
    ({ x, length } = railAt(layout, visible, u))
    const pose = railPose(x, length)
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
  const i = focused ? Math.max(0, visible.indexOf(focused.ship)) : Math.round(u)
  for (let k = Math.max(0, i - LOAD_WINDOW); k <= Math.min(visible.length - 1, i + LOAD_WINDOW); k++) {
    const e = entries.get(visible[k].id); if (e.state === 'idle') loadModel(e)
  }
}
let last = performance.now()
renderer.setAnimationLoop(now => { const dt = Math.min(.1, (now - last) / 1000); last = now; frame(dt); renderer.render(scene, camera) })
export { THREE, OrbitControls, camera, renderer, entries, all, formatFilters, nearestIndex }
