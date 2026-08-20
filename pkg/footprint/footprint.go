// Package footprint reads the KB pipeline's top-down ship footprint SVGs and
// checks them against the asset contract the battle holotable draws against.
//
// The contract, from the holotable design doc: one closed path with
// fill-rule="evenodd", bow pointing at +X, hull length normalised to 1000
// units inside a 1020-wide viewBox with a 10-unit margin, and the filename
// equal to data-ship equal to the ships-catalog id.
package footprint

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// Footprint is one parsed hy3d SVG.
type Footprint struct {
	// Ship is data-ship, which is also the catalog id and the battle log's
	// ship_class. It is the only join key the battle log provides.
	Ship string
	// D is the single path's geometry, drawn as-is by the renderer.
	D string
	// ArtStem is the original art asset name, kept for tracing a bad hull
	// back to the source image.
	ArtStem string
	// KBMatch records how the art was joined to the catalog: verbatim,
	// stripped, fuzzy, or none. Suspect "fuzzy" first if a hull looks wrong.
	KBMatch string

	Width  float64
	Height float64
	// Aspect is hull length over hull width, margins excluded.
	Aspect float64
	// FrameAmbiguous is the pipeline saying it is unsure of the hull's frame.
	FrameAmbiguous bool

	// pathCount is how many <path> elements the file carried. The contract is
	// exactly one; Check reports anything else.
	pathCount int
	// hasAspect is whether data-aspect was present at all. Aspect alone
	// can't distinguish "absent" from "present and zero", so Check needs
	// this to report a missing attribute instead of silently skipping the
	// tolerance comparison.
	hasAspect bool
}

// svgRoot mirrors only the attributes the contract cares about.
type svgRoot struct {
	ViewBox        string `xml:"viewBox,attr"`
	Ship           string `xml:"data-ship,attr"`
	ArtStem        string `xml:"data-art-stem,attr"`
	KBMatch        string `xml:"data-kb-match,attr"`
	Aspect         string `xml:"data-aspect,attr"`
	FrameAmbiguous string `xml:"data-frame-ambiguous,attr"`
	Paths          []struct {
		D        string `xml:"d,attr"`
		FillRule string `xml:"fill-rule,attr"`
	} `xml:"path"`
}

// Parse reads one footprint SVG. It does not validate the contract; use Check
// for that, so a caller can render a slightly off asset while still reporting
// it.
func Parse(data []byte) (Footprint, error) {
	var root svgRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return Footprint{}, fmt.Errorf("parse svg: %w", err)
	}

	f := Footprint{
		Ship:           root.Ship,
		ArtStem:        root.ArtStem,
		KBMatch:        root.KBMatch,
		FrameAmbiguous: root.FrameAmbiguous == "true",
	}

	fields := strings.Fields(root.ViewBox)
	if len(fields) != 4 {
		return Footprint{}, fmt.Errorf("viewBox %q: want 4 numbers, got %d", root.ViewBox, len(fields))
	}
	var err error
	if f.Width, err = strconv.ParseFloat(fields[2], 64); err != nil {
		return Footprint{}, fmt.Errorf("viewBox width %q: %w", fields[2], err)
	}
	if f.Height, err = strconv.ParseFloat(fields[3], 64); err != nil {
		return Footprint{}, fmt.Errorf("viewBox height %q: %w", fields[3], err)
	}
	if root.Aspect != "" {
		f.hasAspect = true
		if f.Aspect, err = strconv.ParseFloat(root.Aspect, 64); err != nil {
			return Footprint{}, fmt.Errorf("data-aspect %q: %w", root.Aspect, err)
		}
	}

	// Collect every path so Check can complain about a second one; the
	// renderer only ever draws the first.
	for _, p := range root.Paths {
		if f.D == "" {
			f.D = p.D
		}
	}
	f.pathCount = len(root.Paths)

	return f, nil
}
