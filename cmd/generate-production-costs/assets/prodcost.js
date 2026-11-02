'use strict';
// Production-cost recalculation for the private production-costs page.
//
// Loaded as a classic script by private/production-costs/index.html and as a
// CommonJS module by tests/js/prodcost.test.js. Everything above the DOM
// section is pure: no DOM, no globals, no fetch.
//
// total() mirrors prodcost.CostUnder and rank() mirrors prodcost.Opportunities
// in pkg/prodcost. At the default settings (rawMode "mine", every fuel 0,
// priceUnpricedAtAsk false, no choices) both must reproduce Go's numbers
// exactly; tests/js/prodcost.test.js checks that against
// pkg/prodcost/testdata/js-golden.json.

const DEFAULT_SETTINGS = Object.freeze({
  rawMode: 'mine', haulFuelPerJump: 0, mineFuelPerUnit: 0, priceUnpricedAtAsk: false, qty: 1,
});

function withDefaults(settings) {
  return Object.assign({}, DEFAULT_SETTINGS, settings || {});
}

// hops mirrors HaulParams.hops: 0 within a known system, else the stored
// pair in either order; undefined when unknown.
function hops(data, a, b) {
  if (a === b) return a ? 0 : undefined;
  const ab = data.hops[a];
  if (ab && b in ab) return ab[b];
  const ba = data.hops[b];
  if (ba && a in ba) return ba[a];
  return undefined;
}

// routeValue is how a choice names a route: its label, or "recipe/label"
// when another route of the item has the same label.
function routeValue(item, r) {
  const dup = item.routes.filter((o) => o.label === r.label).length > 1;
  return dup ? `${r.recipe}/${r.label}` : r.label;
}

// chosenRoute is the index of the route a choice names, or -1.
function chosenRoute(item, choice) {
  if (!choice || !item || !item.routes) return -1;
  return item.routes.findIndex((r) => r.label === choice || `${r.recipe}/${r.label}` === choice);
}

// buyNow walks the item's ask levels cheapest first for qty units, skipping
// stations with no known route home. credits is per unit bought: the average
// price plus haulFuelPerJump x hops for each station used, spread over the
// units. short is true when the book runs out; nothing filled is partial.
function buyNow(data, itemId, qty, s) {
  const asks = (data.items[itemId] && data.items[itemId].asks) || [];
  let filled = 0;
  let spent = 0;
  const used = new Map();
  for (const a of asks) {
    if (filled >= qty) break;
    if (a.hops < 0) continue;
    const take = Math.min(a.qty, qty - filled);
    if (take <= 0) continue;
    filled += take;
    spent += take * a.price;
    used.set(a.station, a.hops);
  }
  for (const h of used.values()) spent += s.haulFuelPerJump * h;
  if (filled <= 0) return {credits: 0, ticks: 0, partial: true, short: true};
  return {credits: spent / filled, ticks: 0, partial: false, short: filled < qty};
}

function hasAsks(data, itemId) {
  const it = data.items[itemId];
  return !!(it && it.asks && it.asks.length);
}

const UNPRICED = Object.freeze({credits: 0, ticks: 0, partial: true, short: false});

// inputCost costs one unit of an input when qty units are needed, as
// costUnder's loop body does, plus the page's settings and choices.
function inputCost(data, itemId, qty, policy, ctx, stack) {
  const it = data.items[itemId];
  const choice = ctx.choices[itemId];
  if (it && it.raw) {
    const mode = choice === 'mine' || choice === 'buy' ? choice : ctx.s.rawMode;
    if (mode === 'buy') return buyNow(data, itemId, qty, ctx.s);
    return {credits: ctx.s.mineFuelPerUnit, ticks: 0, partial: false, short: false};
  }
  if (stack.has(itemId)) return UNPRICED;
  if (choice === 'buy') return buyNow(data, itemId, qty, ctx.s);
  let idx = chosenRoute(it, choice);
  if (idx < 0) {
    if (!it || !it.routes || !it.routes.length || it.cheapest < 0) {
      if (ctx.s.priceUnpricedAtAsk && hasAsks(data, itemId)) return buyNow(data, itemId, qty, ctx.s);
      return UNPRICED;
    }
    idx = policy === 'fastest' ? it.fastest : it.cheapest;
  }
  return routeCost(data, itemId, it.routes[idx], qty, policy, ctx, stack);
}

