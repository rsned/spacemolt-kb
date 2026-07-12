// Wasm entrypoint for cmd/planet-explorer. Builds with GOOS=js
// GOARCH=wasm into cmd/planet-explorer/web/planet-explorer.wasm.
//
// Exposes functions on the JS global scope:
//
//	planetExplorerGenerate(profileJSON string, seedStr string, faceSize int)                   Uint8Array
//	planetExplorerGenerateNight(profileJSON, seedStr, faceSize)                                Uint8Array (empty when civ disabled)
//	planetExplorerBakeEquirect(cubePNG Uint8Array, w int, h int)                               Uint8Array
//	planetExplorerDefaultProfile(planetType string)                                             string  // JSON
//	planetExplorerGenerateDebug(profileJSON, seedStr, faceSize, bypassJSON)                    string  // JSON
//	patchInit(profileJSON string, seedStr string, sTect int)                                   string  // JSON
//	patchSelect(windowJSON string)                                                              string  // "" or JSON error
//	patchLayers()                                                                               string  // JSON
//	patchSetParam(paramPath string, profileJSON string)                                        string  // JSON
//	patchRender(targetLayer int, view string)                                                  Uint8Array
//	patchMinimap(width int, height int)                                                        Uint8Array
//	patchRecomputeSphere()                                                                      string  // JSON {"seaLevel":..., "seaLevel0":...}
//
//go:build js && wasm

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"syscall/js"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/patch"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func main() {
	// Wasm has no filesystem; force planetgen to skip the disk lookup
	// and use the in-code GetProfile defaults exclusively.
	planetgen.SetProfileRoot("")
	registerProgressHooks()
	js.Global().Set("planetExplorerGenerate", js.FuncOf(generate))
	js.Global().Set("planetExplorerGenerateNight", js.FuncOf(generateNight))
	js.Global().Set("planetExplorerGenerateHeightmap", js.FuncOf(generateHeightmap))
	js.Global().Set("planetExplorerBakeEquirect", js.FuncOf(bakeEquirect))
	js.Global().Set("planetExplorerDefaultProfile", js.FuncOf(defaultProfile))
	js.Global().Set("planetExplorerGenerateDebug", js.FuncOf(generateDebug))
	js.Global().Set("planetExplorerGenerateWithBypass", js.FuncOf(generateWithBypass))
	js.Global().Set("patchInit", js.FuncOf(patchInit))
	js.Global().Set("patchSelect", js.FuncOf(patchSelect))
	js.Global().Set("patchLayers", js.FuncOf(patchLayers))
	js.Global().Set("patchSetParam", js.FuncOf(patchSetParam))
	js.Global().Set("patchRender", js.FuncOf(patchRender))
	js.Global().Set("patchMinimap", js.FuncOf(patchMinimap))
	js.Global().Set("patchRecomputeSphere", js.FuncOf(patchRecomputeSphere))
	<-make(chan struct{}) // keep the WASM process alive
}

// generateHeightmap(profileJSON, seedStr, faceSize) → Uint8Array of
// grayscale cube-map cross PNG bytes. Rocky-only debug view; gas-giant
// profiles return an error.
func generateHeightmap(_ js.Value, args []js.Value) any {
	if len(args) != 3 {
		return jsError("generateHeightmap: expected 3 args, got %d", len(args))
	}
	var prof types.PlanetProfile
	if err := json.Unmarshal([]byte(args[0].String()), &prof); err != nil {
		return jsError("generateHeightmap: bad profile JSON: %v", err)
	}
	if prof.Renderer != "rocky" {
		return jsError("generateHeightmap: only rocky profiles support heightmap view")
	}
	s := seed.Hash(args[1].String())
	faceSize := args[2].Int()
	cm := render.RenderRockyHeightmap(&prof, s, faceSize)

	var buf bytes.Buffer
	if err := cubemap.WriteCrossPNGTo(cm, &buf); err != nil {
		return jsError("generateHeightmap: encode: %v", err)
	}
	return jsBytes(buf.Bytes())
}

