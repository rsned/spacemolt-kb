package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"hash"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/rsned/spacemolt-kb/pkg/tectonics"
)

var update = flag.Bool("update", false, "rewrite testdata/golden_face64.json")

type golden struct {
	Recipe string   `json:"recipe"`
	Hashes []string `json:"hashes"`
}

// putF64 writes v's bits to h as 8 little-endian bytes, shared by the grid
// and plate-row hashing below.
func putF64(h hash.Hash, v float64) {
	var b [8]byte
	bits := math.Float64bits(v)
	for k := range 8 {
		b[k] = byte(bits >> (8 * k))
	}
	_, _ = h.Write(b[:])
}

func frameHash(f tectonics.Frame) string {
	h := fnv.New64a()
	for _, v := range f.Th.Cells {
		putF64(h, v)
	}
	for _, v := range f.Labels.Cells {
		_, _ = h.Write([]byte{byte(v), byte(v >> 8)})
	}
	_, _ = h.Write(f.Feature.Cells)
	for _, pl := range f.Plates {
		var idb [4]byte
		id := uint32(pl.ID)
		for k := range 4 {
			idb[k] = byte(id >> (8 * k))
		}
		_, _ = h.Write(idb[:])
		for _, c := range pl.Centroid {
			putF64(h, c)
		}
		for _, c := range pl.Pole {
			putF64(h, c)
		}
		putF64(h, pl.SpeedCmYr)
		putF64(h, pl.Area)
		var major, retired byte
		if pl.Major {
			major = 1
		}
		if pl.Retired {
			retired = 1
		}
		_, _ = h.Write([]byte{major, retired})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func goldenRun(t *testing.T) []string {
	t.Helper()
	p, err := tectonics.DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	p.Face, p.Steps, p.KeyframeEvery = 64, 20, 1
	var hashes []string
	if err := tectonics.Run(p, 2026, func(f tectonics.Frame) error {
		hashes = append(hashes, frameHash(f))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return hashes
}

func TestGoldenFace64(t *testing.T) {
	// The hashes cover raw float64 bits. The Go compiler fuses a*b+c into a
	// single FMA instruction on arm64 (and some other architectures) but not
	// on amd64, which changes the low bits, so the golden is only
	// bit-exact on the architecture it was baked on.
	if runtime.GOARCH != "amd64" {
		t.Skipf("golden hashes are baked on amd64; GOARCH %s may differ in float64 bits (fused multiply-add)", runtime.GOARCH)
	}
	path := filepath.Join("testdata", "golden_face64.json")
	got := golden{Recipe: "terran/2026/64/20/v3", Hashes: goldenRun(t)}
	if *update {
		b, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("golden rewritten: %d frames", len(got.Hashes))
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create)", err)
	}
	var want golden
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if want.Recipe != got.Recipe || len(want.Hashes) != len(got.Hashes) {
		t.Fatalf("golden recipe/length mismatch: %s/%d vs %s/%d", want.Recipe, len(want.Hashes), got.Recipe, len(got.Hashes))
	}
	for i := range want.Hashes {
		if want.Hashes[i] != got.Hashes[i] {
			t.Errorf("frame %d hash %s, golden %s", i, got.Hashes[i], want.Hashes[i])
		}
	}
}

func TestTimingBudgetFace128(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test skipped in -short")
	}
	p, err := tectonics.DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	p.Face, p.Steps, p.KeyframeEvery = 128, 10, 10
	start := time.Now()
	if err := tectonics.Run(p, 1, func(tectonics.Frame) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("face 128 × 10 steps took %s, budget 10s", d)
	} else {
		t.Logf("face 128 × 10 steps: %s", d)
	}
}
