#!/usr/bin/env python3
"""(Re)build the archetype exploration matrices under voidborn-concepts/.

Five manifests drive the local archetype-review galleries:
  - archetype_matrix.json   rows = role archetype, cols = Voidborn body-form
  - {crimson,nebula,outerrim,solarian}_matrix.json
                            rows = role archetype, cols = gender (m/f)

Each cell is a sample portrait (file, seed, full prompt). This script is the
single source of truth for the row set: it keeps every manifest's archetype
list in sync with the classifier taxonomy (tools/classify_archetypes.py), and
for any (archetype x column) cell that does not yet exist it authors the prompt
in that manifest's established style, renders the image via tools/portrait, and
writes the cell back. Existing cells are left untouched (their seeds/prompts —
and any hand-picked favorites — are stable), so this is safe to re-run after the
taxonomy grows.

    python3 tools/build_archetype_matrices.py [--dry-run] [--limit N] [--only NAME]

--dry-run authors prompts + patches manifests WITHOUT rendering (fast wiring
check). --limit N renders at most N images this run (resume-friendly). --only
restricts to one manifest stem (e.g. --only crimson, --only archetype).
Run from the kb repo root; the FLUX daemon (tools/portrait) must be reachable.
"""

import argparse
import json
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
CONCEPTS = ROOT / "voidborn-concepts"
PORTRAIT = ROOT / "tools" / "portrait"

# Canonical taxonomy order, mirroring ARCHETYPES in tools/classify_archetypes.py.
# Only the 10 trailing entries are new; the first 12 already exist in every
# manifest and are read back from disk (never re-authored here).
ARCHETYPE_ORDER = [
    "laborer", "officer", "merchant", "official", "technician", "pilot",
    "medic", "outlaw", "spiritual", "aristocrat", "performer",
    "logistician", "engineer", "scientist", "retailer", "cook", "educator",
    "sanitation", "jurist", "journalist", "diplomat", "spacer",
]

LABEL = {
    "logistician": "Logistician", "engineer": "Engineer", "scientist": "Scientist",
    "retailer": "Retailer", "cook": "Cook", "educator": "Educator",
    "sanitation": "Sanitation", "jurist": "Jurist", "journalist": "Journalist",
    "diplomat": "Diplomat",
}

# Voidborn matrix: cell = "{form prefix}, marked as {VOIDBORN_PROP}, {suffix}".
# The role-prop phrasings for the NEW archetypes, in the established style of the
# existing 12 (positive description only — FLUX ignores negation).
VOIDBORN_PROP = {
    "logistician": "a logistician by a shipping-crew harness, a floating cargo manifest and mag-clamp cargo hooks",
    "engineer": "an engineer by a survey scanner, a draughting slate and a structural field-rig",
    "scientist": "a scientist by floating sample vials, an analytic data-slate and fine research instruments",
    "retailer": "a retailer by a vendor's apron-rig, an inventory scanner and stacked trade goods",
    "cook": "a cook by a galley apron, grown cooking implements and ration canisters",
    "educator": "an educator by a lesson-slate, a halo of floating reference glyphs and a teaching pointer",
    "sanitation": "a sanitation worker by a sealed reclamation coverall, a recycler wand and waste-system gauges",
    "jurist": "a jurist by a contract ledger, a seal-stamp and a floating document case",
    "journalist": "a journalist by a press rig, a hovering recorder drone and a notebook of glyphs",
    "diplomat": "a diplomat by a credential sash, an envoy's accord case and floating treaty glyphs",
}

# Per-empire matrix: cell = "{head}{gender}, {empire desc}, working as {EMPIRE_WORK},
# {tail}". Role-work phrasings for the NEW archetypes (article baked in).
EMPIRE_WORK = {
    "logistician": "a freight and logistics coordinator in a practical shipping-crew jacket with a cargo manifest slate and cargo hooks",
    "engineer": "a structural engineer and architect in a professional field-suit with a survey scanner and a draughting slate",
    "scientist": "a research scientist in a field lab coat with sample vials and analytic instruments",
    "retailer": "a shopkeeper in a tidy retail apron with an inventory scanner and stacked goods crates",
    "cook": "a galley cook in a stained chef's apron and whites with cooking utensils",
    "educator": "a teacher and instructor in tidy academic attire with a lesson slate and reference texts",
    "sanitation": "a sanitation and recycling worker in a sealed coverall with a recycler wand and waste-system gauges",
    "jurist": "a legal arbiter in a formal dark suit with a contract ledger and a document case",
    "journalist": "a field correspondent in a rugged press jacket with a recorder drone and a press badge",
    "diplomat": "a diplomatic envoy in refined formal robes with a credential sash and a sealed accord case",
}

# Per-empire constant frame, reconstructed from the existing cells.
EMP_HEAD = "cinematic photorealistic upper-body portrait of a single "
EMP_TAIL = (", dramatic cinematic lighting, dark studio background, highly detailed, "
            "realistic skin texture, sharp focus, photorealistic, cinematic photoreal portrait")
GENDER_WORD = {"m": "man", "f": "woman"}

# Which manifests are empire (gender cols) vs the voidborn (form cols) matrix.
EMPIRE_STEMS = ["crimson", "nebula", "outerrim", "solarian"]


def manifest_path(stem):
    name = "archetype_matrix.json" if stem == "archetype" else f"{stem}_matrix.json"
    return CONCEPTS / name


