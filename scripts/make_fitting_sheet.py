#!/usr/bin/env python3
"""Generate the ship fitting sheet: kb/ships/fitting.html.

A cyanotype drafting sheet in the same visual language as the blueprint
gallery (data/footprints/blueprints/make_blueprints.py). The whole page is the
blueprint: navy field, drafting grid, heavy double border, Courier line-work.

Left column carries the top-view plan rotated nose-up with concentric shield
outlines sized from the fit. Right column carries the slot selectors on top and
the combined stats sheet below.

The catalog (338 ships, 210 fittable modules) is baked into the page as JSON --
it is small, and baking it keeps the page a single static file with no fetch on
first paint. The per-ship plan silhouette is NOT baked (2.6 MB across the
fleet); it is fetched on demand from kb/ships/plans/<id>.svg, which
scripts/build-fitting-plans.sh restores from the committed
data/footprints/<id>_top.svg at deploy time -- the same shape as
build-footprint-zip.sh, so the repo carries one copy of the data.

    python3 scripts/make_fitting_sheet.py
"""
import json
import sqlite3
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent
DB = REPO / "spacemolt-knowledge.db"
FOOT = REPO / "data" / "footprints"
SCALE = FOOT / "scale" / "ship_scale_est.json"
LEGACY = REPO / "data" / "legacy.json"
OUT = REPO / "kb" / "ships" / "fitting.html"

DAMAGE_TYPES = ["kinetic", "thermal", "energy", "em", "explosive", "void"]

# Mirrors specialLabels in cmd/generate-items-kb/modules.go so the fitting sheet
# and the weapon/defense comparison tables name the same token the same way.
SPECIAL_LABELS = {
    "adaptive_resistance": "Adaptive resistance",
    "aoe_radius": "AoE radius",
    "anti_drone_bonus": "Anti-drone",
    "anti_missile_bonus": "Anti-missile",
    "armor_bypass": "Armor bypass",
    "armor_melt": "Armor melt",
    "capacitor_drain": "Capacitor drain",
    "capacitor_transfer": "Capacitor transfer",
    "chain_lightning": "Chain lightning",
    "cpu_damage": "CPU damage",
    "energy_damage_bonus": "Energy damage",
    "hull_damage_bonus": "Hull damage",
    "ignores_resistance": "Ignores resistance",
    "lifesteal": "Lifesteal",
    "mine_capacity": "Mine capacity",
    "mine_detection": "Mine detection",
    "mine_duration": "Mine duration",
    "mine_tracking_speed": "Mine tracking",
    "module_disable": "Module disable",
    "phase_dodge": "Phase dodge",
    "phase_strike": "Phase strike",
    "random_damage_variance": "Damage variance",
    "reflect_energy": "Reflect energy",
    "shield_bypass": "Shield bypass",
    "shield_damage_bonus": "Shield damage",
    "shock_attackers": "Shock attackers",
    "system_disable": "System disable",
}


def title_case(s):
    return s.replace("_", " ").capitalize()


def decode_specials(special):
    """Split a comma-joined special string into display labels.

    A trailing `_N` on a token is its magnitude, not part of the name:
    `adaptive_resistance_35` renders as "Adaptive resistance 35".
    """
    out = []
    for tok in (special or "").split(","):
        tok = tok.strip()
        if not tok:
            continue
        prefix, mag = tok, ""
        i = tok.rfind("_")
        if i > 0 and tok[i + 1:].isdigit():
            prefix, mag = tok[:i], tok[i + 1:]
        lbl = SPECIAL_LABELS.get(prefix) or title_case(prefix)
        out.append(f"{lbl} {mag}".strip())
    return out


def adaptive_resistance(special):
    """N from an `adaptive_resistance_N` token: N% against *every* damage type."""
    for tok in (special or "").split(","):
        tok = tok.strip()
        if tok.startswith("adaptive_resistance_"):
            rest = tok[len("adaptive_resistance_"):]
            if rest.isdigit():
                return int(rest)
    return 0


def jload(s, default):
    if not s:
        return default
    try:
        return json.loads(s)
    except (ValueError, TypeError):
        return default


def nz(d):
    """Drop zero/null/empty members so the baked JSON stays lean."""
    return {k: v for k, v in d.items() if v not in (0, None, "", [], {})}


