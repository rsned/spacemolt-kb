# seed-planet-profiles

Writes per-planet `PlanetProfile` envelope JSON files to disk for use by `pkg/planetgen`. Output is byte-stable: rerunning produces identical bytes.

## Usage

### Explicit list

```
go run ./cmd/tools/seed-planet-profiles \
    -out=data/planet-profiles \
    -planet=terran:terran_default \
    -planet=super_terran:super_terran_default
```

Each `-planet=<type>:<seed>` writes one envelope to `<out>/<slug>.json`, where `<slug>` is the result of `profilejson.PlanetSlug(seed)`.

### Manifest mode

Pass a TSV manifest with one `<type><TAB><name>` per line. Comment lines start with `#`.

```
go run ./cmd/tools/seed-planet-profiles \
    -out=data/planet-profiles \
    -manifest=data/planet-list.tsv
```

### Flags

- `-out` — output directory (default `data/planet-profiles`)
- `-planet` — `<type>:<seed>` (repeatable)
- `-manifest` — path to a TSV manifest
- `-force` — overwrite existing files (default refuses to clobber)

## Idempotency

Without `-force`, the seeder skips any file that already exists. With `-force`, it overwrites, but two `-force` runs with the same inputs produce byte-identical files because `profilejson.Encode` is canonical.

## When to rerun

- After adding a new planet archetype to `pkg/planetgen.Profiles`.
- After tweaking a per-archetype default that an existing envelope mirrors — use `-force`, then commit the diff (the CI drift guard will flag uncommitted drift).

Do **not** rerun against a file whose `handTuned: true` you want to preserve; the drift guard skips hand-tuned files, but `-force` would overwrite the tune.
