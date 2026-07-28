package shipglyph

import (
	"fmt"
	"html"
	"math"
	"strings"
)

// Options controls glyph rendering.
type Options struct {
	// Size is the square viewBox edge length in SVG user units.
	Size float64
	// ShowHardpoints emits the module mounting markers.
	ShowHardpoints bool
	// Title is the accessible title, normally the ship's display name.
	Title string
}

// glyphMargin is the fraction of Size left empty around the hull.
const glyphMargin = 0.08

// Render returns a self-contained inline SVG for the ship, drawn nose-up.
// Strokes use currentColor so the KB theme controls the ink. Element IDs are
// stable across regenerations so consumers can paint state onto them.
func Render(d Descriptor, s Stats, opts Options) string {
	if opts.Size <= 0 {
		opts.Size = 200
	}
	st := StyleFor(s.Faction)
	seed := SeedOf(s.ID)

	// The hull occupies the full length; beam is length divided by aspect.
	aspect := d.Aspect
	if aspect <= 0 {
		aspect = 3
	}
	usable := opts.Size * (1 - 2*glyphMargin)
	length := usable
	cx := opts.Size / 2
	top := opts.Size * glyphMargin

	outline := Outline(d, st, seed)

	// Aspect is length divided by maximum beam, so the widest point of the
	// hull must map to exactly half of length/aspect. The half-widths in a
	// Descriptor describe shape, not scale, and their maxima differ by
	// archetype; without this normalisation they would multiply with Aspect
	// and each archetype would render narrower than declared by its own
	// factor.
	var maxHalf float64
	for _, p := range outline {
		if a := math.Abs(p.Y); a > maxHalf {
			maxHalf = a
		}
	}
	scaleY := 1.0
	if maxHalf > 0 {
		scaleY = (length / (2 * aspect)) / maxHalf
	}

	// project maps glyph space to SVG user space, nose at the top.
	project := func(p Point) (float64, float64) {
		return cx + p.Y*scaleY, top + p.X*length
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="ship-glyph" viewBox="0 0 %g %g" xmlns="http://www.w3.org/2000/svg" role="img">`,
		opts.Size, opts.Size)
	if opts.Title != "" {
		fmt.Fprintf(&b, `<title>%s</title>`, html.EscapeString(opts.Title))
	}
	b.WriteString(`<g id="hull" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round">`)

	regions := Regions(d, st, seed)
	for _, name := range RegionNames {
		poly := regions[name]
		if len(poly) < 3 {
			continue
		}
		fmt.Fprintf(&b, `<path id="region-%s" class="glyph-region" d="%s"/>`,
			name, pathData(poly, project))
	}
	if len(outline) >= 3 {
		fmt.Fprintf(&b, `<path id="region-outline" class="glyph-outline" stroke-width="1.8" d="%s"/>`,
			pathData(outline, project))
	}
	b.WriteString(`</g>`)

	// Appendages sit outside the hull group so they can be styled separately
	// and are never mistaken for damage regions.
	if shapes := AppendageShapes(d); len(shapes) > 0 {
		b.WriteString(`<g id="appendages" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round">`)
		for _, sh := range shapes {
			fmt.Fprintf(&b, `<path id="%s" class="glyph-appendage glyph-ap-%s" d="%s"/>`,
				sh.ID, sh.Kind, pathData(sh.Poly, project))
		}
		b.WriteString(`</g>`)
	}

	if opts.ShowHardpoints {
		hps := Hardpoints(d, s)
		if len(hps) > 0 {
			b.WriteString(`<g id="hardpoints" fill="none" stroke="currentColor" stroke-width="1">`)
			for _, h := range hps {
				x, y := project(h.Pos)
				fmt.Fprintf(&b, `<circle id="%s" class="glyph-hp glyph-hp-%s" cx="%.2f" cy="%.2f" r="%.2f"/>`,
					h.ID, h.Kind, x, y, opts.Size*0.018)
			}
			b.WriteString(`</g>`)
		}
	}

	b.WriteString(`</svg>`)
	return b.String()
}

// pathData converts a closed polygon in glyph space to an SVG path.
func pathData(poly []Point, project func(Point) (float64, float64)) string {
	var b strings.Builder
	for i, p := range poly {
		x, y := project(p)
		verb := "L"
		if i == 0 {
			verb = "M"
		}
		fmt.Fprintf(&b, "%s%.2f %.2f", verb, x, y)
		if i < len(poly)-1 {
			b.WriteByte(' ')
		}
	}
	b.WriteString("Z")
	return b.String()
}