def load_ammo(con):
    """Damage modifier range per ammo type, so a weapon can show effective DPT."""
    by_type = {}
    for item_id, ammo_type, mods in con.execute(
            "select item_id, ammo_type, modifiers from item_ammo"):
        m = jload(mods, {})
        by_type.setdefault(ammo_type, []).append(float(m.get("damage_mod", 0)))
    return {k: [min(v), max(v)] for k, v in by_type.items()}


def load_ships(con):
    # Metre estimates from the same source the blueprint sheets label with, so
    # the fitting sheet's dimension callout agrees with the registry sheet.
    est = jload(SCALE.read_text() if SCALE.exists() else "", {}).get("ships", {})
    ships = {}
    q = """select id, name, class, category, tier, scale, price,
                  weapon_slots, defense_slots, utility_slots,
                  cpu_capacity, power_capacity,
                  base_hull, base_shield, base_shield_recharge, base_armor,
                  base_speed, base_fuel, cargo_capacity, tow_speed_bonus,
                  special, inherent_capabilities, description, default_modules,
                  starter_ship
           from ships order by name"""
    for r in con.execute(q):
        (sid, name, cls, cat, tier, scale, price, w, d, u, cpu, pwr, hull,
         shield, srec, armor, speed, fuel, cargo, tow, special, inh, desc,
         dmods, starter) = r
        caps = []
        for c in jload(inh, []) or []:
            if isinstance(c, dict) and c.get("Value"):
                caps.append([c.get("Type", ""), c["Value"]])
        ships[sid] = nz({
            "id": sid, "name": name, "cls": cls or "", "cat": cat or "",
            "tier": tier or 0, "scale": scale or 0, "price": price or 0,
            "w": w or 0, "d": d or 0, "u": u or 0,
            "cpu": cpu or 0, "pwr": pwr or 0,
            "hull": hull or 0, "shield": shield or 0, "srec": srec or 0,
            "armor": armor or 0, "speed": speed or 0, "fuel": fuel or 0,
            "cargo": cargo or 0, "tow": tow or 0,
            "special": special or "", "caps": caps,
            # Stock loadout. Fitted by default so the sheet opens on the ship as
            # the catalog ships it, not as a stripped hull.
            "dm": jload(dmods, []) or [],
            "starter": 1 if starter else 0,
            "desc": (desc or "")[:220],
            "plan": 1 if (FOOT / f"{sid}_top.svg").exists() else 0,
            "loa": (est.get(sid) or {}).get("loa_m", 0),
            "beam": (est.get(sid) or {}).get("beam_m", 0),
        }) | {"id": sid, "name": name, "tier": tier or 0}
    return ships


