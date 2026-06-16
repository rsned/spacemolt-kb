# Per-Entity Portrait Override System — Design

**Goal:** Replace the scattered, brittle portrait-override surfaces with one
authoritative per-entity `override.json` that survives DB re-scrapes and
classifier reruns, and that can override every portrait dimension (archetype,
gender, appearance/ethnicity, species, bio, visual cue) for both passengers and
players/agents.

## Problem

Overrides today live in three places with different lifecycles:

- `overlays/portrait_overrides.json` — visual-cue strings only, keyed by id.
- `overlays/generated/archetypes.json` — archetype, but a **machine-generated
  cache keyed by bio SHA**. Manual corrections (e.g. `the_physician`→medic,
  `synai_quorure`→technician) are silently overwritten by the LLM whenever a
  scrape changes the bio (the bio-SHA no longer matches, so it re-queries).
- gender (from bio pronouns), ethnicity/physique (hashed from id), bio (DB),
  species — **no override path at all**.

Result: hand corrections are fragile and several dimensions can't be fixed.

## Design

### Location & format

One hand-authored file per entity, **outside** the generated cache so nothing
ever wipes it:

- Passengers: `overlays/passengers/<citizen_id>/override.json`
- Players/agents: `overlays/players/<player_id>/override.json`

(These dirs already hold contributor overlay content — markdown + images — so the
override sits naturally beside them.)

```json
{
  "archetype":  "medic",
  "gender":     "woman",
  "appearance": "deep teal grown chitin, faintly glowing amber seams",
  "species":    "voidborn",
  "bio_append": "keeps a worn brass locket from a lost ship",
  "visual_cue": "wearing a worn black leather eyepatch over one eye"
}
```

All fields optional. An absent file or absent field falls through to today's
behavior exactly.

### Field semantics & precedence

For each dimension: **override (if non-empty) → DB/classifier → deterministic
default**.

| Field | Overrides | Falls through to |
|-------|-----------|------------------|
| `archetype` | the classifier (`archetypes.json`) | LLM classification |
| `gender` | `bioGenderNoun(bio)` | bio-pronoun inference (`man`/`woman`/`person`/`android`) |
| `appearance` | `physicalTraits(id)` (skin/age/build/ethnicity) | id-hashed traits |
| `species` | injects an alien-form clause (e.g. `voidborn` → grown crystalline-organic descriptor); empty = human | (none) |
| `bio_append` | appended as one extra sentence to the bio | (just the DB bio) |
| `visual_cue` | the prominent cue injected after appearance | (none) — replaces `portrait_overrides.json` |

### Code

- New `PortraitOverride` struct + `loadPortraitOverride(dir)` reading
  `<dir>/override.json` (missing/malformed → zero value, treated as no override).
- `buildPortraitPrompt` takes a resolved override and applies precedence per
  field. `generateAgentPortraitPrompt` (players) does the same.
- `archetypes.json` stays as the classifier cache; the override wins when set.
- Prompt-hash cache needs no change — override fields feed the prompt, so a
  changed override naturally busts that entity's cache and regenerates it.

### Migration (one-time)

- `portrait_overrides.json` → `overlays/passengers/<id>/override.json`
  with `{visual_cue: …}` for `one_eye_oksana`, `ziggy_stardrift`, `adena_vantow`.
- Manual archetype corrections → `{archetype: …}` for `the_physician` (medic),
  `synai_quorure` (technician).
- Retire `loadPortraitOverrides` / `portrait_overrides.json`.

### Tests

- Per-field precedence (override beats DB/classifier; absent falls through).
- `bio_append` appends exactly one sentence.
- Missing/malformed `override.json` → no-op.
- Player/agent path honors the same override file.

## Out of scope

- A UI/editor for overrides (files are hand-edited).
- Rendering full non-human Voidborn *forms* (the `species` clause is a styling
  nudge on a portrait, not the concept-gallery generative work).
