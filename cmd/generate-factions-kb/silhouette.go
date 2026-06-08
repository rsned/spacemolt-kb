package main

import (
	"fmt"
	"hash/fnv"
	htmltpl "html/template"
	"strings"
)

// seedHash is a stable 64-bit FNV-1a hash of s, used to derive deterministic
// visual variation from an entity ID.
func seedHash(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// derivePalette returns (field, accent) CSS colors derived deterministically
// from seed. Used when a profile has no color data of its own. Both are
// hsl() strings, valid as SVG fills.
func derivePalette(seed string) (field, accent string) {
	h := seedHash(seed)
	hue := h % 360
	field = fmt.Sprintf("hsl(%d 45%% 38%%)", hue)
	accent = fmt.Sprintf("hsl(%d 60%% 58%%)", (hue+150)%360)
	return field, accent
}

// silhouetteFill is the dark crew-silhouette color (head + shoulders).
const silhouetteFill = "hsl(222 24% 10%)"

// silhouetteVariants is the number of distinct visor styles.
const silhouetteVariants = 5

// silhouetteSVG returns a self-contained inline <svg> for a stylized sci-fi
// crew silhouette, deterministic from seed and tinted by primary/secondary.
// Empty primary/secondary fall back to a palette derived from seed. The output
// is trusted HTML: it is built entirely from literals and pre-validated colors.
func silhouetteSVG(seed, primary, secondary string) htmltpl.HTML {
	field, accent := primary, secondary
	if field == "" || accent == "" {
		df, da := derivePalette(seed)
		if field == "" {
			field = df
		}
		if accent == "" {
			accent = da
		}
	}
	h := seedHash(seed)
	variant := h % silhouetteVariants
	hasBadge := (h>>8)&1 == 1

	var b strings.Builder
	b.WriteString(`<svg class="silhouette" viewBox="0 0 100 120" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="generated crew silhouette">`)
	b.WriteString(`<rect width="100" height="120" fill="` + field + `"/>`)
	b.WriteString(`<rect width="100" height="120" fill="hsl(0 0% 0%)" opacity="0.12"/>`)
	b.WriteString(`<path d="M12,120 C12,90 30,80 50,80 C70,80 88,90 88,120 Z" fill="` + silhouetteFill + `"/>`)
	b.WriteString(`<circle cx="50" cy="50" r="26" fill="` + silhouetteFill + `"/>`)
	b.WriteString(visorMarkup(variant, accent))
	if hasBadge {
		b.WriteString(`<circle cx="68" cy="98" r="4" fill="` + accent + `"/>`)
	}
	b.WriteString(`</svg>`)
	return htmltpl.HTML(b.String())
}

// visorMarkup returns the variant-specific visor element(s), filled with accent.
func visorMarkup(variant uint64, accent string) string {
	switch variant {
	case 0: // horizontal slit
		return `<rect x="34" y="46" width="32" height="8" rx="4" fill="` + accent + `" opacity="0.9"/>`
	case 1: // T-visor
		return `<path d="M36,42 H64 V48 H54 V60 H46 V48 H36 Z" fill="` + accent + `" opacity="0.9"/>`
	case 2: // full curved visor
		return `<path d="M30,48 a20,14 0 0 1 40,0 a20,8 0 0 1 -40,0 Z" fill="` + accent + `" opacity="0.85"/>`
	case 3: // round single eye
		return `<circle cx="50" cy="50" r="9" fill="` + accent + `" opacity="0.9"/>`
	default: // angled visor
		return `<path d="M32,44 L68,52 L66,58 L34,52 Z" fill="` + accent + `" opacity="0.9"/>`
	}
}
