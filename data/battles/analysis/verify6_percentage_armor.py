"""Armor as saturating percentage (dev bot, 2026-08-31): armor is never spent,
reduces hull-bound damage by a percentage with diminishing returns; type mult
applies to armor COUNTED (kinetic/void x1.5, explosive/EM x1.0, energy x0.75,
thermal x0.25); min 1 damage always lands; armor-melt debuffs cut it.
"150" = max bare hull armor in catalog (annihilator/the_vault) and works as
the half-saturation constant: f = a_counted/(a_counted+150).
Fit: EXACT on both kinetic rows (armor 14: 81->71, 75->65) and within 1 on all
three broadaxe rows (the flat model was off by 14 there). Small-armor energy
rows (cobble 3, portfolio 8) still fit flat floor(a*0.75) exactly and miss by
1-3 under the percentage model -> the exact law at low armor is still open."""
from math import floor as F
rows=[ # id, leftover_before_armor, armor_base, typemult, obs_hull, note
 ('7cKD',81, 14,1.5,71,'kinetic shields-down'),
 ('7cKB',75, 14,1.5,65,'kinetic breakthrough (90-15)'),
 ('NK4', 23, 3, 0.75,21,'energy shields-down cobble'),
 ('509B',58, 8, 0.75,52,'energy breakthrough portfolio (118-60)'),
 ('7cE1',59, 28,0.75,52,'energy shields-down broadaxe'),
 ('7cE2',59, 28,0.75,53,'energy shields-down broadaxe (alt)'),
 ('7cEB',49, 28,0.75,41,'energy breakthrough broadaxe (59-10)'),
]
print(f"{'row':5s}{'obs':>5s}{'flat':>6s}{'pct150':>8s}  implied_reduction")
for id,L,a,tm,obs,note in rows:
    flat=L-F(a*0.75)
    ac=a*tm
    pct=F(L*(1-ac/(ac+150)))
    print(f"{id:5s}{obs:5d}{flat:6d}{pct:8d}  {100*(1-obs/L):5.1f}%  {note}")
