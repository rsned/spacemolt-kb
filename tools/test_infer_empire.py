"""Regression checks for the empire name classifier.

Trains on the live labeled passenger names and asserts that a handful of names
written in each empire's unmistakable style classify correctly, plus the
mechanical invariants of the scorer. Run: ~/sd-venv/bin/python -m pytest
tools/test_infer_empire.py  (or system python3 with pytest)."""

import importlib.util
import os
import sqlite3

import pytest

_spec = importlib.util.spec_from_file_location(
    "infer_empire", os.path.join(os.path.dirname(__file__), "infer_empire.py"))
ie = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(ie)


@pytest.fixture(scope="module")
def model():
    if not os.path.exists(ie.DEFAULT_DB):
        pytest.skip("knowledge DB not present")
    con = sqlite3.connect(ie.DEFAULT_DB)
    rows = con.execute(
        "SELECT name, citizenship FROM passengers "
        "WHERE citizenship IS NOT NULL AND trim(citizenship)!=''").fetchall()
    con.close()
    labeled = [(n, c.strip().lower()) for n, c in rows if (n or "").strip()]
    return ie.train(labeled)


# Names written in each empire's signature style (not verbatim DB rows).
OBVIOUS = [
    ("Hrothek Hammermund", "crimson"),    # Germanic forge: -mund, Hammer-
    ("Brakund Severholm", "crimson"),     # harsh consonants, -holm
    ("Daya Lascor", "nebula"),            # soft, vowel-heavy
    ("Nywene Fossou", "nebula"),          # -ou ending, melodic
    ("Aldo Stonefield", "solarian"),      # Earth-Anglo surname
    ("Echix Hexis", "voidborn"),          # alien x/z-heavy
    ("Vaelene Ochtost", "voidborn"),      # ethereal, -ost
]


@pytest.mark.parametrize("name,expected", OBVIOUS)
def test_obvious_names(model, name, expected):
    pred, conf, _ = ie.score(model, name)
    assert pred == expected, f"{name!r} -> {pred} (conf {conf:.2f}), expected {expected}"


def test_confidence_is_a_probability(model):
    _, conf, logp = ie.score(model, "Hrothek Hammermund")
    assert 0.0 < conf <= 1.0
    assert set(logp) == set(model["classes"])


def test_ngrams_keep_markers_and_titles():
    grams = ie.ngrams("Dr. Aldo")
    assert ie.MARK_START + "d" in grams      # start marker present
    assert "o" + ie.MARK_END in grams        # end marker present
    assert "dr." in grams                    # title/punctuation retained as signal
