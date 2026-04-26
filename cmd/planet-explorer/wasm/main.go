// Wasm entrypoint for cmd/planet-explorer. Builds with GOOS=js
// GOARCH=wasm into cmd/planet-explorer/web/planet-explorer.wasm.
//
// Exposes three functions on the JS global scope:
//
//	planetExplorerGenerate(profileJSON string, seedStr string, faceSize int) Uint8Array
//	planetExplorerBakeEquirect(cubePNG Uint8Array, w int, h int)            Uint8Array
//	planetExplorerDefaultProfile(planetType string)                          string  // JSON
//
//go:build js && wasm

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"syscall/js"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func main() {
	js.Global().Set("planetExplorerGenerate", js.FuncOf(generate))
	js.Global().Set("planetExplorerBakeEquirect", js.FuncOf(bakeEquirect))
	js.Global().Set("planetExplorerDefaultProfile", js.FuncOf(defaultProfile))
	<-make(chan struct{}) // keep the WASM process alive
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
