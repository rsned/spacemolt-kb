"""Modules the knowledge DB has never met still belong on the fitting sheet.

The DB is agent-populated: an item only lands in item_modules once an agent has
seen one. v0.566.0 made 19 hidden modules usable at once, so the catalog leads
the DB by weeks. catalog_items.json carries the same stats under near-identical
names, so it can fill the gap.
"""
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "scripts"))
from make_fitting_sheet import catalog_module, enrich_from_catalog  # noqa: E402


class TestCatalogModule(unittest.TestCase):
    def test_weapon(self):
        m = catalog_module({
            "id": "graviton_beam_i", "name": "Graviton Beam I", "type": "weapon",
            "slot": "weapon", "cpu_usage": 8, "power_usage": 15, "damage": 32,
            "damage_type": "kinetic", "reach": 4, "cooldown": 3,
            "special": "hull_damage_bonus_25", "base_value": 4200,
            "required_skills": {"gunnery": 1, "weapons": 2},
            "description": "Deals 32 kinetic damage.",
        }, ammo={})
        self.assertEqual(m["slot"], "weapon")
        self.assertEqual(m["cpu"], 8)
        self.assertEqual(m["pwr"], 15)
        self.assertEqual(m["req"], {"gunnery": 1, "weapons": 2})
        self.assertEqual(m["w"]["dmg"], 32)
        self.assertEqual(m["w"]["dt"], "kinetic")
        self.assertEqual(m["w"]["cd"], 3)
        # damage per tick is damage/cooldown, the same as the DB path
        self.assertAlmostEqual(m["w"]["dpt"], 32 / 3, places=2)

    def test_defense_and_passive_repair(self):
        m = catalog_module({
            "id": "nebula_bio_regenerator", "name": "Nebula Bio-Regenerator",
            "type": "defense", "slot": "defense", "cpu_usage": 6,
            "power_usage": 10, "hull_bonus": 40, "passive_repair": 8,
            "special": "bio_repair",
        }, ammo={})
        self.assertEqual(m["d"]["hull"], 40)
        self.assertEqual(m["d"]["arep"], 8)

    def test_mining_shares_the_utility_slot(self):
        m = catalog_module({
            "id": "modulated_mining_laser", "name": "Modulated Mining Laser",
            "type": "mining", "slot": "utility", "cpu_usage": 7,
            "power_usage": 12, "mining_power": 18,
        }, ammo={})
        self.assertEqual(m["slot"], "utility")
        self.assertEqual(m["u"]["hpow"], 18)

    def test_utility_penalties(self):
        m = catalog_module({
            "id": "tractor_beam", "name": "Tractor Beam", "type": "utility",
            "slot": "utility", "cpu_usage": 3, "power_usage": 5,
            "tow_speed_penalty": 40,
        }, ammo={})
        self.assertEqual(m["u"]["towpen"], 40)

    def test_adaptive_resistance_is_read_from_special(self):
        m = catalog_module({
            "id": "x", "name": "X", "type": "defense", "slot": "defense",
            "special": "adaptive_resistance_35",
        }, ammo={})
        self.assertEqual(m["adapt"], 35)

    def test_typed_resistance_survives(self):
        m = catalog_module({
            "id": "y", "name": "Y", "type": "defense", "slot": "defense",
            "resistance_bonus": {"thermal": 30},
        }, ammo={})
        self.assertEqual(m["d"]["res"], {"thermal": 30})

    def test_a_weapon_gets_no_defense_block(self):
        """damage/cooldown/range live on both weapons and defenses.

        The DB keeps them in separate tables so the question never arises; a flat
        catalog record needs the slot to decide which group they belong to.
        """
        m = catalog_module({
            "id": "graviton_beam_i", "name": "Graviton Beam I", "type": "weapon",
            "slot": "weapon", "damage": 32, "damage_type": "kinetic",
            "cooldown": 3, "reach": 4,
        }, ammo={})
        self.assertIn("w", m)
        self.assertNotIn("d", m)
        self.assertNotIn("u", m)

    def test_a_defense_keeps_its_own_damage(self):
        """A point defense turret really does deal damage from a defense slot."""
        m = catalog_module({
            "id": "point_defense_turret", "name": "Point Defense Turret",
            "type": "defense", "slot": "defense", "damage": 8,
            "damage_type": "kinetic", "cooldown": 1, "range": 8,
        }, ammo={})
        self.assertEqual(m["d"]["dmg"], 8)
        self.assertNotIn("w", m)

    def test_empty_stat_groups_are_dropped(self):
        m = catalog_module({"id": "z", "name": "Z", "type": "utility",
                            "slot": "utility"}, ammo={})
        self.assertNotIn("u", m)
        self.assertNotIn("d", m)
        self.assertNotIn("w", m)


