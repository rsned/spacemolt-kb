import math, itertools
F=math.floor
# Observations: (P pre_hit, shres_pct, flat_pct, e vs-shields eff, armor, armor_typemult_options key, pool, obs_sh, obs_hl, capped)
# e: energy 0.75, kinetic 1.0 ; armor type mult: energy 'E', kinetic 'K'
OBS=[
 # 509e Artis->MoltenOne (armor 8): A, B, C(kill-capped)
 dict(id='509A',P=118,r=3,flat=0,e=0.75,a=8,t='E',pool=130,sh=85,hl=1,cap=None),
 dict(id='509B',P=118,r=3,flat=0,e=0.75,a=8,t='E',pool=45,sh=45,hl=52,cap=None),
 dict(id='509C',P=118,r=3,flat=0,e=0.75,a=8,t='E',pool=0,sh=0,hl=83,cap=83),
 # 509e MoltenOne->Artis (armor 25, flat 70, pool 600) x1 (identical x4)
 dict(id='509M',P=32,r=1,flat=70,e=0.75,a=25,t='E',pool=600,sh=5,hl=0,cap=None),
]
def nek(vr):  # Nekkar obs with Vera shres unknown vr; MoltenOne shres assumed 3
    return [
 dict(id='NK1',P=23,r=vr,flat=0,e=0.75,a=3,t='E',pool=35,sh=17,hl=0,cap=None),
 dict(id='NK2',P=23,r=vr,flat=0,e=0.75,a=3,t='E',pool=18,sh=17,hl=0,cap=None),
 dict(id='NKB',P=29,r=vr,flat=0,e=0.75,a=3,t='E',pool=1,sh=1,hl=26,cap=None),
 dict(id='NK4',P=23,r=vr,flat=0,e=0.75,a=3,t='E',pool=0,sh=0,hl=21,cap=None),
 dict(id='NK5',P=23,r=vr,flat=0,e=0.75,a=3,t='E',pool=0,sh=0,hl=21,cap=None),
 dict(id='NK6',P=23,r=vr,flat=0,e=0.75,a=3,t='E',pool=0,sh=0,hl=12,cap=12),
 dict(id='NKA',P=8,r=3,flat=0,e=1.0,a=6,t='K',pool=95,sh=7,hl=1,cap=None),
]
AM={'E':[0.75,0.25,1.0,0.0],'K':[1.5,2.0,1.0,0.5]}  # armor effectiveness multiplier candidates per type
def simulate(o, shres_mode, brk, armor_form, am_e, am_k, flat_on, crumb_to_hull, stop_mode):
    P=o['P']; e=o['e']; pool=o['pool']; r=o['r']/100.0
    am = am_e if o['t']=='E' else am_k
    S = F(P*(1-r)) if (shres_mode=='volley' and pool>0) else (F(P*(1-r)) if shres_mode=='always' else P)
    if flat_on=='volley': S=F(S*(1-o['flat']/100.0))
    drain_full=F(S*e)
    if pool>=drain_full and pool>0:
        sh=drain_full
        if stop_mode=='floor': stopped=F(S*(1-e))
        else: stopped=S-drain_full  # all remainder stopped
        crumb=S-drain_full-stopped
        hull_in = crumb if crumb_to_hull else 0
        consumed=True
    else:
        sh=pool
        if e>0:
            used = {'floor':F(pool/e),'ceil':math.ceil(pool/e),'point':pool}[brk]
        else: used=0
        hull_in = S-used
        if hull_in<0: hull_in=0
    # armor stage
    if hull_in>0:
        ae = F(o['a']*am)
        if armor_form=='flat': hl=max(0,hull_in-ae)
        elif armor_form=='pct': hl=F(hull_in*max(0.0,1-o['a']*am/100.0))
        else: hl=hull_in
    else: hl=hull_in
    if flat_on=='hull': hl=F(hl*(1-o['flat']/100.0))
    if o['cap'] is not None:
        hl=min(hl,o['cap'])
    return sh,hl
best=[]
for shres_mode,brk,armor_form,am_e,am_k,flat_on,crumb,stop_mode,vr in itertools.product(
    ['volley','always','shieldonly_skip'],['floor','ceil','point'],['flat','pct','none'],
    AM['E'],AM['K'],['volley','hull'],[True,False],['floor','all'],range(0,6)):
    obs=OBS+nek(vr)
    ok=True; fails=[]
    for o in obs:
        sh,hl=simulate(o,shres_mode,brk,armor_form,am_e,am_k,flat_on,crumb,stop_mode)
        if o['cap'] is not None:
            good = (sh==o['sh'] and hl>=o['cap']-0 and min(hl,o['cap'])==o['hl'])
        else:
            good = (sh==o['sh'] and hl==o['hl'])
        if not good: ok=False; fails.append((o['id'],sh,hl))
    if ok: best.append((shres_mode,brk,armor_form,am_e,am_k,flat_on,crumb,stop_mode,vr))
    elif len(fails)<=1: pass
print('exact fits:',len(best))
for b in best[:20]: print(b)
if not best:
    # relax: report hypotheses with fewest failures
    scored=[]
    for combo in itertools.product(['volley','always','shieldonly_skip'],['floor','ceil','point'],['flat','pct','none'],AM['E'],AM['K'],['volley','hull'],[True,False],['floor','all'],range(0,6)):
        obs=OBS+nek(combo[-1]); n=0; det=[]
        for o in obs:
            sh,hl=simulate(o,*combo[:-1])
            good = (sh==o['sh'] and hl==o['hl']) if o['cap'] is None else (sh==o['sh'] and min(hl,o['cap'])==o['hl'])
            if not good: n+=1; det.append((o['id'],sh,hl,o['sh'],o['hl']))
        scored.append((n,combo,det))
    scored.sort(key=lambda x:x[0])
    for n,c,det in scored[:6]:
        print('misses',n,c)
        for x in det: print('   ',x)
