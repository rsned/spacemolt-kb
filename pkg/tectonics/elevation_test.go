package tectonics

import (
	"math"
	"testing"
)

func TestElevationContinentalMonotone(t *testing.T) {
	p := testParams(t, 16)
	prev := math.Inf(-1)
	for k := 0; k <= 100; k++ {
		th := p.ContinentalThreshold + (1-p.ContinentalThreshold)*float64(k)/100
		h := elevationAt(p, th, 0)
		if h <= prev {
			t.Fatalf("th %.3f: h %.4f not above %.4f", th, h, prev)
		}
		prev = h
	}
	if top := elevationAt(p, 1, 0); math.Abs(top-p.ElevPeakKm) > 1e-9 {
		t.Fatalf("thickness 1 gives %.4f km, want ElevPeakKm %g", top, p.ElevPeakKm)
	}
}

func TestElevationOceanSinksWithAge(t *testing.T) {
	p := testParams(t, 16)
	prev := math.Inf(1)
	for steps := uint16(0); steps <= 400; steps += 10 {
		th := thExpected(p, float64(steps)) // zero deviation
		h := elevationAt(p, th, steps)
		if h > prev {
			t.Fatalf("age %d steps: h %.4f rose above %.4f", steps, h, prev)
		}
		if h < p.AbyssKm-1e-9 {
			t.Fatalf("age %d steps: h %.4f below AbyssKm %g", steps, h, p.AbyssKm)
		}
		prev = h
	}
	if h0 := elevationAt(p, thExpected(p, 0), 0); math.Abs(h0-p.RidgeDepthKm) > 1e-9 {
		t.Fatalf("age 0 ridge sits at %.4f, want %g", h0, p.RidgeDepthKm)
	}
}

func TestElevationBranchesMeet(t *testing.T) {
	p := testParams(t, 16)
	T := p.ContinentalThreshold
	capSteps := uint16(p.OceanAgeCapMyr / p.MyrPerStep())
	for _, age := range []uint16{0, capSteps} {
		lo, hi := elevationAt(p, T-1e-3, age), elevationAt(p, T+1e-3, age)
		if math.Abs(hi-lo) > 0.3 {
			t.Fatalf("age %d: jump %.3f km across the threshold (%.3f vs %.3f)", age, hi-lo, lo, hi)
		}
	}
}

func TestElevationTrenchBelowAgeDepth(t *testing.T) {
	p := testParams(t, 16)
	const age = 50
	expected := thExpected(p, age)
	if elevationAt(p, expected-p.TrenchDepth, age) >= elevationAt(p, expected, age) {
		t.Fatal("a trench-thinned pixel must sit below the age-predicted depth")
	}
}

func TestInitialAgesRoundTrip(t *testing.T) {
	p := testParams(t, 8)
	th := NewGrid[float64](8)
	want := []uint16{0, 10, 50}
	for i := range th.Cells {
		th.Cells[i] = thExpected(p, float64(want[i%3]))
	}
	th.Cells[0] = 0.9 // continent
	th.Cells[1] = 0.1 // thinner than ridge crust
	th.Cells[2] = p.OceanBandMax // thicker than the cap explains
	age := InitialAges(th, p)
	for i := 3; i < th.Len(); i++ {
		if age.Cells[i] != want[i%3] {
			t.Fatalf("pixel %d: age %d, want %d", i, age.Cells[i], want[i%3])
		}
	}
	if cont := uint16(p.ContinentAgeMyr / p.MyrPerStep()); age.Cells[0] != cont {
		t.Fatalf("continent age %d, want %d", age.Cells[0], cont)
	}
	if age.Cells[1] != 0 {
		t.Fatalf("sub-ridge thickness age %d, want 0", age.Cells[1])
	}
	if capSteps := uint16(p.OceanAgeCapMyr / p.MyrPerStep()); age.Cells[2] != capSteps {
		t.Fatalf("thick ocean age %d, want cap %d", age.Cells[2], capSteps)
	}
}

func TestAreaWeightsSumToSphere(t *testing.T) {
	for _, S := range []int{8, 32} {
		sum := 0.0
		for _, w := range AreaWeights(S) {
			if w <= 0 {
				t.Fatalf("S=%d: non-positive weight %g", S, w)
			}
			sum += w
		}
		if math.Abs(sum-4*math.Pi) > 1e-9 {
			t.Fatalf("S=%d: weights sum to %.12f, want 4π", S, sum)
		}
	}
}
