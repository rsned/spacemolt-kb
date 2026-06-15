# WASM Interface — Progressive Layer Rendering

**Date:** 2026-06-14
**Component:** `cmd/planet-explorer/wasm/main.go` export signatures

## Overview

The WASM interface shifts from "one-shot generate" to "incremental walkthrough."
New exports allow the frontend to:
- Navigate layers (next, prev, jump to)
- Get layer metadata (name, description, params)
- Trigger re-render at specific layers
- Retrieve cached state

## New WASM Exports

### State Management

```go
// WizardInit initializes the wizard with a planet type and seed.
// Returns the number of available layers.
// Must be called before any other wizard functions.
//go:export wizardInit
func wizardInit(planetType *byte, seed int64) int

// WizardLayerCount returns the total number of layers.
//go:export wizardLayerCount
func wizardLayerCount() int

// WizardCurrentLayer returns the index of the currently rendered layer.
// Returns -1 if nothing rendered yet.
//go:export wizardCurrentLayer
func wizardCurrentLayer() int

// WizardSetTargetLayer sets the target layer and triggers render.
// The UI should call this and then poll for completion or use the callback.
//go:export wizardSetTargetLayer
func wizardSetTargetLayer(layerIndex int)
```

### Layer Metadata

```go
// WizardLayerID returns the layer ID (null-terminated, caller frees).
//go:export wizardLayerID
func wizardLayerID(layerIndex int) *byte

// WizardLayerName returns the display name (null-terminated, caller frees).
//go:export wizardLayerName
func wizardLayerName(layerIndex int) *byte

// WizardLayerDesc returns the description (null-terminated, caller frees).
//go:export wizardLayerDesc
func wizardLayerDesc(layerIndex int) *byte

// WizardLayerCategory returns the category (null-terminated, caller frees).
//go:export wizardCategory
func wizardCategory(layerIndex int) *byte

// WizardLayerEnabled returns 1 if the layer is enabled for the current profile.
//go:export wizardLayerEnabled
func wizardLayerEnabled(layerIndex int) int

// WizardLayerDependencies returns a JSON array of dependency layer IDs.
// Caller must free the returned string.
//go:export wizardLayerDependencies
func wizardLayerDependencies(layerIndex int) *byte
```

### Parameter Access

```go
// WizardLayerParams returns a JSON object of {paramPath: currentValue}
// for the given layer. Example: {"Crust.MajorPlates": 7, "Crust.Assembly": 0.5}
// Caller must free the returned string.
//go:export wizardLayerParams
func wizardLayerParams(layerIndex int) *byte

// WizardSetParam sets a profile parameter value.
// Param path is dot-notation: "Crust.MajorPlates", "TectonicFX.BeltAmp", etc.
// Value is JSON-encoded (number, string, bool, or object).
// This triggers dirty tracking and may cause re-render on next wizardRenderTo.
//go:export wizardSetParam
func wizardSetParam(path *byte, valueJSON *byte)

// WizardGetParam returns the current value of a parameter as JSON.
// Caller must free the returned string.
//go:export wizardGetParam
func wizardGetParam(path *byte) *byte
```

### Rendering

```go
// WizardRenderTo renders up to the target layer.
// This is the main render function; it updates the cube map and equirect buffers.
// After this call, use wizardGetCubeFace(index) to retrieve face data.
//go:export wizardRenderTo
func wizardRenderTo(layerIndex int)

// WizardRenderPreview renders the tectonic preview (special debug render).
// Ignores layer stack and renders plates + boundaries with glow.
//go:export wizardRenderPreview
func wizardRenderPreview(exaggeration float64)

// WizardGetCubeFace returns a pointer to RGBA data for the given face.
// Face order: 0=Right, 1=Left, 2=Top, 3=Bottom, 4=Front, 5=Back
// Data is S×S×4 bytes. Valid until next render call.
//go:export wizardGetCubeFace
func wizardGetCubeFace(face int) *byte

// WizardGetEquirect returns a pointer to RGBA data for the equirect projection.
// Data is Width×Height×4 bytes. Valid until next render call.
//go:export wizardGetEquirect
func wizardGetEquirect() *byte

// WizardGetFaceSize returns the current cube face edge length in pixels.
//go:export wizardGetFaceSize
func wizardGetFaceSize() int
```

