package footprint

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1020 628"
  data-ship="dirk" data-art-stem="crimson_dirk" data-kb-match="stripped"
  data-aspect="1.6447" data-frame-ambiguous="false">
<title>dirk</title>
<path d="M10 10L1010 10L1010 618L10 618Z" fill-rule="evenodd"/>
</svg>`

func TestParseReadsTheRootAttributes(t *testing.T) {
	f, err := Parse([]byte(goodSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Ship != "dirk" {
		t.Errorf("Ship = %q, want dirk", f.Ship)
	}
	if f.ArtStem != "crimson_dirk" {
		t.Errorf("ArtStem = %q, want crimson_dirk", f.ArtStem)
	}
	if f.KBMatch != "stripped" {
		t.Errorf("KBMatch = %q, want stripped", f.KBMatch)
	}
	if f.Width != 1020 {
		t.Errorf("Width = %v, want 1020", f.Width)
	}
	if f.Height != 628 {
		t.Errorf("Height = %v, want 628", f.Height)
	}
	if math.Abs(f.Aspect-1.6447) > 1e-6 {
		t.Errorf("Aspect = %v, want 1.6447", f.Aspect)
	}
	if f.FrameAmbiguous {
		t.Error("FrameAmbiguous = true, want false")
	}
	if f.D == "" {
		t.Error("D is empty; the path data must be captured")
	}
}

func TestCheckAcceptsAContractCompliantFile(t *testing.T) {
	f, err := Parse([]byte(goodSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if problems := Check(f, "dirk.svg"); len(problems) != 0 {
		t.Errorf("Check reported %v, want none", problems)
	}
}

func TestCheckRejectsContractViolations(t *testing.T) {
	tests := []struct {
		name     string
		svg      string
		filename string
		want     string // substring the report must mention
	}{
		{
			name:     "filename disagrees with data-ship",
			svg:      goodSVG,
			filename: "stiletto.svg",
			want:     "data-ship",
		},
		{
			name: "viewBox is not 1020 wide",
			svg: `<svg viewBox="0 0 999 628" data-ship="dirk" data-aspect="1.6447">` +
				`<path d="M0 0Z"/></svg>`,
			filename: "dirk.svg",
			want:     "width",
		},
		{
			name: "two paths instead of one",
			svg: `<svg viewBox="0 0 1020 628" data-ship="dirk" data-aspect="1.6447">` +
				`<path d="M0 0Z"/><path d="M1 1Z"/></svg>`,
			filename: "dirk.svg",
			want:     "path",
		},
		{
			name: "data-aspect disagrees with the viewBox height",
			svg: `<svg viewBox="0 0 1020 628" data-ship="dirk" data-aspect="9.9">` +
				`<path d="M0 0Z"/></svg>`,
			filename: "dirk.svg",
			want:     "aspect",
		},
		{
			name: "data-aspect attribute missing entirely",
			svg: `<svg viewBox="0 0 1020 628" data-ship="dirk">` +
				`<path d="M0 0Z"/></svg>`,
			filename: "dirk.svg",
			want:     "aspect",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse([]byte(tt.svg))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			problems := Check(f, tt.filename)
			if len(problems) == 0 {
				t.Fatalf("Check reported no problems, want one mentioning %q", tt.want)
			}
			joined := strings.ToLower(strings.Join(problems, "; "))
			if !strings.Contains(joined, tt.want) {
				t.Errorf("Check reported %v, want a problem mentioning %q", problems, tt.want)
			}
		})
	}
}

// footprintDir is the KB pipeline's output the holotable draws against.
const footprintDir = "../../data/footprints/hy3d-svg"

func TestShippedAssetsSatisfyTheContract(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(footprintDir, "*.svg"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) < 300 {
		t.Fatalf("found %d svg files in %s; the asset set is missing or the path moved",
			len(files), footprintDir)
	}

	var bad int
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: read: %v", filepath.Base(path), err)
			bad++
			continue
		}
		f, err := Parse(data)
		if err != nil {
			t.Errorf("%s: parse: %v", filepath.Base(path), err)
			bad++
			continue
		}
		for _, p := range Check(f, path) {
			t.Errorf("%s: %s", filepath.Base(path), p)
			bad++
		}
	}
	t.Logf("checked %d footprints, %d problems", len(files), bad)
}
