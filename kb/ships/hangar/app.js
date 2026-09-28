import * as THREE from 'three'
import { GLTFLoader } from 'three/addons/loaders/GLTFLoader.js'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { filterShips, layoutLineup, railAt, railPose, nearestIndex, parseFilters, formatFilters, FLOAT_M } from './layout.js'

const ACCENT = { solarian: '#c9a227', crimson: '#e63946', nebula: '#2f9e6a', outerrim: '#2fb6c4', voidborn: '#9b6bff', pirate: '#ff6540', independent: '#8bd7ff' }
const LOAD_WINDOW = 14
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
  const w = Math.max(14, ship.lengthM * .9)
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
  const label = labelSprite(ship); label.position.set(0, .05 - (FLOAT_M + ship.heightM / 2), ship.beamM / 2 + ship.lengthM * .12 + 3); group.add(label)
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
    entry.group.add(hull); entry.group.remove(entry.group.userData.ghost); entry.state = 'loaded'
  }, undefined, () => { entry.state = 'failed' })
}

// --- lineup state (Task 5 adds rail + UI; Task 6 adds focus + filters) ---
let filters = parseFilters(location.search)
let visible = filterShips(all, filters)
let layout = layoutLineup(visible)
function placeLineup() {
  const shown = new Set(visible.map(s => s.id))
  for (const [id, e] of entries) e.group.visible = shown.has(id)
  layout.forEach((p, i) => { entries.get(visible[i].id).group.position.x = p.x })
}
placeLineup()

function resize() { const w = innerWidth, h = innerHeight; renderer.setSize(w, h, false); camera.aspect = w / h; camera.updateProjectionMatrix() }
addEventListener('resize', resize); resize()

let u = 0
function frame(dt) {
  // Task 5 replaces this with rail/auto-pan/focus camera logic.
  const { x, length } = railAt(layout, visible, u)
  const pose = railPose(x, length)
  camera.position.set(...pose.position); camera.lookAt(...pose.target)
  key.position.set(x - length, length * 3 + 40, length * 2 + 30); key.target.position.set(x, 0, 0)
  const s = Math.max(40, length * 3); Object.assign(key.shadow.camera, { left: -s, right: s, top: s, bottom: -s, near: 1, far: s * 6 }); key.shadow.camera.updateProjectionMatrix()
  scene.fog.near = length * 4 + 150; scene.fog.far = length * 12 + 700
  stars.position.set(camera.position.x, 0, camera.position.z)
  const i = Math.round(u)
  for (let k = Math.max(0, i - LOAD_WINDOW); k <= Math.min(visible.length - 1, i + LOAD_WINDOW); k++) {
    const e = entries.get(visible[k].id); if (e.state === 'idle') loadModel(e)
  }
}
let last = performance.now()
renderer.setAnimationLoop(now => { const dt = Math.min(.1, (now - last) / 1000); last = now; frame(dt); renderer.render(scene, camera) })
export { THREE, OrbitControls, camera, renderer, entries, all, formatFilters, nearestIndex }
