-- KB metadata schema. Source of truth for poi_metadata_* tables.
-- Embedded by pkg/kbdb and applied via kbdb.Migrate. All statements must
-- be idempotent (IF NOT EXISTS) so this file can be re-applied safely.

CREATE TABLE IF NOT EXISTS poi_metadata_planets (
    poi_id              TEXT PRIMARY KEY,
    poi_name            TEXT NOT NULL,
    seed                INTEGER NOT NULL,
    planet_class        TEXT NOT NULL,
    type_name           TEXT NOT NULL,
    radius_km           REAL NOT NULL,
    mass_earths         REAL NOT NULL,
    gravity_g           REAL NOT NULL,
    temperature_k       REAL NOT NULL,
    temperature_c       REAL NOT NULL,
    atm_pressure_atm    REAL NOT NULL,
    atm_description     TEXT NOT NULL,
    day_length_hours    REAL NOT NULL,
    orbital_period_days REAL NOT NULL,
    orbital_distance_au REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS poi_metadata_stars (
    poi_id           TEXT PRIMARY KEY,
    poi_name         TEXT NOT NULL,
    seed             INTEGER NOT NULL,
    star_class       TEXT NOT NULL,
    spectral_type    TEXT NOT NULL,
    spectral_subtype INTEGER NOT NULL DEFAULT -1,
    luminosity_class TEXT NOT NULL,
    luminosity_name  TEXT NOT NULL,
    color_hex        TEXT NOT NULL,
    color_name       TEXT NOT NULL,
    temp_range       TEXT NOT NULL,
    size_multiplier  REAL NOT NULL,
    render_size      REAL NOT NULL
);
