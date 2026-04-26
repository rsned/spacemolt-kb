// Boots the Wasm runtime, wires the UI to it, and drives renders.
// Called functions exposed by Wasm: planetExplorerGenerate, planetExplorerBakeEquirect, planetExplorerDefaultProfile.

const $ = (sel) => document.querySelector(sel);

const status = $('#status');
const typePicker = $('#type-picker');
const seedInput = $('#seed-input');
const faceSizeSel = $('#face-size');
const renderBtn = $('#render-btn');
const exportBtn = $('#export-json-btn');
const applyBtn = $('#apply-json-btn');
const profileTextarea = $('#profile-json');
const cubeCanvas = $('#cube-canvas');
const equirectCanvas = $('#equirect-canvas');

let wasmReady = false;

async function init() {
  status.textContent = 'Loading wasm…';
  const go = new Go();
  const result = await WebAssembly.instantiateStreaming(fetch('wasm'), go.importObject);
  go.run(result.instance);
  wasmReady = true;
  status.textContent = 'Ready';

  // Load default profile for the initial type.
  loadDefaultProfile();
}

function loadDefaultProfile() {
  const type = typePicker.value;
  const json = planetExplorerDefaultProfile(type);
  if (typeof json === 'string' && json.startsWith('{"error"')) {
    status.textContent = 'Error: ' + json;
    return;
  }
  profileTextarea.value = prettifyJSON(json);
  renderPanels();
}

function prettifyJSON(s) {
  try { return JSON.stringify(JSON.parse(s), null, 2); } catch { return s; }
}

async function regenerate() {
  if (!wasmReady) return;
  status.textContent = 'Rendering…';
  await new Promise(r => setTimeout(r, 0)); // yield to repaint

  const profileJSON = profileTextarea.value;
  const seed = seedInput.value;
  const faceSize = parseInt(faceSizeSel.value, 10);

  const t0 = performance.now();
  const cubePNG = planetExplorerGenerate(profileJSON, seed, faceSize);
  if (cubePNG instanceof Uint8Array) {
    await paintToCanvas(cubeCanvas, cubePNG);
    const equirectPNG = planetExplorerBakeEquirect(cubePNG, equirectCanvas.width, equirectCanvas.height);
    if (equirectPNG instanceof Uint8Array) {
      await paintToCanvas(equirectCanvas, equirectPNG);
    }
  } else {
    status.textContent = 'Error: ' + cubePNG;
    return;
  }
  status.textContent = `Rendered in ${(performance.now() - t0).toFixed(0)} ms`;
  renderPanels();
}

async function paintToCanvas(canvas, pngBytes) {
  const blob = new Blob([pngBytes], { type: 'image/png' });
  const url = URL.createObjectURL(blob);
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => {
      canvas.width = img.width;
      canvas.height = img.height;
      const ctx = canvas.getContext('2d');
      ctx.drawImage(img, 0, 0);
      URL.revokeObjectURL(url);
      resolve();
    };
    img.onerror = () => { URL.revokeObjectURL(url); reject(); };
    img.src = url;
  });
}

typePicker.addEventListener('change', loadDefaultProfile);
renderBtn.addEventListener('click', regenerate);
exportBtn.addEventListener('click', () => {
  navigator.clipboard.writeText(profileTextarea.value);
  status.textContent = 'JSON copied to clipboard';
});
applyBtn.addEventListener('click', () => {
  renderPanels();
  regenerate();
});

function renderPanels() {
  const panels = $('#param-panels');
  panels.innerHTML = '';
  let profile;
  try { profile = JSON.parse(profileTextarea.value); }
  catch { return; }

  // Palette panel — read-only swatch.
  const palette = profile.Palette;
  if (Array.isArray(palette)) {
    const panel = document.createElement('div');
    panel.className = 'panel';
    panel.innerHTML = '<h3>Palette</h3>';
    const strip = document.createElement('div');
    strip.style.height = '24px';
    strip.style.borderRadius = '3px';
    const stops = palette.map(s =>
      `${rgbaCSS(s.Color)} ${(s.Position*100).toFixed(0)}%`).join(', ');
    strip.style.background = `linear-gradient(to right, ${stops})`;
    panel.appendChild(strip);
    panels.appendChild(panel);
  }
}

function rgbaCSS(c) {
  return `rgba(${c.R}, ${c.G}, ${c.B}, ${(c.A/255).toFixed(2)})`;
}

init();
