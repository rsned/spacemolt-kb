import math, itertools
F=math.floor
# obs: P, r(target shield skill pct), flat, e, armor a, type t, pool, sh, hl, cap
def OBS(rm, rv):  # rm = MoltenOne shres now (NKA target), rv = Vera shres
    return [
 dict(id='509A',P=118,r=3, flat=0, e=0.75,a=8, t='E',pool=130,sh=85,hl=1, cap=None),
 dict(id='509B',P=118,r=3, flat=0, e=0.75,a=8, t='E',pool=45, sh=45,hl=52,cap=None),
 dict(id='509C',P=118,r=3, flat=0, e=0.75,a=8, t='E',pool=0,  sh=0, hl=83,cap=83),
 dict(id='509M',P=32, r=1, flat=70,e=0.75,a=25,t='E',pool=600,sh=5, hl=0, cap=None),
 dict(id='NK1', P=23, r=rv,flat=0, e=0.75,a=3, t='E',pool=35, sh=17,hl=0, cap=None),
 dict(id='NK2', P=23, r=rv,flat=0, e=0.75,a=3, t='E',pool=18, sh=17,hl=0, cap=None),
 dict(id='NKB', P=29, r=rv,flat=0, e=0.75,a=3, t='E',pool=1,  sh=1, hl=26,cap=None),
 dict(id='NK4', P=23, r=rv,flat=0, e=0.75,a=3, t='E',pool=0,  sh=0, hl=21,cap=None),
 dict(id='NK6', P=23, r=rv,flat=0, e=0.75,a=3, t='E',pool=0,  sh=0, hl=12,cap=12),
 dict(id='NKA', P=8,  r=rm,flat=0, e=1.0, a=6, t='K',pool=95, sh=7, hl=1, cap=None),
]
def sim(o, mode, brk, crumb_rule, armor_mult_e, armor_mult_k, armor_form, flat_pos, shres_pos):
    P=float(o['P']); r=o['r']/100.0; e=o['e']; pool=o['pool']; flat=o['flat']/100.0
    am = armor_mult_e if o['t']=='E' else armor_mult_k
    # stage values carried as float ('carry') or floored each stage ('stage')
    x=P
    if shres_pos=='pre' and pool>0:
        x = x*(1-r)
        if mode=='stage': x=F(x)
    if flat_pos=='pre':
        x = x*(1-flat)
        if mode=='stage': x=F(x)
    shield_cap_amt = x*e   # what shields would absorb if pool unlimited
    if mode=='stage': drain_full=F(shield_cap_amt)
    else: drain_full=F(shield_cap_amt)
    if pool>0 and pool>=drain_full:
        sh=drain_full
        if flat_pos=='shield': sh=F(sh*(1-flat))
        if shres_pos=='shield': sh=F(sh*(1-r))
        stopped=F(x*(1-e))
        if crumb_rule=='remainder': hull_in = max(0, F(x)-drain_full-stopped)
        elif crumb_rule=='none': hull_in=0
        elif crumb_rule=='shresloss': hull_in = o['P']-F(o['P']*(1-r)) if pool>0 else 0
        else: hull_in=0
    else:
        sh=pool
        used = {'floor':F(pool/e) if e>0 else 0,'ceil':math.ceil(pool/e) if e>0 else 0,'point':pool}[brk]
        hull_in = max(0, (x-used))
        if mode=='stage': hull_in=F(hull_in)
    hl=0
    if hull_in>0:
        ae=o['a']*am
        if armor_form=='flat': hl=max(0, F(hull_in-ae))
        elif armor_form=='flatfloor': hl=max(0, hull_in-F(ae))
        elif armor_form=='pct': hl=F(hull_in*max(0,1-ae/100.0))
        else: hl=F(hull_in)
        if flat_pos=='hull': hl=F(hl*(1-flat))
    hl=int(hl); 
    if o['cap'] is not None: hl=min(hl,o['cap'])
    return int(sh), hl
results=[]
space=list(itertools.product(
    ['stage','carry'],['floor','ceil','point'],['remainder','none','shresloss'],
    [0.75,0.25,1.0,0.0],[1.5,2.0,1.0,0.5,0.0],['flat','flatfloor','pct','none'],
    ['pre','shield','hull'],['pre','shield'],range(0,16),range(0,16)))
best=(99,None,None)
for combo in space:
    rm,rv=combo[-2],combo[-1]
    misses=[]; 
    for o in OBS(rm,rv):
        sh,hl=sim(o,*combo[:-2])
        if not(sh==o['sh'] and hl==o['hl']): misses.append((o['id'],sh,hl,o['sh'],o['hl']))
    if len(misses)==0: results.append(combo)
    if len(misses)<best[0]: best=(len(misses),combo,misses)
print('exact fits:',len(results))
for cbo in results[:15]: print(cbo)
if not results:
    print('best:',best[0],best[1])
    for m in best[2]: print('  ',m)