def load_modules(con, ammo):
    """Every fittable module, keyed by slot. Mining rolls into utility."""
    mods = []
    q = """select m.item_id, i.name, m.slot, m.type, m.cpu_usage, m.power_usage,
                  m.special, i.required_skills, i.base_value, i.description
           from item_modules m join items i on i.id = m.item_id
           order by i.name"""
    rows = list(con.execute(q))

    weapons = {r[0]: r[1:] for r in con.execute(
        """select item_id, damage, damage_type, range, reach, cooldown,
                  ammo_type, magazine_size, armor_bypass_bonus, shield_bypass_bonus
           from item_weapons""")}
    defenses = {r[0]: r[1:] for r in con.execute(
        """select item_id, armor_bonus, hull_bonus, shield_bonus,
                  shield_recharge_bonus, armor_repair_rate, resistance_bonus,
                  damage_reduction, cloak_strength, cooldown, damage,
                  damage_type, range
           from item_defenses""")}
    utils = {r[0]: r[1:] for r in con.execute(
        """select item_id, speed_bonus, cargo_bonus, cloak_strength,
                  scanner_power, accuracy_bonus, tracking_bonus, signature_bonus,
                  fuel_efficiency, drone_bandwidth, drone_capacity, harvest_power,
                  harvest_range, survey_power, survey_range, tow_speed_penalty,
                  cpu_bonus, max_fuel_bonus, hull_penalty, speed_penalty
           from item_utilities""")}

    for (mid, name, slot, mtype, cpu, pwr, special, req, value, desc) in rows:
        m = {
            "id": mid, "name": name,
            # Mining modules occupy a utility slot; they share the dropdown.
            "slot": "utility" if slot == "mining" else slot,
            "kind": mtype or "",
            "cpu": cpu or 0, "pwr": pwr or 0,
            "sp": decode_specials(special),
            "adapt": adaptive_resistance(special),
            "req": jload(req, {}) or {},
            "val": value or 0,
            "desc": (desc or "")[:200],
        }
        if mid in weapons:
            (dmg, dtype, rng, reach, cd, atype, mag, abyp, sbyp) = weapons[mid]
            dpt = (dmg or 0) / cd if cd else 0
            lo, hi = ammo.get(atype, [0.0, 0.0]) if atype else [0.0, 0.0]
            m["w"] = nz({
                "dmg": dmg or 0, "dt": dtype or "", "rng": rng or 0,
                "reach": reach or 0, "cd": cd or 0, "ammo": atype or "",
                "mag": mag or 0, "abyp": abyp or 0, "sbyp": sbyp or 0,
                "dpt": round(dpt, 3),
                "edpt": [round(dpt * (1 + lo), 3), round(dpt * (1 + hi), 3)],
            })
        if mid in defenses:
            (armor, hull, shield, srec, arep, res, dred, cloak, cd, dmg,
             dtype, rng) = defenses[mid]
            m["d"] = nz({
                "armor": armor or 0, "hull": hull or 0, "shield": shield or 0,
                "srec": srec or 0, "arep": arep or 0,
                "res": jload(res, {}) or {}, "dred": dred or 0,
                "cloak": cloak or 0, "cd": cd or 0, "dmg": dmg or 0,
                "dt": dtype or "", "rng": rng or 0,
            })
        if mid in utils:
            (speed, cargo, cloak, scan, acc, track, sig, feff, dbw, dcap,
             hpow, hrng, spow, srng, towpen, cpub, mfuel, hullpen,
             speedpen) = utils[mid]
            m["u"] = nz({
                "speed": speed or 0, "cargo": cargo or 0, "cloak": cloak or 0,
                "scan": scan or 0, "acc": acc or 0, "track": track or 0,
                "sig": sig or 0, "feff": feff or 0, "dbw": dbw or 0,
                "dcap": dcap or 0, "hpow": hpow or 0, "hrng": hrng or 0,
                "spow": spow or 0, "srng": srng or 0,
                "towpen": towpen or 0, "cpub": cpub or 0, "mfuel": mfuel or 0,
                "hullpen": hullpen or 0, "speedpen": speedpen or 0,
            })
        mods.append(m)
    return mods


def load_legacy():
    """Ids the game no longer sells.

    Ships stay fully usable — players still fly a Deeprock Harvester — so the
    sheet fits them normally and only marks them. Modules do NOT: v0.561.0
    unfitted every discontinued module and blocked refitting, so the sheet
    lists them without letting them be installed.
    """
    doc = jload(LEGACY.read_text() if LEGACY.exists() else "", {})
    return doc.get("ships", {}), doc.get("items", {})


def main():
    if not DB.exists():
        sys.exit(f"missing {DB}")
    con = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    ammo = load_ammo(con)
    ships = load_ships(con)
    mods = load_modules(con, ammo)
    con.close()

    legacy_ships, legacy_items = load_legacy()
    for sid, rec in ships.items():
        if sid in legacy_ships:
            rec["legacy"] = legacy_ships[sid].get("last_in_catalog") or 1
    for m in mods:
        if m["id"] in legacy_items:
            m["legacy"] = legacy_items[m["id"]].get("last_in_catalog") or 1

    tmpl = (HERE / "fitting_sheet.tmpl.html").read_text()
    html = (tmpl
            .replace("/*__SHIPS__*/null", json.dumps(ships, separators=(",", ":")))
            .replace("/*__MODULES__*/null", json.dumps(mods, separators=(",", ":")))
            .replace("/*__DAMAGE_TYPES__*/null", json.dumps(DAMAGE_TYPES)))
    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(html)
    nl_s = sum(1 for s in ships.values() if s.get("legacy"))
    nl_m = sum(1 for m in mods if m.get("legacy"))
    print(f"  legacy marked: {nl_s} ships, {nl_m} modules")
    planned = sum(1 for s in ships.values() if s.get("plan"))
    print(f"wrote {OUT.relative_to(REPO)}  "
          f"{len(ships)} ships ({planned} with plan views), {len(mods)} modules, "
          f"{OUT.stat().st_size / 1024:.0f} KB")


if __name__ == "__main__":
    main()