// generate(profileJSON, seedStr, faceSize) → Uint8Array of cube-map cross PNG bytes.
func generate(_ js.Value, args []js.Value) any {
	if len(args) != 3 {
		return jsError("generate: expected 3 args, got %d", len(args))
	}
	var prof types.PlanetProfile
	if err := json.Unmarshal([]byte(args[0].String()), &prof); err != nil {
		return jsError("generate: bad profile JSON: %v", err)
	}
	s := seed.Hash(args[1].String())
	faceSize := args[2].Int()

	var cm *cubemap.CubeMap
	switch prof.Renderer {
	case "rocky":
		cm = render.RenderRocky(&prof, s, faceSize)
	case "gas_giant":
		cm = render.RenderGasGiant(&prof, s, faceSize)
	default:
		return jsError("generate: unknown renderer %q", prof.Renderer)
	}

	var buf bytes.Buffer
	if err := cubemap.WriteCrossPNGTo(cm, &buf); err != nil {
		return jsError("generate: encode: %v", err)
	}
	return jsBytes(buf.Bytes())
}

// generateNight(profileJSON, seedStr, faceSize) → Uint8Array of cube-map
// cross PNG bytes for the Phase 9b Black-Marble nightside, or an empty
// Uint8Array when civ is disabled (Civ.Tier == 0) or the renderer
// doesn't support civ. The JS side treats empty as "no night layer".
func generateNight(_ js.Value, args []js.Value) any {
	if len(args) != 3 {
		return jsError("generateNight: expected 3 args, got %d", len(args))
	}
	var prof types.PlanetProfile
	if err := json.Unmarshal([]byte(args[0].String()), &prof); err != nil {
		return jsError("generateNight: bad profile JSON: %v", err)
	}
	s := seed.Hash(args[1].String())
	faceSize := args[2].Int()

	if prof.Renderer != "rocky" {
		return jsBytes(nil)
	}
	cm := render.RenderNightCubeMap(&prof, s, faceSize)
	if cm == nil {
		return jsBytes(nil)
	}
	var buf bytes.Buffer
	if err := cubemap.WriteCrossPNGTo(cm, &buf); err != nil {
		return jsError("generateNight: encode: %v", err)
	}
	return jsBytes(buf.Bytes())
}

// bakeEquirect(cubePNGBytes, width, height) → Uint8Array of equirect PNG bytes.
func bakeEquirect(_ js.Value, args []js.Value) any {
	if len(args) != 3 {
		return jsError("bakeEquirect: expected 3 args, got %d", len(args))
	}
	cubeBytes := goBytes(args[0])
	w := args[1].Int()
	h := args[2].Int()

	cm, err := cubemap.ReadCrossPNGFrom(bytes.NewReader(cubeBytes))
	if err != nil {
		return jsError("bakeEquirect: read: %v", err)
	}
	img := cubemap.BakeEquirect(cm, w, h)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return jsError("bakeEquirect: encode: %v", err)
	}
	return jsBytes(buf.Bytes())
}

// defaultProfile(planetType) → JSON string of the in-code default for that type.
func defaultProfile(_ js.Value, args []js.Value) any {
	if len(args) != 1 {
		return jsError("defaultProfile: expected 1 arg, got %d", len(args))
	}
	prof := planetgen.GetProfile(args[0].String())
	if prof == nil {
		return jsError("defaultProfile: unknown type %q", args[0].String())
	}
	b, err := json.Marshal(prof)
	if err != nil {
		return jsError("defaultProfile: marshal: %v", err)
	}
	return string(b)
}

// generateWithBypass(profileJSON, seedStr, faceSize, bypassJSON) →
// Uint8Array of cube-map cross PNG bytes for the rocky pipeline with
// the given debug bypass set applied. Mirrors generate() but routes
// through RenderRockyDebug so per-stage bypasses (e.g. LUT) take effect
// on the main sphere render. Falls back to plain generate() semantics
// when bypassJSON is empty or the profile is gas_giant.
func generateWithBypass(_ js.Value, args []js.Value) any {
	if len(args) != 4 {
		return jsError("generateWithBypass: expected 4 args (profileJSON, seed, faceSize, bypassJSON), got %d", len(args))
	}
	var prof types.PlanetProfile
	if err := json.Unmarshal([]byte(args[0].String()), &prof); err != nil {
		return jsError("generateWithBypass: bad profile JSON: %v", err)
	}
	s := seed.Hash(args[1].String())
	faceSize := args[2].Int()

	var bypass render.DebugBypass
	if args[3].Type() == js.TypeString && args[3].String() != "" {
		var arr []string
		if err := json.Unmarshal([]byte(args[3].String()), &arr); err == nil && len(arr) > 0 {
			bypass = make(render.DebugBypass, len(arr))
			for _, name := range arr {
				bypass[name] = true
			}
		}
	}

	var cm *cubemap.CubeMap
	if prof.Renderer == "rocky" && bypass != nil {
		// frame.Final is the fully-composited day-side cube map after
		// the colorize pass — exactly what RenderRocky returns. Reading
		// it directly (instead of reverse-walking Stages) avoids picking
		// up debug-only overlay stages appended after the main pipeline
		// (Cloud: shaded, Civ: <part>, etc.).
		frame := render.RenderRockyDebug(&prof, s, faceSize, bypass)
		cm = frame.Final
		if cm == nil {
			return jsError("generateWithBypass: colorize returned nil")
		}
	} else {
		switch prof.Renderer {
		case "rocky":
			cm = render.RenderRocky(&prof, s, faceSize)
		case "gas_giant":
			cm = render.RenderGasGiant(&prof, s, faceSize)
		default:
			return jsError("generateWithBypass: unknown renderer %q", prof.Renderer)
		}
	}

	var buf bytes.Buffer
	if err := cubemap.WriteCrossPNGTo(cm, &buf); err != nil {
		return jsError("generateWithBypass: encode: %v", err)
	}
	return jsBytes(buf.Bytes())
}

