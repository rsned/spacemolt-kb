// Example concrete layer implementations (Phase 13 PoC).
//
// This file demonstrates how to implement the Layer interface for
// specific pipeline stages. These are representative examples;
// actual implementations will use the real field packages after Phase 12.
//
// +build ignore
package layers

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// =============================================================================
// FLAT LAYER (Layer 0)
// =============================================================================

// FlatLayer is the base layer: a flat heightmap at 0.5.
type FlatLayer struct{}

func (l *FlatLayer) ID() string         { return "flat" }
func (l *FlatLayer) Name() string       { return "Flat Canvas" }
func (l *FlatLayer) Description() string { return "Starting canvas: flat heightmap at 0.5 (mid-world height, like Minecraft Superflat)." }
func (l *FlatLayer) Category() Category  { return CategoryBase }
func (l *FlatLayer) DependsOn() []string { return []string{} }
func (l *FlatLayer) Params() []string    { return []string{} }
func (l *FlatLayer) Enabled(profile *types.PlanetProfile) bool {
	return true
}

func (l *FlatLayer) Render(ctx *Context, hm *cubemap.CubeMapF) *cubemap.CubeMapF {
	// Should already be flat at 0.5, but ensure it
	for face := range hm.Faces {
		for i := range hm.Faces[face] {
			hm.Faces[face][i] = 0.5
		}
	}
	return hm
}

func (l *FlatLayer) RenderDebug(ctx *Context) *cubemap.CubeMap {
	return nil // No debug visualization for flat layer
}

func init() {
	Register(&FlatLayer{})
}

// =============================================================================
// CRUST LAYER
// =============================================================================

// CrustLayer generates the continental mask and base height from cratons.
type CrustLayer struct{}

func (l *CrustLayer) ID() string         { return "crust" }
func (l *CrustLayer) Name() string       { return "Continental Crust" }
func (l *CrustLayer) Description() string {
	return "Rafts of continental crust (cratons) riding on plates. " +
		"Determines land vs ocean at a fundamental level. " +
		"Assembly controls clustering: low = supercontinent, high = fragmented."
}
func (l *CrustLayer) Category() Category { return CategoryTectonics }
func (l *CrustLayer) DependsOn() []string {
	return []string{"plates"}
}

func (l *CrustLayer) Params() []string {
	return []string{
		"Crust.Assembly",
		"Crust.TargetLandFraction",
		"Crust.PlatformHeight",
		"Crust.OceanFloorHeight",
		"Crust.ShelfWidthRad",
		"Crust.EdgeNoiseAmp",
		"Crust.EdgeNoiseFreq",
		"Crust.EdgeNoiseOctaves",
		"Crust.CratonsMax",
		"Crust.MajorGrowthBias",
	}
}

func (l *CrustLayer) Enabled(profile *types.PlanetProfile) bool {
	return profile.Crust.MajorPlates > 0
}

func (l *CrustLayer) Render(ctx *Context, hm *cubemap.CubeMapF) *cubemap.CubeMapF {
	// Generate crust if not cached
	if ctx.Crust == nil {
		// TODO: ctx.Crust = field.GenerateCrust(ctx.Profile, ctx.MasterSeed, ctx.Size, ctx.Plates)
		// For now, placeholder
	}
	if ctx.Crust == nil {
		return hm
	}

	// Apply crust base height to the heightmap
	for face := range hm.Faces {
		for i, baseH := range ctx.Crust.BaseHeight.Faces[face] {
			hm.Faces[face][i] = baseH
		}
	}

	return hm
}

func (l *CrustLayer) RenderDebug(ctx *Context) *cubemap.CubeMap {
	if ctx.Crust == nil {
		return nil
	}
	// Return continental mask as grayscale visualization
	// TODO: implement debug render
	return nil
}

func init() {
	Register(&CrustLayer{})
}

// =============================================================================
// TECTONIC FX LAYER
// =============================================================================

// TectonicFXLayer applies boundary effects: belts, trenches, arcs, ridges, rifts.
type TectonicFXLayer struct{}

func (l *TectonicFXLayer) ID() string         { return "tectonic-fx" }
func (l *TectonicFXLayer) Name() string       { return "Boundary Effects" }
func (l *TectonicFXLayer) Description() string {
	return "Crust-aware boundary effects. Collision belts (Himalayas), " +
		"subduction trenches + cordilleras (Andes), island arcs (Japan), " +
		"mid-ocean ridges, continental rifts (Red Sea). " +
		"TectonicAge controls sharpness: young = tall/sharp, old = wide/eroded."
}
func (l *TectonicFXLayer) Category() Category { return CategoryTectonics }
func (l *TectonicFXLayer) DependsOn() []string {
	return []string{"crust"}
}

func (l *TectonicFXLayer) Params() []string {
	params := []string{
		"TectonicFX.BeltAmp", "TectonicFX.BeltWidthKm", "TectonicFX.BeltFreq", "TectonicFX.BeltOctaves",
		"TectonicFX.CordAmp", "TectonicFX.CordWidthKm",
		"TectonicFX.TrenchDepth", "TectonicFX.TrenchWidthKm",
		"TectonicFX.ArcAmp", "TectonicFX.ArcWidthKm",
		"TectonicFX.RidgeAmp", "TectonicFX.RidgeWidthKm",
		"TectonicFX.RiftDepth", "TectonicFX.RiftWidthKm", "TectonicFX.RiftShoulder",
		"TectonicFX.TransformAmp", "TectonicFX.TransformWidthKm",
		"TectonicFX.ActivityFreq",
		"Crust.TectonicAge",
	}
	return params
}

