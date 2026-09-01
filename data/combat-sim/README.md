# combat-sim

Hermetic 1v1 combat Monte Carlo over the log-verified damage model
(spec: docs/superpowers/specs/2026-08-31-combat-sim-design.md).

    go build -o bin/combat-sim ./cmd/combat-sim
    bin/combat-sim --a data/combat-sim/fits/molten_broadaxe.json \
                   --b data/combat-sim/fits/artis_survey.json --runs 10000

Inputs: a vendored, pinned catalog snapshot (data/combat-sim/catalog/, copied
from snapshot 20260827) + two fitting-spec JSONs + calibration.json. No
databases, no network, no credentials. The spec's `data/snapshots/latest`
default was changed to the vendored copy for hermeticity — `data/snapshots/`
is gitignored, so a fresh clone has no catalog there and tests would fail.

Stances: brace = 75% incoming reduction and weapons DOWN (measured: 513
braced ticks across seven ships in the Haven fixture, zero shots fired);
flee likewise never fires. Brace is a turtle, not a fighting stance.

Measured vs ASSUMED: every uncalibrated constant lives in
data/combat-sim/calibration.json with an `assumed` list; table cells that
depend on one are marked `*`. Phase B (scripted stance-pair duels between
owned agents) exists to measure evade_in_mult, flee_escape_per_tick, and
per-pair hit chances.

Not modeled in v1: drone repair (logs omit it — drone-fit survival is
underestimated), boarding, zones/movement (fixed at engaged), ammo reload,
armor-melt and EM debuffs, capital hulls, wildlife (phase C). Mixed-damage-type
fits are refused outright by the resolver (not silently mis-resolved as a
single type). Gunnery is applied to all damage types as a v1 approximation —
the real per-type skill mapping is unknown. Typed hardener resists are
omitted: no fixture or example fit carries one, so the model is unverified for
them. The spec's `--hit-chance` sweep flag is deferred to phase B; edit
hit_chance_a/hit_chance_b in calibration.json directly instead.
