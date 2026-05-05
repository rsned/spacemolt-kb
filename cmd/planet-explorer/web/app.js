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
const toggleJitterBtn = $('#toggle-jitter-btn');
const profileTextarea = $('#profile-json');
const cubeCanvas = $('#cube-canvas');
const equirectCanvas = $('#equirect-canvas');
const viewModeSel = $('#view-mode');

let wasmReady = false;

// Snapshot of the profile as it was loaded (from the type-picker default
// or an imported file). Used by per-panel Reset buttons to restore the
// originally-loaded values without losing the user's other edits.
let originalProfile = null;
function snapshotOriginal(profile) {
  try { originalProfile = JSON.parse(JSON.stringify(profile)); }
  catch { originalProfile = null; }
}

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
  try { snapshotOriginal(JSON.parse(profileTextarea.value)); } catch {}
  renderPanels();
  refreshJitterButtonLabel();
}

function prettifyJSON(s) {
  try { return JSON.stringify(JSON.parse(s), null, 2); } catch { return s; }
}

// Sweep all in-DOM knots textareas and apply their current values to
// the profile JSON in the big textarea. Catches the case where a user
// typed valid JSON into a knots textarea but the per-keystroke input
// event never landed a successful commit (e.g., focus moved before
// the final valid state). Silently ignores textareas whose content
// isn't valid JSON.
function syncKnotsFromDOM() {
  let profile;
  try { profile = JSON.parse(profileTextarea.value); }
  catch { return; }
  if (!profile.ControlConfig) return;
  let dirty = false;
  const tas = document.querySelectorAll('textarea.knots-input');
  for (const ta of tas) {
    const fieldName = ta.dataset.fieldName;
    const cf = profile.ControlConfig[fieldName];
    if (!cf) continue;
    let knots;
    try { knots = JSON.parse(ta.value); } catch { continue; }
    const existing = JSON.stringify((cf.Spline && cf.Spline.Knots) || null);
    const incoming = JSON.stringify(knots);
    if (existing !== incoming) {
      cf.Spline = {Knots: knots};
      dirty = true;
    }
  }
  if (dirty) {
    profileTextarea.value = prettifyJSON(JSON.stringify(profile));
  }
}

async function regenerate() {
  if (!wasmReady) return;
  syncKnotsFromDOM();
  status.textContent = 'Rendering…';
  await new Promise(r => setTimeout(r, 0)); // yield to repaint

  const profileJSON = profileTextarea.value;
  const seed = seedInput.value;
  const size = parseInt(faceSizeSel.value, 10) || 256;

  const t0 = performance.now();

  // Cube-sphere path for both rocky and gas-giant profiles. Rocky uses the
  // full pipeline (plates, jitter, JFA coastal, erosion, craters); gas
  // giants use the gas-giant renderer.
  const mode = viewModeSel ? viewModeSel.value : 'color';
  let cubePNG;
  if (mode === 'heightmap') {
    cubePNG = planetExplorerGenerateHeightmap(profileJSON, seed, size);
  } else if (debugBypass.size > 0 && window.planetExplorerGenerateWithBypass) {
    cubePNG = planetExplorerGenerateWithBypass(profileJSON, seed, size, JSON.stringify([...debugBypass]));
  } else {
    cubePNG = planetExplorerGenerate(profileJSON, seed, size);
  }
  if (!(cubePNG instanceof Uint8Array)) {
    status.textContent = 'Error: ' + cubePNG;
    return;
  }
  await paintToCanvas(cubeCanvas, cubePNG);
  const equirectPNG = planetExplorerBakeEquirect(cubePNG, equirectCanvas.width, equirectCanvas.height);
  if (equirectPNG instanceof Uint8Array) {
    await paintToCanvas(equirectCanvas, equirectPNG);
    refreshSphereTexture();
  }

  // Phase 9b nightside: when the profile has Civ.Tier > 0 the wasm
  // exposes a separate Black-Marble cube-map. Bake it to an offscreen
  // equirect and stash the ImageData so renderSphere can blend it onto
  // the unlit hemisphere via the sun-direction dot product. Empty
  // Uint8Array means civ disabled — clear the texture so we fall back
  // to the original day-only render.
  if (window.planetExplorerGenerateNight) {
    const nightCubePNG = planetExplorerGenerateNight(profileJSON, seed, size);
    if (nightCubePNG instanceof Uint8Array && nightCubePNG.length > 0) {
      const nightEqPNG = planetExplorerBakeEquirect(nightCubePNG, nightEquirectCanvas.width, nightEquirectCanvas.height);
      if (nightEqPNG instanceof Uint8Array) {
        await paintToCanvas(nightEquirectCanvas, nightEqPNG);
        refreshSphereNightTexture();
      }
    } else {
      sphereNightTextureData = null;
    }
  }

  const elapsed = (performance.now() - t0).toFixed(0);
  status.textContent = `Rendered in ${elapsed} ms`;
  renderPanels();

  // Debug panel: now supported for both rocky (flat path) and non-rocky (cube path).
  const debugPanel = document.getElementById('debug-panel');
  if (debugPanel && debugPanel.open) {
    refreshDebugView();
  }
}

function refreshJitterButtonLabel() {
  if (!toggleJitterBtn) return;
  let on = false;
  try { on = !!JSON.parse(profileTextarea.value).JitterEnabled; } catch {}
  toggleJitterBtn.textContent = on ? 'Jitter: ON' : 'Jitter: OFF';
}

