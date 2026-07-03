package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// findCatalogItems locates the live catalog_items.json snapshot relative to the
// repo, skipping the test if the sibling spacemolt checkout is not present.
func findCatalogItems(t *testing.T) string {
	t.Helper()
	for _, p := range []string{
		"../../../spacemolt/data/game-api/latest/catalog_items.json",
		"../../../../spacemolt/data/game-api/latest/catalog_items.json",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("catalog_items.json snapshot not found")
	return ""
}

// TestItemOverlayAndStats decodes the real catalog and renders stats for a few
// representative module types, asserting the new fields surface on item pages.
func TestItemOverlayAndStats(t *testing.T) {
	path := findCatalogItems(t)

	// Seed the items map with every catalog ID (the overlay only enriches IDs
	// already present, mirroring the crafting-DB-sourced item map).
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var probe struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("probe: %v", err)
	}
	items := make(map[string]*Item, len(probe.Items))
	for _, p := range probe.Items {
		items[p.ID] = &Item{ID: p.ID}
	}

	if err := loadItemOverlay(path, items); err != nil {
		t.Fatalf("loadItemOverlay: %v", err)
	}

	// Find an item exercising each section and confirm it renders.
	var withWeapon, withResist, withSkills, withEffect, withPassenger *Item
	for _, it := range items {
		if withWeapon == nil && it.Damage > 0 {
			withWeapon = it
		}
		if withResist == nil && len(it.ResistanceBonus) > 0 {
			withResist = it
		}
		if withSkills == nil && len(it.RequiredSkills) > 0 {
			withSkills = it
		}
		if withEffect == nil && it.Effect != nil && len(it.Effect.Ammo) > 0 {
			withEffect = it
		}
		if withPassenger == nil && it.PassengerEconomyBerths > 0 {
			withPassenger = it
		}
	}

	cases := map[string]*Item{
		"weapon":    withWeapon,
		"resist":    withResist,
		"skills":    withSkills,
		"ammo":      withEffect,
		"passenger": withPassenger,
	}
	wants := map[string]string{
		"weapon":    "Damage",
		"resist":    "Resistances",
		"skills":    "Required Skills",
		"ammo":      "Effect",
		"passenger": "Passenger Berths",
	}
	for name, it := range cases {
		if it == nil {
			t.Errorf("no catalog item found exercising %q section", name)
			continue
		}
		html := string(itemStatsHTML(it))
		if !strings.Contains(html, wants[name]) {
			t.Errorf("%s item %s: stats HTML missing %q\n%s", name, it.ID, wants[name], html)
		}
	}

	// A plain resource (no module fields) must render nothing.
	plain := &Item{ID: "iron_ore", Name: "Iron Ore", Category: "ore"}
	if got := string(itemStatsHTML(plain)); got != "" {
		t.Errorf("plain resource should render no stats, got: %s", got)
	}
}
