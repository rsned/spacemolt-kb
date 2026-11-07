import json,re,html,difflib,sys,gzip,glob,os
D='/home/robert/spacemolt/spacemolt/server_docs/'
CACHE={}
def spec(d):
    if d not in CACHE:
        CACHE[d]=json.load(open(f'{D}openapi.{d}.json'))['components']['schemas']
    return CACHE[d]
INIT={'id':'ID','url':'URL','xp':'XP','api':'API','poi':'POI','hp':'HP','npc':'NPC'}
def camel(s): return ''.join(INIT.get(p,p[:1].upper()+p[1:]) for p in re.split(r'[_\-]',s) if p)
def nullable(n):
    t=n.get('type') if isinstance(n,dict) else None
    return isinstance(t,list) and 'null' in t
def base(n):
    t=n.get('type')
    return next((x for x in t if x!='null'),None) if isinstance(t,list) else t
def gotype(n,enum):
    if not isinstance(n,dict): return 'any'
    if '$ref' in n: return n['$ref'].split('/')[-1]
    if enum and 'enum' in n: return enum
    b=base(n)
    if b=='array': return '[]'+gotype(n.get('items',{}),None)
    if b=='object':
        ap=n.get('additionalProperties')
        return 'map[string]'+gotype(ap,None) if isinstance(ap,dict) else 'map[string]any'
    return {'integer':'int','number':'float64','boolean':'bool','string':'string'}.get(b,'any')
def tstype(n,enum):
    if not isinstance(n,dict): return 'unknown'
    if '$ref' in n: return n['$ref'].split('/')[-1]
    if enum and 'enum' in n: return enum
    b=base(n)
    if b=='array':
        x=tstype(n.get('items',{}),None); r=x+'[]'
    elif b=='object':
        ap=n.get('additionalProperties')
        r=f'Record<string, {tstype(ap,None)}>' if isinstance(ap,dict) else 'Record<string, unknown>'
    else: r={'integer':'number','number':'number','boolean':'boolean','string':'string'}.get(b,'unknown')
    return r+' | null' if nullable(n) else r
def field(name,k,v,req):
    en=name+camel(k) if isinstance(v,dict) and 'enum' in v else None
    g=gotype(v,en); opt=k not in req
    if opt and isinstance(v,dict) and '$ref' in v: g='*'+g
    return dict(k=k,go=(camel(k),g,'`json:"%s%s"`'%(k,',omitempty' if opt else '')),
        ts=f"{k}{'?' if opt else ''}: {tstype(v,en)};",desc=(v.get('description','') if isinstance(v,dict) else ''),
        enum=(v.get('enum') if isinstance(v,dict) else None),enumname=en)
def fields(s,name):
    n=s.get(name)
    if not n: return None
    req=set(n.get('required',[])); return {k:field(name,k,v,req) for k,v in n.get('properties',{}).items()}
E=html.escape
def wrap(tokens,width,prefix):
    # tokens: (word,marked)
    lines=[];cur='';l=0
    def piece(w,m): return f'<mark>{E(w)}</mark>' if m else E(w)
    for w,m in tokens:
        if l and l+1+len(w)>width:
            lines.append(cur);cur='';l=0
        if l: cur+=' ';l+=1
        cur+=piece(w,m);l+=len(w)
    if cur: lines.append(cur)
    return [f'<span class="c">{prefix}{x.replace("</mark> <mark>"," ")}</span>' for x in lines]
def plain(text,width=84,prefix='// '):
    return wrap([(w,False) for w in text.split()],width,prefix) if text else []
