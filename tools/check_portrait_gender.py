#!/usr/bin/env python3
"""Check that generated passenger portraits roughly match bio-implied gender.

For each passenger with a generated portrait, derive the expected gender from
the bio's pronouns (whole-word he/him/his/himself vs she/her/hers/herself) and
classify the portrait image with a CLIP zero-shot man/woman head, then report the
agreement rate and the mismatches.

Run from the kb repo root:
    HF_HUB_DISABLE_XET=1 python3 tools/check_portrait_gender.py [knowledge.db]
"""

import os, re, sqlite3, sys

DB = sys.argv[1] if len(sys.argv) > 1 else "../spacemolt-knowledge.db"
CACHE = "overlays/generated/passengers"

MASC = re.compile(r"\b(he|him|his|himself)\b", re.I)
FEM = re.compile(r"\b(she|her|hers|herself)\b", re.I)


def expected_gender(bio):
    m, f = bool(MASC.search(bio or "")), bool(FEM.search(bio or ""))
    if m and not f:
        return "male"
    if f and not m:
        return "female"
    return None  # ambiguous / none


def main():
    import torch
    from PIL import Image
    from transformers import CLIPModel, CLIPProcessor

    model_id = "openai/clip-vit-base-patch32"
    # use_safetensors avoids the torch<2.6 .bin loading restriction.
    model = CLIPModel.from_pretrained(model_id, use_safetensors=True).to("cuda").eval()
    proc = CLIPProcessor.from_pretrained(model_id)
    labels = ["a portrait of a man", "a portrait of a woman"]

    con = sqlite3.connect(DB)
    rows = con.execute("SELECT citizen_id, bio FROM passengers").fetchall()
    con.close()

    checked = agree = 0
    ambiguous = missing = 0
    mismatches = []
    for cid, bio in rows:
        exp = expected_gender(bio)
        img_path = os.path.join(CACHE, cid, "portrait.png")
        if not os.path.exists(img_path):
            missing += 1
            continue
        if exp is None:
            ambiguous += 1
            continue
        img = Image.open(img_path).convert("RGB")
        inp = proc(text=labels, images=img, return_tensors="pt", padding=True).to("cuda")
        with torch.no_grad():
            logits = model(**inp).logits_per_image[0]
        pred = "male" if logits[0] > logits[1] else "female"
        checked += 1
        if pred == exp:
            agree += 1
        else:
            mismatches.append((cid, exp, pred))

    print(f"portraits checked (clear bio gender): {checked}")
    print(f"  agree:    {agree} ({100*agree/checked:.1f}%)" if checked else "  none")
    print(f"  mismatch: {len(mismatches)}")
    print(f"  skipped:  {ambiguous} ambiguous/none, {missing} missing portrait")
    if mismatches:
        print("\nmismatches (id: expected -> predicted):")
        for cid, exp, pred in sorted(mismatches):
            print(f"  {cid}: {exp} -> {pred}")


if __name__ == "__main__":
    main()