### Profile Export/Import

```go
// WizardExportProfile returns the full profile as JSON.
// Caller must free the returned string.
//go:export wizardExportProfile
func wizardExportProfile() *byte

// WizardImportProfile loads a profile from JSON.
// Replaces the current profile and marks all layers dirty.
// Returns 0 on success, non-zero on parse error.
//go:export wizardImportProfile
func wizardImportProfile(json *byte) int
```

### Template Access

```go
// WizardTemplateList returns a JSON array of available template planet types.
// Example: ["terran", "super_terran", "oceanic", "arid", ...]
// Caller must free the returned string.
//go:export wizardTemplateList
func wizardTemplateList() *byte

// WizardApplyTemplate applies the default template for a planet type.
// Resets all layer params to their template defaults using the current seed.
//go:export wizardApplyTemplate
func wizardApplyTemplate(planetType *byte)
```

### Async Completion (Optional)

```go
// WizardSetRenderCallback sets a JS callback invoked when render completes.
// Callback signature: function(layerIndex, successFlag)
//go:export wizardSetRenderCallback
func wizardSetRenderCallback(callback uintptr)

// WizardPollStatus returns the render status.
// Returns: 0=idle, 1=rendering, 2=error
//go:export wizardPollStatus
func wizardPollStatus() int
```

## Existing Exports (Unchanged)

These continue to work as before:

```go
//go:export generate
func generate(planetType, planetName *byte, faceSize int)

//go:export getFace
func getFace(index int) *byte

//go:export setParam
func setParam(path, valueJSON *byte)

// ... other existing exports
```

## Implementation Sketch

```go
package main

var (
	wizardStack   *layers.LayerStack
	wizardCtx     *layers.Context
	wizardProfile *types.PlanetProfile
	wizardSeed    int64
	wizardSize    int = 1024
	cubeMap       *cubemap.CubeMap
)

//export wizardInit
func wizardInit(planetType *byte, seed int64) int {
	wizardSeed = seed

	// Load profile for type
	ptype := goString(planetType)
	wizardProfile = planetogen.GetProfile(ptype)
	if wizardProfile == nil {
		return 0
	}

	// Initialize context
	wizardCtx = &layers.Context{
		MasterSeed: seed,
		Size:       wizardSize,
		RadiusKm:   6371,
		Profile:    wizardProfile,
	}

	// Create layer stack
	wizardStack = layers.NewStack()

	return len(wizardStack.Layers())
}

//export wizardSetTargetLayer
func wizardSetTargetLayer(layerIndex int) {
	hm, err := wizardStack.RenderTo(layerIndex, wizardCtx)
	if err != nil {
		// Set error state, invoke callback if set
		return
	}

	// Colorize the heightmap
	cubeMap = render.ColorizeHeightmap(hm, wizardProfile, wizardCtx, wizardSeed, wizardSize)
}

//export wizardRenderPreview
func wizardRenderPreview(exaggeration float64) {
	// Generate plates, crust, tectonicFX if not cached
	if wizardCtx.Plates == nil {
		wizardCtx.Plates = field.GeneratePlates(wizardProfile, wizardSeed, wizardSize)
	}
	if wizardCtx.Crust == nil {
		wizardCtx.Crust = field.GenerateCrust(wizardProfile, wizardSeed, wizardSize, wizardCtx.Plates)
	}
	if wizardCtx.TectonicFX == nil {
		wizardCtx.TectonicFX = field.ClassifyTectonics(wizardCtx.Plates, wizardCtx.Crust, 6371)
	}

	// Render tectonic preview
	cubeMap = render.RenderTectonicPreview(wizardCtx, exaggeration)
}

//export wizardLayerParams
func wizardLayerParams(layerIndex int) *byte {
	layer := wizardStack.LayerByIndex(layerIndex)
	params := make(map[string]interface{})
	for _, p := range layer.Params() {
		// Reflect into profile to get current value
		params[p] = getProfileValue(wizardProfile, p)
	}
	j, _ := json.Marshal(params)
	return jsonString(j)
}

//export wizardSetParam
func wizardSetParam(path, valueJSON *byte) {
	paramPath := goString(path)
	valueBytes := CBytes(valueJSON)

	// Unmarshal and set into profile
	var value interface{}
	json.Unmarshal(valueBytes, &value)
	setProfileValue(wizardProfile, paramPath, value)

	// Mark dirty
	wizardStack.MarkDirty(paramPath)
}
```