def voidborn_templates(data):
    """Return (form_prefix_by_slug, suffix) reconstructed from an existing cell."""
    forms = [c["slug"] for c in data["forms"]]
    # Split a known existing cell on its role-prop to isolate prefix/suffix.
    ref_arch = data["archetypes"][0]["key"]  # laborer
    ref_prop = None
    suffix = None
    prefixes = {}
    for f in forms:
        prompt = data["cells"][f"{ref_arch}__{f}"]["prompt"]
        # role segment is "marked as <prop>," — find it via the suffix anchor.
        anchor = ", grown crystalline-organic Voidborn of The Collective"
        head, tail = prompt.split(anchor, 1)
        suffix = anchor.lstrip(", ") + tail
        pre, _prop = head.split(", marked as ", 1)
        prefixes[f] = pre
        ref_prop = _prop
    _ = ref_prop
    return prefixes, suffix


def empire_desc(data):
    """Reconstruct the empire description clause from an existing cell."""
    ref = data["archetypes"][0]["key"]
    prompt = data["cells"][f"{ref}__m"]["prompt"]
    body = prompt[len(EMP_HEAD):-len(EMP_TAIL)]
    _gender, rest = body.split(", ", 1)
    desc, _work = rest.split(", working as ", 1)
    return desc


def voidborn_prompt(prefix, prop, suffix):
    return f"{prefix}, marked as {prop}, {suffix}"


def empire_prompt(desc, work, col):
    return f"{EMP_HEAD}{GENDER_WORD[col]}, {desc}, working as {work}{EMP_TAIL}"


def render(prompt, seed, out_file, dry_run):
    out_path = CONCEPTS / out_file
    if dry_run:
        return
    subprocess.run(
        [str(PORTRAIT), "-s", str(seed), "-o", str(out_path), prompt],
        check=True,
    )


def process(stem, dry_run, limit, rendered):
    path = manifest_path(stem)
    data = json.load(open(path))
    is_empire = stem in EMPIRE_STEMS
    cols = [c["slug"] for c in data.get("columns", data.get("forms", []))]
    existing = {a["key"] for a in data["archetypes"]}

    if is_empire:
        desc = empire_desc(data)
    else:
        prefixes, suffix = voidborn_templates(data)

    # Append any missing archetype rows in canonical order.
    have = {a["key"]: a for a in data["archetypes"]}
    data["archetypes"] = [
        have.get(k, {"key": k, "label": LABEL.get(k, k.capitalize())})
        for k in ARCHETYPE_ORDER
        if k in have or k in VOIDBORN_PROP  # new ones are exactly the 10 keys
    ]

    seeds = [c["seed"] for c in data["cells"].values() if isinstance(c.get("seed"), int)]
    next_seed = max(seeds) + 1

    new_cells = 0
    samples = []
    for art in [a["key"] for a in data["archetypes"]]:
        if art in existing and all(f"{art}__{col}" in data["cells"] for col in cols):
            continue  # fully rendered already
        for col in cols:
            key = f"{art}__{col}"
            if key in data["cells"]:
                continue
            if is_empire:
                prompt = empire_prompt(desc, EMPIRE_WORK[art], col)
                fname = f"emp_{stem}_{art}_{col}.png"
            else:
                prompt = voidborn_prompt(prefixes[col], VOIDBORN_PROP[art], suffix)
                fname = f"ar_{art}_{col}.png"
            if limit is not None and rendered[0] >= limit:
                continue
            seed = next_seed
            next_seed += 1
            if dry_run:
                if len(samples) < 2:
                    samples.append(f"      [{key} seed={seed}] {prompt}")
                new_cells += 1
                rendered[0] += 1
                continue
            render(prompt, seed, fname, dry_run)
            data["cells"][key] = {"file": fname, "seed": seed, "prompt": prompt}
            new_cells += 1
            rendered[0] += 1

    if not dry_run:
        with open(path, "w") as f:
            json.dump(data, f, indent=2)
            f.write("\n")
    for s in samples:
        print(s)
    return new_cells


def build_html(stem):
    out = CONCEPTS / ("archetypes.html" if stem == "archetype" else f"{stem}.html")
    subprocess.run(
        [sys.executable, str(ROOT / "tools" / "build_voidborn_archetype_grid.py"),
         str(manifest_path(stem)), str(out)],
        check=True,
    )


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true", help="author prompts/manifests but do not render")
    ap.add_argument("--limit", type=int, default=None, help="render at most N images this run")
    ap.add_argument("--only", default=None, help="restrict to one manifest stem")
    args = ap.parse_args()

    stems = ["archetype"] + EMPIRE_STEMS
    if args.only:
        stems = [args.only]

    rendered = [0]
    for stem in stems:
        n = process(stem, args.dry_run, args.limit, rendered)
        if not args.dry_run:
            build_html(stem)
        print(f"  {stem:9}: +{n} new cells -> {manifest_path(stem).name}")

    # Rebuild the combined all-empires + Voidborn contact sheet from the manifests.
    if not args.dry_run:
        subprocess.run(
            [sys.executable, str(ROOT / "tools" / "build_archetype_combined.py")],
            check=True,
        )
    print(f"total new cells this run: {rendered[0]}{' (dry-run, not rendered)' if args.dry_run else ''}")


if __name__ == "__main__":
    main()