class TestEnrichFromCatalog(unittest.TestCase):
    """Fields the knowledge DB schema has no column for.

    item_defenses has no reactive_resistance column and the combat_effects blob
    has no table at all, so a DB-loaded module arrives without them however
    fresh the DB is. They have to be overlaid from the catalog onto every
    module, not just the ones the DB has never met.
    """

    def enrich(self, mods, catalog):
        enrich_from_catalog(mods, {c["id"]: c for c in catalog})
        return {m["id"]: m for m in mods}

    def test_reactive_resistance_reaches_a_db_module(self):
        mods = [{"id": "reactive_armor_hardener", "slot": "defense", "d": {}}]
        got = self.enrich(mods, [{"id": "reactive_armor_hardener",
                                  "reactive_resistance": 60}])
        self.assertEqual(got["reactive_armor_hardener"]["d"]["react"], 60)

    def test_combat_effects_pools_are_lifted(self):
        mods = [
            {"id": "voidborn_phase_armor", "slot": "defense", "d": {"armor": 30}},
            {"id": "solarian_aegis", "slot": "defense", "d": {"shield": 100}},
            {"id": "flak_cannon_ii", "slot": "weapon", "w": {"dmg": 28}},
        ]
        got = self.enrich(mods, [
            {"id": "voidborn_phase_armor", "combat_effects": {"phase_dodge_pct": 25}},
            {"id": "solarian_aegis", "combat_effects": {"reflect_energy_pct": 20}},
            {"id": "flak_cannon_ii", "combat_effects": {"anti_missile_pct": 150}},
        ])
        self.assertEqual(got["voidborn_phase_armor"]["d"]["dodge"], 25)
        self.assertEqual(got["voidborn_phase_armor"]["d"]["armor"], 30)
        self.assertEqual(got["solarian_aegis"]["d"]["reflect"], 20)
        self.assertEqual(got["flak_cannon_ii"]["d"]["amis"], 150)

    def test_weapon_resistance_bypass(self):
        mods = [{"id": "void_lance_ii", "slot": "weapon", "w": {"dmg": 55}}]
        got = self.enrich(mods, [{"id": "void_lance_ii",
                                  "combat_effects": {"ignore_resistance_pct": 50}}])
        self.assertEqual(got["void_lance_ii"]["w"]["ignres"], 50)

    def test_a_stat_group_is_created_when_absent(self):
        mods = [{"id": "x", "slot": "defense"}]
        got = self.enrich(mods, [{"id": "x", "reactive_resistance": 60}])
        self.assertEqual(got["x"]["d"], {"react": 60})

    def test_missing_and_zero_values_add_nothing(self):
        mods = [{"id": "a", "slot": "defense", "d": {"armor": 5}},
                {"id": "b", "slot": "defense", "d": {"armor": 5}}]
        got = self.enrich(mods, [{"id": "a"},
                                 {"id": "b", "reactive_resistance": 0,
                                  "combat_effects": {"phase_dodge_pct": 0}}])
        self.assertEqual(got["a"]["d"], {"armor": 5})
        self.assertEqual(got["b"]["d"], {"armor": 5})

    def test_a_module_absent_from_the_catalog_is_left_alone(self):
        mods = [{"id": "ghost", "slot": "defense", "d": {"armor": 5}}]
        got = self.enrich(mods, [])
        self.assertEqual(got["ghost"]["d"], {"armor": 5})


if __name__ == "__main__":
    unittest.main()
