# Passenger empire inference from names — design

**Date:** 2026-06-16
**Status:** approved (design)

## Problem

A batch of ~1447 passenger records was imported from a source that did not
collect the `citizenship` (empire) field, leaving them with `NULL` citizenship
in `spacemolt-knowledge.db` (`passengers` table). Only 257 passengers carry a
labeled empire: crimson 89, nebula 56, outerrim 40, solarian 30, voidborn 28,
pirates 14.

The portrait pipeline (`cmd/generate-factions-kb`) styles each passenger by
`citizenship` (empire = palette/cut sensibility in `buildPortraitPrompt`). With
empty citizenship, all ~1447 new passengers fall back to the generic "practical
spacer style", wasting the empire-variety the portraits are meant to show.

The empires have distinct, morphology-driven naming conventions (e.g. Crimson
Germanic/forge — Hammermund, Severholm; Nebula soft/vowel-heavy — Belor,
Fosseno; Solarian Earth-Anglo — Stonefield, Millcott; Outer Rim short/gritty
with quoted nicknames — Sgt. Blunt, Gus 'Cinch'; Voidborn alien x/z-heavy —
Echix Hexis, Statyn; Pirates rogue epithets — One-Eye Oksana, Captain Phasgar).
We can infer a likely empire from the name alone.

## Decisions (settled in brainstorming)

1. **Use scope: portraits only.** The inferred empire feeds ONLY the portrait
   prompt's empire styling. The source DB and the visible "faction" on KB
   passenger pages stay untouched / unknown. A wrong guess means slightly-off
   attire, never a false published fact.
2. **Always assign a best guess.** Every unlabeled passenger gets its single
   most-likely empire; no confidence threshold gating (low stakes).
3. **Method: character n-gram Naive Bayes.** Deterministic, instant, GPU-free
   (leaves the GPU for the portrait regen), reproducible, and validatable with
   leave-one-out cross-validation before we trust it. Chosen over an LLM
   few-shot classifier (slower, GPU-contending, not reproducible, no honest
   accuracy number).

## Component 1: `tools/infer_empire.py`

A standalone tool, pure Python stdlib (no sklearn/numpy), runnable with system
`python3`, matching the style of the other `tools/` scripts.

- **Training data:** passengers with a non-empty `citizenship`, read from the
  canonical DB (`/home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db`),
  labeled by their 6 empires.
- **Features:** character n-grams of the full `name` (n = 2–4) with explicit
  start/end markers. Titles and quoted nicknames are KEPT (they are themselves
  empire signal — "Dr."/"Brother" → Solarian, "Captain"/"One-Eye" → pirates).
- **Model:** multinomial Naive Bayes with Laplace (add-1) smoothing, computed in
  log-space (dict of n-gram counts per class + class priors). Prediction returns
  the argmax empire and a normalized posterior as the confidence.
- **Validation (runs first, always printed):** leave-one-out cross-validation
  over the labeled set — overall accuracy plus a per-empire confusion matrix.
  This is the honesty gate: if the model cannot recover the labels it trained
  on, we rethink before predicting.
- **Prediction:** for every passenger with empty/`NULL` citizenship, assign the
  best-guess empire.
- **Output:** `overlays/generated/empire_guess.json`, keyed by `citizen_id`:
  `{ "<id>": { "empire": "<empire>", "confidence": <0..1>, "name": "<name>" } }`.
  Mirrors `overlays/generated/archetypes.json`. Only contains the unlabeled
  passengers (labeled ones already have authoritative DB citizenship).
- **CLI:** `python3 tools/infer_empire.py [db] [--no-write]` — `--no-write`
  prints the cross-validation report + a prediction distribution without writing
  the JSON (dry inspection).

## Component 2: portrait wiring in `cmd/generate-factions-kb`

- **`loadEmpireGuess(root) map[string]string`** — a loader parallel to
  `loadArchetypes` (`archetypes.go`), reading `generated/empire_guess.json` and
  returning `citizen_id → empire`. A missing file yields an empty map (the
  feature is an optional enrichment, never a hard dependency).
- **Resolution at the portrait call site only.** In `generatePassengerPortraits`
  (`portraits.go`), where the prompt is built, resolve an effective citizenship:
  `cit := p.Citizenship; if strings.TrimSpace(cit) == "" { cit = guess[p.ID] }`,
  and pass `cit` to `buildPortraitPrompt`. The guess map is loaded once alongside
  `loadArchetypes`.
- **Scope guard:** the fallback is applied ONLY in the portrait-generation path.
  `loadPassengers`, the passenger page rendering, and the DB are untouched, so
  the displayed faction stays empty/unknown. Existing labeled passengers are
  unaffected (their `p.Citizenship` is non-empty, so the guess is never
  consulted).

## Data flow

```
DB passengers (citizenship: 257 labeled, 1447 NULL)
   │
   ├─ infer_empire.py  ── train NB on 257 ── LOO cross-val report
   │                   └─ predict 1447 ──▶ overlays/generated/empire_guess.json
   │
   └─ generate-factions-kb -portraits
          loadArchetypes() + loadEmpireGuess()
          per passenger: cit = p.Citizenship or guess[id]
          buildPortraitPrompt(..., cit, ...) ──▶ empire-styled portrait
```

## Testing

- `infer_empire.py`: leave-one-out cross-validation is the built-in accuracy
  check (printed every run). A small unit check on the NB scorer (a few
  hand-labeled obvious names classify correctly) guards regressions.
- Go side: a unit test for `loadEmpireGuess` (missing file → empty map; present
  file → correct mapping) and for the effective-citizenship fallback (empty
  citizenship + guess present → guessed empire; non-empty citizenship → DB value
  wins; no guess → empty).

## Out of scope

- Writing the guess back to the DB or showing it as the visible faction.
- Re-inferring archetypes (already done) or any change to labeled passengers.
- Confidence-threshold gating / manual review UI (we always assign best guess;
  the stored confidence is retained for possible later review only).
