from math import floor as F
def spill(v): return 1 if (v-F(v))>=0.5 else 0
cases=[ # id,P,shres,flat,e,armor,recharge,pool,obs_sh,obs_hl,cap,note
 ('509A ',118,3,0,0.75,8, 2,130,85,1,None,''), ('509M ',32,1,70,0.75,25,4,600,5,0,None,'x4'),
 ('509B ',118,3,0,0.75,8, 2,45,45,52,None,'brk'), ('509C ',118,3,0,0.75,8,2,0,0,83,83,'cap'),
 ('NK1  ',23,0,0,0.75,3, 1,35,17,0,None,''), ('NK2  ',23,0,0,0.75,3,1,18,17,0,None,''),
 ('NKB  ',29,0,0,0.75,3, 1,1,1,26,None,'brk'), ('NK4  ',23,0,0,0.75,3,1,0,0,21,None,''),
 ('NK6  ',23,0,0,0.75,3, 1,0,0,12,12,'cap'), ('NKA  ',8,4,0,1.0,6,2,95,7,1,None,'kin'),
 ('7cK  ',81,1,0,1.0, 14,9,400,77,0,None,'x5 kin'), ('7cKB ',90,1,0,1.0,14,9,15,15,65,None,'brk'),
 ('7cKD ',81,1,0,1.0, 14,9,0,0,71,None,''), ('7cEB ',59,4,0,0.75,28,2,8,8,41,None,'brk BROADAXE'),
 ('7cE1 ',59,4,0,0.75,28,2,0,0,52,None,'BROADAXE'), ('7cE2 ',59,4,0,0.75,28,2,0,0,53,None,'BROADAXE'),
 ('7cEC ',59,4,0,0.75,28,2,0,0,3,3,'cap'),
]
ok=0; miss=[]
print(f"{'case':6s}{'p_sh':>6s}{'p_hl':>6s}{'o_sh':>6s}{'o_hl':>6s}  verdict")
for id,P,r,fl,e,a,rec,pool,osh,ohl,cap,note in cases:
    g=F(rec/3); eff=F(a*0.75)
    if pool>0:
        v1=P*(1-r/100); s1=spill(v1); x1=F(v1)
        v2=x1*e; s2=spill(v2); d=F(v2)
        v3=d*(1-fl/100); s3=spill(v3); d2=F(v3)
        drain=d2
        if pool>=drain:
            sh=drain-g
            hl=F((s1+s2+s3)*(1-fl/100))
        else:
            sh=pool; hl=P-(F(pool/e) if e>0 else 0)-eff
    else:
        sh=0; hl=P-eff
    if cap is not None: hl=min(hl,cap)
    v='OK' if (sh==osh and hl==ohl) else 'MISS'
    if v=='OK': ok+=1
    else: miss.append(id.strip())
    print(f"{id:6s}{sh:6d}{hl:6d}{osh:6d}{ohl:6d}  {v}  {note}")
print(f"\n{ok}/{len(cases)} exact. misses: {miss}")
print("broadaxe rows need armor_eff ~6-8, not floor(28*0.75)=21 -> armor anomaly (cap? stale catalog? plate inactive?)")
