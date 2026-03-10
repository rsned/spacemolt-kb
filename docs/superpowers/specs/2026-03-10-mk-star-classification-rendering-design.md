# MK Star Classification Rendering Design

**Date:** 2026-03-10
**Status:** Approved
**Author:** Claude (with user collaboration)

## Overview

Add visual rendering of Morgan-Keenan (MK) star classifications and planet class types to the knowledge base system maps. Stars will display with spectral-type colors and luminosity-based sizes, while planets will show class-specific styling including rings for gas giants.

## Goals

1. **Visual clarity**: Make system maps more informative at a glance
2. **Scientific accuracy**: Use real astronomical classification data
3. **Backward compatibility**: Work with existing systems lacking classification data
4. **Performance**: Minimal rendering overhead
5. **Maintainability**: Clean, testable code structure

## Data Model

### POI Class Field

Add `Class` field to POI structs across the codebase:

```go
// pkg/game/types.go
// pkg/game/serverapi/types.go
// pkg/systemmap/systemmap.go

type POI struct {
    ID          string
    Name        string
    Type        string
    Class       string `json:"class,omitempty"` // MK classification or planet class
    Description string
    Position    Position
    // ... other fields
}
```

The field is `omitempty` to maintain backward compatibility.

### Classification Formats

**Stars**: `"{spectral}{number} {luminosity}"` or `"{spectral}{number}{luminosity}"`
- Examples: `"G2 V"`, `"B3 Ia"`, `"M9V"`, `"L5 V"`

**Planets**: Class name string
- Examples: `"terran"`, `"jovian"`, `"ice_giant"`, `"rocky"`, `"dwarf"`

## Rendering Specifications

### Star Colors by Spectral Type

| Spectral | Color       | Hex     | Name          |
|----------|-------------|---------|---------------|
| O        | Blue        | #a0c8ff | ~40,000 K     |
| B        | Blue-White  | #c8d8ff | 10-30K K      |
| A        | White       | #e8eeff | 7.5-10K K     |
| F        | Yellow-White| #fffff0 | 6-7.5K K      |
| G        | Yellow      | #fff4a0 | 5.2-6K K      |
| K        | Orange      | #ffcf6e | 3.7-5.2K K    |
| M        | Red         | #ff8060 | 2.4-3.7K K    |
| L        | Dark Red    | #cc4020 | 1.3-2.4K K    |
| T        | Infrared    | #882010 | 700-1300 K    |
| Y        | Near-IR     | #440a05 | <700 K        |

### Star Sizes by Luminosity Class (Logarithmic Scale)

| Class | Name              | Size (px) | Multiplier |
|-------|-------------------|-----------|------------|
| Ia    | Hypergiant        | 28        | 2.8×       |
| Ib    | Supergiant        | 24        | 2.4×       |
| II    | Bright Giant      | 20        | 2.0×       |
| III   | Giant             | 16        | 1.6×       |
| IV    | Subgiant          | 14        | 1.4×       |
| V     | Main Sequence     | 10        | 1.0×       |
| VI    | Subdwarf          | 8         | 0.8×       |
| VII   | White Dwarf       | 6         | 0.6×       |

Baseline: Current sun rendering is 10px radius.

### Glow Effects

- **Main sequence stars**: Standard radial gradient glow
- **Brown dwarfs (L/T/Y)**: Reduced glow opacity (0.3× normal)
- **White dwarfs (VII)**: White-blue core with distinctive small halo
- **Giants/Supergiants (I-III)**: Extended glow radius proportional to size

### Planet Colors by Class

| Class       | Color      | Hex     | Notes                    |
|-------------|------------|---------|--------------------------|
| terran      | Blue-green | #4a9c6d | Earth-like              |
| jovian      | Orange     | #e8a86c | Gas giant + rings       |
| ice_giant   | Pale blue  | #9fc5e8 | Neptune/Uranus-like     |
| gas_dwarf   | Tan        | #d4b596 | Mini-Neptune            |
| rocky       | Brown-gray | #8b7355 | Mercury/Mars-like       |
| dwarf       | Tan-brown  | #c4a484 | Pluto-like              |
| proto       | Reddish    | #cc6666 | Forming planet          |
| captured    | Gray-purple| #b0a0c0 | Captured body           |

### Jovian Ring System

- Ellipse: rx = planetRadius × 2.5, ry = planetRadius × 0.6
- Tilt: 30° from horizontal
- 2-3 nested ellipses for banding
- Color: Golden-orange, opacity 0.4-0.6

## Labels and Tooltips

### On-Map Labels

- **Stars**: Full MK notation (e.g., "G2 V", "B3 Ia")
- **Planets**: Planet name only
- Font: 11px sans-serif, color #d8dee9
- Positioning: Below for Y≥0, above for Y<0 (preserved)

### Tooltip Content

**Classified star:**
```
{Star Name} — {Spectral} {Luminosity} ({Color Name})
Example: "Sol — G2 V (Yellow Main Sequence)"
```

**Unclassified star:**
```
{Star Name} — Star (classification unknown)
```

**Classified planet:**
```
{Planet Name} — {Class Name}
Example: "Jupiter — Jovian Gas Giant"
```

## Implementation Structure

### New Files

**`pkg/systemmap/stars.go`**
```go
type SpectralType struct {
    Letter      string
    Color       string
    Name        string
    TempRange   string
}

type LuminosityClass struct {
    Roman       string
    Name        string
    Multiplier  float64
    Size        float64
}

// ParseStarClass parses "G2 V" or "G2V" into spectral + luminosity
func ParseStarClass(class string) (spectral, luminosity string, err error)

// GetStarColor returns hex color for spectral type
func GetStarColor(spectral string) string

// GetStarSize returns pixel radius for luminosity class
func GetStarSize(luminosity string) float64

// renderStar generates SVG for a classified star
func renderStar(poi POI, cx, cy float64) string
```

