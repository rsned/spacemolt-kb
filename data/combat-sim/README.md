# combat-sim

Hermetic 1v1 combat Monte Carlo over the log-verified damage model
(spec: docs/superpowers/specs/2026-08-31-combat-sim-design.md).

    go build -o bin/combat-sim ./cmd/combat-sim
    bin/combat-sim --a data/combat-sim/fits/molten_broadaxe.json \
                   --b data/combat-sim/fits/artis_survey.json --runs 10000

Inputs: committed catalog snapshots + two fitting-spec JSONs + calibration.json.
No databases, no network, no credentials.

Measured vs ASSUMED: every uncalibrated constant lives in
data/combat-sim/calibration.json with an `assumed` list; table cells that
depend on one are marked `*`. Phase B (scripted stance-pair duels between
owned agents) exists to measure evade_in_mult, flee_escape_per_tick, and
per-pair hit chances.

Not modeled in v1: drone repair (logs omit it — drone-fit survival is
underestimated), boarding, zones/movement (fixed at engaged), ammo reload,
armor-melt and EM debuffs, mixed-damage-type volleys, capital hulls,
wildlife (phase C).