// generateDebug(profileJSON, seedStr, faceSize, bypassJSON) → JSON string
// with per-stage base64-encoded equirect PNG thumbnails. bypassJSON is an
// optional JSON array of stage names to dry-run (pass "" to skip).
func generateDebug(_ js.Value, args []js.Value) any {
	if len(args) < 4 {
		return jsError("generateDebug: expected 4 args (profileJSON, seed, faceSize, bypassJSON), got %d", len(args))
	}
	var prof types.PlanetProfile
	if err := json.Unmarshal([]byte(args[0].String()), &prof); err != nil {
		return jsError("generateDebug: bad profile JSON: %v", err)
	}
	s := seed.Hash(args[1].String())
	faceSize := args[2].Int()

	var bypass render.DebugBypass
	if args[3].Type() == js.TypeString && args[3].String() != "" {
		var arr []string
		if err := json.Unmarshal([]byte(args[3].String()), &arr); err == nil && len(arr) > 0 {
			bypass = make(render.DebugBypass, len(arr))
			for _, name := range arr {
				bypass[name] = true
			}
		}
	}

	frame := render.RenderRockyDebug(&prof, s, faceSize, bypass)
	eqW, eqH := faceSize*2, faceSize

	// encodeCM encodes a *cubemap.CubeMap as a base64 PNG string.
	encodeCM := func(cm *cubemap.CubeMap) string {
		if cm == nil {
			return ""
		}
		img := cubemap.BakeEquirect(cm, eqW, eqH)
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		return base64.StdEncoding.EncodeToString(buf.Bytes())
	}

	// encodeCMF encodes a *cubemap.CubeMapF as a base64 PNG string.
	// When signed is true the field is treated as signed (rendered with
	// SignedToRGBA); otherwise it is treated as [0,1] grayscale.
	encodeCMF := func(cmF *cubemap.CubeMapF, signed bool) string {
		if cmF == nil {
			return ""
		}
		var cm *cubemap.CubeMap
		if signed {
			// Auto-scale signed thumbnails to per-stage max-abs so small-
			// amplitude deltas (e.g. Coastal at Amp=0.05) stay visible
			// instead of mapping to near-black under a fixed hi=1.0.
			maxAbs := 0.0
			for face := range cmF.Faces {
				for _, v := range cmF.Faces[face] {
					if v < 0 {
						v = -v
					}
					if v > maxAbs {
						maxAbs = v
					}
				}
			}
			if maxAbs < 1e-9 {
				maxAbs = 1.0
			}
			cm = render.SignedToRGBA(cmF, cmF.Size, maxAbs)
		} else {
			cm = cubemap.GrayscaleFromF(cmF)
		}
		return encodeCM(cm)
	}

	stages := make([]map[string]any, 0, len(frame.Stages))
	for _, st := range frame.Stages {
		row := map[string]any{
			"name":    st.Name,
			"kind":    st.Kind,
			"skipped": st.Skipped,
		}
		switch st.Kind {
		case "color":
			row["color_after"] = encodeCM(st.ColorAfter)
		case "field":
			switch {
			case st.CategoricalAfter != nil:
				row["field_after"] = encodeCM(st.CategoricalAfter)
			case st.BooleanAfter != nil:
				row["field_after"] = encodeCM(st.BooleanAfter)
			case st.ScalarAfter != nil:
				// SDFs are in km (non-negative, range 0..π·R); auto-scale via the
				// signed encoder so faint near-boundary values stay visible.
				row["field_after"] = encodeCMF(st.ScalarAfter, true)
			}
			if st.Combined != nil {
				row["combined"] = encodeCM(st.Combined)
			}
		default:
			row["raw"] = encodeCMF(st.RawFbm, true)
			row["input_bands"] = encodeCM(st.InputBands)
			row["output_bands"] = encodeCM(st.OutputBands)
			row["sum_after"] = encodeCMF(st.SumAfter, false)
		}
		stages = append(stages, row)
	}
	out, _ := json.Marshal(map[string]any{"stages": stages})
	return js.ValueOf(string(out))
}

