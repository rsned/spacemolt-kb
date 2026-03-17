package planetgen

import (
	"fmt"
	"math"
	"math/rand/v2"
)

// PlanetStats holds the physical properties of a planet.
type PlanetStats struct {
	Type             string
	TypeName         string
	RadiusKm         float64 // Equatorial radius in km
	MassEarths       float64 // Mass in Earth masses
	GravityG         float64 // Surface gravity in g
	TemperatureK     float64 // Mean surface temperature in Kelvin
	TemperatureC     float64 // Mean surface temperature in Celsius
	AtmPressureAtm   float64 // Atmospheric pressure in Earth atmospheres
	AtmDescription   string  // e.g. "Thin CO2", "Dense N2/O2"
	DayLengthHours   float64 // Sidereal day length in hours
	OrbitalPeriodDays float64 // Orbital period in Earth days
	OrbitalDistanceAU float64 // Distance from star in AU
}

// planetBaseline holds the reference physical values for a planet type.
type planetBaseline struct {
	TypeName       string
	RadiusKm       float64
	MassEarths     float64
	GravityG       float64
	TemperatureK   float64
	AtmPressureAtm float64
	AtmDescription string
	DayLengthMin   float64 // min day length in hours
	DayLengthMax   float64 // max day length in hours
}

var baselines = map[string]planetBaseline{
	"scorched": {
		TypeName: "Scorched", RadiusKm: 2440, MassEarths: 0.055, GravityG: 0.38,
		TemperatureK: 440, AtmPressureAtm: 0, AtmDescription: "None",
		DayLengthMin: 200, DayLengthMax: 1400,
	},
	"arid": {
		TypeName: "Arid", RadiusKm: 3390, MassEarths: 0.107, GravityG: 0.38,
		TemperatureK: 210, AtmPressureAtm: 0.006, AtmDescription: "Thin CO2",
		DayLengthMin: 20, DayLengthMax: 30,
	},
	"terran": {
		TypeName: "Terran", RadiusKm: 6371, MassEarths: 1.0, GravityG: 1.0,
		TemperatureK: 288, AtmPressureAtm: 1.0, AtmDescription: "N2/O2",
		DayLengthMin: 18, DayLengthMax: 30,
	},
	"tundra": {
		TypeName: "Tundra", RadiusKm: 5740, MassEarths: 0.85, GravityG: 0.9,
		TemperatureK: 220, AtmPressureAtm: 0.7, AtmDescription: "Thin N2/CO2",
		DayLengthMin: 16, DayLengthMax: 40,
	},
	"glacial": {
		TypeName: "Glacial", RadiusKm: 6371, MassEarths: 1.0, GravityG: 0.95,
		TemperatureK: 170, AtmPressureAtm: 0.8, AtmDescription: "N2/Ar",
		DayLengthMin: 18, DayLengthMax: 35,
	},
	"ice_world": {
		TypeName: "Ice World", RadiusKm: 7000, MassEarths: 1.2, GravityG: 0.85,
		TemperatureK: 100, AtmPressureAtm: 0.01, AtmDescription: "Trace O2",
		DayLengthMin: 20, DayLengthMax: 80,
	},
	"super_terran": {
		TypeName: "Super Terran", RadiusKm: 7960, MassEarths: 2.5, GravityG: 1.25,
		TemperatureK: 280, AtmPressureAtm: 1.5, AtmDescription: "Dense N2/O2",
		DayLengthMin: 14, DayLengthMax: 28,
	},
	"hothouse": {
		TypeName: "Hothouse", RadiusKm: 6052, MassEarths: 0.815, GravityG: 0.9,
		TemperatureK: 737, AtmPressureAtm: 92, AtmDescription: "Supercritical CO2",
		DayLengthMin: 1000, DayLengthMax: 5800,
	},
	"lava_world": {
		TypeName: "Lava World", RadiusKm: 5100, MassEarths: 0.7, GravityG: 0.85,
		TemperatureK: 1200, AtmPressureAtm: 0.001, AtmDescription: "Volcanic SO2",
		DayLengthMin: 10, DayLengthMax: 60,
	},
	"oceanic": {
		TypeName: "Oceanic", RadiusKm: 6371, MassEarths: 1.0, GravityG: 0.95,
		TemperatureK: 295, AtmPressureAtm: 1.1, AtmDescription: "N2/O2 (humid)",
		DayLengthMin: 16, DayLengthMax: 32,
	},
	"jovian": {
		TypeName: "Jovian", RadiusKm: 58232, MassEarths: 95, GravityG: 1.06,
		TemperatureK: 134, AtmPressureAtm: 0, AtmDescription: "H2/He (no surface)",
		DayLengthMin: 10, DayLengthMax: 16,
	},
	"ice_giant": {
		TypeName: "Ice Giant", RadiusKm: 24622, MassEarths: 17, GravityG: 1.14,
		TemperatureK: 72, AtmPressureAtm: 0, AtmDescription: "H2/He/CH4 (no surface)",
		DayLengthMin: 14, DayLengthMax: 19,
	},
	"unknown": {
		TypeName: "Unclassified", RadiusKm: 5500, MassEarths: 0.8, GravityG: 0.85,
		TemperatureK: 250, AtmPressureAtm: 0.5, AtmDescription: "Unknown composition",
		DayLengthMin: 15, DayLengthMax: 60,
	},
}