**`pkg/systemmap/planets.go`**
```go
type PlanetClass struct {
    ID        string
    Name      string
    Color     string
    HasRings  bool
}

// GetPlanetColor returns hex color for planet class
func GetPlanetColor(class string) string

// renderPlanet generates SVG for a planet with class styling
func renderPlanet(poi POI, px, py float64) string

// renderJovianRings generates ring system SVG
func renderJovianRings(cx, cy, planetRadius float64) string
```

### Modified Files

**`pkg/systemmap/systemmap.go`**
- Update `POI` struct to include `Class string`
- Update `RenderSystemMap()` to call new render functions
- Add conflict detection: check star radius vs nearest orbit
- Update `renderLegendSwatch()` for planet class variations

**`pkg/game/types.go` & `pkg/game/serverapi/types.go`**
- Add `Class string` field to `POI` struct

**`pkg/knowledge/sqlite.go`**
- Migration: `ALTER TABLE pois ADD COLUMN class TEXT;`
- Update POI queries to include class field

## Parsing Logic

### Class String Parsing

Handle both formats:
1. **With space**: `"G2 V"` → split on space
2. **Without space**: `"G2V"`, `"B3Ia"` → regex extract

**Algorithm:**
1. Trim whitespace
2. Try splitting on space first (yield 2 parts)
3. If no space or invalid split:
   - Extract spectral: first letter + optional digit (0-9)
   - Extract luminosity: Roman numeral pattern (Ia, Ib, I- VII)
4. Validate both parts against known values

**Fallbacks:**
- Missing luminosity: default to "V" (main sequence)
- Invalid spectral: log warning, use generic rendering
- Invalid luminosity: log warning, use generic rendering

## Error Handling

### Malformed Class Strings
- Log warning with system ID, POI ID, and the invalid class
- Fall back to generic sun/planet rendering
- Continue rendering remaining POIs

### Missing Class Field
- Use existing generic rendering (no logging)
- Expected for older/unclassified data

### Unknown Planet Class
- Log warning with class value
- Fall back to default green `#A3BE8C`

### Brown Dwarf Edge Cases
- Fixed size ~8px regardless of luminosity
- Log warning if luminosity > III (brown dwarfs shouldn't be giants)

### Star-Orbit Conflicts
- Log: `"System {systemID}: Star radius {r}px overlaps nearest orbit at {orbit}px"`
- Render anyway (visual inspection)
- Track frequent conflicts for potential algorithm adjustment

## Testing Strategy

### Unit Tests

**`stars_test.go`:**
- `TestParseStarClass()` - Valid formats, edge cases, invalid inputs
- `TestGetStarColor()` - All spectral types
- `TestGetStarSize()` - All luminosity classes
- `TestBrownDwarfSize()` - L/T/Y classes use correct size

**`planets_test.go`:**
- `TestGetPlanetColor()` - All known planet classes
- `TestUnknownPlanetClass()` - Returns default color
- `TestJovianHasRings()` - Ring flag set correctly

### Integration Tests

Create test system with:
- O-type hypergiant star
- G2V main sequence star
- M-type red dwarf
- L/T/Y brown dwarfs
- Various planet classes
- Invalid/malformed classes

Validate:
- No panics on malformed data
- Correct colors in SVG output
- Size variations present
- Tooltips contain expected text

### Visual Regression

Generate sample maps:
- System with O Ia hypergiant
- System with G2 V (Sun-like)
- System with M9 V red dwarf
- System with L5 brown dwarf
- System with jovian + terran planets

Manual review for visual correctness.

### Performance

Benchmark `RenderSystemMap()` before/after:
- Target: <10% overhead
- Measure: Memory allocation, execution time

## Migration and Compatibility

### Backward Compatibility
- `Class` field is optional (`omitempty`)
- Existing systems render unchanged
- No breaking changes to API

### Database Migration

```sql
ALTER TABLE pois ADD COLUMN class TEXT;
```

Update `pkg/knowledge/sqlite.go`:
- Add `Class` field to POI struct
- Update INSERT/SELECT statements

### Client Impact
- No WebSocket protocol changes
- Frontend can optionally display classifications
- System maps update on next regeneration

### Rollout Plan
1. Implement code changes
2. Run tests
3. Deploy to staging
4. Test with production data
5. Deploy to production
6. Systems gain classifications gradually as they're re-scanned

## Future Enhancements

### Phase 2 (Potential)
- Binary/multiple star systems
- Variable stars (pulsating, eclipsing)
- Stellar evolution indicators
- Protoplanetary disks
- Asteroid/belt composition classes

### Phase 3 (Potential)
- Spectral gradient mapping within classes (G0-G9)
- Luminosity fine-tuning (V vs V-zero)
- Metallicity effects on color
- Interactive tooltips with full star data

## Success Criteria

1. All spectral types (O-Y) render with correct colors
2. All luminosity classes (Ia-VII) render with correct sizes
3. All planet classes render with appropriate styling
4. Jovian planets display rings
5. Brown dwarfs appear distinct from main sequence stars
6. Malformed data doesn't crash rendering
7. Performance overhead <10%
8. Existing systems without classification render unchanged
9. Tooltips provide helpful classification information
10. Code is testable and maintainable

## References

- Harvard Spectral Classification (OBAFGKMLTY)
- Morgan-Keenan (MK) System
- Yerkes Luminosity Classes
- OpenAPI spec: `server_docs/openapi.20260309.json`
- Current rendering: `pkg/systemmap/systemmap.go`
