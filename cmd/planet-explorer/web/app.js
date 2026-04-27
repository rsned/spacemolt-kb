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

  if (Array.isArray(profile.Palette)) {
    const panel = makePanel('Palette', 'Read-only swatch of the legacy gradient palette. Used as the base color when no BiomeTable is set.');
    const strip = document.createElement('div');
    strip.style.cssText = 'height:24px;border-radius:3px;margin-top:6px';
    const stops = profile.Palette.map(s =>
      `${rgbaCSS(s.Color)} ${(s.Position*100).toFixed(0)}%`).join(', ');
    strip.style.background = `linear-gradient(to right, ${stops})`;
    panel.appendChild(strip);
    panels.appendChild(panel);
  }
  renderControlFieldsPanel(profile, panels);
  renderWarpPanel(profile, panels);
  renderBiomePanel(profile, panels);
  renderLUTPanel(profile, panels);
}

function renderLUTPanel(profile, panels) {
  const panel = makePanel('Color LUT',
    'Final color-grade pass. Each archetype ships with a 16³ LUT that applies a subtle hue/sat/value shift for "look unification". Bypass to compare against the un-graded output.');
  const status = document.createElement('div');
  status.className = 'lut-status';
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'reset-btn';
  let savedLUT = profile.LUT;
  function refresh() {
    if (profile.LUT) {
      status.textContent = `Active: ${profile.LUT.Name || 'unnamed'} (${profile.LUT.Size}³)`;
      btn.textContent = 'Bypass LUT';
    } else if (savedLUT) {
      status.textContent = 'Bypassed';
      btn.textContent = 'Restore LUT';
    } else {
      status.textContent = 'No LUT';
      btn.textContent = 'Restore LUT';
      btn.disabled = true;
    }
  }
  btn.addEventListener('click', () => {
    if (profile.LUT) {
      savedLUT = profile.LUT;
      profile.LUT = null;
    } else if (savedLUT) {
      profile.LUT = savedLUT;
    }
    commitProfile(profile);
    refresh();
  });
  refresh();
  panel.appendChild(status);
  panel.appendChild(btn);
  panels.appendChild(panel);
}

function renderBiomePanel(profile, panels) {
  const t = profile.BiomeTable;
  if (!t || !Array.isArray(t.Cells) || t.Cells.length === 0) return;
  const panel = makePanel('Biome table (T × M)',
    'Whittaker biome cells. Rows = Temperature (cold→hot), columns = Moisture (dry→wet). Each swatch is a 2-stop gradient (Low→High) used to color heightmap values, then bilinearly OkLab-blended across neighboring cells. Edit colors directly in the Profile JSON textarea below.');
  const grid = document.createElement('div');
  grid.style.cssText = `display:grid;grid-template-columns:repeat(${t.MBuckets}, 1fr);gap:2px;margin-top:6px;margin-left:16px`;
  for (let i = 0; i < t.TBuckets; i++) {
    for (let j = 0; j < t.MBuckets; j++) {
      const cell = t.Cells[i][j];
      const sw = document.createElement('div');
      sw.style.cssText = 'height:36px;border-radius:2px';
      sw.style.background = `linear-gradient(to right, ${rgbCSS(cell.Low)}, ${rgbCSS(cell.High)})`;
      sw.title = `T=${i} M=${j}\nLow rgb(${cell.Low.R}, ${cell.Low.G}, ${cell.Low.B})\nHigh rgb(${cell.High.R}, ${cell.High.G}, ${cell.High.B})`;
      grid.appendChild(sw);
    }
  }
  panel.appendChild(grid);
  const legend = document.createElement('div');
  legend.style.cssText = 'font-size:10px;color:#777;margin:4px 0 0 16px';
  legend.textContent = `↓ T=cold→hot (rows)   → M=dry→wet (cols)`;
  panel.appendChild(legend);
  panels.appendChild(panel);
}

function rgbCSS(c) {
  return `rgb(${c.R}, ${c.G}, ${c.B})`;
}

const FIELD_HELP = {
  Continentalness: 'Macro land/ocean shape. Spline output adds to height — typical curve is steep in the middle for sharp coastlines.',
  Erosion: 'Smooths highlands. Spline output is usually negative — subtracted from height where erosion is high.',
  PeaksValleys: 'High-frequency mountain detail. Spline output adds small-scale roughness on top of the macro shape.',
  Temperature: 'Drives biome row selection in the Whittaker table. Combined with cos(latitude) so poles are colder than the equator.',
  Humidity: 'Drives biome column selection. No latitude bias — purely from this noise field.',
};

const PARAM_HELP = {
  Amp: 'Amplitude. Output multiplier on top of the [0,1] fBm result. Spline Input domain runs from 0 to Amp. Typical: 1.0.',
  Freq: 'Base frequency. 1.0 ≈ one noise period across the planet. Higher = smaller, more fragmented features. Useful range: 0.5–8.',
  Octaves: 'Number of stacked noise layers. Each octave adds finer detail at exponentially smaller scale. Typical: 3–5.',
  Lacunarity: 'Frequency multiplier per octave. Standard fBm = 2.0. Higher spreads the octaves wider in scale.',
  Persistence: 'Amplitude decay per octave. Standard fBm = 0.5. Higher = noisier output (more high-frequency detail).',
};

