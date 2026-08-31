"""Armor-cap hypothesis (2026-08-31): armor_eff = min(floor(armor*0.75), floor(pre_hit*0.125)).
Fits 8/11 armor-touching rows exactly (all within +-1) across fixtures 509e1ef4, b7847bbc, 7c044558.
Residual: the +-1 alternation on consecutive identical broadaxe volleys. See memory/README."""
from math import floor as F
rows=[ # id, pre_hit, armor_total, consumed_by_shield, obs_hull, note
 ('509B',118,8, 60,52,'energy breakthrough (Portfolio)'),
 ('NKB', 29, 3, 1, 26,'energy breakthrough crit (Cobble)'),
 ('NK4', 23, 3, 0, 21,'energy shields-down (Cobble)'),
 ('NK5', 23, 3, 0, 21,'energy shields-down (Cobble)'),
 ('7cKB',90, 14,15,65,'kinetic breakthrough crit (Survey Vessel)'),
 ('7cKD',81, 14,0, 71,'kinetic shields-down (Survey Vessel)'),
 ('7cEB',59, 28,10,41,'energy breakthrough (Broadaxe) — off+1'),
 ('7cE1',59, 28,0, 52,'energy shields-down (Broadaxe)'),
 ('7cE2',59, 28,0, 53,'energy shields-down (Broadaxe) — off-1'),
 ('7cE3',59, 28,0, 52,'energy shields-down (Broadaxe)'),
 ('7cE4',59, 28,0, 53,'energy shields-down (Broadaxe) — off-1'),
]
ok=0
print(f"{'row':5s}{'eff_old':>8s}{'eff_cap':>8s}{'pred':>6s}{'obs':>5s}  verdict")
for id,P,a,con,obs,note in rows:
    eff=min(F(a*0.75), F(P*0.125))
    pred=P-con-eff
    v='OK' if pred==obs else f'off{pred-obs:+d}'
    ok+=pred==obs
    print(f"{id:5s}{F(a*0.75):8d}{F(P*0.125):8d}{pred:6d}{obs:5d}  {v:6s} {note}")
print(f"\n{ok}/{len(rows)} exact")
