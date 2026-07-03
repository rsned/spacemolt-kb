# Planet Gen Phase 2 — Legacy Parameter Sliders Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the `planet-explorer` slider tool with editor panels for the legacy `PlanetProfile` parameters that Phase 1 didn't touch — ocean, cryosphere (polar caps + snow line), and craters — so every rocky-archetype knob is tunable live in the browser. Read-only swatch panels for `EquatorialPalette` / `PolarPalette` round out the rocky-surface coverage. Gas-giant band/storm controls and live palette editing are explicitly out of scope (deferred to a later phase).

**Architecture:** Pure browser additions in `cmd/planet-explorer/web/app.js` + minor `style.css` tweaks. Each new panel follows the established pattern from Phase 1 (`makePanel` / `makeSubPanel`, `input`-event commit, `bindCollapseState` for persistence, Randomize/Reset/Clear buttons where useful, tooltips on every row). No Go-side changes are needed — every parameter already round-trips through the existing `PlanetProfile` JSON.

**Tech Stack:** No new dependencies. Plain HTML/CSS/JS, same as Phase 1.

---

## File structure

**Slider tool extensions:**
- Modify: `cmd/planet-explorer/web/app.js` — add `renderOceanPanel`, `renderCryospherePanel`, `renderCratersPanel`, extend palette rendering to cover `EquatorialPalette` / `PolarPalette`, register them in `renderPanels`
- Modify: `cmd/planet-explorer/web/style.css` — color-input row styling, checkbox row styling
- Modify: `cmd/planet-explorer/web/index.html` — extend Help / Parameter reference section with brief docs for the new panels

**No Go-side changes.** All fields already exist on `types.PlanetProfile` and serialize through the existing JSON path.

---

## Task 1: Ocean panel

**Files:**
- Modify: `cmd/planet-explorer/web/app.js` (add `renderOceanPanel`, register in `renderPanels`)
- Modify: `cmd/planet-explorer/web/style.css` (color-input row layout)

Adds a panel with two controls bound to `profile.OceanLevel` and `profile.OceanColor`. Hidden when `profile.Renderer !== 'rocky'`.

- [ ] **Step 1: Write the panel function**

```js
function renderOceanPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Ocean',
    'Below the OceanLevel cutoff, height is painted with OceanColor (with depth shading). Set OceanLevel = 0 to disable oceans entirely.');
  bindCollapseState(panel, 'ocean');

  const lvl = makeNumberRow('OceanLevel', 'Normalized [0,1] sea-level cutoff. Pixels with height < OceanLevel are painted ocean.',
    profile.OceanLevel ?? 0, 0, 1, 0.01,
    v => { profile.OceanLevel = v; commit(profile); });
  panel.appendChild(lvl);

  const color = makeColorRow('OceanColor', 'Base ocean color (RGBA). Depth shading darkens this toward black for deeper pixels.',
    profile.OceanColor ?? {R:0,G:0,B:0,A:0},
    rgba => { profile.OceanColor = rgba; commit(profile); });
  panel.appendChild(color);

  panels.appendChild(panel);
}
```

- [ ] **Step 2: Add `makeNumberRow` and `makeColorRow` helpers if not already present**

```js
function makeNumberRow(label, tooltip, value, min, max, step, onCommit) {
  const row = document.createElement('label');
  row.className = 'row';
  row.title = tooltip;
  const span = document.createElement('span');
  span.textContent = label;
  const input = document.createElement('input');
  input.type = 'number';
  input.min = min; input.max = max; input.step = step;
  input.value = value;
  input.addEventListener('input', () => {
    const v = parseFloat(input.value);
    if (!Number.isNaN(v)) onCommit(v);
  });
  row.appendChild(span); row.appendChild(input);
  return row;
}

function makeColorRow(label, tooltip, rgba, onCommit) {
  const row = document.createElement('label');
  row.className = 'row';
  row.title = tooltip;
  const span = document.createElement('span');
  span.textContent = label;
  const input = document.createElement('input');
  input.type = 'color';
  input.value = '#' + [rgba.R, rgba.G, rgba.B].map(v => v.toString(16).padStart(2,'0')).join('');
  input.addEventListener('input', () => {
    const hex = input.value.slice(1);
    onCommit({
      R: parseInt(hex.slice(0,2),16),
      G: parseInt(hex.slice(2,4),16),
      B: parseInt(hex.slice(4,6),16),
      A: rgba.A ?? 255,
    });
  });
  row.appendChild(span); row.appendChild(input);
  return row;
}
```

- [ ] **Step 3: Register in `renderPanels`**

