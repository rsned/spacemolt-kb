package layers

import (
	"fmt"
	"sync"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// LayerStack manages the layer rendering pipeline with caching and
// dirty tracking. It maintains:
// - The current render position (highest completed layer)
// - Cached heightmap at that position
// - Cached context with intermediate fields
// - Dirty tracking for efficient re-render
type LayerStack struct {
	layers []Layer // sorted by dependency (from All())

	mu sync.RWMutex

	// Current render state
	currentLayer    int                  // index of highest rendered layer
	cachedHeightmap *cubemap.CubeMapF   // state at currentLayer
	cachedContext   *Context            // includes all intermediate fields

	// Dirty tracking
	dirtyFrom       int  // 0 = clean, N = layer N changed, re-render from N
	dirtyProfile    bool // true if profile itself changed (full re-render)
}

// NewStack creates a LayerStack with all registered layers.
func NewStack() *LayerStack {
	return &LayerStack{
		layers:    All(),
		dirtyFrom: len(All()), // mark clean initially
	}
}

// MarkDirty invalidates the cache starting from the layer that touches
// the given profile parameter. If the parameter is unknown, it's ignored
// (may be a field not referenced by any layer, e.g., palette colors).
func (ls *LayerStack) MarkDirty(paramPath string) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	// Find which layer touches this param
	for i, layer := range ls.layers {
		for _, p := range layer.Params() {
			if p == paramPath {
				if i < ls.dirtyFrom {
					ls.dirtyFrom = i
				}
				return
			}
		}
	}
	// Unknown param — ignore (may be decorative only)
}

// MarkProfileDirty invalidates the entire cache because the profile
// structure itself changed (e.g., different planet type).
func (ls *LayerStack) MarkProfileDirty() {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	ls.dirtyFrom = 0
	ls.dirtyProfile = true
}

// RenderTo renders all enabled layers up to and including targetLayer.
// Returns the heightmap at that layer. Uses cached state if possible.
// Thread-safe: concurrent calls will serialize correctly.
func (ls *LayerStack) RenderTo(targetLayer int, ctx *Context) (*cubemap.CubeMapF, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if targetLayer < 0 || targetLayer >= len(ls.layers) {
		return nil, fmt.Errorf("target layer %d out of range [0, %d)", targetLayer, len(ls.layers))
	}

	// Determine starting point
	startFrom := ls.dirtyFrom

	// If target is before dirty point, we need earlier cache that's gone
	// → re-render from 0
	if targetLayer < startFrom {
		startFrom = 0
	}

	// If profile is dirty, always start from 0
	if ls.dirtyProfile {
		startFrom = 0
		ls.dirtyProfile = false
	}

	// If we have a valid cache at or before startFrom, use it
	var hm *cubemap.CubeMapF
	if startFrom > 0 && ls.cachedHeightmap != nil && ls.cachedContext != nil {
		// Check if cache is compatible (same seed, size, profile semantically equal)
		if ls.cachedContext.MasterSeed == ctx.MasterSeed &&
			ls.cachedContext.Size == ctx.Size &&
			ls.cachedContext.Profile.Type == ctx.Profile.Type {
			hm = ls.cachedHeightmap
			ctx = ls.cachedContext // reuse cached intermediate fields
		} else {
			// Incompatible cache — start from scratch
			startFrom = 0
		}
	}

	// Initialize flat canvas if starting from 0
	if startFrom == 0 {
		hm = cubemap.NewF(ctx.Size)
		for face := range hm.Faces {
			for i := range hm.Faces[face] {
				hm.Faces[face][i] = 0.5 // flat 0.5
			}
		}
	} else if hm == nil {
		// Shouldn't happen: startFrom > 0 but no cache
		hm = cubemap.NewF(ctx.Size)
		for face := range hm.Faces {
			for i := range hm.Faces[face] {
				hm.Faces[face][i] = 0.5
			}
		}
	}

	// Render each enabled layer up to target
	for i := startFrom; i <= targetLayer; i++ {
		layer := ls.layers[i]
		if !layer.Enabled(ctx.Profile) {
			continue
		}
		newHm := layer.Render(ctx, hm)
		if newHm != nil {
			hm = newHm
		}
		// else: layer returned same hm (no effect)
	}

	// Update cache
	ls.cachedHeightmap = hm
	ls.cachedContext = ctx
	ls.currentLayer = targetLayer
	ls.dirtyFrom = len(ls.layers) // mark clean

	return hm, nil
}

// CurrentLayer returns the index of the highest rendered layer.
// Returns -1 if nothing has been rendered yet.
func (ls *LayerStack) CurrentLayer() int {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.currentLayer
}

// Layers returns the layer list in dependency order.
func (ls *LayerStack) Layers() []Layer {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.layers
}

// LayerByIndex returns the layer at the given index.
// Panics if out of range.
func (ls *LayerStack) LayerByIndex(i int) Layer {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.layers[i]
}

// FindLayer returns the index of the layer with the given ID.
// Returns -1 if not found.
func (ls *LayerStack) FindLayer(id string) int {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	for i, layer := range ls.layers {
		if layer.ID() == id {
			return i
		}
	}
	return -1
}

// InvalidateBefore marks all layers up to and including layer i as dirty.
// Used when a fundamental change (e.g., seed, size) requires re-render.
func (ls *LayerStack) InvalidateBefore(i int) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if i < ls.dirtyFrom {
		ls.dirtyFrom = i
	}
}

// ClearCache resets all cached state.
func (ls *LayerStack) ClearCache() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.cachedHeightmap = nil
	ls.cachedContext = nil
	ls.currentLayer = -1
	ls.dirtyFrom = len(ls.layers)
	ls.dirtyProfile = false
}

// IsDirty returns true if the stack has dirty layers.
func (ls *LayerStack) IsDirty() bool {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.dirtyFrom < len(ls.layers) || ls.dirtyProfile
}

// DirtyRange returns the range of dirty layers [from, to).
// If not dirty, returns (len, len).
func (ls *LayerStack) DirtyRange() (from, to int) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.dirtyFrom, len(ls.layers)
}