// patchSession holds the Patch Lab wizard's server-side (in-wasm)
// state across the patchInit → patchSelect → patchLayers/patchSetParam
// /patchRender/patchMinimap call sequence. Only one patch session is
// live at a time (mirrors the single-sphere planetExplorerGenerate
// model). profile is kept alongside the brief's sketch fields because
// patch.Context requires a *types.PlanetProfile and patchSelect's
// signature (windowJSON only) has no way to pass one in — patchInit's
// decoded profile is threaded through from here.
var patchSession struct {
	sd      *patch.SphereData
	profile *types.PlanetProfile
	stack   *patch.Stack
	window  patch.Window
	sTect   int
	master  int64
}

// registerProgressHooks forwards Go pipeline progress to the JS global
// __pxProgress(stage, i, n) when the embedder (worker.js) defined it
// before booting the wasm. No-op in a context without the global.
func registerProgressHooks() {
	cb := js.Global().Get("__pxProgress")
	if cb.Type() != js.TypeFunction {
		return
	}
	fn := func(stage string, i, n int) { cb.Invoke(stage, i, n) }
	patch.SetProgressHook(fn)
	render.SetProgressHook(fn)
}

// patchInit(profileJSON, seedStr, sTect) → JSON string
// {"seaLevel":..., "seaLevel0":..., "candidates":[{"window":{...},"score":...}, ...]}.
// Runs the sphere-global tectonic precompute and picks the top-12
// candidate 512x512 (virtual S_prod=1024) patch windows, storing the
// sphere for the subsequent patchSelect call.
func patchInit(_ js.Value, args []js.Value) any {
	if len(args) != 3 {
		return jsError("patchInit: expected 3 args, got %d", len(args))
	}
	var prof types.PlanetProfile
	if err := json.Unmarshal([]byte(args[0].String()), &prof); err != nil {
		return jsError("patchInit: bad profile JSON: %v", err)
	}
	master := seed.Hash(args[1].String())
	sTect := args[2].Int()

	sd, err := patch.ComputeSphere(&prof, master, sTect)
	if err != nil {
		return jsError("patchInit: %v", err)
	}
	cands := patch.Pick(sd, 512, 1024, 12)

	patchSession.sd = sd
	patchSession.profile = &prof
	patchSession.master = master
	patchSession.sTect = sTect
	patchSession.stack = nil
	patchSession.window = patch.Window{}

	out, err := json.Marshal(map[string]any{
		"seaLevel":   sd.SeaLevel,
		"seaLevel0":  sd.SeaLevel0,
		"candidates": cands,
	})
	if err != nil {
		return jsError("patchInit: marshal: %v", err)
	}
	return string(out)
}

// patchSelect(windowJSON) → "" on success, or a jsError JSON string.
// Extracts the patch fields for the given window and builds a fresh
// layer Stack, storing both for subsequent calls.
func patchSelect(_ js.Value, args []js.Value) any {
	if len(args) != 1 {
		return jsError("patchSelect: expected 1 arg, got %d", len(args))
	}
	if patchSession.sd == nil {
		return jsError("patchSelect: call patchInit first")
	}
	var w patch.Window
	if err := json.Unmarshal([]byte(args[0].String()), &w); err != nil {
		return jsError("patchSelect: bad window JSON: %v", err)
	}
	if err := w.Valid(); err != nil {
		return jsError("patchSelect: %v", err)
	}
	fields, err := patch.ExtractFields(patchSession.sd, w)
	if err != nil {
		return jsError("patchSelect: %v", err)
	}
	ctx := &patch.Context{
		Sphere:  patchSession.sd,
		Fields:  fields,
		Profile: patchSession.profile,
		Master:  patchSession.master,
	}
	patchSession.stack = patch.NewStack(ctx)
	patchSession.window = w
	return ""
}