// routeCost costs one unit of itemId made on route r (costUnder).
function routeCost(data, itemId, r, qty, policy, ctx, stack) {
  if (!(r.out > 0)) return UNPRICED;
  let credits = (r.labor + r.fee) / r.out;
  let ticks = r.ticks / r.out;
  let partial = false;
  let short = false;
  stack.add(itemId);
  for (const inp of r.inputs) {
    const c = inputCost(data, inp.item, inp.qty * qty, policy, ctx, stack);
    credits += inp.qty * c.credits;
    ticks += inp.qty * c.ticks;
    partial = partial || c.partial;
    short = short || c.short;
  }
  stack.delete(itemId);
  return {credits, ticks, partial, short};
}

// topCost costs qty units of itemId on the chosen top route (or buy), else
// on the policy's route, else the first route, as the Go page's totals do.
function topCost(data, itemId, choices, s, policy) {
  const ctx = {choices: choices || {}, s};
  const it = data.items[itemId];
  const choice = ctx.choices[itemId];
  const routed = it && it.routes && it.routes.length;
  if (!routed || choice === 'buy' || choice === 'mine') return inputCost(data, itemId, s.qty, policy, ctx, new Set());
  let idx = chosenRoute(it, choice);
  if (idx < 0) idx = Math.max(policy === 'fastest' ? it.fastest : it.cheapest, 0);
  return routeCost(data, itemId, it.routes[idx], s.qty, policy, ctx, new Set());
}

// total costs one unit of itemId (or settings.qty units, for buy-now depth)
// under choices and settings: {credits, ticks, partial, short}.
function total(data, itemId, choices, settings) {
  return topCost(data, itemId, choices, withDefaults(settings), 'cheapest');
}

// ---------------------------------------------------------------------------
// Opportunities
// ---------------------------------------------------------------------------

// candidates lists the top route x policy pairs an opportunity may use, in
// Go's order: every usable route, cheapest then fastest. A top choice
// narrows them to that route (or to buying the item).
function candidates(data, itemId, qty, ctx) {
  const it = data.items[itemId];
  const choice = ctx.choices[itemId];
  const out = [];
  if (choice === 'buy') {
    const c = buyNow(data, itemId, qty, ctx.s);
    return [Object.assign(c, {route: 'buy', policy: 'buy', kind: 'hand', sys: data.home})];
  }
  const chosen = chosenRoute(it, choice);
  it.routes.forEach((r, i) => {
    if (chosen >= 0 ? i !== chosen : !r.usable) return;
    for (const policy of ['cheapest', 'fastest']) {
      const c = routeCost(data, itemId, r, qty, policy, ctx, new Set());
      out.push(Object.assign(c, {route: r.label, policy, kind: r.kind, sys: r.sys}));
    }
  });
  return out;
}

// haulJumps mirrors haulJumps in opportunities.go.
function haulJumps(data, c, bidSys) {
  if (!bidSys) return undefined;
  if (c.kind === 'hand') {
    const d = hops(data, data.home, bidSys);
    return d === undefined ? undefined : 2 * d;
  }
  if (!c.sys) return undefined;
  const a = hops(data, data.home, c.sys);
  const b = hops(data, c.sys, bidSys);
  const d = hops(data, bidSys, data.home);
  return a === undefined || b === undefined || d === undefined ? undefined : a + b + d;
}

// evaluate mirrors evaluate in opportunities.go, with haul fuel spread over
// the depth added to the cost.
function evaluate(data, bid, size, c, s) {
  const o = {item: bid.item, station: bid.station, bid: bid.price, depth: bid.qty,
    route: c.route, policy: c.policy, partial: c.partial, short: c.short, unreachable: false,
    makeHours: 0, haulHours: 0, hours: 0, profitPerHour: 0, jumps: 0, trips: 0};
  const tick = data.tickSeconds;
  o.makeHours = o.depth * c.ticks * tick / 3600;
  const jumps = haulJumps(data, c, bid.sys);
  if (jumps === undefined) {
    o.unreachable = true;
    o.cost = c.credits;
    o.margin = o.bid - o.cost;
    o.total = o.margin * o.depth;
    return o;
  }
  const trips = data.hold > 0 ? Math.max(1, Math.ceil(o.depth * size / data.hold)) : 1;
  o.jumps = jumps;
  o.trips = trips;
  o.cost = c.credits + trips * jumps * s.haulFuelPerJump / o.depth;
  o.margin = o.bid - o.cost;
  o.total = o.margin * o.depth;
  o.haulHours = trips * jumps * data.jumpTicks * tick / 3600;
  o.hours = o.makeHours + o.haulHours;
  o.profitPerHour = o.total / Math.max(o.hours, tick / 3600);
  return o;
}