def sents(t): return [x for x in re.split(r'(?<=[.!?;:])\s+',t.strip()) if x]
def desc_rows(old,new,width=84):
    a,b=sents(old),sents(new);rows=[]
    for tag,i,j,k,l in difflib.SequenceMatcher(None,a,b,autojunk=False).get_opcodes():
        if tag=='equal':
            rows.append(('ctx','<span class="c">// …</span>')); continue
        x=' '.join(a[i:j]).split();y=' '.join(b[k:l]).split()
        sm=difflib.SequenceMatcher(None,x,y,autojunk=False)
        mx=[(w,False) for w in x];my=[(w,False) for w in y]
        if x and y and sm.ratio()>.35:
            for t,p,q,r,s in sm.get_opcodes():
                if t!='equal':
                    for q2 in range(p,q): mx[q2]=(x[q2],True)
                    for s2 in range(r,s): my[s2]=(y[s2],True)
        if any(m for _,m in mx+my) and x and y:
            p=0
            while p<min(len(x),len(y)) and x[p]==y[p]: p+=1
            q=0
            while q<min(len(x),len(y))-p and x[-1-q]==y[-1-q]: q+=1
            def trim(t):
                C=6;st=max(0,p-C);en=len(t)-max(0,q-C)
                return ([('…',False)] if st else [])+t[st:en]+([('…',False)] if en<len(t) else [])
            mx,my=trim(mx),trim(my)
        rows+= [('del',h) for h in wrap(mx,width,'// ')]+[('add',h) for h in wrap(my,width,'// ')]
    out=[]
    for r in rows:
        if r[0]=='ctx' and out and out[-1][0]=='ctx': continue
        out.append(r)
    return out
def gocode(f,w):
    a,b,c=f['go']
    return f'    <span class="n">{E(a.ljust(w[0]))}</span> <span class="t">{E(b.ljust(w[1]))}</span> <span class="s">{E(c)}</span>'
def tscode(f):
    m=re.match(r'^(\S+?\??): (.*);$',f['ts'])
    return f'    <span class="n">{E(m[1])}</span>: <span class="t">{E(m[2])}</span>;'
def build(name,old,new,lang):
    keys=list((new or {}).keys())
    for i,k in enumerate((old or {}).keys()):
        if k not in keys: keys.insert(min(i,len(keys)),k)
    allf=[f for d in (old,new) if d for f in d.values()]
    w=(max(len(f['go'][0]) for f in allf),max(len(f['go'][1]) for f in allf))
    code=(lambda f:gocode(f,w)) if lang=='go' else tscode
    isnew=old is None
    head=(f'<span class="k">type</span> <span class="t">{name}</span> <span class="k">struct</span> {{' if lang=='go' else f'<span class="k">export interface</span> <span class="t">{name}</span> {{')
    R=[('add' if isnew else 'hdr',head)]
    stats=dict(add=0,rem=0,typ=0,enum=0,txt=0)
    enums=[]
    ind=lambda s:'    '+s
    for k in keys:
        o=(old or {}).get(k);n=(new or {}).get(k)
        if o is None:
            stats['add']+=1
            R+=[('add',ind(h)) for h in plain(n['desc'])]+[('add',code(n))]
        elif n is None:
            stats['rem']+=1
            R+=[('del',ind(h)) for h in plain(o['desc'])]+[('del',code(o))]
        else:
            sc=(o['go'][1:]!=n['go'][1:]) if lang=='go' else o['ts']!=n['ts']
            if o['desc']!=n['desc']:
                stats['txt']+=1
                R+=[(t,ind(h)) for t,h in desc_rows(o['desc'],n['desc'])]
            if sc:
                stats['typ']+=1;R+=[('del',code(o)),('add',code(n))]
            else: R.append(('ctx',code(n)))
        for x,y in ((o,n),):
            a=(x or {}).get('enum');b=(y or {}).get('enum')
            if x and y and a and b and a!=b: enums.append((y['enumname'],a,b))
    R.append(('add' if isnew else 'hdr','}'))
    for en,a,b in enums:
        stats['enum']+=len(set(a)^set(b))
        R.append(('hdr',''))
        R.append(('hdr',f'<span class="k">const</span> (' if lang=='go' else f'<span class="k">export type</span> <span class="t">{en}</span> ='))
        order=list(a)+[v for v in b if v not in a]
        for v in order:
            t='del' if v not in b else ('add' if v not in a else 'ctx')
            l=(f'    <span class="n">{E(en+camel(v))}</span> <span class="t">{en}</span> = <span class="s">"{E(v)}"</span>' if lang=='go' else f'    | <span class="s">"{E(v)}"</span>')
            R.append((t,l))
        R.append(('hdr',')' if lang=='go' else ';'))
    return R,stats
