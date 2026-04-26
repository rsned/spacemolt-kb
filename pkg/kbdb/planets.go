package kbdb

import (
	"database/sql"
	"fmt"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/stats"
)

// PlanetMetadata holds the persisted metadata for a planet POI.
type PlanetMetadata struct {
	POIID             string
	POIName           string
	Seed              int64
	PlanetClass       string
	TypeName          string
	RadiusKm          float64
	MassEarths        float64
	GravityG          float64
	TemperatureK      float64
	TemperatureC      float64
	AtmPressureAtm    float64
	AtmDescription    string
	DayLengthHours    float64
	OrbitalPeriodDays float64
	OrbitalDistanceAU float64
}

// Stats converts persisted metadata back to a PlanetStats for template rendering.
func (m PlanetMetadata) Stats() stats.PlanetStats {
	return stats.PlanetStats{
		Type:              m.PlanetClass,
		TypeName:          m.TypeName,
		RadiusKm:          m.RadiusKm,
		MassEarths:        m.MassEarths,
		GravityG:          m.GravityG,
		TemperatureK:      m.TemperatureK,
		TemperatureC:      m.TemperatureC,
		AtmPressureAtm:    m.AtmPressureAtm,
		AtmDescription:    m.AtmDescription,
		DayLengthHours:    m.DayLengthHours,
		OrbitalPeriodDays: m.OrbitalPeriodDays,
		OrbitalDistanceAU: m.OrbitalDistanceAU,
	}
}

// FromPlanetStats creates a PlanetMetadata from a generated PlanetStats.
func FromPlanetStats(poiID, poiName string, ps stats.PlanetStats) PlanetMetadata {
	return PlanetMetadata{
		POIID:             poiID,
		POIName:           poiName,
		Seed:              planetgen.HashSeedPublic(poiName),
		PlanetClass:       ps.Type,
		TypeName:          ps.TypeName,
		RadiusKm:          ps.RadiusKm,
		MassEarths:        ps.MassEarths,
		GravityG:          ps.GravityG,
		TemperatureK:      ps.TemperatureK,
		TemperatureC:      ps.TemperatureC,
		AtmPressureAtm:    ps.AtmPressureAtm,
		AtmDescription:    ps.AtmDescription,
		DayLengthHours:    ps.DayLengthHours,
		OrbitalPeriodDays: ps.OrbitalPeriodDays,
		OrbitalDistanceAU: ps.OrbitalDistanceAU,
	}
}

// UpsertPlanet inserts or replaces a planet's metadata.
func UpsertPlanet(db *sql.DB, m PlanetMetadata) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO poi_metadata_planets
		(poi_id, poi_name, seed, planet_class, type_name,
		 radius_km, mass_earths, gravity_g, temperature_k, temperature_c,
		 atm_pressure_atm, atm_description, day_length_hours,
		 orbital_period_days, orbital_distance_au)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.POIID, m.POIName, m.Seed, m.PlanetClass, m.TypeName,
		m.RadiusKm, m.MassEarths, m.GravityG, m.TemperatureK, m.TemperatureC,
		m.AtmPressureAtm, m.AtmDescription, m.DayLengthHours,
		m.OrbitalPeriodDays, m.OrbitalDistanceAU,
	)
	if err != nil {
		return fmt.Errorf("upsert planet %s: %w", m.POIID, err)
	}
	return nil
}

// LoadPlanet loads a planet's persisted metadata. Returns nil if not found.
func LoadPlanet(db *sql.DB, poiID string) (*PlanetMetadata, error) {
	var m PlanetMetadata
	err := db.QueryRow(`SELECT
		poi_id, poi_name, seed, planet_class, type_name,
		radius_km, mass_earths, gravity_g, temperature_k, temperature_c,
		atm_pressure_atm, atm_description, day_length_hours,
		orbital_period_days, orbital_distance_au
		FROM poi_metadata_planets WHERE poi_id = ?`, poiID).Scan(
		&m.POIID, &m.POIName, &m.Seed, &m.PlanetClass, &m.TypeName,
		&m.RadiusKm, &m.MassEarths, &m.GravityG, &m.TemperatureK, &m.TemperatureC,
		&m.AtmPressureAtm, &m.AtmDescription, &m.DayLengthHours,
		&m.OrbitalPeriodDays, &m.OrbitalDistanceAU,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load planet %s: %w", poiID, err)
	}
	return &m, nil
}

// LoadAllPlanets loads all persisted planet metadata, keyed by POI ID.
func LoadAllPlanets(db *sql.DB) (map[string]*PlanetMetadata, error) {
	rows, err := db.Query(`SELECT
		poi_id, poi_name, seed, planet_class, type_name,
		radius_km, mass_earths, gravity_g, temperature_k, temperature_c,
		atm_pressure_atm, atm_description, day_length_hours,
		orbital_period_days, orbital_distance_au
		FROM poi_metadata_planets`)
	if err != nil {
		return nil, fmt.Errorf("load all planets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]*PlanetMetadata)
	for rows.Next() {
		var m PlanetMetadata
		if err := rows.Scan(
			&m.POIID, &m.POIName, &m.Seed, &m.PlanetClass, &m.TypeName,
			&m.RadiusKm, &m.MassEarths, &m.GravityG, &m.TemperatureK, &m.TemperatureC,
			&m.AtmPressureAtm, &m.AtmDescription, &m.DayLengthHours,
			&m.OrbitalPeriodDays, &m.OrbitalDistanceAU,
		); err != nil {
			return nil, fmt.Errorf("scan planet row: %w", err)
		}
		result[m.POIID] = &m
	}
	return result, rows.Err()
}
