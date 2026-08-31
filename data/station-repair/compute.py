import json, math, sqlite3, statistics
from collections import defaultdict, Counter

CAT = '/home/robert/spacemolt/kb/data/snapshots/latest/catalog_facilities.json'
cat = {e['id']: e for e in json.load(open(CAT))['items']}
damaged = json.load(open('damaged.json'))['repairs']['facilities']
levels = json.load(open('levels.json'))

issues = []
def chain(defid):
    """Walk upgrades_from down to base; return list base-first."""
    seen, out, cur = set(), [], defid
    while cur:
        if cur in seen: issues.append(f'CYCLE at {cur}'); break
        seen.add(cur)
        e = cat.get(cur)
        if not e: issues.append(f'MISSING catalog entry: {cur}'); break
        out.append(e)
        cur = e.get('upgrades_from')
    return list(reversed(out))

per_fac = []          # rows per damaged instance
bill = Counter()      # aggregated 30% materials (ceil per facility per item)
bill_raw = defaultdict(float)
credits30 = 0.0

for f in damaged:
    ch = chain(f['definition_id'])
    top = cat.get(f['definition_id'], {})
    lvl_station = levels.get(f['instance_id'])
    lvl_cat = top.get('level')
    if lvl_station != lvl_cat:
        issues.append(f"LEVEL MISMATCH {f['definition_id']} ({f['instance_id'][:8]}): station={lvl_station} catalog={lvl_cat} chain_len={len(ch)}")
    if len(ch) != (lvl_cat or 0):
        issues.append(f"CHAIN/LEVEL note {f['definition_id']}: catalog level {lvl_cat} but chain has {len(ch)} entries ({' -> '.join(e['id'] for e in ch)})")
    mats = Counter(); cost = 0
    for e in ch:
        cost += e.get('build_cost') or 0
        for m in e.get('build_materials') or []:
            mats[m['item_id']] += m['quantity']
    fac_bill = {k: math.ceil(v*0.30) for k, v in mats.items()}
    for k, v in fac_bill.items(): bill[k] += v
    for k, v in mats.items(): bill_raw[k] += v*0.30
    credits30 += cost*0.30
    per_fac.append({'name': f['name'], 'def': f['definition_id'], 'inst': f['instance_id'][:8],
                    'level': lvl_cat, 'chain': [e['id'] for e in ch],
                    'full_cost': cost, 'repair_credits_30pct': round(cost*0.30),
                    'repair_materials': fac_bill})

# ---- pricing from crafting.db ----
con = sqlite3.connect('/home/robert/spacemolt/kb/crafting.db')
names = dict(con.execute('SELECT id,name FROM items'))
price = {}
for iid in bill:
    rows = [r[0] for r in con.execute(
        "SELECT avg_price_7d FROM market_price_summary WHERE item_id=? AND price_type='sell' AND avg_price_7d IS NOT NULL AND avg_price_7d < 900000", (iid,))]
    if rows: price[iid] = statistics.median(rows)
    else:
        bv = con.execute('SELECT base_value FROM items WHERE id=?', (iid,)).fetchone()
        price[iid] = (bv[0] if bv else 0) or 0

out = {'per_facility': per_fac, 'aggregate_materials_30pct': dict(bill),
       'aggregate_materials_30pct_raw': {k: round(v,1) for k,v in bill_raw.items()},
       'credits_30pct_of_build_cost': round(credits30), 'issues': issues}
json.dump(out, open('result.json','w'), indent=1)

print(f"facilities: {len(per_fac)}   issues: {len(issues)}")
for i in issues: print('  !', i)
print(f"\n30% of summed build_cost (credits): {credits30:,.0f}")
total_px = 0
print(f"\n{'item':34s} {'qty(30%)':>9s} {'unit px':>10s} {'est value':>14s}")
for iid, q in sorted(bill.items(), key=lambda kv: -kv[1]*price.get(kv[0],0)):
    v = q*price[iid]; total_px += v
    print(f"{names.get(iid,iid):34s} {q:9d} {price[iid]:10,.1f} {v:14,.0f}")
print(f"\nestimated market value of material bill: {total_px:,.0f} cr")