// better mirrors better in opportunities.go.
function better(a, b) {
  if (a.unreachable !== b.unreachable) return !a.unreachable;
  if (a.partial !== b.partial) return !a.partial;
  if (a.unreachable) return a.total > b.total;
  return a.profitPerHour > b.profitPerHour;
}

// compareRows is the page's default order: reachable first, complete before
// partial, then profit/hr descending; unreachable rows by total.
function compareRows(a, b) {
  if (a.unreachable !== b.unreachable) return a.unreachable ? 1 : -1;
  if (a.unreachable) return b.total - a.total;
  if (a.partial !== b.partial) return a.partial ? 1 : -1;
  return b.profitPerHour - a.profitPerHour;
}

// rank returns one row per bid level with its best candidate, sorted by
// compareRows.
function rank(data, choices, settings) {
  const s = withDefaults(settings);
  const ctx = {choices: choices || {}, s};
  const out = [];
  for (const bid of data.bids) {
    const it = data.items[bid.item];
    if (!it || !it.routes || it.cheapest < 0) continue;
    let best = null;
    for (const c of candidates(data, bid.item, bid.qty, ctx)) {
      const o = evaluate(data, bid, it.size, c, s);
      if (best === null || better(o, best)) best = o;
    }
    if (best) out.push(best);
  }
  return out.sort(compareRows);
}

// ---------------------------------------------------------------------------
// Trees
// ---------------------------------------------------------------------------

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[c]));
}

// num formats v with up to 2 decimals, like num in render.go.
function num(v) {
  return String(Math.round(v * 100) / 100);
}

// stepOptions lists a tree step's <select> options; "" follows the settings
// (raw) or the policy's route (everything else).
function stepOptions(data, itemId) {
  const it = data.items[itemId] || {};
  const buy = hasAsks(data, itemId) ? [{value: 'buy', text: 'buy now'}] : [];
  if (it.raw) return [{value: '', text: 'raw: per settings'}, {value: 'mine', text: 'mine'}, ...buy];
  if (!it.routes || !it.routes.length) return [{value: '', text: 'unpriced (0)'}, ...buy];
  const opts = [{value: '', text: 'cheapest'}, ...buy];
  for (const r of it.routes) {
    opts.push({value: routeValue(it, r), text: r.usable ? r.label : `${r.label} (unbuilt)`});
  }
  return opts;
}

function selectHTML(data, itemId, choices) {
  const cur = choices[itemId] || '';
  const opts = stepOptions(data, itemId).map((o) =>
    `<option value="${escapeHTML(o.value)}"${o.value === cur ? ' selected' : ''}>${escapeHTML(o.text)}</option>`);
  return `<select class="pc-step" data-item="${escapeHTML(itemId)}">${opts.join('')}</select>`;
}

// stepRoute is the route a tree step expands under choices, or null.
function stepRoute(data, itemId, choices) {
  const it = data.items[itemId];
  const choice = choices[itemId];
  if (!it || it.raw || choice === 'buy' || !it.routes || !it.routes.length) return null;
  const idx = chosenRoute(it, choice);
  if (idx >= 0) return it.routes[idx];
  return it.cheapest >= 0 ? it.routes[it.cheapest] : null;
}

// treeHTML renders itemId's tree under choices: a <select> per step, each
// item expanded once.
function treeHTML(data, itemId, choices, settings) {
  const s = withDefaults(settings);
  const seen = new Set();
  const node = (id, qty) => {
    const it = data.items[id] || {name: id};
    const c = topCost(data, id, choices, s, 'cheapest');
    const mark = c.partial ? ' <span class="note">partial</span>' : '';
    const head = `${qty === null ? '' : num(qty) + ' &times; '}${escapeHTML(it.name)} ${selectHTML(data, id, choices)}` +
      ` <span class="pc-cost">${num(c.credits)} cr/u &middot; ${num(c.ticks)} t/u</span>${mark}`;
    const r = stepRoute(data, id, choices);
    if (!r || !r.inputs.length) return `<li>${head}</li>`;
    if (seen.has(id)) return `<li>${head} <span class="note">expanded above</span></li>`;
    seen.add(id);
    return `<li>${head}<ul>${r.inputs.map((inp) => node(inp.item, inp.qty)).join('')}</ul></li>`;
  };
  return `<ul class="pc-tree">${node(itemId, null)}</ul>`;
}