```js
renderControlFieldsPanel(profile, panels);
renderWarpPanel(profile, panels);
renderOceanPanel(profile, panels);          // ← new
renderCryospherePanel(profile, panels);     // ← Task 2
renderCratersPanel(profile, panels);        // ← Task 3
renderBiomePanel(profile, panels);
renderLUTPanel(profile, panels);
```

- [ ] **Step 4: Style the color-input row**

```css
#param-panels input[type="color"] {
  width: 70px; height: 24px; padding: 0; border: 1px solid #333;
  background: #16161a; cursor: pointer; border-radius: 3px;
}
```

- [ ] **Step 5: Manual smoke test**

Run: `(cd cmd/planet-explorer && go run . 2>&1 &)` then open `http://localhost:8080/`.

Expected:
- Type=terran shows Ocean panel with `OceanLevel=0.5`, `OceanColor` swatch matching the blue tone.
- Drag the OceanLevel number up to 0.7 → on Regenerate the planet floods (more blue).
- Click the color input → pick a green → on Regenerate the ocean is green.
- Type=scorched (no ocean): panel still shows but `OceanLevel=0` defaults; setting it positive should produce visible water — confirms wiring.

- [ ] **Step 6: Commit**

```bash
git add cmd/planet-explorer/web/app.js cmd/planet-explorer/web/style.css
git commit -m "Add ocean editor panel to planet-explorer"
```

---

## Task 2: Cryosphere panel (polar caps + snow line)

**Files:**
- Modify: `cmd/planet-explorer/web/app.js` (add `renderCryospherePanel`)

Single panel with sub-rows for `HasPolarCaps`, `PolarCapSize`, `PolarCapNoise`, `SnowLine`. Bundled because they all paint white-ish overlays based on latitude/elevation.

- [ ] **Step 1: Write the panel function**

```js
function renderCryospherePanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Cryosphere',
    'Polar ice caps (latitude-based) and snow line (elevation-based). Both paint white overlays as a final color step.');
  bindCollapseState(panel, 'cryosphere');

  const caps = makeCheckboxRow('HasPolarCaps', 'Master toggle for polar-cap rendering.',
    profile.HasPolarCaps ?? false,
    v => { profile.HasPolarCaps = v; commit(profile); });
  panel.appendChild(caps);

  const sz = makeNumberRow('PolarCapSize', 'Latitude fraction covered by caps (0 = none, 0.4 ≈ Hoth-like).',
    profile.PolarCapSize ?? 0, 0, 0.5, 0.01,
    v => { profile.PolarCapSize = v; commit(profile); });
  panel.appendChild(sz);

  const noise = makeNumberRow('PolarCapNoise', 'Edge roughness of the cap boundary. 0 = smooth circle, 0.5 = jagged.',
    profile.PolarCapNoise ?? 0, 0, 0.5, 0.01,
    v => { profile.PolarCapNoise = v; commit(profile); });
  panel.appendChild(noise);

  const snow = makeNumberRow('SnowLine', 'Elevation [0,1] above which pixels get a snow tint. 0 = disabled.',
    profile.SnowLine ?? 0, 0, 1, 0.01,
    v => { profile.SnowLine = v; commit(profile); });
  panel.appendChild(snow);

  panels.appendChild(panel);
}

function makeCheckboxRow(label, tooltip, value, onCommit) {
  const row = document.createElement('label');
  row.className = 'row';
  row.title = tooltip;
  const span = document.createElement('span');
  span.textContent = label;
  const input = document.createElement('input');
  input.type = 'checkbox';
  input.checked = !!value;
  input.addEventListener('input', () => onCommit(input.checked));
  row.appendChild(span); row.appendChild(input);
  return row;
}
```

- [ ] **Step 2: Smoke test**

Type=terran:
- Toggle HasPolarCaps off → north/south caps disappear on Regenerate.
- Re-enable, set PolarCapSize=0.3 → bigger caps.
- Set PolarCapNoise=0.4 → cap edges look rough/jagged.
- Set SnowLine=0.6 → mountain peaks below 0.6 stay colored, above 0.6 turn white.