function makePanel(title, helpText) {
  const panel = document.createElement('details');
  panel.className = 'panel';
  panel.open = true;
  const summary = document.createElement('summary');
  summary.title = helpText;
  summary.innerHTML = `<h3>${title}</h3>`;
  panel.appendChild(summary);
  return panel;
}

function makeSubPanel(title, helpText, onReset) {
  const sub = document.createElement('details');
  sub.className = 'subpanel';
  sub.open = true;
  const summary = document.createElement('summary');
  summary.title = helpText;
  summary.innerHTML = `<strong>${title}</strong>`;
  if (onReset) {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'reset-btn';
    btn.textContent = 'Reset';
    btn.title = `Zero out all values for ${title}`;
    btn.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      onReset();
    });
    summary.appendChild(btn);
  }
  sub.appendChild(summary);
  return sub;
}

function makeParamRow(param, getValue, setValue, helpText) {
  const row = document.createElement('label');
  row.className = 'row';
  row.title = helpText;
  row.innerHTML = `<span>${param}</span>`;
  const input = document.createElement('input');
  input.type = 'number';
  input.step = (param === 'Octaves') ? '1' : '0.1';
  input.value = getValue();
  input.addEventListener('input', () => {
    setValue((param === 'Octaves') ? parseInt(input.value, 10) : parseFloat(input.value));
  });
  row.appendChild(input);
  return row;
}

function commitProfile(profile) {
  profileTextarea.value = prettifyJSON(JSON.stringify(profile));
}

function renderWarpPanel(profile, panels) {
  if (!profile.Warp) profile.Warp = {Amp: 0, Freq: 0, Octaves: 0, Lacunarity: 0, Persistence: 0};
  const panel = makePanel('Domain warp',
    'Quilez domain warp. Displaces sphere directions before sampling noise so features bend/curl instead of being axis-aligned. Amp=0 disables warp entirely.');
  const reset = () => {
    profile.Warp = {Amp: 0, Freq: 0, Octaves: 0, Lacunarity: 0, Persistence: 0};
    commitProfile(profile);
    renderPanels();
  };
  const resetBtn = document.createElement('button');
  resetBtn.type = 'button';
  resetBtn.className = 'reset-btn';
  resetBtn.textContent = 'Reset';
  resetBtn.title = 'Zero out all warp params';
  resetBtn.addEventListener('click', (e) => { e.preventDefault(); e.stopPropagation(); reset(); });
  panel.querySelector('summary').appendChild(resetBtn);
  for (const param of ['Amp', 'Freq', 'Octaves', 'Lacunarity', 'Persistence']) {
    panel.appendChild(makeParamRow(param,
      () => profile.Warp[param] || 0,
      (v) => { profile.Warp[param] = v; commitProfile(profile); },
      `Warp ${param}: ${PARAM_HELP[param]}`));
  }
  panels.appendChild(panel);
}

function renderControlFieldsPanel(profile, panels) {
  const cc = profile.ControlConfig;
  if (!cc) return;
  if (!Array.isArray(profile.Splines)) profile.Splines = [{}, {}, {}, {}, {}];
  const panel = makePanel('Control fields',
    'Five 3D fBm noise fields. Continentalness/Erosion/PeaksValleys are summed via splines to build the heightmap. Temperature/Humidity feed the Whittaker biome lookup. Each field has independent fBm settings + an optional spline.');
  const fields = ['Continentalness', 'Erosion', 'PeaksValleys', 'Temperature', 'Humidity'];
  for (let i = 0; i < fields.length; i++) {
    const fieldName = fields[i];
    const cf = cc[fieldName];
    if (!cf) continue;
    const reset = () => {
      cf.Amp = 0; cf.Freq = 0; cf.Octaves = 0; cf.Lacunarity = 0; cf.Persistence = 0;
      commitProfile(profile);
      renderPanels();
    };
    const sub = makeSubPanel(fieldName, FIELD_HELP[fieldName], reset);
    for (const param of ['Amp', 'Freq', 'Octaves', 'Lacunarity', 'Persistence']) {
      sub.appendChild(makeParamRow(param,
        () => cf[param],
        (v) => { cf[param] = v; commitProfile(profile); },
        `${fieldName} ${param}: ${PARAM_HELP[param]}`));
    }
    const knotsLabel = document.createElement('div');
    knotsLabel.className = 'knots-label';
    knotsLabel.textContent = 'Knots';
    knotsLabel.title = 'Fritsch-Carlson monotone-cubic spline knots. Maps fBm output [0, Amp] to a height contribution. JSON array like [{"Input":0,"Output":0},{"Input":1,"Output":0.5}]. Sorted by Input ascending. Knots outside [first, last] are clamped.';
    sub.appendChild(knotsLabel);
    const sp = profile.Splines[i] || {Knots: []};
    const ta = document.createElement('textarea');
    ta.rows = 2;
    ta.value = JSON.stringify(sp.Knots || []);
    ta.title = knotsLabel.title;
    ta.addEventListener('input', () => {
      try {
        const knots = JSON.parse(ta.value);
        profile.Splines[i] = {Knots: knots};
        commitProfile(profile);
      } catch (e) {
        status.textContent = 'Bad knots JSON';
      }
    });
    sub.appendChild(ta);
    panel.appendChild(sub);
  }
  panels.appendChild(panel);
}

function rgbaCSS(c) {
  return `rgba(${c.R}, ${c.G}, ${c.B}, ${(c.A/255).toFixed(2)})`;
}

init();