func (l *TectonicFXLayer) Enabled(profile *types.PlanetProfile) bool {
	return profile.Crust.MajorPlates > 0 && profile.TectonicFX.BeltAmp > 0
}

func (l *TectonicFXLayer) Render(ctx *Context, hm *cubemap.CubeMapF) *cubemap.CubeMapF {
	// Generate tectonic FX if not cached
	if ctx.TectonicFX == nil {
		// TODO: ctx.TectonicFX = field.ClassifyTectonics(ctx.Plates, ctx.Crust, ctx.RadiusKm)
	}
	if ctx.TectonicFX == nil {
		return hm
	}

	// Apply boundary effects
	// TODO: field.ApplyTectonicFX(hm, ctx.TectonicFX, ctx.Crust, ctx.Plates, ctx.Profile.TectonicFX, ctx.MasterSeed, ctx.Size)

	return hm
}

func (l *TectonicFXLayer) RenderDebug(ctx *Context) *cubemap.CubeMap {
	if ctx.TectonicFX == nil {
		return nil
	}
	// Return boundary classification visualization
	// Red = belt, Orange = subduction, Yellow = arc, Blue = ridge, Cyan = rift
	// TODO: implement debug render
	return nil
}

func init() {
	Register(&TectonicFXLayer{})
}

// =============================================================================
// SEA LEVEL LAYER
// =============================================================================

// SeaLevelLayer derives ocean level from target land fraction via histogram quantile.
type SeaLevelLayer struct{}

func (l *SeaLevelLayer) ID() string         { return "sealevel" }
func (l *SeaLevelLayer) Name() string       { return "Sea Level" }
func (l *SeaLevelLayer) Description() string {
	return "Derived sea level: histogram quantile such that ocean covers " +
		"(1 - TargetLandFraction) of the planet. No user parameter; " +
		"computed automatically from the finished heightmap."
}
func (l *SeaLevelLayer) Category() Category { return CategoryOceans }
func (l *SeaLevelLayer) DependsOn() []string {
	return []string{"tectonic-fx"}
}
func (l *SeaLevelLayer) Params() []string {
	return []string{} // derived, no direct params
}

func (l *SeaLevelLayer) Enabled(profile *types.PlanetProfile) bool {
	return true
}

func (l *SeaLevelLayer) Render(ctx *Context, hm *cubemap.CubeMapF) *cubemap.CubeMapF {
	// Compute quantile sea level
	// targetLandFraction is resolved from Crust.TargetLandFraction
	// TODO: oceanFrac := field.QuantileSeaLevel(hm, 1 - targetLandFraction)
	// Store ocean level in context for downstream use (coastal, erosion, etc.)
	// ctx.OceanLevel = oceanFrac
	return hm
}

func (l *SeaLevelLayer) RenderDebug(ctx *Context) *cubemap.CubeMap {
	// Return a visualization showing land vs ocean after sea level application
	// Green = above sea level, Blue = below
	return nil
}

func init() {
	Register(&SeaLevelLayer{})
}

// =============================================================================
// EROSION LAYER
// =============================================================================

// ErosionLayer applies hydraulic erosion (droplets carving channels).
type ErosionLayer struct{}

func (l *ErosionLayer) ID() string         { return "erosion" }
func (l *ErosionLayer) Name() string       { return "Hydraulic Erosion" }
func (l *ErosionLayer) Description() string {
	return "Particle-based hydraulic erosion. Droplets walk the heightmap, " +
		"carrying and depositing sediment to form river channels. " +
		"Droplets=0 disables this layer."
}
func (l *ErosionLayer) Category() Category { return CategorySurface }
func (l *ErosionLayer) DependsOn() []string {
	return []string{"sealevel"}
}

func (l *ErosionLayer) Params() []string {
	return []string{
		"Erosion.Droplets",
		"Erosion.Inertia",
		"Erosion.Capacity",
		"Erosion.ErosionRate",
		"Erosion.Deposition",
		"Erosion.Evaporation",
		"Erosion.MinSlope",
		"Erosion.MaxStepsPerDrop",
		"Erosion.Gravity",
		"Erosion.BrushFalloff",
	}
}

func (l *ErosionLayer) Enabled(profile *types.PlanetProfile) bool {
	return profile.Erosion.Droplets > 0
}

func (l *ErosionLayer) Render(ctx *Context, hm *cubemap.CubeMapF) *cubemap.CubeMapF {
	// Apply erosion
	// TODO: field.Erode(hm, ctx.Profile.Erosion, ctx.MasterSeed, ctx.Size)
	return hm
}

func (l *ErosionLayer) RenderDebug(ctx *Context) *cubemap.CubeMap {
	// Return visualization of erosion magnitude (where channels formed)
	return nil
}

func init() {
	Register(&ErosionLayer{})
}
