// Wasm entrypoint for cmd/planet-explorer. Builds with GOOS=js
// GOARCH=wasm into cmd/planet-explorer/web/planet-explorer.wasm.
//
// Exposes functions on the JS global scope:
//
//	planetExplorerGenerate(profileJSON string, seedStr string, faceSize int)                   Uint8Array
//	planetExplorerBakeEquirect(cubePNG Uint8Array, w int, h int)                               Uint8Array
//	planetExplorerDefaultProfile(planetType string)                                             string  // JSON
//	planetExplorerGenerateDebug(profileJSON, seedStr, faceSize, bypassJSON)                    string  // JSON
//	planetExplorerGenerateDebugSwatch(profileJSON, seedStr, faceSize, bypassJSON, faceIndex)   string  // JSON
//	planetExplorerGenerateFlatDebug(profileJSON, seedStr, size, bypassJSON)                    string  // JSON
//	planetExplorerFlatCacheStats()                                                             string  // JSON {"hits":N,"misses":M}
//
//go:build js && wasm

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"syscall/js"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func main() {
	js.Global().Set("planetExplorerGenerate", js.FuncOf(generate))
	js.Global().Set("planetExplorerGenerateHeightmap", js.FuncOf(generateHeightmap))
	js.Global().Set("planetExplorerBakeEquirect", js.FuncOf(bakeEquirect))
	js.Global().Set("planetExplorerDefaultProfile", js.FuncOf(defaultProfile))
	js.Global().Set("planetExplorerGenerateDebug", js.FuncOf(generateDebug))
	js.Global().Set("planetExplorerGenerateDebugSwatch", js.FuncOf(generateDebugSwatch))
	js.Global().Set("planetExplorerGenerateWithBypass", js.FuncOf(generateWithBypass))
	js.Global().Set("planetExplorerGenerateFlat", js.FuncOf(generateFlat))
	js.Global().Set("planetExplorerGenerateFlatDebug", js.FuncOf(generateFlatDebug))
	js.Global().Set("planetExplorerFlatCacheStats", js.FuncOf(flatCacheStats))
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
		frame := render.RenderRockyDebug(&prof, s, faceSize, bypass)
		// The final composited cube map is the ColorAfter of the last
		// color-kind stage. Walk in reverse to find it.
		for i := len(frame.Stages) - 1; i >= 0; i-- {
			if frame.Stages[i].Kind == "color" && frame.Stages[i].ColorAfter != nil {
				cm = frame.Stages[i].ColorAfter
				break
			}
		}
		if cm == nil {
			return jsError("generateWithBypass: no color stage produced output")
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
		if st.Kind == "color" {
			row["color_after"] = encodeCM(st.ColorAfter)
		} else {
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

// generateDebugSwatch is like generateDebug but encodes each stage as a
// single-face PNG instead of an equirect bake. faceIndex selects which
// face (0..5, matching cubemap.Face order); out-of-range falls back to
// FacePosZ (the front-of-cube center face).
func generateDebugSwatch(_ js.Value, args []js.Value) any {
	if len(args) < 5 {
		return jsError("generateDebugSwatch: expected 5 args (profileJSON, seed, faceSize, bypassJSON, faceIndex), got %d", len(args))
	}
	var prof types.PlanetProfile
	if err := json.Unmarshal([]byte(args[0].String()), &prof); err != nil {
		return jsError("generateDebugSwatch: bad profile JSON: %v", err)
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

	faceIdx := args[4].Int()
	if faceIdx < 0 || faceIdx >= cubemap.NumFaces {
		faceIdx = int(cubemap.FacePosZ)
	}
	face := cubemap.Face(faceIdx)

	frame := render.RenderRockyDebug(&prof, s, faceSize, bypass)

	encodeFace := func(cm *cubemap.CubeMap) string {
		if cm == nil {
			return ""
		}
		S := cm.Size
		img := image.NewRGBA(image.Rect(0, 0, S, S))
		for py := range S {
			for px := range S {
				img.SetRGBA(px, py, cm.Get(face, px, py))
			}
		}
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		return base64.StdEncoding.EncodeToString(buf.Bytes())
	}

	encodeFaceF := func(cmF *cubemap.CubeMapF, signed bool) string {
		if cmF == nil {
			return ""
		}
		var cm *cubemap.CubeMap
		if signed {
			maxAbs := 0.0
			for fi := range cmF.Faces {
				for _, v := range cmF.Faces[fi] {
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
		return encodeFace(cm)
	}

	stages := make([]map[string]any, 0, len(frame.Stages))
	for _, st := range frame.Stages {
		row := map[string]any{
			"name":    st.Name,
			"kind":    st.Kind,
			"skipped": st.Skipped,
		}
		if st.Kind == "color" {
			row["color_after"] = encodeFace(st.ColorAfter)
		} else {
			row["raw"] = encodeFaceF(st.RawFbm, true)
			row["input_bands"] = encodeFace(st.InputBands)
			row["output_bands"] = encodeFace(st.OutputBands)
			row["sum_after"] = encodeFaceF(st.SumAfter, false)
		}
		stages = append(stages, row)
	}
	out, _ := json.Marshal(map[string]any{"stages": stages})
	return js.ValueOf(string(out))
}

// generateFlat(profileJSON, seedStr, size) → Uint8Array of PNG bytes for the
// 2D flat preview. Rocky-only; bypasses cube-sphere entirely for fast iteration.
func generateFlat(_ js.Value, args []js.Value) any {
	if len(args) != 3 {
		return jsError("generateFlat: expected 3 args (profileJSON, seed, size), got %d", len(args))
	}
	var prof types.PlanetProfile
	if err := json.Unmarshal([]byte(args[0].String()), &prof); err != nil {
		return jsError("generateFlat: bad profile JSON: %v", err)
	}
	if prof.Renderer != "rocky" {
		return jsError("generateFlat: only rocky profiles supported")
	}
	s := seed.Hash(args[1].String())
	size := args[2].Int()
	img := render.RenderFlat(&prof, s, size)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return jsError("generateFlat: encode: %v", err)
	}
	return jsBytes(buf.Bytes())
}

// generateFlatDebug(profileJSON, seedStr, size, bypassJSON) → JSON string
// with per-stage square PNG thumbnails for the flat pipeline. bypassJSON is an
// optional JSON array of stage names to suppress (pass "" to skip).
func generateFlatDebug(_ js.Value, args []js.Value) any {
	if len(args) < 4 {
		return jsError("generateFlatDebug: expected 4 args (profileJSON, seed, size, bypassJSON), got %d", len(args))
	}
	var prof types.PlanetProfile
	if err := json.Unmarshal([]byte(args[0].String()), &prof); err != nil {
		return jsError("generateFlatDebug: bad profile JSON: %v", err)
	}
	if prof.Renderer != "rocky" {
		return jsError("generateFlatDebug: only rocky profiles supported")
	}
	s := seed.Hash(args[1].String())
	size := args[2].Int()
	var bypass render.FlatDebugBypass
	if args[3].Type() == js.TypeString && args[3].String() != "" {
		var arr []string
		if err := json.Unmarshal([]byte(args[3].String()), &arr); err == nil && len(arr) > 0 {
			bypass = make(render.FlatDebugBypass, len(arr))
			for _, name := range arr {
				bypass[name] = true
			}
		}
	}
	frame := render.RenderFlatDebug(&prof, s, size, bypass)

	// encodeHeight encodes a []float64 as a square grayscale PNG.
	encodeHeight := func(hm []float64, signed bool) string {
		if hm == nil {
			return ""
		}
		img := image.NewRGBA(image.Rect(0, 0, size, size))
		if signed {
			// Auto-scale signed delta so small-amplitude stages stay visible.
			maxAbs := 0.0
			for _, v := range hm {
				if v < 0 {
					v = -v
				}
				if v > maxAbs {
					maxAbs = v
				}
			}
			if maxAbs < 1e-9 {
				maxAbs = 1.0
			}
			for py := range size {
				for px := range size {
					v := hm[py*size+px]
					var c color.RGBA
					if v >= 0 {
						// White = positive (accumulation).
						b := uint8(math.Min(255, 255*v/maxAbs))
						c = color.RGBA{R: b, G: b, B: b, A: 255}
					} else {
						// Red = negative (erosion/carve).
						r := uint8(math.Min(255, 255*(-v)/maxAbs))
						c = color.RGBA{R: r, G: 0, B: 0, A: 255}
					}
					img.SetRGBA(px, py, c)
				}
			}
		} else {
			// Unsigned grayscale [0,1].
			for py := range size {
				for px := range size {
					v := hm[py*size+px]
					if v < 0 {
						v = 0
					} else if v > 1 {
						v = 1
					}
					b := uint8(v * 255)
					img.SetRGBA(px, py, color.RGBA{R: b, G: b, B: b, A: 255})
				}
			}
		}
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		return base64.StdEncoding.EncodeToString(buf.Bytes())
	}

	// encodeColor encodes a []color.RGBA as a square PNG.
	encodeColor := func(rgba []color.RGBA) string {
		if rgba == nil {
			return ""
		}
		img := image.NewRGBA(image.Rect(0, 0, size, size))
		for py := range size {
			for px := range size {
				img.SetRGBA(px, py, rgba[py*size+px])
			}
		}
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		return base64.StdEncoding.EncodeToString(buf.Bytes())
	}

	stages := make([]map[string]any, 0, len(frame.Stages))
	for _, st := range frame.Stages {
		row := map[string]any{
			"name":         st.Name,
			"kind":         st.Kind,
			"skipped":      st.Skipped,
			"input_bands":  "",
			"output_bands": "",
		}
		if st.Kind == "color" {
			row["color_after"] = encodeColor(st.ColorAfter)
			row["raw"] = ""
			row["sum_after"] = ""
		} else {
			row["raw"] = encodeHeight(st.RawDelta, true)
			row["sum_after"] = encodeHeight(st.SumAfter, false)
			row["color_after"] = ""
		}
		stages = append(stages, row)
	}
	out, _ := json.Marshal(map[string]any{"stages": stages})
	return js.ValueOf(string(out))
}

// flatCacheStats() → JSON string {"hits": N, "misses": M} for diagnostics.
func flatCacheStats(_ js.Value, _ []js.Value) any {
	hits, misses := render.FlatCacheStats()
	out, _ := json.Marshal(map[string]any{"hits": hits, "misses": misses})
	return js.ValueOf(string(out))
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