## JS Binding Sketch

```javascript
const API = {
  // Init
  wizardInit: Module.cwrap('wizardInit', 'number', ['string', 'number']),
  wizardLayerCount: Module.cwrap('wizardLayerCount', 'number', []),
  wizardCurrentLayer: Module.cwrap('wizardCurrentLayer', 'number', []),

  // Navigation
  wizardSetTargetLayer: Module.cwrap('wizardSetTargetLayer', 'void', ['number']),
  wizardRenderPreview: Module.cwrap('wizardRenderPreview', 'void', ['number']),

  // Metadata
  wizardLayerID: Module.cwrap('wizardLayerID', 'string', ['number']),
  wizardLayerName: Module.cwrap('wizardLayerName', 'string', ['number']),
  wizardLayerDesc: Module.cwrap('wizardLayerDesc', 'string', ['number']),
  wizardLayerCategory: Module.cwrap('wizardCategory', 'string', ['number']),
  wizardLayerEnabled: Module.cwrap('wizardLayerEnabled', 'number', ['number']),

  // Parameters
  wizardLayerParams: Module.cwrap('wizardLayerParams', 'string', ['number']),
  wizardSetParam: Module.cwrap('wizardSetParam', 'void', ['string', 'string']),
  wizardGetParam: Module.cwrap('wizardGetParam', 'string', ['string']),

  // Rendering
  wizardRenderTo: Module.cwrap('wizardRenderTo', 'void', ['number']),
  wizardGetCubeFace: Module.cwrap('wizardGetCubeFace', 'number', ['number']),
  wizardGetFaceSize: Module.cwrap('wizardGetFaceSize', 'number', []),

  // Profile
  wizardExportProfile: Module.cwrap('wizardExportProfile', 'string', []),
  wizardImportProfile: Module.cwrap('wizardImportProfile', 'number', ['string']),
};

// Usage
class Wizard {
  constructor(planetType, seed) {
    this.layerCount = API.wizardInit(planetType, seed);
    this.currentLayer = -1;
  }

  getLayers() {
    const layers = [];
    for (let i = 0; i < this.layerCount; i++) {
      layers.push({
        id: API.wizardLayerID(i),
        name: API.wizardLayerName(i),
        desc: API.wizardLayerDesc(i),
        category: API.wizardCategory(i),
        enabled: API.wizardLayerEnabled(i) === 1,
      });
    }
    return layers;
  }

  renderTo(layerIndex) {
    API.wizardSetTargetLayer(layerIndex);
    this.currentLayer = API.wizardCurrentLayer();
  }

  renderPreview(exaggeration) {
    API.wizardRenderPreview(exaggeration);
  }

  getCubeFace(face) {
    const ptr = API.wizardGetCubeFace(face);
    const size = API.wizardGetFaceSize();
    const data = new Uint8ClampedArray(Module.HEAPU8.buffer, ptr, size * size * 4);
    return new ImageData(data, size, size);
  }

  setParam(path, value) {
    API.wizardSetParam(path, JSON.stringify(value));
  }

  getLayerParams(layerIndex) {
    const json = API.wizardLayerParams(layerIndex);
    return JSON.parse(json);
  }
}
```

## Differences from Current WASM

| Current | New (Wizard) |
|---------|--------------|
| `generate(type, name)` → final result only | `wizardInit()` → layer metadata, navigate incrementally |
| No layer awareness | Full layer list with dependencies |
| No dirty tracking | Automatic cache invalidation on param change |
| No tectonic preview | `wizardRenderPreview()` special debug render |
| `setParam()` immediate re-render | `setParam()` dirty flag, re-render on next `renderTo()` |
| No template access | `wizardApplyTemplate()` defaults per planet type |