function toggleJitter() {
  let prof;
  try { prof = JSON.parse(profileTextarea.value); }
  catch { status.textContent = 'Cannot toggle jitter: profile JSON invalid'; return; }
  prof.JitterEnabled = !prof.JitterEnabled;
  profileTextarea.value = prettifyJSON(JSON.stringify(prof));
  refreshJitterButtonLabel();
  renderPanels();
  regenerate();
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
if (toggleJitterBtn) toggleJitterBtn.addEventListener('click', toggleJitter);
if (viewModeSel) viewModeSel.addEventListener('change', regenerate);
exportBtn.addEventListener('click', () => {
  navigator.clipboard.writeText(profileTextarea.value);
  status.textContent = 'JSON copied to clipboard';
});
applyBtn.addEventListener('click', () => {
  renderPanels();
  refreshJitterButtonLabel();
  regenerate();
});

const importFileInput = $('#import-json-file');
if (importFileInput) {
  importFileInput.addEventListener('change', async () => {
    const file = importFileInput.files && importFileInput.files[0];
    if (!file) return;
    try {
      const text = await file.text();
      const parsed = JSON.parse(text); // validate before assigning
      profileTextarea.value = prettifyJSON(text);
      snapshotOriginal(parsed);
      status.textContent = `Imported ${file.name}`;
      renderPanels();
      refreshJitterButtonLabel();
      regenerate();
    } catch (e) {
      status.textContent = `Import failed: ${e.message || e}`;
    } finally {
      importFileInput.value = ''; // allow re-picking same file
    }
  });
}

function renderPanels() {
  const panels = $('#param-panels');
  panels.innerHTML = '';
  let profile;
  try { profile = JSON.parse(profileTextarea.value); }
  catch { return; }

  renderPaletteSwatch(profile, panels, 'Palette',
    'Palette',
    'Read-only swatch of the legacy gradient palette. Used as the base color when no BiomeTable is set.');
  renderPaletteSwatch(profile, panels, 'EquatorialPalette',
    'Equatorial palette',
    'Optional palette blended in near the equator (cos(latitude) weight). Read-only — edit via JSON if needed.');
  renderPaletteSwatch(profile, panels, 'PolarPalette',
    'Polar palette',
    'Optional palette blended in near the poles (before ice caps). Read-only — edit via JSON if needed.');
  renderControlFieldsPanel(profile, panels);
  renderWarpPanel(profile, panels);
  renderRidgedPanel(profile, panels);
  renderProvincePanel(profile, panels);
  renderShadingPanel(profile, panels);
  renderOceanPanel(profile, panels);
  renderCryospherePanel(profile, panels);
  renderHeightSmoothPanel(profile, panels);
  renderCoastalPanel(profile, panels);
  renderContinentsPanel(profile, panels);
  renderCratersPanel(profile, panels);
  renderErosionPanel(profile, panels);
  renderBiomePanel(profile, panels);
  renderCurlPanel(profile, panels);
  renderStormBandsPanel(profile, panels);
  renderLUTPanel(profile, panels);
}

function renderPaletteSwatch(profile, panels, key, title, helpText) {
  const stops = profile[key];
  if (!Array.isArray(stops) || stops.length === 0) return;
  const panel = makePanel(title, helpText);
  const strip = document.createElement('div');
  strip.style.cssText = 'height:24px;border-radius:3px;margin-top:6px';
  const css = stops.map(s => `${rgbaCSS(s.Color)} ${(s.Position*100).toFixed(0)}%`).join(', ');
  strip.style.background = `linear-gradient(to right, ${css})`;
  panel.appendChild(strip);
  panels.appendChild(panel);
}

function renderCurlPanel(profile, panels) {
  if (profile.Renderer !== 'gas_giant') return;
  const panel = makePanel('Curl Advection',
    'Semi-Lagrangian backward-trace with curl-of-fbm tangent field. Each step subtracts dt·(jet+curl) from the position. Amp=0 and JetAmp=0 disables.');
  if (!profile.Curl) profile.Curl = { Amp: 0, Iterations: 0, DT: 0, Freq: 0, JetAmp: 0 };

  const reset = () => {
    const orig = (originalProfile && originalProfile.Curl) || {};
    profile.Curl = { Amp: orig.Amp||0, Iterations: orig.Iterations||0, DT: orig.DT||0, Freq: orig.Freq||0, JetAmp: orig.JetAmp||0 };
    commitProfile(profile);
    renderPanels();
  };
  const clear = () => { profile.Curl = { Amp: 0, Iterations: 0, DT: 0, Freq: 0, JetAmp: 0 }; commitProfile(profile); renderPanels(); };
  const randomize = () => {
    profile.Curl = {
      Amp:        round2(0.1 + Math.random() * 0.4),
      Iterations: Math.floor(6 + Math.random() * 12),
      DT:         round2(0.05 + Math.random() * 0.15),
      Freq:       round2(0.5 + Math.random() * 4),
      JetAmp:     round2(Math.random() * 0.5),
    };
    commitProfile(profile);
    renderPanels();
  };

  const summary = panel.querySelector('summary');
  summary.appendChild(makeAuxBtn('Randomize', 'Roll new in-range curl values', randomize));
  summary.appendChild(makeAuxBtn('Reset', 'Restore to loaded JSON values', reset));
  summary.appendChild(makeAuxBtn('Clear', 'Zero out curl config', clear));

  panel.appendChild(makeNumberRow('Amp', 'Displacement strength per iteration (0 disables; useful 0.1–0.5).',
    profile.Curl.Amp, 0, 1, '0.01',
    v => { profile.Curl.Amp = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Iterations', 'Backward-trace step count (4–16 typical).',
    profile.Curl.Iterations, 0, 24, '1',
    v => { profile.Curl.Iterations = Math.round(v); commitProfile(profile); }));
  panel.appendChild(makeNumberRow('DT', 'Step size per iteration (0.05–0.2 typical).',
    profile.Curl.DT, 0, 0.5, '0.01',
    v => { profile.Curl.DT = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Freq', 'Curl-noise base frequency.',
    profile.Curl.Freq, 0, 10, '0.1',
    v => { profile.Curl.Freq = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('JetAmp', 'Zonal-jet contribution per latitude band (0 disables).',
    profile.Curl.JetAmp, 0, 1, '0.01',
    v => { profile.Curl.JetAmp = v; commitProfile(profile); }));

  panels.appendChild(panel);
}

function rgbToHex(c) {
  if (!c) return '#cccccc';
  const h = n => Math.round(n).toString(16).padStart(2, '0');
  return '#' + h(c.R||0) + h(c.G||0) + h(c.B||0);
}

function hexToRgb(s) {
  const m = /^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(s);
  if (!m) return { R: 200, G: 80, B: 80 };
  return { R: parseInt(m[1], 16), G: parseInt(m[2], 16), B: parseInt(m[3], 16) };
}

function renderStormBandsPanel(profile, panels) {
  if (profile.Renderer !== 'gas_giant') return;
  const panel = makePanel('Storm Bands',
    'Hand-authored colored bands at specific latitudes (e.g. red spot, polar collars). Each entry: Lat (radians, -π/2 … π/2), HalfWidth, Color, Strength.');
  if (!profile.StormBands) profile.StormBands = [];

  const refresh = () => { commitProfile(profile); renderPanels(); };

  // Per-band rows
  profile.StormBands.forEach((band, idx) => {
    const row = document.createElement('div');
    row.className = 'storm-band-row';
    row.style.cssText = 'display:flex; gap:6px; align-items:center; margin:4px 0; padding:4px; border:1px solid #444; border-radius:4px;';
    row.innerHTML = `
      <span style="opacity:0.6">#${idx}</span>
      <label>Lat <input type="number" step="0.01" min="-1.6" max="1.6" value="${band.Lat ?? 0}" data-field="Lat" style="width:70px"></label>
      <label>HW <input type="number" step="0.01" min="0" max="1.6" value="${band.HalfWidth ?? 0.1}" data-field="HalfWidth" style="width:60px"></label>
      <label>Color <input type="color" value="${rgbToHex(band.Color)}" data-field="Color"></label>
      <label>Strength <input type="number" step="0.01" min="0" max="1" value="${band.Strength ?? 0.5}" data-field="Strength" style="width:60px"></label>
      <button data-act="del">×</button>
    `;
    row.querySelectorAll('input').forEach(inp => {
      inp.addEventListener('change', () => {
        const f = inp.dataset.field;
        if (f === 'Color') {
          band.Color = hexToRgb(inp.value);
        } else {
          band[f] = parseFloat(inp.value);
        }
        commitProfile(profile);
      });
    });
    row.querySelector('button[data-act="del"]').addEventListener('click', () => {
      profile.StormBands.splice(idx, 1);
      refresh();
    });
    panel.appendChild(row);
  });

  // Add-band footer
  const addBtn = document.createElement('button');
  addBtn.textContent = 'Add band';
  addBtn.addEventListener('click', () => {
    profile.StormBands.push({ Lat: 0, HalfWidth: 0.1, Color: { R: 200, G: 80, B: 80 }, Strength: 0.5 });
    refresh();
  });
  panel.appendChild(addBtn);

  panels.appendChild(panel);
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
      status.textContent = `Active: ${profile.LUT}`;
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
      profile.LUT = '';
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
  Detail: 'High-frequency detail noise. Adds bumpy variation to the heightmap. (Despite the legacy name "Erosion", this layer is purely additive — Phase 5 will add a separate flow-based erosion stage.)',
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

// Tracks which panels the user has manually collapsed. Survives panel
// rebuilds caused by renderPanels(), so collapsing a panel and then
// clicking any input/button on a sibling panel keeps it collapsed.
const collapsedPanels = new Set();

function bindCollapseState(details, key) {
  details.dataset.panelKey = key;
  details.open = !collapsedPanels.has(key);
  details.addEventListener('toggle', () => {
    if (details.open) collapsedPanels.delete(key);
    else collapsedPanels.add(key);
  });
}

function makePanel(title, helpText) {
  const panel = document.createElement('details');
  panel.className = 'panel';
  bindCollapseState(panel, `panel:${title}`);
  const summary = document.createElement('summary');
  summary.title = helpText;
  summary.innerHTML = `<h3>${title}</h3>`;
  panel.appendChild(summary);
  return panel;
}

function makeSubPanel(title, helpText, opts = {}) {
  const sub = document.createElement('details');
  sub.className = 'subpanel';
  bindCollapseState(sub, `sub:${title}`);
  const summary = document.createElement('summary');
  summary.title = helpText;
  summary.innerHTML = `<strong>${title}</strong>`;
  if (opts.onRandomize) {
    summary.appendChild(makeAuxBtn('Randomize',
      `Roll new random in-range values for ${title}`, opts.onRandomize));
  }
  if (opts.onReset) {
    summary.appendChild(makeAuxBtn('Reset',
      `Restore ${title} to the loaded JSON values`, opts.onReset));
  }
  if (opts.onClear) {
    summary.appendChild(makeAuxBtn('Clear',
      `Zero out all values for ${title}`, opts.onClear));
  }
  sub.appendChild(summary);
  return sub;
}

function makeAuxBtn(label, tooltip, handler) {
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'reset-btn';
  btn.textContent = label;
  btn.title = tooltip;
  btn.addEventListener('click', (e) => {
    e.preventDefault();
    e.stopPropagation();
    handler();
  });
  return btn;
}

function round2(v) { return Math.round(v * 100) / 100; }

// Reasonable in-range fBm draws for control fields.
// Ranges come from the Help panel: Amp 0.5–2, Freq 0.5–6, Octaves 2–6,
// Lacunarity 1.5–3, Persistence 0.2–0.8.
function randomFBMParams() {
  return {
    Amp: round2(0.5 + Math.random() * 1.5),
    Freq: round2(0.5 + Math.random() * 5.5),
    Octaves: 2 + Math.floor(Math.random() * 5),
    Lacunarity: round2(1.5 + Math.random() * 1.5),
    Persistence: round2(0.2 + Math.random() * 0.6),
  };
}

// Tighter ranges for warp — high warp Amp gets chaotic fast.
function randomWarpParams() {
  return {
    Amp: round2(Math.random() * 0.4),
    Freq: round2(0.5 + Math.random() * 3.5),
    Octaves: 1 + Math.floor(Math.random() * 4),
    Lacunarity: round2(1.5 + Math.random() * 1.5),
    Persistence: round2(0.2 + Math.random() * 0.6),
  };
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

// Generic numeric input row. Step controls precision; integer mode when step is "1".
function makeNumberRow(label, helpText, value, min, max, step, onCommit) {
  const row = document.createElement('label');
  row.className = 'row';
  row.title = helpText;
  row.innerHTML = `<span>${label}</span>`;
  const input = document.createElement('input');
  input.type = 'number';
  if (min !== undefined && min !== null) input.min = min;
  if (max !== undefined && max !== null) input.max = max;
  input.step = step;
  input.value = value;
  input.addEventListener('input', () => {
    const v = (step === '1' || step === 1)
      ? parseInt(input.value, 10)
      : parseFloat(input.value);
    if (!Number.isNaN(v)) onCommit(v);
  });
  row.appendChild(input);
  return row;
}

// Color picker row. Reads/writes {R,G,B,A} where A is preserved if not 0.
function makeColorRow(label, helpText, rgba, onCommit) {
  const row = document.createElement('label');
  row.className = 'row';
  row.title = helpText;
  row.innerHTML = `<span>${label}</span>`;
  const input = document.createElement('input');
  input.type = 'color';
  const hex = (n) => Math.max(0, Math.min(255, n|0)).toString(16).padStart(2, '0');
  input.value = '#' + hex(rgba.R) + hex(rgba.G) + hex(rgba.B);
  input.addEventListener('input', () => {
    const h = input.value.slice(1);
    // The browser's color input doesn't expose alpha. Always commit A=255
    // (otherwise a zero-default RGBA would persist transparent ocean even
    // after the user picks a color in the picker).
    onCommit({
      R: parseInt(h.slice(0,2), 16),
      G: parseInt(h.slice(2,4), 16),
      B: parseInt(h.slice(4,6), 16),
      A: 255,
    });
  });
  row.appendChild(input);
  return row;
}

// Boolean checkbox row.
function makeCheckboxRow(label, helpText, value, onCommit) {
  const row = document.createElement('label');
  row.className = 'row';
  row.title = helpText;
  row.innerHTML = `<span>${label}</span>`;
  const input = document.createElement('input');
  input.type = 'checkbox';
  input.checked = !!value;
  input.addEventListener('input', () => onCommit(input.checked));
  row.appendChild(input);
  return row;
}

function renderCratersPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Craters',
    'Stamped circular depressions on the heightmap. Applied after fBm/control fields, before coloring. Set Count=0 to disable.');

  const reset = () => {
    const orig = originalProfile || {};
    profile.CraterCount        = orig.CraterCount        != null ? orig.CraterCount        : 0;
    profile.CraterMinRadius    = orig.CraterMinRadius    != null ? orig.CraterMinRadius    : 0;
    profile.CraterMaxRadius    = orig.CraterMaxRadius    != null ? orig.CraterMaxRadius    : 0;
    profile.CraterDepth        = orig.CraterDepth        != null ? orig.CraterDepth        : 0;
    profile.PowerLawAlpha      = orig.PowerLawAlpha      != null ? orig.PowerLawAlpha      : 0;
    profile.MariaDensityFactor = orig.MariaDensityFactor != null ? orig.MariaDensityFactor : 0;
    profile.SurfaceAge         = orig.SurfaceAge         != null ? orig.SurfaceAge         : 0;
    profile.SecondaryDensity   = orig.SecondaryDensity   != null ? orig.SecondaryDensity   : 0;
    commitProfile(profile);
    renderPanels();
  };
  const clear = () => {
    profile.CraterCount = 0;
    profile.CraterMinRadius = 0;
    profile.CraterMaxRadius = 0;
    profile.CraterDepth = 0;
    profile.PowerLawAlpha = 0;
    profile.MariaDensityFactor = 0;
    profile.SurfaceAge = 0;
    profile.SecondaryDensity = 0;
    commitProfile(profile);
    renderPanels();
  };
  const randomize = () => {
    const minR = round3(0.001 + Math.random() * 0.019);   // 0.001–0.020
    const maxR = round3(Math.max(minR + 0.005, 0.01 + Math.random() * 0.09)); // 0.01–0.10
    profile.CraterCount        = Math.floor(Math.random() * 201);  // 0–200
    profile.CraterMinRadius    = minR;
    profile.CraterMaxRadius    = maxR;
    profile.CraterDepth        = round2(0.02 + Math.random() * 0.28); // 0.02–0.30
    profile.PowerLawAlpha      = round2(1.5 + Math.random() * 1);     // 1.5–2.5
    profile.MariaDensityFactor = round2(Math.random() * 0.6);
    profile.SurfaceAge         = round2(0.3 + Math.random() * 0.6);   // 0.3–0.9
    profile.SecondaryDensity   = round2(Math.random() * 0.5);
    commitProfile(profile);
    renderPanels();
  };

  const summary = panel.querySelector('summary');
  summary.appendChild(makeAuxBtn('Randomize', 'Roll new random in-range crater params', randomize));
  summary.appendChild(makeAuxBtn('Reset', 'Restore craters to the loaded JSON values', reset));
  summary.appendChild(makeAuxBtn('Clear', 'Zero out all crater params', clear));

  panel.appendChild(makeNumberRow('CraterCount',
    'How many craters to stamp. 0 = none; 200 ≈ Mercury-dense.',
    profile.CraterCount || 0, 0, 500, '1',
    v => { profile.CraterCount = Math.max(0, Math.round(v)); commitProfile(profile); }));
  panel.appendChild(makeNumberRow('CraterMinRadius',
    'Smallest crater angular radius (radians on the unit sphere). Useful 0.001–0.02.',
    profile.CraterMinRadius || 0, 0, 0.05, '0.001',
    v => { profile.CraterMinRadius = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('CraterMaxRadius',
    'Largest crater angular radius. Should be > MinRadius. Useful 0.01–0.1.',
    profile.CraterMaxRadius || 0, 0, 0.2, '0.001',
    v => { profile.CraterMaxRadius = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('CraterDepth',
    'How much each crater carves into the heightmap. Useful 0.02–0.3.',
    profile.CraterDepth || 0, 0, 0.5, '0.01',
    v => { profile.CraterDepth = v; commitProfile(profile); }));

  panel.appendChild(makeNumberRow('PowerLawAlpha',
    'Size-frequency slope. 0 = legacy uniform-with-quadratic-bias (also disables age + ejecta). 2.0 ≈ realistic many-small/few-large; higher = even more skewed.',
    profile.PowerLawAlpha || 0, 0, 4, '0.1',
    v => { profile.PowerLawAlpha = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('MariaDensityFactor',
    'Strength of the maria mask: bright low-freq fBm regions reject 70% of crater candidates. 0 = disabled.',
    profile.MariaDensityFactor || 0, 0, 1, '0.05',
    v => { profile.MariaDensityFactor = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('SurfaceAge',
    'Beta-distributed age bias. 0 = mostly young/sharp; 1 = mostly old/eroded. Default ~0.7.',
    profile.SurfaceAge || 0, 0, 1, '0.05',
    v => { profile.SurfaceAge = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('SecondaryDensity',
    'Small ejecta-cluster bowls around each large primary. 0 = disabled; 1 = up to ~15 secondaries per large.',
    profile.SecondaryDensity || 0, 0, 1, '0.05',
    v => { profile.SecondaryDensity = v; commitProfile(profile); }));

  panels.appendChild(panel);
}

function renderErosionPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Erosion',
    'Particle hydraulic erosion: droplets walk the heightmap, carving channels and depositing sediment. Droplets=0 disables. The renderer auto-scales droplet count by face area, with a 5000 floor so face=64 previews still show channels; full canonical count runs at face=1024.');
  if (!profile.Erosion) profile.Erosion = {};
  const e = profile.Erosion;
  const reset = () => {
    const orig = (originalProfile && originalProfile.Erosion) || {};
    profile.Erosion = JSON.parse(JSON.stringify(orig));
    commitProfile(profile);
    renderPanels();
  };
  const clear = () => { profile.Erosion = {}; commitProfile(profile); renderPanels(); };
  const randomize = () => {
    profile.Erosion = {
      Droplets:        Math.round(50000 + Math.random() * 150000),
      Inertia:         round2(0.02 + Math.random() * 0.15),
      Capacity:        round2(2 + Math.random() * 6),
      ErosionRate:     round2(0.1 + Math.random() * 0.5),
      Deposition:      round2(0.1 + Math.random() * 0.5),
      Evaporation:     round2(0.005 + Math.random() * 0.04),
      MinSlope:        round2(0.005 + Math.random() * 0.03),
      MaxStepsPerDrop: 30 + Math.floor(Math.random() * 50),
      Gravity:         round2(2 + Math.random() * 6),
      BrushFalloff:    round2(0.5 + Math.random() * 5),
    };
    commitProfile(profile);
    renderPanels();
  };

  const summary = panel.querySelector('summary');
  summary.appendChild(makeAuxBtn('Randomize', 'Roll new in-range erosion params', randomize));
  summary.appendChild(makeAuxBtn('Reset', 'Restore to loaded JSON values', reset));
  summary.appendChild(makeAuxBtn('Clear', 'Zero out erosion config', clear));

  panel.appendChild(makeNumberRow('Droplets',
    'Canonical droplet count at face=1024. Auto-scaled by face area; floor 5000 so previews still carve channels. 0 disables.',
    e.Droplets || 0, 0, 500000, '1000',
    v => { profile.Erosion.Droplets = Math.max(0, Math.round(v)); commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Inertia',
    '[0,1]: how much velocity carries between steps. 0.05 default; higher = straighter channels.',
    e.Inertia || 0, 0, 1, '0.01',
    v => { profile.Erosion.Inertia = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Capacity',
    'Sediment capacity multiplier. 4 default; higher = droplets carry more before depositing.',
    e.Capacity || 0, 0, 20, '0.1',
    v => { profile.Erosion.Capacity = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('ErosionRate',
    'Fraction of "missing capacity" carved per step. 0.3 default.',
    e.ErosionRate || 0, 0, 1, '0.05',
    v => { profile.Erosion.ErosionRate = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Deposition',
    'Fraction of "excess sediment" dropped per step. 0.3 default.',
    e.Deposition || 0, 0, 1, '0.05',
    v => { profile.Erosion.Deposition = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Evaporation',
    'Water lost per step. 0.01 default.',
    e.Evaporation || 0, 0, 0.1, '0.005',
    v => { profile.Erosion.Evaporation = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('MinSlope',
    'Floor on slope used in capacity calc to avoid 0. 0.01 default.',
    e.MinSlope || 0, 0, 0.5, '0.005',
    v => { profile.Erosion.MinSlope = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('MaxStepsPerDrop',
    'Hard cap on steps per droplet. 50 default.',
    e.MaxStepsPerDrop || 0, 0, 200, '5',
    v => { profile.Erosion.MaxStepsPerDrop = Math.max(0, Math.round(v)); commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Gravity',
    'Speed gain from -Δh per step. 4 default.',
    e.Gravity || 0, 0, 20, '0.1',
    v => { profile.Erosion.Gravity = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('BrushFalloff',
    'Brush sharpness exponent in 1/(1+r)^k. 0/missing = 1.0 (3-pixel wide channels). 4-8 = near-single-pixel for narrow rivers.',
    e.BrushFalloff || 0, 0, 16, '0.5',
    v => { profile.Erosion.BrushFalloff = v; commitProfile(profile); }));

  panels.appendChild(panel);
}

function round3(v) { return Math.round(v * 1000) / 1000; }

function renderCryospherePanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Cryosphere',
    'Polar ice caps (latitude-based) and snow line (elevation-based). Both paint white-tinted overlays as a final color step.');
  panel.appendChild(makeCheckboxRow('HasPolarCaps',
    'Master toggle for polar-cap rendering.',
    profile.HasPolarCaps,
    v => { profile.HasPolarCaps = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('PolarCapSize',
    'Latitude fraction covered by caps (0 = none; 0.4 ≈ Hoth-like).',
    profile.PolarCapSize || 0, 0, 0.5, '0.01',
    v => { profile.PolarCapSize = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('PolarCapNoise',
    'Edge roughness of the cap boundary. 0 = smooth circle; 0.5 = jagged.',
    profile.PolarCapNoise || 0, 0, 0.5, '0.01',
    v => { profile.PolarCapNoise = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('SnowLine',
    'Elevation [0,1] above which pixels get a snow tint. 0 = disabled.',
    profile.SnowLine || 0, 0, 1, '0.01',
    v => { profile.SnowLine = v; commitProfile(profile); }));
  panels.appendChild(panel);
}

function renderHeightSmoothPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Height Smoothing',
    'Per-face disc blur applied to the heightmap before Normalize. 0 disables. 2-3 smooths fbm popcorn so erosion can form coherent channels; larger values produce broader, gentler terrain at the cost of surface texture.');
  panel.appendChild(makeNumberRow('Radius',
    'Blur radius in pixels. 0 disables; 2-3 typical for terran/super_terran; up to 5 for very smooth worlds.',
    profile.HeightSmoothRadius || 0, 0, 8, '1',
    v => { profile.HeightSmoothRadius = Math.max(0, Math.round(v)); commitProfile(profile); }));
  panels.appendChild(panel);
}

function renderCoastalPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Coastal',
    'Localized roughening of pixels near coast lines (requires OceanLevel > 0). Combines three high-frequency fBm bands modulated by distance-to-coast. Amp=0 disables.');
  if (!profile.Coastal) profile.Coastal = { Amp: 0, Threshold: 0, Freq: 0 };

  const reset = () => {
    const orig = (originalProfile && originalProfile.Coastal) || {};
    profile.Coastal = { Amp: orig.Amp||0, Threshold: orig.Threshold||0, Freq: orig.Freq||0 };
    commitProfile(profile);
    renderPanels();
  };
  const clear = () => { profile.Coastal = { Amp: 0, Threshold: 0, Freq: 0 }; commitProfile(profile); renderPanels(); };
  const randomize = () => {
    profile.Coastal = {
      Amp:       round2(0.05 + Math.random() * 0.15),
      Threshold: round2(0.05 + Math.random() * 0.20),
      Freq:      round2(1 + Math.random() * 4),
    };
    commitProfile(profile);
    renderPanels();
  };

  const summary = panel.querySelector('summary');
  summary.appendChild(makeAuxBtn('Randomize', 'Roll new in-range coastal values', randomize));
  summary.appendChild(makeAuxBtn('Reset', 'Restore to loaded JSON values', reset));
  summary.appendChild(makeAuxBtn('Clear', 'Zero out coastal config', clear));

  panel.appendChild(makeNumberRow('Amp', 'Master strength (0 disables; useful 0.05–0.2).',
    profile.Coastal.Amp, 0, 0.5, '0.01',
    v => { profile.Coastal.Amp = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Threshold', 'Distance-to-coast cutoff [0,1]. Effect dies above this.',
    profile.Coastal.Threshold, 0, 1, '0.01',
    v => { profile.Coastal.Threshold = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Freq', 'Base fbm frequency for the n4 octave (n5/n6 derive from it).',
    profile.Coastal.Freq, 0, 20, '0.1',
    v => { profile.Coastal.Freq = v; commitProfile(profile); }));

  panels.appendChild(panel);
}

function renderContinentsPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Continents',
    'Voronoi continents from Fibonacci-spiral seed points. Each seed gets a random base height in [HeightLo, HeightHi]; the heightmap is raised to that floor, so continents form a baseline. Seeds=0 disables.');
  if (!profile.Continents) profile.Continents = { Seeds: 0, WarpAmp: 0, WarpFreq: 0, HeightLo: 0.3, HeightHi: 0.7 };

  const reset = () => {
    const orig = (originalProfile && originalProfile.Continents) || {};
    profile.Continents = {
      Seeds:    orig.Seeds||0,
      WarpAmp:  orig.WarpAmp||0,
      WarpFreq: orig.WarpFreq||0,
      HeightLo: orig.HeightLo!==undefined ? orig.HeightLo : 0.3,
      HeightHi: orig.HeightHi!==undefined ? orig.HeightHi : 0.7,
    };
    commitProfile(profile);
    renderPanels();
  };
  const clear = () => {
    profile.Continents = { Seeds: 0, WarpAmp: 0, WarpFreq: 0, HeightLo: 0.3, HeightHi: 0.7 };
    commitProfile(profile);
    renderPanels();
  };
  const randomize = () => {
    const lo = round2(0.25 + Math.random() * 0.20);
    const hi = round2(lo + 0.10 + Math.random() * 0.20);
    profile.Continents = {
      Seeds:    Math.floor(10 + Math.random() * 40),
      WarpAmp:  round2(Math.random() * 0.2),
      WarpFreq: round2(0.5 + Math.random() * 4),
      HeightLo: lo,
      HeightHi: hi,
    };
    commitProfile(profile);
    renderPanels();
  };

  const summary = panel.querySelector('summary');
  summary.appendChild(makeAuxBtn('Randomize', 'Roll new in-range continent values', randomize));
  summary.appendChild(makeAuxBtn('Reset', 'Restore to loaded JSON values', reset));
  summary.appendChild(makeAuxBtn('Clear', 'Zero out continents config', clear));

  panel.appendChild(makeNumberRow('Seeds', 'Number of continent seeds (0 disables; 10–50 typical).',
    profile.Continents.Seeds, 0, 50, '1',
    v => { profile.Continents.Seeds = Math.round(v); commitProfile(profile); }));
  panel.appendChild(makeNumberRow('WarpAmp', 'Sphere-warp displacement amplitude (0 disables warp).',
    profile.Continents.WarpAmp, 0, 0.3, '0.01',
    v => { profile.Continents.WarpAmp = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('WarpFreq', 'fbm frequency for the sphere-warp.',
    profile.Continents.WarpFreq, 0, 10, '0.1',
    v => { profile.Continents.WarpFreq = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('HeightLo', 'Per-continent base height lower bound [0,1].',
    profile.Continents.HeightLo, 0, 1, '0.01',
    v => { profile.Continents.HeightLo = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('HeightHi', 'Per-continent base height upper bound [0,1].',
    profile.Continents.HeightHi, 0, 1, '0.01',
    v => { profile.Continents.HeightHi = v; commitProfile(profile); }));

  panels.appendChild(panel);
}

function renderProvincePanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  if (!profile.Provinces) profile.Provinces = {Count: 0, Jitter: 0, WarpAmp: 0};
  const panel = makePanel('Provinces',
    'Voronoi cells over the sphere, each with a per-cell amp/freq scalar applied to the control fields. Gives each archetype regional roughness variety. Count=0 disables.');

  const reset = () => {
    if (originalProfile && originalProfile.Provinces) {
      profile.Provinces = JSON.parse(JSON.stringify(originalProfile.Provinces));
    } else {
      profile.Provinces = {Count: 0, Jitter: 0, WarpAmp: 0};
    }
    commitProfile(profile); renderPanels();
  };
  const clear = () => {
    profile.Provinces = {Count: 0, Jitter: 0, WarpAmp: 0};
    commitProfile(profile); renderPanels();
  };
  const randomize = () => {
    profile.Provinces = {
      Count:   8 + Math.floor(Math.random() * 24),  // 8-32
      Jitter:  round2(0.1 + Math.random() * 0.3),   // 0.1-0.4
      WarpAmp: round2(Math.random() * 0.15),        // 0-0.15
    };
    commitProfile(profile); renderPanels();
  };

  const summary = panel.querySelector('summary');
  summary.appendChild(makeAuxBtn('Randomize', 'Roll random province params', randomize));
  summary.appendChild(makeAuxBtn('Reset', 'Restore Provinces to loaded JSON', reset));
  summary.appendChild(makeAuxBtn('Clear', 'Disable provinces', clear));

  panel.appendChild(makeNumberRow('Count',
    'Number of Voronoi cells (8-40 typical; 0 = disabled).',
    profile.Provinces.Count, 0, 64, '1',
    v => { profile.Provinces.Count = Math.max(0, Math.round(v)); commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Jitter',
    'Per-cell scalar jitter strength. 0 = uniform; 0.5 = high regional variety.',
    profile.Provinces.Jitter, 0, 0.5, '0.01',
    v => { profile.Provinces.Jitter = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('WarpAmp',
    'Sphere-warp displacement before nearest-cell lookup. 0 = clean Voronoi; >0 = curvy boundaries.',
    profile.Provinces.WarpAmp, 0, 0.3, '0.01',
    v => { profile.Provinces.WarpAmp = v; commitProfile(profile); }));
  panels.appendChild(panel);
}

function renderShadingPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Shading',
    'Slope-based Lambertian shading. Computes a surface normal from the heightmap gradient and modulates color by light·normal. Strength 0 = no shading; the planet looks flat.');
  panel.appendChild(makeNumberRow('ShadingStrength',
    'How strongly diffuse lighting modulates color. 0 = off; 0.5 is a reasonable starting point.',
    profile.ShadingStrength || 0, 0, 1, '0.05',
    v => { profile.ShadingStrength = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('ShadingExaggeration',
    'Heightmap-gradient multiplier. Higher = more dramatic relief. 0 = use 8.0 default.',
    profile.ShadingExaggeration || 0, 0, 50, '0.5',
    v => { profile.ShadingExaggeration = v; commitProfile(profile); }));
  panels.appendChild(panel);
}

function renderRidgedPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  if (!profile.Ridged) {
    profile.Ridged = {Amp:0, Freq:0, Octaves:0, Lacunarity:0, Gain:0, Offset:0, MaskLow:0, MaskHigh:0};
  }
  const panel = makePanel('Ridged mountains',
    'Ridged-multifractal mountain belts. Masked by Continentalness output so ridges only form on land. Amp=0 disables.');

  const reset = () => {
    if (originalProfile && originalProfile.Ridged) {
      profile.Ridged = JSON.parse(JSON.stringify(originalProfile.Ridged));
    } else {
      profile.Ridged = {Amp:0, Freq:0, Octaves:0, Lacunarity:0, Gain:0, Offset:0, MaskLow:0, MaskHigh:0};
    }
    commitProfile(profile); renderPanels();
  };
  const clear = () => {
    profile.Ridged = {Amp:0, Freq:0, Octaves:0, Lacunarity:0, Gain:0, Offset:0, MaskLow:0, MaskHigh:0};
    commitProfile(profile); renderPanels();
  };
  const randomize = () => {
    profile.Ridged = {
      Amp:        round2(0.05 + Math.random() * 0.25),
      Freq:       round2(0.5 + Math.random() * 4),
      Octaves:    3 + Math.floor(Math.random() * 4),
      Lacunarity: round2(1.8 + Math.random() * 0.6),
      Gain:       round2(0.4 + Math.random() * 0.4),
      Offset:     round2(0.8 + Math.random() * 0.4),
      MaskLow:    round2(0.3 + Math.random() * 0.2),
      MaskHigh:   round2(0.6 + Math.random() * 0.2),
    };
    commitProfile(profile); renderPanels();
  };

  const summary = panel.querySelector('summary');
  summary.appendChild(makeAuxBtn('Randomize', 'Roll random in-range ridged params', randomize));
  summary.appendChild(makeAuxBtn('Reset', 'Restore Ridged to loaded JSON values', reset));
  summary.appendChild(makeAuxBtn('Clear', 'Zero out all ridged params', clear));

  const help = {
    Amp:        'Overall mountain contribution to the heightmap. 0 = disabled. Useful 0.05–0.3.',
    Freq:       'Base spatial frequency of the ridges. Higher = more rugged. Useful 0.5–5.',
    Octaves:    'Stacked ridged-fbm layers. More = more detail. Typical 4–6.',
    Lacunarity: 'Frequency multiplier per octave. Standard = 2.0.',
    Gain:       'Per-octave weight gain. Standard = 0.5. Higher = sharper ridges.',
    Offset:     'Ridge sharpness. 1.0 default; values > 1 produce sharper peaks.',
    MaskLow:    'Continentalness-spline output ≤ this = no ridges (deep ocean).',
    MaskHigh:   'Continentalness-spline output ≥ this = full ridges (interior).',
  };
  for (const param of ['Amp','Freq','Octaves','Lacunarity','Gain','Offset','MaskLow','MaskHigh']) {
    const isInt = (param === 'Octaves');
    panel.appendChild(makeNumberRow(param, help[param],
      profile.Ridged[param] || 0, 0, isInt ? 8 : 5, isInt ? '1' : '0.01',
      v => { profile.Ridged[param] = v; commitProfile(profile); }));
  }
  panels.appendChild(panel);
}

function renderOceanPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  if (!profile.OceanColor) profile.OceanColor = {R: 0, G: 0, B: 0, A: 0};
  const panel = makePanel('Ocean',
    'Below the OceanLevel cutoff, height is painted with OceanColor (depth-shaded). Set OceanLevel = 0 to disable oceans entirely.');
  panel.appendChild(makeNumberRow('OceanLevel',
    'Normalized [0,1] sea-level cutoff. Pixels with height < OceanLevel are painted ocean.',
    profile.OceanLevel || 0, 0, 1, '0.01',
    v => { profile.OceanLevel = v; commitProfile(profile); }));
  panel.appendChild(makeColorRow('OceanColor',
    'Base ocean color. Depth shading darkens it for deeper pixels.',
    profile.OceanColor,
    rgba => { profile.OceanColor = rgba; commitProfile(profile); }));
  panels.appendChild(panel);
}

function renderWarpPanel(profile, panels) {
  if (!profile.Warp) profile.Warp = {Amp: 0, Freq: 0, Octaves: 0, Lacunarity: 0, Persistence: 0};
  const panel = makePanel('Domain warp',
    'Quilez domain warp. Displaces sphere directions before sampling noise so features bend/curl instead of being axis-aligned. Amp=0 disables warp entirely.');
  const reset = () => {
    if (originalProfile?.Warp) {
      profile.Warp = JSON.parse(JSON.stringify(originalProfile.Warp));
    } else {
      profile.Warp = {Amp: 0, Freq: 0, Octaves: 0, Lacunarity: 0, Persistence: 0};
    }
    commitProfile(profile);
    renderPanels();
  };
  const clear = () => {
    profile.Warp = {Amp: 0, Freq: 0, Octaves: 0, Lacunarity: 0, Persistence: 0};
    commitProfile(profile);
    renderPanels();
  };
  const randomize = () => {
    profile.Warp = randomWarpParams();
    commitProfile(profile);
    renderPanels();
  };
  const summary = panel.querySelector('summary');
  summary.appendChild(makeAuxBtn('Randomize', 'Roll new random in-range warp params', randomize));
  summary.appendChild(makeAuxBtn('Reset', 'Restore Warp to the loaded JSON values', reset));
  summary.appendChild(makeAuxBtn('Clear', 'Zero out all warp params', clear));
  for (const param of ['Amp', 'Freq', 'Octaves', 'Lacunarity', 'Persistence']) {
    panel.appendChild(makeParamRow(param,
      () => profile.Warp[param] || 0,
      (v) => { profile.Warp[param] = v; commitProfile(profile); },
      `Warp ${param}: ${PARAM_HELP[param]}`));
  }
  panels.appendChild(panel);
}

function renderControlFieldsPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const cc = profile.ControlConfig;
  if (!cc) return;
  const panel = makePanel('Control fields',
    'Five 3D fBm noise fields. Continentalness/Detail/PeaksValleys are summed via splines to build the heightmap. Temperature/Humidity feed the Whittaker biome lookup. Each field has independent fBm settings + an optional spline.');
  const fields = ['Continentalness', 'Detail', 'PeaksValleys', 'Temperature', 'Humidity'];
  for (let i = 0; i < fields.length; i++) {
    const fieldName = fields[i];
    const cf = cc[fieldName];
    if (!cf) continue;
    const reset = () => {
      const orig = originalProfile?.ControlConfig?.[fieldName];
      if (orig) {
        // Deep-restore so the embedded Spline knots reset back to the
        // original loaded values, not a shared reference.
        Object.assign(cf, JSON.parse(JSON.stringify(orig)));
      }
      if (!cf.Spline) cf.Spline = {Knots: []};
      commitProfile(profile);
      renderPanels();
    };
    const clear = () => {
      cf.Amp = 0; cf.Freq = 0; cf.Octaves = 0; cf.Lacunarity = 0; cf.Persistence = 0;
      cf.Spline = {Knots: []};
      commitProfile(profile);
      renderPanels();
    };
    const randomize = () => {
      Object.assign(cf, randomFBMParams());
      commitProfile(profile);
      renderPanels();
    };
    const sub = makeSubPanel(fieldName, FIELD_HELP[fieldName],
      {onReset: reset, onClear: clear, onRandomize: randomize});
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
    if (!cf.Spline) cf.Spline = {Knots: []};
    const ta = document.createElement('textarea');
    ta.rows = 2;
    ta.className = 'knots-input';
    ta.dataset.fieldName = fieldName;
    ta.value = JSON.stringify(cf.Spline.Knots || []);
    ta.placeholder = '[{"Input":0,"Output":0},{"Input":1,"Output":0.5}]';
    ta.title = knotsLabel.title;
    const knotsErr = document.createElement('div');
    knotsErr.className = 'knots-err';
    ta.addEventListener('input', () => {
      try {
        const knots = JSON.parse(ta.value);
        if (!Array.isArray(knots)) throw new Error('expected array');
        for (const k of knots) {
          if (typeof k !== 'object' || k === null
              || typeof k.Input !== 'number' || typeof k.Output !== 'number') {
            throw new Error('each knot needs {"Input":<num>,"Output":<num>}');
          }
        }
        cf.Spline = {Knots: knots};
        commitProfile(profile);
        knotsErr.textContent = '';
        ta.classList.remove('knots-bad');
        status.textContent = `Knots committed: ${fieldName} (${knots.length})`;
      } catch (e) {
        knotsErr.textContent = `${e.message}`;
        ta.classList.add('knots-bad');
        status.textContent = `Bad knots JSON in ${fieldName}`;
      }
    });
    sub.appendChild(ta);
    sub.appendChild(knotsErr);
    panel.appendChild(sub);
  }
  panels.appendChild(panel);
}

function rgbaCSS(c) {
  return `rgba(${c.R}, ${c.G}, ${c.B}, ${(c.A/255).toFixed(2)})`;
}

// --- Rotating-sphere preview (adapted from kb planet-detail pages) ---
// Pure 2D canvas, no external libs. Texture is the equirect bake;
// since equirect is generated from the cube map, the sphere visually
// represents the cube-map content.

const sphereCanvas = $('#sphere-canvas');
let sphereTextureData = null, sphereTexW = 0, sphereTexH = 0;
let sphereRotY = 0.4, sphereRotX = 0.15;
let sphereDragging = false, sphereLastMX = 0, sphereLastMY = 0;
let sphereAutoRotate = true;
let sphereResumeTimer = null;

// Offscreen canvas for the Phase 9b Black-Marble nightside equirect.
// Same dimensions as the visible equirect canvas so the (u, v) sample
// indices line up with the day texture. Document-detached so it does
// not show up in the page; the renderSphere loop reads its ImageData
// directly via refreshSphereNightTexture.
const nightEquirectCanvas = (() => {
  const c = document.createElement('canvas');
  c.width = equirectCanvas ? equirectCanvas.width : 800;
  c.height = equirectCanvas ? equirectCanvas.height : 400;
  return c;
})();
let sphereNightTextureData = null;

function refreshSphereTexture() {
  if (!sphereCanvas || !equirectCanvas) return;
  const ectx = equirectCanvas.getContext('2d', { willReadFrequently: true });
  try {
    sphereTextureData = ectx.getImageData(0, 0, equirectCanvas.width, equirectCanvas.height).data;
    sphereTexW = equirectCanvas.width;
    sphereTexH = equirectCanvas.height;
  } catch (e) {
    sphereTextureData = null;
  }
}

function refreshSphereNightTexture() {
  const ectx = nightEquirectCanvas.getContext('2d', { willReadFrequently: true });
  try {
    sphereNightTextureData = ectx.getImageData(0, 0, nightEquirectCanvas.width, nightEquirectCanvas.height).data;
  } catch (e) {
    sphereNightTextureData = null;
  }
}

function sampleSphereTex(u, v) {
  if (!sphereTextureData) return [80, 80, 80];
  let px = Math.floor(u * sphereTexW) % sphereTexW;
  let py = Math.floor(v * sphereTexH);
  if (px < 0) px += sphereTexW;
  py = Math.max(0, Math.min(sphereTexH - 1, py));
  const i = (py * sphereTexW + px) * 4;
  return [sphereTextureData[i], sphereTextureData[i + 1], sphereTextureData[i + 2]];
}

function sampleSphereNightTex(u, v) {
  if (!sphereNightTextureData) return [0, 0, 0];
  let px = Math.floor(u * sphereTexW) % sphereTexW;
  let py = Math.floor(v * sphereTexH);
  if (px < 0) px += sphereTexW;
  py = Math.max(0, Math.min(sphereTexH - 1, py));
  const i = (py * sphereTexW + px) * 4;
  return [sphereNightTextureData[i], sphereNightTextureData[i + 1], sphereNightTextureData[i + 2]];
}

function renderSphere() {
  if (!sphereCanvas) return;
  const ctx = sphereCanvas.getContext('2d');
  const W = sphereCanvas.width, H = sphereCanvas.height;
  const CX = W / 2, CY = H / 2, RADIUS = Math.min(W, H) / 2 - 20;
  const id = ctx.createImageData(W, H);
  const d = id.data;
  const cRX = Math.cos(sphereRotX), sRX = Math.sin(sphereRotX);
  const cRY = Math.cos(sphereRotY), sRY = Math.sin(sphereRotY);
  // Sun direction. Pushed mostly to the west (-X) so the day/night
  // terminator runs near the horizontal center of the visible disc.
  // This makes the Phase 9b Black-Marble nightside visible during a
  // normal rotation; previously lz=0.7 kept the entire viewport lit
  // and the night cube-map only showed at the very edge.
  const lx = -0.85, ly = 0.30, lz = 0.45;
  const ll = Math.sqrt(lx * lx + ly * ly + lz * lz);
  for (let py = 0; py < H; py++) {
    for (let px = 0; px < W; px++) {
      const dx = px - CX, dy = py - CY, dSq = dx * dx + dy * dy;
      const idx = (py * W + px) * 4;
      let pR = 17, pG = 17, pB = 17;
      if (dSq <= RADIUS * RADIUS) {
        const nx = dx / RADIUS, ny = -dy / RADIUS;
        const nz = Math.sqrt(Math.max(0, 1 - nx * nx - ny * ny));
        const y1 = ny * cRX - nz * sRX, z1 = ny * sRX + nz * cRX;
        const x2 = nx * cRY + z1 * sRY, z2 = -nx * sRY + z1 * cRY;
        const lat = Math.asin(Math.max(-1, Math.min(1, y1)));
        const lon = Math.atan2(x2, z2);
        const u = (lon / (2 * Math.PI)) + 0.5;
        const v = 0.5 - lat / Math.PI;
        const [tr, tg, tb] = sampleSphereTex(u, v);
        const dot = (nx * lx + (-ny) * ly + nz * lz) / ll;
        const lit = Math.max(0.08, dot * 0.7 + 0.3);
        const rim = 1 - nz, rg = rim * rim * rim * 0.3;
        // Phase 9b Black-Marble blend: when civ is enabled, lights on
        // the unlit hemisphere glow at full intensity. nightWeight
        // ramps from 0 at the terminator (dot=0) to 1 at the antipode
        // of the sun (dot=-1). When sphereNightTextureData is null
        // (civ disabled or non-rocky archetype) the term vanishes.
        let nr = 0, ng = 0, nb = 0;
        if (sphereNightTextureData) {
          const nightWeight = Math.max(0, -dot);
          if (nightWeight > 0) {
            const [er, eg, eb] = sampleSphereNightTex(u, v);
            nr = er * nightWeight;
            ng = eg * nightWeight;
            nb = eb * nightWeight;
          }
        }
        pR = Math.min(255, Math.floor(tr * lit + nr + rg * 80));
        pG = Math.min(255, Math.floor(tg * lit + ng + rg * 120));
        pB = Math.min(255, Math.floor(tb * lit + nb + rg * 180));
      }
      d[idx] = pR; d[idx + 1] = pG; d[idx + 2] = pB; d[idx + 3] = 255;
    }
  }
  ctx.putImageData(id, 0, 0);
}

function animateSphere() {
  if (!sphereCanvas) return;
  if (sphereAutoRotate && !sphereDragging) sphereRotY += 0.003;
  renderSphere();
  requestAnimationFrame(animateSphere);
}

if (sphereCanvas) {
  sphereCanvas.addEventListener('mousedown', e => {
    sphereDragging = true; sphereAutoRotate = false;
    sphereLastMX = e.clientX; sphereLastMY = e.clientY;
    if (sphereResumeTimer) clearTimeout(sphereResumeTimer);
  });
  window.addEventListener('mousemove', e => {
    if (!sphereDragging) return;
    sphereRotY += (e.clientX - sphereLastMX) * 0.01;
    sphereRotX += (e.clientY - sphereLastMY) * 0.01;
    sphereRotX = Math.max(-Math.PI / 2, Math.min(Math.PI / 2, sphereRotX));
    sphereLastMX = e.clientX; sphereLastMY = e.clientY;
  });
  window.addEventListener('mouseup', () => {
    if (sphereDragging) {
      sphereDragging = false;
      sphereResumeTimer = setTimeout(() => {
        if (!sphereDragging) sphereAutoRotate = true;
      }, 3000);
    }
  });
  animateSphere();
}

// === Phase 6: pipeline debug view ===
const debugBypass = new Set();

function refreshDebugView() {
  if (!window.planetExplorerGenerateDebug) {
    console.warn('debug API not available; rebuild wasm');
    return;
  }
  const profileJSON = profileTextarea.value;
  const seed = seedInput.value;
  const size = parseInt(faceSizeSel.value, 10) || 256;
  const bypassJSON = JSON.stringify([...debugBypass]);
  const result = window.planetExplorerGenerateDebug(profileJSON, seed, size, bypassJSON);
  let parsed;
  try { parsed = JSON.parse(result); }
  catch (e) { console.error('debug parse', e); return; }
  if (parsed.error) { console.error(parsed.error); return; }
  renderDebugGrid(parsed.stages);
}

function renderDebugGrid(stages) {
  const grid = $('#debug-grid');
  grid.innerHTML = '';
  const makeHeader = (titles) => {
    const h = document.createElement('div');
    h.className = 'debug-row debug-header';
    const blank = document.createElement('div');
    blank.className = 'label';
    h.appendChild(blank);
    for (const t of titles) {
      const c = document.createElement('div');
      c.className = 'col-title';
      c.textContent = t;
      h.appendChild(c);
    }
    return h;
  };
  const headerFor = (kind) => {
    if (kind === 'color') return ['color after', '—', '—', '—'];
    if (kind === 'field') return ['field', '—', '—', '—'];
    return ['raw', 'input bands', 'output bands', 'sum after'];
  };
  let lastKind = null;
  for (const s of stages) {
    if (s.kind !== lastKind) {
      grid.appendChild(makeHeader(headerFor(s.kind)));
      lastKind = s.kind;
    }
    const row = document.createElement('div');
    row.className = 'debug-row' + (s.skipped ? ' skipped' : '');

    const label = document.createElement('div');
    label.className = 'label';
    label.appendChild(document.createTextNode(s.name));
    const toggle = document.createElement('label');
    toggle.className = 'bypass-toggle';
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.checked = debugBypass.has(s.name);
    cb.addEventListener('change', () => {
      if (cb.checked) debugBypass.add(s.name); else debugBypass.delete(s.name);
      // Toggling a bypass affects both the debug grid AND the main
      // sphere preview, so re-render both. regenerate() refreshes the
      // debug grid as well when the panel is open, so just call it.
      regenerate();
    });
    toggle.appendChild(cb);
    toggle.appendChild(document.createTextNode(' bypass'));
    label.appendChild(toggle);
    row.appendChild(label);

    let keys;
    if (s.kind === 'color') keys = ['color_after', null, null, null];
    else if (s.kind === 'field') keys = ['field_after', null, null, null];
    else keys = ['raw', 'input_bands', 'output_bands', 'sum_after'];
    const splineStages = new Set(['Continentalness', 'Detail', 'PeaksValleys', 'Temperature', 'Humidity']);
    for (const key of keys) {
      if (!key || !s[key]) {
        const ph = document.createElement('div');
        ph.className = 'placeholder';
        if (s.kind === 'height' && (key === 'input_bands' || key === 'output_bands') && splineStages.has(s.name)) {
          ph.textContent = 'spline has only 1 band — nothing to visualize';
        } else {
          ph.textContent = '—';
        }
        row.appendChild(ph);
        continue;
      }
      const img = new Image();
      img.src = 'data:image/png;base64,' + s[key];
      const cv = document.createElement('canvas');
      cv.title = `${s.name} — ${key.replace(/_/g, ' ')} (click to enlarge)`;
      img.onload = () => {
        cv.width = img.width;
        cv.height = img.height;
        cv.getContext('2d').drawImage(img, 0, 0);
      };
      cv.addEventListener('click', () => openDebugZoom(img.src, `${s.name} — ${key.replace(/_/g, ' ')}`));
      // For band thumbnails, attach a small color/index legend below
      // the canvas so the operator can read which palette entry
      // corresponds to which knot interval.
      if ((key === 'input_bands' || key === 'output_bands') && splineStages.has(s.name)) {
        const wrap = document.createElement('div');
        wrap.className = 'bands-cell';
        wrap.appendChild(cv);
        let bandCount = 0;
        try {
          const prof = JSON.parse(profileTextarea.value);
          const knots = prof && prof.ControlConfig && prof.ControlConfig[s.name]
            && prof.ControlConfig[s.name].Spline && prof.ControlConfig[s.name].Spline.Knots;
          if (Array.isArray(knots)) bandCount = Math.max(0, knots.length - 1);
        } catch {}
        if (bandCount > 0) wrap.appendChild(bandLegend(bandCount));
        row.appendChild(wrap);
      } else {
        row.appendChild(cv);
      }
    }
    grid.appendChild(row);
  }
}

// openDebugZoom shows a debug thumbnail at its natural baked size in a
// click-to-dismiss overlay. Thumbnails are CSS-clamped to 200px wide
// in the grid but baked at faceSize*2 wide (128–1024+ depending on the
// face-size slider), so the zoom view often reveals real detail.
function openDebugZoom(src, caption) {
  let overlay = $('#debug-zoom-overlay');
  if (!overlay) {
    overlay = document.createElement('div');
    overlay.id = 'debug-zoom-overlay';
    overlay.addEventListener('click', () => overlay.style.display = 'none');
    document.body.appendChild(overlay);
  }
  overlay.innerHTML = '';
  const img = document.createElement('img');
  img.src = src;
  overlay.appendChild(img);
  const cap = document.createElement('div');
  cap.className = 'debug-zoom-caption';
  cap.textContent = caption + '  —  click anywhere to close';
  overlay.appendChild(cap);
  overlay.style.display = 'flex';
}

// Matches pkg/planetgen/render/debug_palette.go bandPalette, in CSS hex.
const BAND_PALETTE = ['#46c8dc', '#3c6ee6', '#50c850', '#f0dc3c', '#e6503c'];

function bandLegend(numBands) {
  const wrap = document.createElement('div');
  wrap.className = 'band-legend';
  for (let i = 0; i < numBands; i++) {
    const item = document.createElement('span');
    item.className = 'band-legend-item';
    const swatch = document.createElement('span');
    swatch.className = 'band-legend-swatch';
    swatch.style.background = BAND_PALETTE[i % BAND_PALETTE.length];
    item.appendChild(swatch);
    const label = document.createElement('span');
    label.textContent = String(i);
    item.appendChild(label);
    wrap.appendChild(item);
  }
  return wrap;
}

// Wire the manual refresh button. Use addEventListener once when the
// DOM is ready; if app.js runs after DOMContentLoaded the element
// exists already.
(function wireDebugRefresh() {
  const btn = $('#debug-refresh');
  if (btn) btn.addEventListener('click', refreshDebugView);
  const clearBtn = $('#debug-clear-bypass');
  if (clearBtn) clearBtn.addEventListener('click', () => {
    if (debugBypass.size === 0) return;
    debugBypass.clear();
    regenerate();
  });
})();

init();