// ---------------------------------------------------------------------------
// DOM
// ---------------------------------------------------------------------------

function oppRowHTML(data, o) {
  const name = (data.items[o.item] || {name: o.item}).name;
  const badge = o.partial ? ' <span class="pc-badge">partial — unpriced inputs at 0</span>' : '';
  const short = o.short ? ' <span class="pc-badge">short</span>' : '';
  const hours = o.unreachable ? 'unreachable' : num(o.hours);
  const pph = o.unreachable ? 'unreachable' : num(o.profitPerHour);
  return `<tr${o.partial ? ' class="pc-partial"' : ''}><td>${escapeHTML(name)}${badge}${short}</td>` +
    `<td>${escapeHTML(o.station)}</td><td>${num(o.bid)}</td><td>${num(o.depth)}</td><td>${num(o.cost)}</td>` +
    `<td>${num(o.margin)}</td><td>${num(o.total)}</td><td>${hours}</td><td>${pph}</td>` +
    `<td>${escapeHTML(o.route)}</td><td>${escapeHTML(o.policy)}</td></tr>`;
}

function initPage() {
  const el = document.getElementById('pc-data');
  if (!el) return;
  const data = JSON.parse(el.textContent);
  const choices = {};
  let sortKey = null;
  let sortDir = -1;
  const $ = (id) => document.getElementById(id);
  const settings = () => ({
    rawMode: $('pc-raw').value,
    haulFuelPerJump: Number($('pc-haul-fuel').value) || 0,
    mineFuelPerUnit: Number($('pc-mine-fuel').value) || 0,
    priceUnpricedAtAsk: $('pc-unpriced').checked,
  });
  const renderOpps = () => {
    const rows = rank(data, choices, settings());
    if (sortKey) {
      rows.sort((a, b) => {
        const x = a[sortKey];
        const y = b[sortKey];
        return sortDir * (typeof x === 'string' ? x.localeCompare(y) : x - y);
      });
    }
    $('pc-opps').innerHTML = rows.map((o) => oppRowHTML(data, o)).join('');
  };
  const renderTree = (det) => {
    det.querySelector('.pc-tree-box').innerHTML = treeHTML(data, det.dataset.item, choices, settings());
    const c = total(data, det.dataset.item, choices, settings());
    det.querySelector('.pc-total').textContent = `${num(c.credits)} cr/u · ${num(c.ticks)} t/u${c.partial ? ' (partial)' : ''}`;
    det.dataset.fresh = '1';
  };
  const renderAll = () => {
    renderOpps();
    document.querySelectorAll('details.pc-item').forEach((det) => {
      if (det.open) renderTree(det);
      else det.dataset.fresh = '';
    });
  };
  document.querySelectorAll('details.pc-item').forEach((det) => {
    // The generated tree's selects hold only their default option, so a
    // detail is rendered here the first time it opens and after any change.
    det.addEventListener('toggle', () => { if (det.open && det.dataset.fresh !== '1') renderTree(det); });
  });
  document.addEventListener('change', (ev) => {
    const t = ev.target;
    if (t.classList && t.classList.contains('pc-step')) {
      if (t.value) choices[t.dataset.item] = t.value;
      else delete choices[t.dataset.item];
    } else if (!t.closest || !t.closest('#pc-settings')) {
      return;
    }
    renderAll();
  });
  $('pc-reset').addEventListener('click', () => {
    for (const k of Object.keys(choices)) delete choices[k];
    renderAll();
  });
  document.querySelectorAll('#pc-opp-table th[data-key]').forEach((th) => {
    th.addEventListener('click', () => {
      const k = th.dataset.key;
      sortDir = sortKey === k ? -sortDir : (k === 'item' || k === 'station' || k === 'route' ? 1 : -1);
      sortKey = k;
      renderOpps();
    });
  });
  renderOpps();
}

if (typeof document !== 'undefined') {
  document.addEventListener('DOMContentLoaded', initPage);
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    total, rank, buyNow, hops, stepOptions, treeHTML, compareRows, routeValue,
  };
} else if (typeof window !== 'undefined') {
  window.ProdCost = {total, rank, treeHTML};
}