def fold(R,ctx=2):
    keep=[r[0] in('add','del','hdr') for r in R]
    ch=[i for i,r in enumerate(R) if r[0] in('add','del')]
    for i in ch:
        for j in range(max(0,i-ctx),min(len(R),i+ctx+1)): keep[j]=True
    out=[];i=0
    while i<len(R):
        if keep[i]: out.append(R[i]);i+=1;continue
        j=i
        while j<len(R) and not keep[j]: j+=1
        if j-i<3: out+=R[i:j]
        else: out.append(('fold',R[i:j]))
        i=j
    return out
SIGN={'add':'+','del':'−','ctx':' ','hdr':' '}
def rows(R):
    o=[]
    for r in fold(R):
        if r[0]=='fold':
            n=sum(1 for x in r[1] if '<span class="n">' in x[1] or x[1].lstrip().startswith('| '))
            noun='value' if any('json:' not in x[1] and '="s">"' in x[1] for x in r[1]) else 'field'
            inner=''.join(f'<div class="l ctx"><span class="g"> </span><span class="x">{h}</span></div>' for _,h in r[1])
            o.append(f'<details class="fold"><summary><span class="g">⋯</span><span class="x">{n} unchanged {noun}{"s" if n!=1 else ""}</span></summary>{inner}</details>')
        else: o.append(f'<div class="l {r[0]}"><span class="g">{SIGN[r[0]]}</span><span class="x">{r[1] or " "}</span></div>')
    return ''.join(o)
def card(name,a,b,note):
    S0,S1=spec(a),spec(b)
    old,new=fields(S0,name),fields(S1,name)
    rg,st=build(name,old,new,'go'); rt,_=build(name,old,new,'ts')
    p=[]
    if old is None: p.append(f'<span class="p new">New type · {len(new)} fields</span>')
    else:
        if st['add']:p.append(f'<span class="p add">+{st["add"]} field{"s"*(st["add"]>1)}</span>')
        if st['rem']:p.append(f'<span class="p del">−{st["rem"]} field{"s"*(st["rem"]>1)}</span>')
        if st['enum']:p.append(f'<span class="p del">−{st["enum"]} enum values</span>')
        if st['typ']:p.append(f'<span class="p mod">{st["typ"]} retyped</span>')
        if st['txt']:p.append(f'<span class="p mod">{st["txt"]} description{"s"*(st["txt"]>1)}</span>')
    return f'''<article class="card"><header><h3>{name}</h3><div class="pills">{"".join(p)}</div></header><p class="note">{E(note)}</p>
<div class="code go"><div class="lines">{rows(rg)}</div></div><div class="code ts"><div class="lines">{rows(rt)}</div></div></article>'''
fmt=lambda d:f'{d[:4]}-{d[4:6]}-{d[6:]}'
GROUPS=[('20261106','20261107',[('GiftGoodwill','Four boss-flagship outcomes dropped from the enum, plus the matching wording.'),('FacilityEntry','One word-level wording change on a field that stays the same.')]),
('20261102','20261106',[('EmpireGiftResponse','New bounty field, and the wording of six related fields updated to match.'),('FacilityEntry','The upkeep note that now covers markets burning trade_authenticator.'),('FactionInfoResponse','New field with no description in the spec.'),('Notification_pirate_radio','A required field removed.')]),
('20261027','20261102',[('GetBaseResponse','Three station tourism fields, all already seen in the Cloverfield log.')]),
('20261021','20261027',[('PirateRecoveryNeeds','A whole new type shows as one green block.')]),
('20260914','20260918',[('FacilityFactionOwnedResponse','Rent details replaced by a single nested block: adds and removes together.')])]
body=''
for a,b,items in GROUPS:
    body+=f'<section><h2><time>{fmt(a)}</time><span>→</span><time>{fmt(b)}</time></h2>'+''.join(card(n,a,b,t) for n,t in items)+'</section>'
tpl=open(sys.argv[1]).read()
open(sys.argv[2],'w').write(tpl.replace('<!--BODY-->',body))
print(len(body))
