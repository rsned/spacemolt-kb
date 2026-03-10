# MK Classification Test System Map Generator

Generates HTML test files demonstrating the MK (Morgan-Keenan) star classification rendering system.

## Usage

```bash
cd cmd/test-system-map
go run .
```

This generates HTML files in the current directory showing:

### Spectral Type Samples (10 files)
- `test-spectral-O.html` - O-type blue hypergiant (30,000+ K)
- `test-spectral-B.html` - B-type blue-white giant
- `test-spectral-A.html` - A-type white main sequence
- `test-spectral-F.html` - F-type yellow-white subgiant
- `test-spectral-G.html` - G-type yellow main sequence (Sun-like)
- `test-spectral-K.html` - K-type orange main sequence
- `test-spectral-M.html` - M-type red dwarf
- `test-spectral-L.html` - L-type brown dwarf
- `test-spectral-T.html` - T-type brown dwarf
- `test-spectral-Y.html` - Y-type brown dwarf

### Luminosity Class Samples (8 files)
- `test-lum-Ia.html` - Hypergiant (largest: 28px)
- `test-lum-Ib.html` - Supergiant (24px)
- `test-lum-II.html` - Bright giant (20px)
- `test-lum-III.html` - Giant (16px)
- `test-lum-IV.html` - Subgiant (14px)
- `test-lum-V.html` - Main sequence (10px baseline)
- `test-lum-VI.html` - Subdwarf (8px)
- `test-lum-VII.html` - White dwarf (6px, smallest)

### Planet Class Samples (8 files)
- `test-planet-terran.html` - Earth-like (blue-green)
- `test-planet-jovian.html` - Gas giant with rings (orange)
- `test-planet-ice_giant.html` - Ice giant (pale blue)
- `test-planet-gas_dwarf.html` - Mini-Neptune (tan)
- `test-planet-rocky.html` - Mercury/Mars-like (brown-gray)
- `test-planet-dwarf.html` - Pluto-like (tan-brown)
- `test-planet-proto.html` - Protoplanet (reddish)
- `test-planet-captured.html` - Rogue planet (gray-purple)

### Edge Case Samples (6 files)
- `test-edge-unclassified-star.html` - No classification data
- `test-edge-invalid-class.html` - Invalid classification format
- `test-edge-compact-format.html` - Compact format "G2V" (no space)
- `test-edge-supergiant-compact.html` - Compact format "B0Ib"
- `test-edge-missing-luminosity.html` - Spectral only "M9" (defaults to V)
- `test-edge-brown-dwarf-giant.html` - L5 III (brown dwarf with giant class)

## Visual Guide

### Star Colors by Spectral Type
| Type | Color | Temp Range | Example |
|------|-------|------------|---------|
| O | #a0c8ff (Blue) | >30,000 K | Zeta Puppis |
| B | #c8d8ff (Blue-White) | 10,000–30,000 K | Rigel |
| A | #e8eeff (White) | 7,500–10,000 K | Sirius |
| F | #fffff0 (Yellow-White) | 6,000–7,500 K | Procyon |
| G | #fff4a0 (Yellow) | 5,200–6,000 K | **The Sun** |
| K | #ffcf6e (Orange) | 3,700–5,200 K | Arcturus |
| M | #ff8060 (Red) | 2,400–3,700 K | Proxima Centauri |
| L | #cc4020 (Dark Red) | 1,300–2,400 K | Teide 1 |
| T | #882010 (Infrared) | 700–1,300 K | Gliese 229B |
| Y | #440a05 (Near-IR) | <700 K | WISE 1828+2650 |

### Star Sizes by Luminosity Class
| Class | Name | Size (px) | Multiplier | Example |
|-------|------|-----------|------------|---------|
| Ia | Hypergiant | 28 | 2.8× | Eta Carinae |
| Ib | Supergiant | 24 | 2.4× | Betelgeuse |
| II | Bright Giant | 20 | 2.0× | Canopus |
| III | Giant | 16 | 1.6× | Arcturus |
| IV | Subgiant | 14 | 1.4× | Procyon |
| **V** | **Main Sequence** | **10** | **1.0×** | **The Sun** |
| VI | Subdwarf | 8 | 0.8× | Mu Cassiopeiae |
| VII | White Dwarf | 6 | 0.6× | Sirius B |

### Planet Colors by Class
| Class | Color | Has Rings | Example |
|-------|-------|-----------|---------|
| terran | #4a9c6d (Blue-green) | No | Earth |
| jovian | #e8a86c (Orange) | **Yes** | Jupiter, Saturn |
| ice_giant | #9fc5e8 (Pale blue) | No | Neptune, Uranus |
| gas_dwarf | #d4b596 (Tan) | No | Mini-Neptune |
| rocky | #8b7355 (Brown-gray) | No | Mercury, Mars |
| dwarf | #c4a484 (Tan-brown) | No | Pluto |
| proto | #cc6666 (Reddish) | No | Forming planet |
| captured | #b0a0c0 (Gray-purple) | No | Rogue planet |

## Special Rendering Features

### Brown Dwarfs (L/T/Y)
- Smaller size (8px) regardless of luminosity class
- Muted glow (15% opacity vs 45% for normal stars)
- Indicates substellar nature (no hydrogen fusion)

### Jovian Planets
- Three tilted elliptical rings
- Color gradient from outer to inner: #d4a86c → #e8b87c → #f0c88c
- 30° rotation for realistic appearance

### Graceful Degradation
- Unclassified POIs render with default styling
- Invalid classifications use default colors/sizes
- No errors or broken rendering

## File Format

Each generated HTML file contains:
1. **Header** with system name and classification category
2. **POI Data** section listing all objects with their classes
3. **Rendered Map** showing the actual SVG output
4. Responsive dark theme styling

Open any HTML file in a browser to see the rendered system map with its classified POIs.
