# Battle-mechanics analysis scripts (2026-08-30/31 session)

Scripts that derived and verify the damage-mitigation model recorded in the
combat-mechanics memory note and `data/battles/README.md`. Observations are
hardcoded from fixtures `509e1ef4`, `b7847bbc` (Nekkar), `7c044558`.

- `solver.py`, `solver2.py` — brute-force enumeration of the mitigation
  pipeline space (stage order, floor placement, breakthrough conversion,
  armor form). Historical; kept to show what was ruled out.
- `verify3.py` — first regen-net model (10/17 exact).
- `verify4.py` — adds the fractional spill rule (14/17 exact; the 3 misses
  are the broadaxe armor rows).
- `verify5_armor_cap.py` — armor cap at floor(pre_hit*0.125) (8/11 armor
  rows exact, all within ±1; residual: the ±1 broadaxe alternation).

Run with `python3 <script>`; no dependencies.
- `verify6_percentage_armor.py` — dev-bot model: armor as saturating
  percentage f = a_counted/(a_counted+150), type mult on armor counted
  (kinetic ×1.5, energy ×0.75). Exact on kinetic rows, ±1 on broadaxe;
  low-armor energy rows still prefer flat floor(a×0.75) — law open.