// patchLayers() → JSON array
// [{"index":0,"id":"tectonic-base","name":"Tectonic base","enabled":true}, ...].
func patchLayers(_ js.Value, _ []js.Value) any {
	if patchSession.stack == nil {
		return jsError("patchLayers: call patchSelect first")
	}
	ctx := patchSession.stack.Ctx()
	ls := patch.Layers()
	out := make([]map[string]any, len(ls))
	for i, l := range ls {
		enabled := true
		if l.Enabled != nil {
			enabled = l.Enabled(ctx)
		}
		out[i] = map[string]any{
			"index":   l.Index,
			"id":      l.ID,
			"name":    l.Name,
			"enabled": enabled,
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return jsError("patchLayers: marshal: %v", err)
	}
	return string(b)
}

// clampWindow re-validates a window after a sphere recompute, clamping
// X0/Y0 so the window stays within [0, SProd-Size]. SProd (the virtual
// production face size) doesn't change when sTect changes, so this is
// normally a no-op — kept for the general case per the task brief.
func clampWindow(w patch.Window) patch.Window {
	maxX := max(w.SProd-w.Size, 0)
	if w.X0 > maxX {
		w.X0 = maxX
	}
	if w.X0 < 0 {
		w.X0 = 0
	}
	if w.Y0 > maxX {
		w.Y0 = maxX
	}
	if w.Y0 < 0 {
		w.Y0 = 0
	}
	return w
}

// patchSetParam(paramPath, profileJSON) → JSON {"sphereRecomputed":bool}.
// Applies a profile-param edit to the live stack's Context. Most edits,
// including tectonic FX, control noise, and height smooth, are owned by
// a patch layer (patch.Layers()'s Params lists), so MarkDirty returns
// false and only that layer and everything downstream of it re-runs —
// the sphere-global precompute (SphereData: Jitter/Plates/Crust/FX) is
// NOT re-run. Only edits to params no layer owns (MajorPlates, Assembly,
// CratonsMax, TargetLandFraction, …) require a full ComputeSphere+
// ExtractFields re-run. Consequence: HMin/HMax/SeaLevel0/SeaLevel are
// derived once by ComputeSphere and cached on SphereData — heavy
// tectonic FX / control noise / height-smooth retuning can drift a
// patch's absolute height normalization and sea level away from what a
// fresh sphere compute at the new params would produce, until a
// genuinely sphere-level param changes (or the user re-enters Patch
// Lab) resyncs them. This is intentional (keeps slider drags
// interactive) but is a documented divergence — see the Patch Lab
// README and spec §7. paramPath == "seaLevelView" is a special case:
// it's a stack-side view override (Context.SeaLevelView), not a
// profile field, so it never triggers a sphere recompute.
func patchSetParam(_ js.Value, args []js.Value) any {
	if len(args) != 2 {
		return jsError("patchSetParam: expected 2 args, got %d", len(args))
	}
	if patchSession.stack == nil {
		return jsError("patchSetParam: call patchSelect first")
	}
	paramPath := args[0].String()
	profileJSON := args[1].String()
	ctx := patchSession.stack.Ctx()

	sphereRecomputed := false
	if paramPath == "seaLevelView" {
		var payload struct {
			SeaLevelView float64 `json:"seaLevelView"`
		}
		if err := json.Unmarshal([]byte(profileJSON), &payload); err != nil {
			return jsError("patchSetParam: bad seaLevelView JSON: %v", err)
		}
		ctx.SeaLevelView = payload.SeaLevelView
		patchSession.stack.MarkDirty(paramPath)
	} else {
		var prof types.PlanetProfile
		if err := json.Unmarshal([]byte(profileJSON), &prof); err != nil {
			return jsError("patchSetParam: bad profile JSON: %v", err)
		}
		ctx.Profile = &prof
		patchSession.profile = &prof
		if patchSession.stack.MarkDirty(paramPath) {
			sd, err := patch.ComputeSphere(ctx.Profile, patchSession.master, patchSession.sTect)
			if err != nil {
				return jsError("patchSetParam: recompute sphere: %v", err)
			}
			w := clampWindow(patchSession.window)
			fields, err := patch.ExtractFields(sd, w)
			if err != nil {
				return jsError("patchSetParam: extract fields: %v", err)
			}
			patchSession.sd = sd
			patchSession.window = w
			ctx.Sphere = sd
			ctx.Fields = fields
			patchSession.stack.MarkAllDirty()
			sphereRecomputed = true
		}
	}

	out, err := json.Marshal(map[string]any{"sphereRecomputed": sphereRecomputed})
	if err != nil {
		return jsError("patchSetParam: marshal: %v", err)
	}
	return string(out)
}

// patchRender(targetLayer, view) → Uint8Array of PNG bytes. view is one
// of "color" (ColorPNG), "height" (ShadedHeightPNG), or "tectonic"
// (TectonicDebugPNG).
func patchRender(_ js.Value, args []js.Value) any {
	if len(args) != 2 {
		return jsError("patchRender: expected 2 args, got %d", len(args))
	}
	if patchSession.stack == nil {
		return jsError("patchRender: call patchSelect first")
	}
	target := args[0].Int()
	view := args[1].String()

	st, err := patchSession.stack.RenderTo(target)
	if err != nil {
		return jsError("patchRender: %v", err)
	}

	var b []byte
	switch view {
	case "color":
		b, err = patch.ColorPNG(st)
	case "height":
		b, err = patch.ShadedHeightPNG(patchSession.stack.Ctx(), st)
	case "tectonic":
		b, err = patch.TectonicDebugPNG(patchSession.stack.Ctx(), st)
	default:
		return jsError("patchRender: unknown view %q", view)
	}
	if err != nil {
		return jsError("patchRender: encode: %v", err)
	}
	return jsBytes(b)
}

// patchMinimap(width, height) → Uint8Array of an equirect PNG of the
// stored sphere's continental mask (FX-tinted) with the selected
// window's footprint outlined in white. Requires patchSelect to have
// run so the outlined window is meaningful.
func patchMinimap(_ js.Value, args []js.Value) any {
	if len(args) != 2 {
		return jsError("patchMinimap: expected 2 args, got %d", len(args))
	}
	if patchSession.stack == nil {
		return jsError("patchMinimap: call patchSelect first")
	}
	width := args[0].Int()
	height := args[1].Int()
	b, err := patch.MinimapPNG(patchSession.sd, patchSession.window, width, height)
	if err != nil {
		return jsError("patchMinimap: %v", err)
	}
	return jsBytes(b)
}

// patchRecomputeSphere() → JSON {"seaLevel":..., "seaLevel0":...}.
// Re-runs the sphere-global precompute with the session's CURRENT
// profile and re-extracts the window's fields, resyncing the four
// sphere-derived scalars (HMin/HMax/SeaLevel0/SeaLevel) that heavy FX
// / control-noise / height-smooth retuning drifts away from a fresh
// compute — the stale-scalar divergence documented in the Patch Lab
// spec §7. Marks every layer dirty; the caller re-renders.
func patchRecomputeSphere(_ js.Value, _ []js.Value) any {
	if patchSession.stack == nil {
		return jsError("patchRecomputeSphere: call patchSelect first")
	}
	sd, err := patch.ComputeSphere(patchSession.profile, patchSession.master, patchSession.sTect)
	if err != nil {
		return jsError("patchRecomputeSphere: %v", err)
	}
	w := clampWindow(patchSession.window)
	fields, err := patch.ExtractFields(sd, w)
	if err != nil {
		return jsError("patchRecomputeSphere: extract fields: %v", err)
	}
	patchSession.sd = sd
	patchSession.window = w
	ctx := patchSession.stack.Ctx()
	ctx.Sphere = sd
	ctx.Fields = fields
	patchSession.stack.MarkAllDirty()
	out, err := json.Marshal(map[string]any{"seaLevel": sd.SeaLevel, "seaLevel0": sd.SeaLevel0})
	if err != nil {
		return jsError("patchRecomputeSphere: marshal: %v", err)
	}
	return string(out)
}

func goBytes(uint8Array js.Value) []byte {
	n := uint8Array.Length()
	out := make([]byte, n)
	js.CopyBytesToGo(out, uint8Array)
	return out
}

func jsBytes(b []byte) js.Value {
	out := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(out, b)
	return out
}

func jsError(format string, args ...any) js.Value {
	m := map[string]any{"error": fmt.Sprintf(format, args...)}
	b, _ := json.Marshal(m)
	return js.ValueOf(string(b))
}