// GenerateStats produces physical stats for a planet, seeded by name.
// orbitalDistanceAU is the planet's distance from the star (from POI position).
func GenerateStats(planetType, planetName string, orbitalDistanceAU float64) PlanetStats {
	baseline, ok := baselines[planetType]
	if !ok {
		baseline = baselines["unknown"]
	}

	seed := hashSeed(planetName)
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed*37+13)))

	// Vary each stat ±20% around baseline
	vary := func(base float64) float64 {
		return base * (0.8 + rng.Float64()*0.4)
	}

	tempK := vary(baseline.TemperatureK)
	dayLen := baseline.DayLengthMin + rng.Float64()*(baseline.DayLengthMax-baseline.DayLengthMin)

	// Orbital period from Kepler's third law: P² = a³ (P in years, a in AU)
	// P_days = 365.25 * a^1.5
	orbitalPeriod := 365.25 * math.Pow(math.Max(0.1, orbitalDistanceAU), 1.5)

	return PlanetStats{
		Type:              planetType,
		TypeName:          baseline.TypeName,
		RadiusKm:          math.Round(vary(baseline.RadiusKm)),
		MassEarths:        roundTo(vary(baseline.MassEarths), 3),
		GravityG:          roundTo(vary(baseline.GravityG), 2),
		TemperatureK:      math.Round(tempK),
		TemperatureC:      math.Round(tempK - 273.15),
		AtmPressureAtm:    roundTo(vary(baseline.AtmPressureAtm), 3),
		AtmDescription:    baseline.AtmDescription,
		DayLengthHours:    roundTo(dayLen, 1),
		OrbitalPeriodDays: math.Round(orbitalPeriod),
		OrbitalDistanceAU: roundTo(orbitalDistanceAU, 2),
	}
}

// FormatRadius returns a human-readable radius string.
func (s PlanetStats) FormatRadius() string {
	if s.RadiusKm >= 10000 {
		return fmt.Sprintf("%.1f km", s.RadiusKm)
	}
	return fmt.Sprintf("%.0f km", s.RadiusKm)
}

// FormatMass returns a human-readable mass string.
func (s PlanetStats) FormatMass() string {
	if s.MassEarths >= 10 {
		return fmt.Sprintf("%.1f M⊕", s.MassEarths)
	}
	if s.MassEarths >= 0.1 {
		return fmt.Sprintf("%.2f M⊕", s.MassEarths)
	}
	return fmt.Sprintf("%.3f M⊕", s.MassEarths)
}

// FormatGravity returns a human-readable gravity string.
func (s PlanetStats) FormatGravity() string {
	return fmt.Sprintf("%.2f g", s.GravityG)
}

// FormatTemperature returns temperature in both K and C.
func (s PlanetStats) FormatTemperature() string {
	return fmt.Sprintf("%.0f K (%.0f°C)", s.TemperatureK, s.TemperatureC)
}

// FormatPressure returns a human-readable pressure string.
func (s PlanetStats) FormatPressure() string {
	if s.AtmPressureAtm == 0 {
		return "None"
	}
	if s.AtmPressureAtm < 0.01 {
		return fmt.Sprintf("%.4f atm", s.AtmPressureAtm)
	}
	if s.AtmPressureAtm < 1 {
		return fmt.Sprintf("%.3f atm", s.AtmPressureAtm)
	}
	return fmt.Sprintf("%.1f atm", s.AtmPressureAtm)
}

// FormatDayLength returns a human-readable day length.
func (s PlanetStats) FormatDayLength() string {
	if s.DayLengthHours >= 48 {
		days := s.DayLengthHours / 24
		return fmt.Sprintf("%.1f days (%.0f hrs)", days, s.DayLengthHours)
	}
	return fmt.Sprintf("%.1f hours", s.DayLengthHours)
}

// FormatOrbitalPeriod returns a human-readable orbital period.
func (s PlanetStats) FormatOrbitalPeriod() string {
	if s.OrbitalPeriodDays >= 365 {
		years := s.OrbitalPeriodDays / 365.25
		return fmt.Sprintf("%.1f years", years)
	}
	return fmt.Sprintf("%.0f days", s.OrbitalPeriodDays)
}

func roundTo(v float64, decimals int) float64 {
	pow := math.Pow(10, float64(decimals))
	return math.Round(v*pow) / pow
}
