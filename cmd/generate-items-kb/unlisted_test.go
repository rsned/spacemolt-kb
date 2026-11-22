package main

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBuildUnlisted(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, q := range []string{
		`CREATE TABLE mission_templates (id TEXT PRIMARY KEY, title TEXT, type TEXT)`,
		`CREATE TABLE mission_completions (mission_id TEXT PRIMARY KEY, template_id TEXT, title TEXT, recipes_unlocked TEXT)`,
		`INSERT INTO mission_templates VALUES ('the_method', 'The Method', 'exploration')`,
		`INSERT INTO mission_completions VALUES ('m1', 'the_method', 'The Method', '["assemble_lane_anchor","assemble_fold_shim"]')`,
		`INSERT INTO mission_completions VALUES ('m2', 'the_method', 'The Method', '["assemble_lane_anchor"]')`,
		`INSERT INTO mission_completions VALUES ('m3', 'proto', 'Taking the Machine Apart', '["assemble_lane_anchor_prototype"]')`,
		`INSERT INTO mission_completions VALUES ('m4', 'other', 'Other', '[]')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	unlocks, err := loadRecipeUnlocks(db)
	if err != nil {
		t.Fatal(err)
	}
	if got := unlocks["assemble_lane_anchor"]; len(got) != 1 || got[0].URL() != "../missions/exploration/the_method.html" {
		t.Errorf("lane anchor unlocks = %+v", got)
	}
	if got := unlocks["assemble_lane_anchor_prototype"]; len(got) != 1 || got[0].URL() != "" {
		t.Errorf("prototype unlocks = %+v (no template row, so no link)", got)
	}

	captured := []*UnlistedRecipe{{ID: "assemble_lane_anchor", Name: "Assemble Lane Anchor",
		Inputs:  []UnlistedItem{{ItemID: "seam_glass", Name: "Seam Glass", Quantity: 50}, {ItemID: "mirror_facet", Name: "Mirror Facet", Quantity: 10}},
		Outputs: []UnlistedItem{{ItemID: "lane_anchor", Name: "Lane Anchor", Quantity: 1}}}}
	recipes := map[string]*Recipe{"assemble_fold_shim": {ID: "assemble_fold_shim", Category: "Fold Engineering"}}
	items := map[string]*Item{"seam_glass": {ID: "seam_glass", Category: "ore"}}
	list := buildUnlisted(captured, unlocks, recipes, items)

	if len(list) != 3 {
		t.Fatalf("got %d recipes, want 3 (captured + 2 placeholders)", len(list))
	}
	a := list[0]
	if a.ID != "assemble_lane_anchor" || a.Missing || a.Inputs[0].URL != "../items/ore/seam_glass.html" || a.Inputs[1].URL != "" {
		t.Errorf("lane anchor = %+v", a)
	}
	for _, r := range list[1:] {
		if !r.Missing {
			t.Errorf("%s should be a placeholder", r.ID)
		}
	}
	var shim *UnlistedRecipe
	for _, r := range list {
		if r.ID == "assemble_fold_shim" {
			shim = r
		}
	}
	if shim == nil || shim.CatalogURL != "Fold_Engineering/assemble_fold_shim.html" {
		t.Errorf("fold shim catalog link = %+v", shim)
	}

	var b strings.Builder
	if err := renderUnlistedPage(&b, list); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Assemble Lane Anchor", "Mirror Facet <span class=\"text-muted\">(not in catalog)", "../items/ore/seam_glass.html", "craft assemble_lane_anchor_prototype --dry_run=true", "../missions/exploration/the_method.html"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("page missing %q", want)
		}
	}
}
