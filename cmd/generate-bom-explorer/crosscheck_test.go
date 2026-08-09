package main

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

const craftingDBPath = "../../crafting.db"

// TestDefaultsPreferObtainableInputs checks the one property that actually
// protects users of the explorer: for every item with more than one
// producing recipe, if any candidate recipe's inputs are all structurally
// obtainable (ore/material, or themselves built from obtainable inputs), the
// chosen default must be one of those candidates too.
//
// This does NOT assert equality with the committed bill_of_materials table.
// That table resolves ties with live market availability, which
// systematically prefers mob-drop inputs that happened to be for sale over
// mineable ones nothing stops being obtainable (e.g. gold_bar via
// gilded_chitin_smelting, a drop, over mint_gold_bar, from gold_ore) — worse
// answers for a page whose job is "what do I gather". computeDefaults
// deliberately diverges from that table on such items; see its doc comment
// and docs/superpowers/specs/2026-08-08-bom-explorer-design.md, "Cross-check
// against the existing tables".
//
// It also does NOT assert that no default ever reaches a dead end: 72
// craftable items have no fully-obtainable recipe at all, because the game
// genuinely requires drops (cooking recipes needing raw_xeno_meat,
// mantis_claw, and similar). Those items correctly have no obtainable
// candidate to prefer, so this test has nothing to say about them.
func TestDefaultsPreferObtainableInputs(t *testing.T) {
	if _, err := os.Stat(craftingDBPath); err != nil {
		t.Skip("crafting.db not present; skipping cross-check")
	}
	db, err := sql.Open("sqlite", "file:"+craftingDBPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	items, err := loadItems(db)
	if err != nil {
		t.Fatal(err)
	}
	recipes, err := loadRecipes(db)
	if err != nil {
		t.Fatal(err)
	}
	doc := BuildDoc(items, recipes, nil, nil)

	obtainable := computeObtainableForTest(doc)

	producers := map[string][]string{}
	for id, r := range doc.Recipes {
		for _, o := range r.Outputs {
			item, _ := o[0].(string)
			producers[item] = append(producers[item], id)
		}
	}

	checked := 0
	for item, ids := range producers {
		if len(ids) < 2 {
			continue
		}
		anyObtainable := false
		for _, id := range ids {
			if allInputsObtainable(doc.Recipes[id], obtainable) {
				anyObtainable = true
				break
			}
		}
		if !anyObtainable {
			// No candidate qualifies — a genuine drop-only item. Nothing to
			// require of the default here.
			continue
		}
		checked++
		chosen := doc.Defaults[item]
		if chosen == "" {
			t.Errorf("%s: an obtainable-input candidate exists but no default is recorded", item)
			continue
		}
		if !allInputsObtainable(doc.Recipes[chosen], obtainable) {
			t.Errorf("%s: default recipe %s does not have all-obtainable inputs, "+
				"though an alternative recipe does", item, chosen)
		}
	}

	if checked == 0 {
		t.Fatal("no multi-recipe items with an obtainable candidate found; the invariant check is vacuous")
	}
	t.Logf("checked obtainability preference for %d multi-recipe items", checked)
}

// computeObtainableForTest independently re-derives the structural
// obtainability fixed point straight from doc.Items/doc.Recipes, rather than
// calling build.go's computeObtainable, so this test verifies the observable
// Doc output rather than merely re-running the code under test.
func computeObtainableForTest(doc Doc) map[string]bool {
	obtainable := map[string]bool{}
	for id, it := range doc.Items {
		if it.Category == "ore" || it.Category == "material" {
			obtainable[id] = true
		}
	}
	for {
		changed := false
		for _, r := range doc.Recipes {
			if !allInputsObtainable(r, obtainable) {
				continue
			}
			for _, o := range r.Outputs {
				oid, _ := o[0].(string)
				if !obtainable[oid] {
					obtainable[oid] = true
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	return obtainable
}

// allInputsObtainable reports whether every input of r is in obtainable. A
// recipe with no inputs at all (none exist among kept, non-packaging
// recipes) is vacuously true, matching bom.allInputsSourceable's behavior.
func allInputsObtainable(r RecipeRec, obtainable map[string]bool) bool {
	for _, in := range r.Inputs {
		iid, _ := in[0].(string)
		if !obtainable[iid] {
			return false
		}
	}
	return true
}
