from math import floor as F
# Model v2:
#  x = pre_hit ; if shields up: x1 = floor(x*(1-shres)); drain = floor(floor(x1*e)*(1-flat))
#  sh_net(logged) = drain - g, g = floor(target_recharge/3) on hit ticks
#  breakthrough: sh = pool ; hull = x(NO shres) - floor(pool/e) - floor(armor*0.75)
#  shields down: hull = x - floor(armor*0.75)
#  kill cap: hull dmg capped at remaining hull
cases=[
 # id, P, shres%, flat%, e, armor, recharge, pool, obs_sh, obs_hl, cap, note
 ('509A ',118,3,0,0.75,8, 2,130, 85,1, None,'full-shield energy'),
 ('509M ',32, 1,70,0.75,25,4,600, 5,0,  None,'x4 identical'),
 ('509B ',118,3,0,0.75,8, 2,45,  45,52, None,'breakthrough'),
 ('509C ',118,3,0,0.75,8, 2,0,   0,83,  83,  'kill cap'),
 ('NK1  ',23, 0,0,0.75,3, 1,35,  17,0,  None,''),
 ('NK2  ',23, 0,0,0.75,3, 1,18,  17,0,  None,''),
 ('NKB  ',29, 0,0,0.75,3, 1,1,   1,26,  None,'crit breakthrough'),
 ('NK4  ',23, 0,0,0.75,3, 1,0,   0,21,  None,'shields down'),
 ('NK6  ',23, 0,0,0.75,3, 1,0,   0,12,  12,  'kill cap'),
 ('NKA  ',8,  4,0,1.0, 6, 2,95,  7,1,   None,'kinetic full-shield'),
 ('7cK  ',81, 1,0,1.0, 14,9,400, 77,0,  None,'x5 kinetic vs Artis'),
 ('7cKB ',90, 1,0,1.0, 14,9,15,  15,65, None,'crit breakthrough'),
 ('7cKD ',81, 1,0,1.0, 14,9,0,   0,71,  None,'shields down'),
 ('7cEB ',59, 4,0,0.75,28,2,8,   8,41,  None,'breakthrough vs broadaxe'),
 ('7cE1 ',59, 4,0,0.75,28,2,0,   0,52,  None,'shields down (alt 52)'),
 ('7cE2 ',59, 4,0,0.75,28,2,0,   0,53,  None,'shields down (alt 53)'),
 ('7cEC ',59, 4,0,0.75,28,2,0,   0,3,   3,   'kill cap'),
]
print(f"{'case':6s}{'pred_sh':>8s}{'pred_hl':>8s}{'obs_sh':>7s}{'obs_hl':>7s}  verdict")
for id,P,r,fl,e,a,rec,pool,osh,ohl,cap,note in cases:
    g=F(rec/3); eff=F(a*0.75)
    if pool>0:
        x1=F(P*(1-r/100))
        drain=F(F(x1*e)*(1-fl/100))
        if pool>=drain:
            sh=drain-g
            hl=x1-F(x1*e)-F(x1*(1-e))  # integer crumb
        else:
            sh=pool
            hl=P-(F(pool/e) if e>0 else 0)-eff
    else:
        sh=0; hl=P-eff
    if cap is not None: hl=min(hl,cap)
    ok='OK' if (sh==osh and hl==ohl) else 'MISS'
    print(f"{id:6s}{sh:8d}{hl:8d}{osh:7d}{ohl:7d}  {ok}  {note}")
