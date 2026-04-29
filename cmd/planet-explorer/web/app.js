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
  const faceSize = parseInt(faceSizeSel.value, 10);

  const t0 = performance.now();
  const mode = viewModeSel ? viewModeSel.value : 'color';
  const cubePNG = (mode === 'heightmap')
    ? planetExplorerGenerateHeightmap(profileJSON, seed, faceSize)
    : planetExplorerGenerate(profileJSON, seed, faceSize);
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
  refreshSphereTexture();
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
if (viewModeSel) viewModeSel.addEventListener('change', regenerate);
exportBtn.addEventListener('click', () => {
  navigator.clipboard.writeText(profileTextarea.value);
  status.textContent = 'JSON copied to clipboard';
});
applyBtn.addEventListener('click', () => {
  renderPanels();
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
  renderCratersPanel(profile, panels);
  renderBiomePanel(profile, panels);
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

function refreshSphereTexture() {
  if (!equirectCanvas.width || !equirectCanvas.height) return;
  const ectx = equirectCanvas.getContext('2d', { willReadFrequently: true });
  try {
    sphereTextureData = ectx.getImageData(0, 0, equirectCanvas.width, equirectCanvas.height).data;
    sphereTexW = equirectCanvas.width;
    sphereTexH = equirectCanvas.height;
  } catch (e) {
    sphereTextureData = null;
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

function renderSphere() {
  if (!sphereCanvas) return;
  const ctx = sphereCanvas.getContext('2d');
  const W = sphereCanvas.width, H = sphereCanvas.height;
  const CX = W / 2, CY = H / 2, RADIUS = Math.min(W, H) / 2 - 20;
  const id = ctx.createImageData(W, H);
  const d = id.data;
  const cRX = Math.cos(sphereRotX), sRX = Math.sin(sphereRotX);
  const cRY = Math.cos(sphereRotY), sRY = Math.sin(sphereRotY);
  const lx = -0.4, ly = 0.5, lz = 0.7;
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
        const [tr, tg, tb] = sampleSphereTex((lon / (2 * Math.PI)) + 0.5, 0.5 - lat / Math.PI);
        const dot = (nx * lx + (-ny) * ly + nz * lz) / ll;
        const lit = Math.max(0.08, dot * 0.7 + 0.3);
        const rim = 1 - nz, rg = rim * rim * rim * 0.3;
        pR = Math.min(255, Math.floor(tr * lit + rg * 80));
        pG = Math.min(255, Math.floor(tg * lit + rg * 120));
        pB = Math.min(255, Math.floor(tb * lit + rg * 180));
      }
      d[idx] = pR; d[idx + 1] = pG; d[idx + 2] = pB; d[idx + 3] = 255;
    }
  }
  ctx.putImageData(id, 0, 0);
}

function animateSphere() {
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

init();