- [ ] **Step 3: Commit**

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "Add cryosphere (polar caps + snow line) editor panel"
```

---

## Task 3: Craters panel

**Files:**
- Modify: `cmd/planet-explorer/web/app.js` (add `renderCratersPanel`)

Numeric sub-panel with Randomize / Reset / Clear, matching the warp panel's UX.

- [ ] **Step 1: Write the panel function**

```js
function renderCratersPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Craters',
    'Stamped circular depressions on the heightmap. Applied after fBm/control fields, before coloring.');
  bindCollapseState(panel, 'craters');

  const summary = panel.querySelector('summary');
  summary.appendChild(makeAuxBtn('Randomize', 'Sample crater params from the documented useful ranges.', () => {
    profile.CraterCount      = randInt(0, 200);
    profile.CraterMinRadius  = randFloat(0.001, 0.02);
    profile.CraterMaxRadius  = Math.max(profile.CraterMinRadius + 0.005, randFloat(0.01, 0.1));
    profile.CraterDepth      = randFloat(0.02, 0.3);
    commit(profile);
  }));
  summary.appendChild(makeAuxBtn('Reset', 'Restore crater values from the loaded profile snapshot.', () => {
    const orig = (originalProfile && originalProfile.profile) || {};
    profile.CraterCount      = orig.CraterCount     ?? 0;
    profile.CraterMinRadius  = orig.CraterMinRadius ?? 0;
    profile.CraterMaxRadius  = orig.CraterMaxRadius ?? 0;
    profile.CraterDepth      = orig.CraterDepth     ?? 0;
    commit(profile);
  }));
  summary.appendChild(makeAuxBtn('Clear', 'Zero out all crater params (no craters render).', () => {
    profile.CraterCount = 0;
    profile.CraterMinRadius = 0;
    profile.CraterMaxRadius = 0;
    profile.CraterDepth = 0;
    commit(profile);
  }));

  panel.appendChild(makeNumberRow('CraterCount', 'How many craters to stamp. 0 = none. 200 ≈ Mercury-dense.',
    profile.CraterCount ?? 0, 0, 500, 1,
    v => { profile.CraterCount = Math.round(v); commit(profile); }));
  panel.appendChild(makeNumberRow('CraterMinRadius', 'Smallest crater angular radius (radians on the unit sphere). Useful 0.001-0.02.',
    profile.CraterMinRadius ?? 0, 0, 0.05, 0.001,
    v => { profile.CraterMinRadius = v; commit(profile); }));
  panel.appendChild(makeNumberRow('CraterMaxRadius', 'Largest crater angular radius. Must be > MinRadius. Useful 0.01-0.1.',
    profile.CraterMaxRadius ?? 0, 0, 0.2, 0.001,
    v => { profile.CraterMaxRadius = v; commit(profile); }));
  panel.appendChild(makeNumberRow('CraterDepth', 'How much each crater carves into the heightmap. Useful 0.02-0.3.',
    profile.CraterDepth ?? 0, 0, 0.5, 0.01,
    v => { profile.CraterDepth = v; commit(profile); }));

  panels.appendChild(panel);
}

function randInt(lo, hi) { return Math.floor(lo + Math.random() * (hi - lo + 1)); }
function randFloat(lo, hi) { return +(lo + Math.random() * (hi - lo)).toFixed(3); }
```

- [ ] **Step 2: Smoke test**

Type=scorched (200 craters by default):
- Click Clear → planet smooths out on Regenerate.
- Click Reset → 200 craters return.
- Click Randomize → numbers reshuffle, click Regenerate → visible change.
- Type=terran (10 craters): rare but visible; raise Count to 50 → craters dot the surface.

- [ ] **Step 3: Commit**

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "Add craters editor panel to planet-explorer"
```

---

## Task 4: EquatorialPalette and PolarPalette swatch panels

**Files:**
- Modify: `cmd/planet-explorer/web/app.js` (extend palette rendering)

Mirror the existing `Palette` swatch for the two latitude-blended palettes (used by terran and a few others). Read-only — same as the existing Palette panel. Hidden when the profile field is null/empty.

- [ ] **Step 1: Refactor existing Palette code into a reusable helper**

```js
function renderPaletteSwatch(profile, panels, key, title, helpText) {
  const stops = profile[key];
  if (!Array.isArray(stops) || stops.length === 0) return;
  const panel = makePanel(title, helpText);
  bindCollapseState(panel, 'palette-' + key);
  const strip = document.createElement('div');
  strip.style.cssText = 'height:24px;border-radius:3px;margin-top:6px';
  const css = stops.map(s => `${rgbaCSS(s.Color)} ${(s.Position*100).toFixed(0)}%`).join(', ');
  strip.style.background = `linear-gradient(to right, ${css})`;
  panel.appendChild(strip);
  panels.appendChild(panel);
}
```

- [ ] **Step 2: Replace inline palette rendering with three calls**

In `renderPanels`, replace the inline `if (Array.isArray(profile.Palette))` block with:

```js
renderPaletteSwatch(profile, panels, 'Palette',
  'Palette',
  'Read-only swatch of the legacy gradient palette. Used as the base color when no BiomeTable is set.');
renderPaletteSwatch(profile, panels, 'EquatorialPalette',
  'Equatorial palette',
  'Optional palette blended in near the equator (cos(latitude) weight). Read-only — edit via JSON if needed.');
renderPaletteSwatch(profile, panels, 'PolarPalette',
  'Polar palette',
  'Optional palette blended in near the poles (before ice caps). Read-only — edit via JSON if needed.');
```

- [ ] **Step 3: Smoke test**

Type=terran: all three palette panels appear (Palette greens, Equatorial sand-tones, Polar grays).
Type=scorched: only Palette appears (other two are null in the profile).

- [ ] **Step 4: Commit**

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "Add equatorial/polar palette swatch panels"
```

---

## Task 5: Help-panel docs update

**Files:**
- Modify: `cmd/planet-explorer/web/index.html` (extend the static Help section)

Add a "Legacy parameters" subsection to the existing collapsible Help / Parameter reference section so users discover what the new panels actually do.

- [ ] **Step 1: Add a `<h4>Legacy parameters</h4>` block**

Insert after the existing "fBm parameters (per field)" block:

```html
<h4>Legacy parameters (per profile)</h4>
<ul>
  <li><strong>OceanLevel</strong> — normalized [0,1] cutoff. Pixels below get OceanColor with depth shading. 0 = no oceans.</li>
  <li><strong>OceanColor</strong> — base sea color. Depth shading darkens it for deeper water.</li>
  <li><strong>HasPolarCaps / PolarCapSize / PolarCapNoise</strong> — latitude-based white overlay (caps). Size = fraction of pole-to-equator covered. Noise = edge jaggedness.</li>
  <li><strong>SnowLine</strong> — elevation cutoff for snow tint on peaks. 0 = disabled.</li>
  <li><strong>CraterCount / Min / Max / Depth</strong> — stamped craters on the heightmap. Radii are in radians on the unit sphere; depth in normalized height units.</li>
  <li><strong>EquatorialPalette / PolarPalette</strong> — read-only swatches. Optional latitude-blended palettes layered on top of the base Palette.</li>
</ul>
```

- [ ] **Step 2: Verify it renders**

Reload the page, expand Help. Confirm the new section is present and readable.

- [ ] **Step 3: Commit**

```bash
git add cmd/planet-explorer/web/index.html
git commit -m "Document legacy-parameter panels in explorer Help"
```

---

## Task 6: Acceptance

**Files:** none (verification only)

- [ ] **Step 1: Wasm build green**

Run: `(cd cmd/planet-explorer && GOOS=js GOARCH=wasm go build -o web/planet-explorer.wasm ./wasm)`
Expected: exits 0, produces ~5-6 MB binary.

- [ ] **Step 2: Lint + tests still pass**

Run: `golangci-lint run ./pkg/planetgen/... ./cmd/planet-explorer/...`
Expected: 0 issues.

Run: `go test ./cmd/generate-planet-maps/... -run TestGolden`
Expected: all 13 sub-tests PASS (no Go-side changes, so goldens must be byte-identical).

- [ ] **Step 3: Manual checklist**

Pick three archetypes (terran, scorched, glacial) and for each:
- All six new/extended panels show up where applicable (rocky-only).
- Editing any value → Regenerate → visible change matches expectation.
- Reset / Clear / Randomize on Craters work as documented.
- Collapsed-state of each panel persists across Regenerate.
- Export JSON contains the edited values; Import file round-trips them.

- [ ] **Step 4: Final commit**

If any small fixes were needed during smoke testing:
```bash
git commit -am "Phase 2 acceptance: <brief>"
```

---

## Self-review notes

- All five new sliders bind to fields that already exist on `PlanetProfile`, so no Go-side schema change.
- Gas-giant params (`BandCount`, `TurbulenceAmp`, `BandBlendWidth`, `StormCount`, `StormSize`) are deferred; a later phase plan will cover them with their own panels gated on `Renderer === 'gas_giant'`.
- The legacy fBm fallback (`NoiseOctaves` etc.) is intentionally NOT exposed — that path becomes dead code once Phase 1 T15 finishes populating every archetype's `ControlConfig`.
- Palette editing remains out of scope; `EquatorialPalette` / `PolarPalette` are read-only swatches matching the existing `Palette` panel. If interactive palette stop editing becomes desired later, it gets its own phase.
