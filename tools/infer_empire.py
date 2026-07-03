#!/usr/bin/env python3
"""Infer a likely empire for passengers imported without a citizenship field.

A batch of passenger records arrived with NULL `citizenship`; only a minority
carry a labeled empire. The six empires have distinct, morphology-driven naming
conventions (Crimson Germanic/forge, Nebula soft/vowel-heavy, Solarian
Earth-Anglo, Outer Rim short/gritty, Voidborn alien x/z-heavy, Pirates rogue
epithets), so the name alone predicts the empire well.

This trains a character n-gram multinomial Naive Bayes on the labeled names and
predicts the unlabeled ones. The guess feeds ONLY the portrait prompt's empire
styling (see cmd/generate-factions-kb); it never touches the DB or the visible
faction. Leave-one-out cross-validation runs every invocation as the accuracy
gate.

    python3 tools/infer_empire.py [db] [--no-write]

Writes overlays/generated/empire_guess.json = {citizen_id: {empire, confidence,
name}} for the unlabeled passengers. --no-write prints the report only.
"""

import argparse
import json
import math
import os
import sqlite3
import sys

CACHE = "overlays/generated/empire_guess.json"
DEFAULT_DB = "/home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db"
NGRAM_MIN, NGRAM_MAX = 2, 4
MARK_START, MARK_END = "\x02", "\x03"


def ngrams(name):
    """Character n-grams (n in [NGRAM_MIN, NGRAM_MAX]) over the marked, lowercased
    name. Titles, quoted nicknames, spaces, and punctuation are kept — they are
    themselves empire signal (Dr./Brother -> Solarian, Captain/One-Eye -> pirates)."""
    s = MARK_START + (name or "").strip().lower() + MARK_END
    out = []
    for n in range(NGRAM_MIN, NGRAM_MAX + 1):
        for i in range(len(s) - n + 1):
            out.append(s[i:i + n])
    return out


def train(docs):
    """docs = [(name, empire)]. Returns a model: per-class n-gram counts, class
    totals, vocabulary size, and log priors.

    Priors are UNIFORM, not empirical: the labeled set is imbalanced (crimson 89
    vs pirates 14) but the unlabeled batch's true empire mix is unknown, so an
    empirical prior would wrongly bias predictions toward the larger labeled
    classes and crush the small ones. Uniform priors measured better in
    leave-one-out (79.4% vs 78.2%, with pirates recall 7%->21%)."""
    counts = {}          # empire -> {ngram: count}
    totals = {}          # empire -> total ngram occurrences
    class_docs = {}      # empire -> document count
    vocab = set()
    for name, empire in docs:
        c = counts.setdefault(empire, {})
        class_docs[empire] = class_docs.get(empire, 0) + 1
        for g in ngrams(name):
            c[g] = c.get(g, 0) + 1
            totals[empire] = totals.get(empire, 0) + 1
            vocab.add(g)
    log_prior = {e: math.log(1 / len(class_docs)) for e in class_docs}
    return {"counts": counts, "totals": totals, "vocab": len(vocab),
            "log_prior": log_prior, "classes": sorted(class_docs)}


def score(model, name):
    """Return (best_empire, confidence, {empire: logprob}) for a name under the
    model. Multinomial NB with add-1 (Laplace) smoothing, computed in log-space."""
    grams = ngrams(name)
    v = model["vocab"]
    logp = {}
    for e in model["classes"]:
        c = model["counts"][e]
        denom = model["totals"][e] + v
        lp = model["log_prior"][e]
        for g in grams:
            lp += math.log((c.get(g, 0) + 1) / denom)
        logp[e] = lp
    best = max(logp, key=logp.get)
    # Normalized posterior as confidence (softmax over the class log-probs).
    mx = max(logp.values())
    denom = sum(math.exp(lp - mx) for lp in logp.values())
    conf = math.exp(logp[best] - mx) / denom
    return best, conf, logp


def cross_validate(labeled):
    """Leave-one-out accuracy + confusion matrix over the labeled set."""
    classes = sorted({e for _, e in labeled})
    confusion = {a: {b: 0 for b in classes} for a in classes}
    correct = 0
    for i in range(len(labeled)):
        held_name, held_empire = labeled[i]
        model = train(labeled[:i] + labeled[i + 1:])
        pred, _, _ = score(model, held_name)
        confusion[held_empire][pred] += 1
        if pred == held_empire:
            correct += 1
    return correct / len(labeled), confusion, classes


def print_confusion(confusion, classes):
    w = max(len(c) for c in classes)
    header = " " * (w + 2) + " ".join(f"{c[:7]:>7}" for c in classes) + "   recall"
    print(header)
    for a in classes:
        row = confusion[a]
        tot = sum(row.values())
        rec = row[a] / tot if tot else 0.0
        cells = " ".join(f"{row[b]:>7}" for b in classes)
        print(f"{a:<{w}}  {cells}   {rec:5.0%} ({row[a]}/{tot})")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("db", nargs="?", default=DEFAULT_DB)
    ap.add_argument("--no-write", action="store_true", help="print report only; do not write JSON")
    args = ap.parse_args()

    con = sqlite3.connect(args.db)
    rows = con.execute("SELECT citizen_id, name, citizenship FROM passengers").fetchall()
    con.close()

    labeled, unlabeled = [], []
    for cid, name, cz in rows:
        cz = (cz or "").strip().lower()
        if not (name or "").strip():
            continue
        if cz:
            labeled.append((name, cz))
        else:
            unlabeled.append((cid, name))

    print(f"labeled {len(labeled)}, unlabeled {len(unlabeled)}")
    acc, confusion, classes = cross_validate(labeled)
    print(f"\nleave-one-out accuracy: {acc:.1%}\n")
    print_confusion(confusion, classes)

    model = train(labeled)
    guesses, dist = {}, {}
    for cid, name in unlabeled:
        empire, conf, _ = score(model, name)
        guesses[cid] = {"empire": empire, "confidence": round(conf, 4), "name": name}
        dist[empire] = dist.get(empire, 0) + 1
    print("\npredicted empire distribution (unlabeled):",
          dict(sorted(dist.items(), key=lambda kv: -kv[1])))

    if args.no_write:
        print("\n--no-write: not writing", CACHE)
        return
    os.makedirs(os.path.dirname(CACHE), exist_ok=True)
    tmp = CACHE + ".tmp"
    with open(tmp, "w") as f:
        json.dump(guesses, f, indent=2, sort_keys=True)
    os.replace(tmp, CACHE)
    print(f"\nwrote {len(guesses)} guesses -> {CACHE}")


if __name__ == "__main__":
    main()
