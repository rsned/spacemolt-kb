package kbdb

import (
	"database/sql"
	"fmt"
	"hash/fnv"

	"github.com/rsned/spacemolt-kb/pkg/systemmap"
)

// StarMetadata holds the persisted metadata for a star/sun POI.
type StarMetadata struct {
	POIID           string
	POIName         string
	Seed            int64
	StarClass       string // Raw classification string (e.g., "G2 V")
	SpectralType    string // Parsed spectral letter/type (e.g., "G", "DA")
	SpectralSubtype int    // Parsed subtype 0-9, or -1 if unknown
	LuminosityClass string // Parsed luminosity Roman numeral (e.g., "V")
	LuminosityName  string // Human-readable name (e.g., "Main Sequence")
	ColorHex        string // Hex color for rendering (e.g., "#fff4a0")
	ColorName       string // Human-readable color name (e.g., "Yellow")
	TempRange       string // Temperature range string (e.g., "5,200–6,000 K")
	SizeMultiplier  float64
	RenderSize      float64
}

// FromStarClass creates a StarMetadata by parsing a star classification string.
func FromStarClass(poiID, poiName, starClass string) (*StarMetadata, error) {
	spectral, subtype, luminosity, err := systemmap.ParseStarClass(starClass)
	if err != nil {
		return nil, fmt.Errorf("parse star class %q: %w", starClass, err)
	}

	colorHex := systemmap.GetStarColorRefined(spectral, subtype)
	renderSize := systemmap.GetStarSize(luminosity)

	// Look up human-readable names from the systemmap package.
	colorName := systemmap.GetSpectralColorName(spectral)
	tempRange := systemmap.GetSpectralTempRange(spectral)
	lumName := systemmap.GetLuminosityName(luminosity)
	sizeMult := systemmap.GetLuminosityMultiplier(luminosity)

	h := fnv.New64a()
	h.Write([]byte(poiName))

	return &StarMetadata{
		POIID:           poiID,
		POIName:         poiName,
		Seed:            int64(h.Sum64()),
		StarClass:       starClass,
		SpectralType:    spectral,
		SpectralSubtype: subtype,
		LuminosityClass: luminosity,
		LuminosityName:  lumName,
		ColorHex:        colorHex,
		ColorName:       colorName,
		TempRange:       tempRange,
		SizeMultiplier:  sizeMult,
		RenderSize:      renderSize,
	}, nil
}

// UpsertStar inserts or replaces a star's metadata.
func UpsertStar(db *sql.DB, m StarMetadata) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO poi_metadata_stars
		(poi_id, poi_name, seed, star_class,
		 spectral_type, spectral_subtype, luminosity_class, luminosity_name,
		 color_hex, color_name, temp_range,
		 size_multiplier, render_size)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.POIID, m.POIName, m.Seed, m.StarClass,
		m.SpectralType, m.SpectralSubtype, m.LuminosityClass, m.LuminosityName,
		m.ColorHex, m.ColorName, m.TempRange,
		m.SizeMultiplier, m.RenderSize,
	)
	if err != nil {
		return fmt.Errorf("upsert star %s: %w", m.POIID, err)
	}
	return nil
}

// LoadStar loads a star's persisted metadata. Returns nil if not found.
func LoadStar(db *sql.DB, poiID string) (*StarMetadata, error) {
	var m StarMetadata
	err := db.QueryRow(`SELECT
		poi_id, poi_name, seed, star_class,
		spectral_type, spectral_subtype, luminosity_class, luminosity_name,
		color_hex, color_name, temp_range,
		size_multiplier, render_size
		FROM poi_metadata_stars WHERE poi_id = ?`, poiID).Scan(
		&m.POIID, &m.POIName, &m.Seed, &m.StarClass,
		&m.SpectralType, &m.SpectralSubtype, &m.LuminosityClass, &m.LuminosityName,
		&m.ColorHex, &m.ColorName, &m.TempRange,
		&m.SizeMultiplier, &m.RenderSize,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load star %s: %w", poiID, err)
	}
	return &m, nil
}

// LoadAllStars loads all persisted star metadata, keyed by POI ID.
func LoadAllStars(db *sql.DB) (map[string]*StarMetadata, error) {
	rows, err := db.Query(`SELECT
		poi_id, poi_name, seed, star_class,
		spectral_type, spectral_subtype, luminosity_class, luminosity_name,
		color_hex, color_name, temp_range,
		size_multiplier, render_size
		FROM poi_metadata_stars`)
	if err != nil {
		return nil, fmt.Errorf("load all stars: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]*StarMetadata)
	for rows.Next() {
		var m StarMetadata
		if err := rows.Scan(
			&m.POIID, &m.POIName, &m.Seed, &m.StarClass,
			&m.SpectralType, &m.SpectralSubtype, &m.LuminosityClass, &m.LuminosityName,
			&m.ColorHex, &m.ColorName, &m.TempRange,
			&m.SizeMultiplier, &m.RenderSize,
		); err != nil {
			return nil, fmt.Errorf("scan star row: %w", err)
		}
		result[m.POIID] = &m
	}
	return result, rows.Err()
}
