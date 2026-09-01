# Phase B: Calibration Duels

## Arena Decision

**System**: Ashford (lawless, police 0)  
**Rationale**: One jump from staging (treasure_cache), quiet wildlife activity (2 recorded battles as of 2026-08-29), adjacent to treasure_cache trading post for rapid bot repositioning.

## Campaign Geography

- **Arena**: ashford
- **Staging system**: treasure_cache
- **Staging station**: treasure_cache_trading_post (both bots set home_base here)
- **Duelists**: battle_bot1, battle_bot2 (fresh accounts, 250 credits, stock Prospect)
- **Guest**: craftsman-1 (appears S6c only; Tactics 2, will be Artis empire)

## Shopping List & Funding

**Required Equipment** (prices from game catalog):

| Item | Qty | Unit Price | Total | Notes |
|------|-----|-----------|-------|-------|
| pulse_laser_i | 4 | 2,500 cr | 10,000 cr | S0-S6 weapon; reach 3 |
| autocannon_i | 2 | 710 cr | 1,420 cr | S4 weapon; reach 2; needs kinetic ammo |
| missile_launcher_i | 2 | 1,800 cr | 3,600 cr | S1 far-ring; reach 6; needs explosive ammo |
| armor_plate_i | 4+ | 490 cr | 2,000+ cr | S7 ladder; +5 armor per plate |
| Kinetic Ammo | 1 stack | — | ~300 cr | S4 support |
| Explosive Ammo | 1 stack | — | ~300 cr | S1 support |
| **Dualism hull** | 1 | ~10,000 cr | 10,000 cr | 3 defense slots, base armor 7 (S7 target) |
| **Lawn_dart hull** | 1 | ~5,000 cr | 5,000 cr | Fast hull, speed 6 (S2, S6d) |
| **Respawn buffer** | — | — | 5,000 cr | Contingency for attrition |

**Budget ceiling**: ~100k credits  
**Expected spend**: ~38k credits  
**Margin**: ~62k credits

## Funding Checklist

- [ ] **Donor agent**: [main-account agent name TBD]
- [ ] **Transfer method**: dock at treasure_cache_trading_post; battle_bot1 and battle_bot2 receive credits via trade offer or direct gift
- [ ] **Verify balance**: Both bots confirm ≥50k credits after transfer
- [ ] **Set home_base**: Both bots run `set_home_base(treasure_cache_trading_post)` so destruction respawns them at staging with free Prospect replacement
- [ ] **Verify skills**: Run `get_skills` on both bots before S0 — expect all zeros (crit 0, weapon_skill_pct 0, shields 0, armor 0, tactics 0)
- [ ] **Check worker**: `ps aux | grep bin/worker` — stop any live session (S6c needs craftsman-1 login; 36s between logins required)

## S0 Gate

**Nothing proceeds until S0 is reviewed.**

S0-probe (1 repeat, ~5 ticks) is a sanity check:
1. Verify friendly fire between own accounts in lawless space draws **no police/rep response**
2. Verify free battle actions (attack, stance changes) execute as scripted
3. Verify manifest/export loop round-trips correctly
4. Confirm both bots can dock/undock and jump to/from arena

If S0 reveals unexpected rep damage, faction flags, or other consequences, halt and reassess with the owner before proceeding to S1.

## Run Log

| Date | Scenarios Run | Battles | Notes |
|------|---------------|---------|-------|
| | | | |
| | | | |
| | | | |

---

**Campaign file**: `campaign.json` (33 duel repeats; S0-probe, S1 hit table ×9, S2 speed ×5, S3 evade ×2, S4 brace ×2, S5 regen ×2, S6 flee ×6, S7 armor ×6)

**Analysis scripts** (forthcoming):
- `phaseb_hit_table.py` — ring-by-ring hit chance fitted values
- `phaseb_stances.py` — evade/brace/flee fixture stats
- `phaseb_armor.py` — armor law (flat vs saturating)
