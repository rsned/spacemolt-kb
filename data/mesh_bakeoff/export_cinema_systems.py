#!/usr/bin/env python3
"""Export per-system backdrop data for the battle cinematic (Part B).

Writes <out>/systems.json -- per system: its star (class, colour, size) and
its planets in orbit order, with a texture path where kb/images/planets has
one -- and links <out>/planets to that image folder so the dev server can
serve the equirectangular maps. Keyed by poi id, so the new terrain
generator's output later replaces textures without code changes.

    python3 export_cinema_systems.py [--out ~/spacemolt-www/public/cinema-systems]
"""
import argparse
import json
import sqlite3
from pathlib import Path

HERE = Path(__file__).resolve().parent
DEFAULT_DB = Path.home() / "spacemolt" / "spacemolt-knowledge.db"
DEFAULT_OUT = Path.home() / "spacemolt-www" / "public" / "cinema-systems"
TEXTURES = HERE.parent.parent / "kb" / "images" / "planets"

PLANETS_SQL = """SELECT p.system_id, p.id, m.planet_class, m.radius_km, m.orbital_distance_au
                 FROM pois p JOIN poi_metadata_planets m ON m.poi_id = p.id"""
STARS_SQL = """SELECT p.system_id, s.star_class, s.color_hex, s.size_multiplier, s.render_size
               FROM poi_metadata_stars s JOIN pois p ON p.id = s.poi_id"""


def build_systems(planets, stars, texture_names):
    """planets: (system_id, poi_id, class, radius_km, orbit_au); stars: (system_id, class, color, size_mult, render_size)."""
    systems = {}
    for system_id, star_class, color, size_multiplier, render_size in stars:
        systems.setdefault(system_id, {"star": None, "planets": []})["star"] = {
            "class": star_class, "color": color, "sizeMultiplier": size_multiplier, "renderSize": render_size}
    for system_id, poi_id, planet_class, radius_km, orbit_au in sorted(planets, key=lambda row: (row[0], row[4], row[1])):
        planet = {"id": poi_id, "class": planet_class, "radiusKm": radius_km, "orbitAu": orbit_au}
        name = f"{system_id}_{poi_id}.png"
        if name in texture_names:
            planet["texture"] = f"planets/{name}"
        systems.setdefault(system_id, {"star": None, "planets": []})["planets"].append(planet)
    return {"version": 1, "systems": dict(sorted(systems.items()))}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", type=Path, default=DEFAULT_DB)
    parser.add_argument("--out", type=Path, default=DEFAULT_OUT)
    args = parser.parse_args()
    if not args.db.exists():
        raise SystemExit(f"knowledge DB not found: {args.db}")
    db = sqlite3.connect(args.db)
    try:
        data = build_systems(db.execute(PLANETS_SQL).fetchall(), db.execute(STARS_SQL).fetchall(),
                             {p.name for p in TEXTURES.glob("*.png")})
    finally:
        db.close()
    args.out.mkdir(parents=True, exist_ok=True)
    (args.out / "systems.json").write_text(json.dumps(data, separators=(",", ":")))
    link = args.out / "planets"
    if not link.exists():
        link.symlink_to(TEXTURES, target_is_directory=True)
    planets = [p for s in data["systems"].values() for p in s["planets"]]
    print(f"{len(data['systems'])} systems, {len(planets)} planets, {sum('texture' in p for p in planets)} textured -> {args.out}")


if __name__ == "__main__":
    main()
