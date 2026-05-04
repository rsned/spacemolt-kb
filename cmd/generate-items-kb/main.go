// Command generate-items-kb reads the crafting database and produces
// KB-styled HTML pages for all items, organized by category.
package main

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	htmltpl "html/template"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	humanize "github.com/dustin/go-humanize"
	"github.com/rsned/spacemolt-kb/pkg/bom"
	"github.com/rsned/spacemolt-kb/pkg/kbdb"
	"github.com/rsned/spacemolt-kb/pkg/systemmap"
	_ "modernc.org/sqlite"
)

// Item holds every column from the items table plus its recipe relationships.
type Item struct {
	ID          string
	Name        string
	Description string
	Category    string
	Rarity      string
	Size        int
	BaseValue   int
	Stackable   bool
	Tradeable   bool

	PowerBonus int
	Hazardous  bool
	Hidden     bool

	HasImage bool

	ProducedBy  []ProducedBy
	UsedIn      []UsedIn
	UsedInShips []ShipBuildRef
	BoM *bom.BoMResult
}

// ShipBuildRef links an item to a ship that requires it as a build material.
type ShipBuildRef struct {
	ShipID       string
	ShipName     string
	ShipCategory string
	ShipClass    string
	Quantity     int
}

// ProducedBy describes a recipe that produces this item.
type ProducedBy struct {
	RecipeID       string
	RecipeName     string
	RecipeCategory string
	Quantity       int
	CraftingTime   int
}

// UsedIn describes a recipe that consumes this item and what it produces.
type UsedIn struct {
	RecipeID       string
	RecipeName     string
	RecipeCategory string
	Quantity       int
	OutputID       string
	OutputName     string
	OutputCategory string
}

// CategoryInfo groups items for page generation.
type CategoryInfo struct {
	Name        string
	Description string
	Count       int
	Items       []*Item
}

// Recipe holds a crafting recipe with its inputs and outputs.
type Recipe struct {
	ID           string
	Name         string
	Description  string
	Category     string
	CraftingTime int
	Hidden       bool
	Inputs       []RecipeItem
	Outputs      []RecipeItem
}

// RecipeItem is an item reference within a recipe (input or output).
type RecipeItem struct {
	ItemID       string
	ItemName     string
	ItemCategory string
	Quantity     int
	HasImage     bool
}

// RecipeCategoryInfo groups recipes for page generation.
type RecipeCategoryInfo struct {
	Name        string
	DirName     string // Name with spaces replaced by underscores, used for directory/URL paths.
	Description string
	Count       int
	Recipes     []*Recipe
}

// dirName converts a category name to a filesystem-safe directory name.
func dirName(s string) string {
	return strings.ReplaceAll(s, " ", "_")
}

// resourceAnchor converts an item ID (with underscores) to a resource page anchor (with dashes).
// For example: "aluminum_ore" -> "aluminum-ore", "argon_gas" -> "argon-gas"
func resourceAnchor(itemID string) string {
	return strings.ReplaceAll(itemID, "_", "-")
}

// isResourceItem returns true if the item category is one that appears in the Resources KB section.
// The Resources section includes: ore, material (crystals, exotic matter, etc.)
func isResourceItem(category string) bool {
	switch category {
	case "ore", "material":
		return true
	default:
		return false
	}
}

var categoryDescriptions = map[string]string{
	"artifact":   "Rare relics and ancient objects from lost civilizations.",
	"component":  "Crafted parts and assemblies used to build ships, stations, and equipment.",
	"consumable": "Single-use items including ammunition, stims, repair kits, and fuel.",
	"contraband": "Illegal goods that carry severe penalties if caught in possession.",
	"defense":    "Defensive equipment and shield systems.",
	"document":   "Blueprints, maps, and encrypted data files.",
	"drone":      "Autonomous craft for combat, mining, repair, and reconnaissance.",
	"material":   "Rare raw materials with special properties.",
	"misc":       "Collectibles, souvenirs, medals, and other miscellaneous items.",
	"ore":        "Raw ores, gases, ice, and biological samples harvested from space.",
	"refined":    "Processed materials refined from raw ores and gases.",
	"weapon":     "Weapons and weapon systems.",
	"data_chip":  "Encoded data chips containing navigation, trade, and intelligence data.",
	"mining":     "Mining equipment, laser upgrades, and extraction tools.",
	"utility":    "Utility modules, scanners, cloaking devices, and support equipment.",
}

// System holds data for a star system page.
type System struct {
	ID              string
	Name            string
	PositionX       float64
	PositionY       float64
	PoliceLevel     int
	Empire          string
	Description     string
	IsStronghold    bool
	SecurityStatus  string
	LastUpdatedTick int
	Connections     []SystemConnection
	POIs            []SystemPOI
	Bases           []SystemBase
}

// SystemConnection is a jump gate connection to another system.
type SystemConnection struct {
	SystemID string
	Name     string
	Distance int
}

// SystemPOI is a point of interest within a system.
type SystemPOI struct {
	ID          string
	Name        string
	Type        string
	Class       string
	Description string
	PositionX   float64
	PositionY   float64
	Hidden      bool
	RevealDifficulty int
	Resources   []POIResource
}

// POIResource is a resource found at a POI.
type POIResource struct {
	ResourceID   string
	ResourceName string
	Richness     float64
	Remaining    float64
}

// SystemBase is a base/station in a system.
type SystemBase struct {
	ID                string
	POIID             string
	Name              string
	Description       string
	Story             string
	Empire            string
	DefenseLevel      int
	HasDrones         bool
	PublicAccess      bool
	Condition         string
	ConditionText     string
	SatisfactionPct   int
	SatisfiedCount    int
	TotalServiceInfra int
	Services          []BaseService
	Facilities        []BaseFacility
}

// BaseService is a service available at a base.
type BaseService struct {
	Name      string
	Available bool
}

// BaseFacility is a facility at a base.
type BaseFacility struct {
	Name     string
	Category string
	Level    int
}

// CondensedFacility is a facility with a count for duplicate collapsing.
type CondensedFacility struct {
	Name     string
	Category string
	Level    int
	Count    int
}

// EmpireGroup holds an empire's systems for the systems index page.
type EmpireGroup struct {
	Name    string       // Canonical empire name (title case)
	ID      string       // Lowercase slug for anchor links
	Color   string       // CSS hex color
	Systems []*System
	MapSVG  htmltpl.HTML // Pre-rendered SVG map HTML
}

// empireColors maps lowercase empire names to their theme colors.
var empireColors = map[string]string{
	"solarian": "#FFD700",
	"voidborn": "#9932CC",
	"crimson":  "#DC143C",
	"nebula":   "#00CED1",
	"outerrim": "#2E8B57",
}

// empireOrder defines the display order for empires.
var empireOrder = []string{"solarian", "voidborn", "crimson", "nebula", "outerrim"}

// empireCapitals maps empire name to its capital system ID.
var empireCapitals = map[string]string{
	"solarian": "sol",
	"voidborn": "nexus",
	"crimson":  "krynn",
	"nebula":   "haven",
	"outerrim": "frontier",
}

// SystemsIndexData holds all data for the systems index template.
type SystemsIndexData struct {
	Systems            []*System
	Empires           []EmpireGroup
	TotalSystems      int
	ExploredSystems   int
	ExplorationPct    float64
}

var recipeCategoryDescriptions = map[string]string{
	"Components":          "Intermediate parts and assemblies used to build ships, modules, and equipment.",
	"Consumables":         "Ammunition, repair kits, fuel cells, mines, and other single-use items.",
	"Defense":             "Shield generators, armor hardeners, and defensive module construction.",
	"Drones":              "Autonomous combat, mining, repair, and electronic warfare drones.",
	"Electronic Warfare":  "ECM jammers and electronic countermeasure systems.",
	"Equipment":           "Specialized tools and survey equipment.",
	"Gas Processing":      "Compression and refinement of harvested nebula gases.",
	"Ice Refining":        "Processing of ice deposits into fuel and industrial materials.",
	"Legendary":           "Extremely rare and powerful items requiring exotic components.",
	"Mining":              "Mining laser and extraction equipment construction.",
	"Modules":             "Ship module fabrication including drone bays and specialized systems.",
	"Production":          "Beverages, luxury goods, and other manufactured products.",
	"Refining":            "Smelting ores into refined metals, alloys, and processed materials.",
	"Shipbuilding":        "Hull frames, superstructures, and complete ship assembly.",
	"Stealth":             "Cloaking devices and stealth system construction.",
	"Utility":             "Tow rigs, afterburners, salvage tools, and support modules.",
	"Weapons":             "Lasers, autocannons, missile launchers, and weapon system fabrication.",
}

// loadBoMFromDB loads BoM results from the database and attaches them to items, ships, and facilities.
func loadBoMFromDB(db *sql.DB, items map[string]*Item, ships []*Ship, facilities []*Facility) error {
	// Load items
	itemRows, err := db.Query(`SELECT target_id, target_type, base_item_id, quantity FROM bill_of_materials WHERE target_type = 'item'`)
	if err != nil {
		return err
	}
	defer func() { _ = itemRows.Close() }()

	// Group materials by item ID
	itemMaterials := make(map[string][]bom.MaterialRequirement)
	for itemRows.Next() {
		var targetID string
		var targetType string
		var itemID string
		var quantity int
		if err := itemRows.Scan(&targetID, &targetType, &itemID, &quantity); err != nil {
			return err
		}
		itemMaterials[targetID] = append(itemMaterials[targetID], bom.MaterialRequirement{
			ItemID:  itemID,
			Quantity: quantity,
		})
	}

	// Attach to items
	for itemID, materials := range itemMaterials {
		if item, ok := items[itemID]; ok {
			item.BoM = &bom.BoMResult{
				TargetID:      itemID,
				TargetType:    "item",
				BaseMaterials: materials,
			}
		}
	}

	// Load ships
	shipMaterials := make(map[string][]bom.MaterialRequirement)
	shipRows, err := db.Query(`SELECT target_id, target_type, base_item_id, quantity FROM bill_of_materials WHERE target_type = 'ship'`)
	if err != nil {
		return err
	}
	defer func() { _ = shipRows.Close() }()

	for shipRows.Next() {
		var targetID string
		var targetType string
		var itemID string
		var quantity int
		if err := shipRows.Scan(&targetID, &targetType, &itemID, &quantity); err != nil {
			return err
		}
		shipMaterials[targetID] = append(shipMaterials[targetID], bom.MaterialRequirement{
			ItemID:  itemID,
			Quantity: quantity,
		})
	}

	// Attach to ships
	for shipID, materials := range shipMaterials {
		for _, ship := range ships {
			if ship.ID == shipID {
				ship.BoM = &bom.BoMResult{
					TargetID:      shipID,
					TargetType:    "ship",
					BaseMaterials: materials,
				}
				break
			}
		}
	}

	// Load facilities
	facMaterials := make(map[string][]bom.MaterialRequirement)
	facRows, err := db.Query(`SELECT target_id, target_type, base_item_id, quantity FROM bill_of_materials WHERE target_type = 'facility'`)
	if err != nil {
		return err
	}
	defer func() { _ = facRows.Close() }()

	for facRows.Next() {
		var targetID string
		var targetType string
		var itemID string
		var quantity int
		if err := facRows.Scan(&targetID, &targetType, &itemID, &quantity); err != nil {
			return err
		}
		facMaterials[targetID] = append(facMaterials[targetID], bom.MaterialRequirement{
			ItemID:  itemID,
			Quantity: quantity,
		})
	}

	// Attach to facilities
	for facilityID, materials := range facMaterials {
		for _, facility := range facilities {
			if facility.ID == facilityID {
				facility.BoM = &bom.BoMResult{
					TargetID:      facilityID,
					TargetType:    "facility",
					BaseMaterials: materials,
				}
				break
			}
		}
	}

	return nil
}

func main() {
	systemOnly := flag.String("system", "", "regenerate only this system's page (by system ID)")
	flag.Parse()

	dbPath := "../../spacemolt-crafting-server/database/crafting.db"
	catalogDir := findLatestCatalogDir("../spacemolt/data/game-api")
	outDir := "kb/items"

	args := flag.Args()
	if len(args) > 0 {
		dbPath = args[0]
	}
	if len(args) > 1 {
		outDir = args[1]
	}

	// --- Single-system mode ---
	if *systemOnly != "" {
		generateOneSystem(*systemOnly)
		return
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	items, err := loadItems(db)
	if err != nil {
		log.Fatalf("load items: %v", err)
	}

	// Overlay additional fields from catalog JSON (power_bonus, hazardous, hidden).
	itemCatalogPath := filepath.Join(catalogDir, "catalog_items.json")
	if err := loadItemOverlay(itemCatalogPath, items); err != nil {
		log.Printf("warning: load item overlay: %v (extra fields will be omitted)", err)
	}

	if err := loadProducedBy(db, items); err != nil {
		log.Fatalf("load produced-by: %v", err)
	}

	if err := loadUsedIn(db, items); err != nil {
		log.Fatalf("load used-in: %v", err)
	}

	// Load ship build materials and passive recipes from catalog JSON.
	shipCatalogPath := filepath.Join(catalogDir, "catalog_ships.json")
	if err := loadShipBuildMaterials(shipCatalogPath, items); err != nil {
		log.Printf("warning: load ship build materials: %v (ship links will be omitted)", err)
	}

	// Clean generated HTML files, preserving images/ and items.css.
	if err := cleanGeneratedFiles(outDir); err != nil {
		log.Fatalf("clean output dir: %v", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	// Item thumbnails are fan-made — intentionally skip setting HasImage
	// so images are kept on disk but not displayed in the KB.

	// Group items by category.
	catItems := make(map[string][]*Item)
	for _, it := range items {
		catItems[it.Category] = append(catItems[it.Category], it)
	}
	for _, itemList := range catItems {
		slices.SortFunc(itemList, func(a, b *Item) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}

	categories := make([]CategoryInfo, 0, len(catItems))
	for cat, itemList := range catItems {
		categories = append(categories, CategoryInfo{
			Name:        cat,
			Description: categoryDescriptions[cat],
			Count:       len(itemList),
			Items:       itemList,
		})
	}
	slices.SortFunc(categories, func(a, b CategoryInfo) int {
		return cmp.Compare(a.Name, b.Name)
	})

	// --- Recipe generation ---
	// Recipes must load before BoM calculation so the calculator can resolve
	// ingredients; ship and facility catalogs must load before BoM so build
	// materials can be traced down to base components in the same pass.
	recipeOutDir := "kb/recipes"
	if len(args) > 2 {
		recipeOutDir = args[2]
	}

	recipes, err := loadRecipes(db)
	if err != nil {
		log.Fatalf("load recipes: %v", err)
	}

	// Overlay hidden flag from catalog JSON.
	recipeCatalogPath := filepath.Join(catalogDir, "catalog_recipes.json")
	if err := loadRecipeOverlay(recipeCatalogPath, recipes); err != nil {
		log.Printf("warning: load recipe overlay: %v (hidden flag will be omitted)", err)
	}

	// Load ship catalog and facilities up-front so their build materials
	// participate in the BoM calculation.
	shipCatalog, shipErr := loadShipCatalog(shipCatalogPath)
	if shipErr != nil {
		log.Printf("warning: load ship catalog: %v (ship pages will be skipped)", shipErr)
	}

	facilityJSONDir := filepath.Join(catalogDir, "facility_details")
	facilities, facErr := loadFacilitiesFromJSON(facilityJSONDir)
	if facErr != nil {
		log.Printf("warning: load facilities: %v (facility pages will be skipped)", facErr)
	} else {
		validateFacilityRecipes(facilities, recipes)
	}

	// --- BOM Calculation ---
	bomItems := make(map[string]*bom.Item)
	for id, item := range items {
		bomItems[id] = &bom.Item{
			ID:       item.ID,
			Name:     item.Name,
			Category: item.Category,
			IsBase:   item.Category == "ore" || item.Category == "material",
		}
	}

	bomRecipes := make(map[string]*bom.Recipe)
	for id, recipe := range recipes {
		inputs := make([]bom.RecipeItem, len(recipe.Inputs))
		for i, inp := range recipe.Inputs {
			inputs[i] = bom.RecipeItem{ItemID: inp.ItemID, Quantity: inp.Quantity}
		}
		outputs := make([]bom.RecipeItem, len(recipe.Outputs))
		for i, out := range recipe.Outputs {
			outputs[i] = bom.RecipeItem{ItemID: out.ItemID, Quantity: out.Quantity}
		}
		bomRecipes[id] = &bom.Recipe{ID: recipe.ID, Inputs: inputs, Outputs: outputs}
	}

	bomShips := make(map[string]*bom.Ship)
	for _, ship := range shipCatalog {
		buildMats := make([]bom.ShipBuildRef, len(ship.BuildMaterials))
		for i, mat := range ship.BuildMaterials {
			buildMats[i] = bom.ShipBuildRef{ItemID: mat.ItemID, Quantity: mat.Quantity}
		}
		bomShips[ship.ID] = &bom.Ship{ID: ship.ID, Name: ship.Name, BuildMaterials: buildMats}
	}

	bomFacilities := make(map[string]*bom.Facility)
	for id, fac := range facilities {
		buildMats := make([]bom.FacilityMaterial, len(fac.BuildMaterials))
		for i, mat := range fac.BuildMaterials {
			buildMats[i] = bom.FacilityMaterial{ItemID: mat.ItemID, Name: mat.Name, Quantity: mat.Quantity}
		}
		bomFacilities[id] = &bom.Facility{ID: fac.ID, Name: fac.Name, BuildMaterials: buildMats}
	}

	if err := bom.Migrate(db); err != nil {
		log.Fatalf("migrate BOM schema: %v", err)
	}

	calculator, err := bom.NewCalculator(db, bomRecipes, bomItems)
	if err != nil {
		log.Fatalf("initialize BOM calculator: %v", err)
	}
	log.Println("Calculating BOM for all items, ships, and facilities...")
	if err := calculator.CalculateAll(bomItems, bomShips, bomFacilities); err != nil {
		log.Fatalf("calculate BOM: %v", err)
	}

	// Attach BoM data to in-memory items, ships, and facilities so all page
	// writers (items first, then ships, then facilities) can render
	// Construction sections.
	facilitySlice := make([]*Facility, 0, len(facilities))
	for _, fac := range facilities {
		facilitySlice = append(facilitySlice, fac)
	}
	if err := loadBoMFromDB(db, items, shipCatalog, facilitySlice); err != nil {
		log.Fatalf("load BOM from database: %v", err)
	}

	if err := writeHTMLPages(outDir, categories, items); err != nil {
		log.Fatalf("write HTML pages: %v", err)
	}

	fmt.Printf("Generated %d item pages + %d category pages in %s/\n", len(items), len(categories), outDir)

	// Group recipes by category.
	catRecipes := make(map[string][]*Recipe)
	for _, r := range recipes {
		catRecipes[r.Category] = append(catRecipes[r.Category], r)
	}
	for _, recipeList := range catRecipes {
		slices.SortFunc(recipeList, func(a, b *Recipe) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}

	recipeCategories := make([]RecipeCategoryInfo, 0, len(catRecipes))
	for cat, recipeList := range catRecipes {
		recipeCategories = append(recipeCategories, RecipeCategoryInfo{
			Name:        cat,
			DirName:     dirName(cat),
			Description: recipeCategoryDescriptions[cat],
			Count:       len(recipeList),
			Recipes:     recipeList,
		})
	}
	slices.SortFunc(recipeCategories, func(a, b RecipeCategoryInfo) int {
		return cmp.Compare(a.Name, b.Name)
	})

	if err := writeRecipePages(recipeOutDir, recipeCategories); err != nil {
		log.Fatalf("write recipe pages: %v", err)
	}

	fmt.Printf("Generated %d recipe pages + %d category pages in %s/\n", len(recipes), len(recipeCategories), recipeOutDir)

	// --- System generation ---
	generateAllSystems()

	// --- Skill generation ---
	skillCatalogPath := filepath.Join(catalogDir, "catalog_skills.json")
	skillOutDir := "kb/skills"
	skills, err := loadSkills(skillCatalogPath)
	if err != nil {
		log.Printf("warning: load skills: %v (skill pages will be skipped)", err)
	} else {
		if err := writeSkillPages(skillOutDir, skills); err != nil {
			log.Fatalf("write skill pages: %v", err)
		}
		fmt.Printf("Generated %d skill pages in %s/\n", len(skills), skillOutDir)
	}

	// --- Ship generation ---
	// shipCatalog was loaded earlier (before BoM calculation) and already has
	// BoM data attached via loadBoMFromDB.
	if shipErr == nil {
		recipeNames := make(map[string]string)
		for _, r := range recipes {
			recipeNames[r.ID] = r.Name
		}
		if err := writeShipPages("kb/ships", shipCatalog, recipeNames, items); err != nil {
			log.Fatalf("write ship pages: %v", err)
		}
		fmt.Printf("Generated %d ship entries in kb/ships/\n", len(shipCatalog))
	}

	// --- Facilities generation ---
	// facilities were loaded earlier (before BoM calculation) and already have
	// BoM data attached via loadBoMFromDB.
	if facErr == nil {
		facilityOutDir := "kb/facilities"
		if err := writeFacilityPages(facilityOutDir, facilities, recipes, items); err != nil {
			log.Fatalf("write facility pages: %v", err)
		}
		fmt.Printf("Generated %d facility pages in kb/facilities/\n", len(facilities))
	}

	// --- Missions generation ---
	generateAllMissions(items)
}

// generateAllMissions loads mission templates from the knowledge database and
// writes out the missions KB section.
func generateAllMissions(items map[string]*Item) {
	knowledgeDBPath := "../spacemolt-knowledge.db"
	missionOutDir := "kb/missions"

	knowledgeDB, err := sql.Open("sqlite", knowledgeDBPath)
	if err != nil {
		log.Printf("warning: open knowledge database for missions: %v (mission pages will be skipped)", err)
		return
	}
	defer func() { _ = knowledgeDB.Close() }()

	missions, err := loadMissions(knowledgeDB)
	if err != nil {
		log.Printf("warning: load missions: %v (mission pages will be skipped)", err)
		return
	}

	enrichMissionItemLinks(missions, items)
	if err := enrichMissionLocationNames(knowledgeDB, missions); err != nil {
		log.Printf("warning: enrich mission location names: %v", err)
	}

	if err := writeMissionPages(missionOutDir, missions); err != nil {
		log.Fatalf("write mission pages: %v", err)
	}
	fmt.Printf("Generated %d mission pages in %s/\n", len(missions), missionOutDir)
}

// generateAllSystems loads all systems and generates all system/planet/resource pages.
func generateAllSystems() {
	knowledgeDBPath := "../spacemolt-knowledge.db"
	systemOutDir := "kb/systems"

	knowledgeDB, err := sql.Open("sqlite", knowledgeDBPath)
	if err != nil {
		log.Printf("warning: open knowledge database: %v (system pages will be skipped)", err)
		return
	}
	defer func() { _ = knowledgeDB.Close() }()

	if err := kbdb.Migrate(knowledgeDB); err != nil {
		log.Fatalf("migrate metadata tables: %v", err)
	}

	systems, err := loadSystems(knowledgeDB)
	if err != nil {
		log.Fatalf("load systems: %v", err)
	}

	if err := writeSystemPages(systemOutDir, systems); err != nil {
		log.Fatalf("write system pages: %v", err)
	}
	fmt.Printf("Generated %d system pages in %s/\n", len(systems), systemOutDir)

	if err := persistStarMetadata(knowledgeDB, systems); err != nil {
		log.Fatalf("persist star metadata: %v", err)
	}

	if err := writePlanetPages(knowledgeDB, systemOutDir, systems); err != nil {
		log.Fatalf("write planet pages: %v", err)
	}

	resourceOutDir := "kb/resources"
	if err := writeResourcePages(resourceOutDir, knowledgeDB); err != nil {
		log.Fatalf("write resource pages: %v", err)
	}
	fmt.Printf("Generated resource index in %s/\n", resourceOutDir)
}

// generateOneSystem regenerates pages for a single system by ID.
func generateOneSystem(systemID string) {
	knowledgeDBPath := "../spacemolt-knowledge.db"
	systemOutDir := "kb/systems"

	knowledgeDB, err := sql.Open("sqlite", knowledgeDBPath)
	if err != nil {
		log.Fatalf("open knowledge database: %v", err)
	}
	defer func() { _ = knowledgeDB.Close() }()

	if err := kbdb.Migrate(knowledgeDB); err != nil {
		log.Fatalf("migrate metadata tables: %v", err)
	}

	// Load all systems — needed for connection/map rendering on the target page.
	systems, err := loadSystems(knowledgeDB)
	if err != nil {
		log.Fatalf("load systems: %v", err)
	}

	// Find the target system.
	var target *System
	for _, s := range systems {
		if s.ID == systemID {
			target = s
			break
		}
	}
	if target == nil {
		log.Fatalf("system %q not found in knowledge database", systemID)
	}

	// Write only this system's page (no index rebuild, no cleanup of other pages).
	if err := writeOneSystemPage(systemOutDir, target, systems); err != nil {
		log.Fatalf("write system page: %v", err)
	}
	fmt.Printf("Generated system page for %s in %s/%s/\n", target.Name, systemOutDir, target.ID)

	// Regenerate planet pages for this system.
	if err := writePlanetPages(knowledgeDB, systemOutDir, []*System{target}); err != nil {
		log.Fatalf("write planet pages: %v", err)
	}
}

func loadItems(db *sql.DB) (map[string]*Item, error) {
	rows, err := db.Query(`SELECT id, name, COALESCE(description,''), COALESCE(category,''), COALESCE(rarity,''), size, base_value, stackable, tradeable FROM items ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make(map[string]*Item)
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name, &it.Description, &it.Category, &it.Rarity, &it.Size, &it.BaseValue, &it.Stackable, &it.Tradeable); err != nil {
			return nil, err
		}
		items[it.ID] = &it
	}
	return items, rows.Err()
}

func loadProducedBy(db *sql.DB, items map[string]*Item) error {
	rows, err := db.Query(`
		SELECT ro.item_id, r.id, r.name, COALESCE(r.category,''), ro.quantity, r.crafting_time
		FROM recipe_outputs ro
		JOIN recipes r ON ro.recipe_id = r.id
		ORDER BY ro.item_id, r.id`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var itemID, recipeID, recipeName, recipeCat string
		var qty, craftTime int
		if err := rows.Scan(&itemID, &recipeID, &recipeName, &recipeCat, &qty, &craftTime); err != nil {
			return err
		}
		if it, ok := items[itemID]; ok {
			it.ProducedBy = append(it.ProducedBy, ProducedBy{
				RecipeID:       recipeID,
				RecipeName:     recipeName,
				RecipeCategory: recipeCat,
				Quantity:       qty,
				CraftingTime:   craftTime,
			})
		}
	}
	return rows.Err()
}

func loadUsedIn(db *sql.DB, items map[string]*Item) error {
	rows, err := db.Query(`
		SELECT ri.item_id, r.id, r.name, COALESCE(r.category,''), ri.quantity, ro.item_id, oi.name, COALESCE(oi.category, '')
		FROM recipe_inputs ri
		JOIN recipes r ON ri.recipe_id = r.id
		JOIN recipe_outputs ro ON r.id = ro.recipe_id
		JOIN items oi ON ro.item_id = oi.id
		ORDER BY ri.item_id, r.id, oi.name`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type key struct{ itemID, recipeID, outputID string }
	seen := make(map[key]struct{})

	for rows.Next() {
		var u UsedIn
		var itemID string
		if err := rows.Scan(&itemID, &u.RecipeID, &u.RecipeName, &u.RecipeCategory, &u.Quantity, &u.OutputID, &u.OutputName, &u.OutputCategory); err != nil {
			return err
		}
		k := key{itemID, u.RecipeID, u.OutputID}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		if it, ok := items[itemID]; ok {
			it.UsedIn = append(it.UsedIn, u)
		}
	}
	return rows.Err()
}

func yesno(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func fmtValue(v int) string {
	return humanize.Comma(int64(v)) + " cr"
}

func rarityClass(r string) string {
	switch strings.ToLower(r) {
	case "common":
		return "badge badge-common"
	case "uncommon":
		return "badge badge-uncommon"
	case "rare":
		return "badge badge-rare"
	case "exotic":
		return "badge badge-exotic"
	case "legendary":
		return "badge badge-legendary"
	default:
		return "badge"
	}
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	words := strings.Split(s, "_")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func loadShipBuildMaterials(shipCatalogPath string, items map[string]*Item) error {
	data, err := os.ReadFile(shipCatalogPath)
	if err != nil {
		return err
	}

	var catalog struct {
		Items []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			Category       string `json:"category"`
			Class          string `json:"class"`
			BuildMaterials []struct {
				ItemID   string `json:"item_id"`
				Quantity int    `json:"quantity"`
			} `json:"build_materials"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return err
	}

	for _, ship := range catalog.Items {
		for _, mat := range ship.BuildMaterials {
			if it, ok := items[mat.ItemID]; ok {
				it.UsedInShips = append(it.UsedInShips, ShipBuildRef{
					ShipID:       ship.ID,
					ShipName:     ship.Name,
					ShipCategory: ship.Category,
					ShipClass:    ship.Class,
					Quantity:     mat.Quantity,
				})
			}
		}
	}

	// Sort each item's ship refs by name.
	for _, it := range items {
		slices.SortFunc(it.UsedInShips, func(a, b ShipBuildRef) int {
			return cmp.Compare(a.ShipName, b.ShipName)
		})
	}
	return nil
}

func loadItemOverlay(catalogPath string, items map[string]*Item) error {
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return err
	}

	var catalog struct {
		Items []struct {
			ID         string `json:"id"`
			PowerBonus int    `json:"power_bonus"`
			Hazardous  bool   `json:"hazardous"`
			Hidden     bool   `json:"hidden"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return err
	}

	for _, ci := range catalog.Items {
		if it, ok := items[ci.ID]; ok {
			it.PowerBonus = ci.PowerBonus
			it.Hazardous = ci.Hazardous
			it.Hidden = ci.Hidden
		}
	}
	return nil
}

func loadRecipeOverlay(catalogPath string, recipes map[string]*Recipe) error {
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return err
	}

	var catalog struct {
		Items []struct {
			ID     string `json:"id"`
			Hidden bool   `json:"hidden"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return err
	}

	for _, cr := range catalog.Items {
		if r, ok := recipes[cr.ID]; ok {
			r.Hidden = cr.Hidden
		}
	}
	return nil
}

// findLatestCatalogDir finds the most recent YYYYMMDD snapshot directory
// under the given base path. Falls back to the base path itself if no
// date-named subdirectories exist.
func findLatestCatalogDir(base string) string {
	entries, err := os.ReadDir(base)
	if err != nil {
		log.Printf("warning: cannot read catalog base dir %s: %v (using as-is)", base, err)
		return base
	}
	var latest string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Match YYYYMMDD pattern (8 digits).
		if len(name) == 8 && name > latest {
			latest = name
		}
	}
	if latest == "" {
		log.Printf("warning: no date-stamped snapshot dirs in %s (using as-is)", base)
		return base
	}
	dir := filepath.Join(base, latest)
	log.Printf("Using catalog snapshot: %s", dir)
	return dir
}

func loadSystems(db *sql.DB) ([]*System, error) {
	rows, err := db.Query(`SELECT id, name, position_x, position_y, police_level, COALESCE(empire,''), COALESCE(description,''), is_stronghold, COALESCE(security_status,''), last_updated_tick FROM systems ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	systemMap := make(map[string]*System)
	var systems []*System
	for rows.Next() {
		var s System
		if err := rows.Scan(&s.ID, &s.Name, &s.PositionX, &s.PositionY, &s.PoliceLevel, &s.Empire, &s.Description, &s.IsStronghold, &s.SecurityStatus, &s.LastUpdatedTick); err != nil {
			return nil, err
		}
		if s.ID == "" {
			log.Printf("WARNING: skipping system with empty id: name=%q empire=%q pos=(%.1f,%.1f) police=%d stronghold=%v security=%q tick=%d",
				s.Name, s.Empire, s.PositionX, s.PositionY, s.PoliceLevel, s.IsStronghold, s.SecurityStatus, s.LastUpdatedTick)
			continue
		}
		systemMap[s.ID] = &s
		systems = append(systems, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load connections.
	connRows, err := db.Query(`SELECT from_system, to_system, distance FROM connections ORDER BY from_system`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = connRows.Close() }()

	for connRows.Next() {
		var fromID, toID string
		var distance int
		if err := connRows.Scan(&fromID, &toID, &distance); err != nil {
			return nil, err
		}
		if from, ok := systemMap[fromID]; ok {
			toName := toID
			if to, ok := systemMap[toID]; ok {
				toName = to.Name
			}
			from.Connections = append(from.Connections, SystemConnection{
				SystemID: toID,
				Name:     toName,
				Distance: distance,
			})
		}
	}
	if err := connRows.Err(); err != nil {
		return nil, err
	}

	// Sort connections by name.
	for _, s := range systems {
		slices.SortFunc(s.Connections, func(a, b SystemConnection) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}

	// Load POIs.
	poiRows, err := db.Query(`SELECT system_id, id, name, type, COALESCE(class,''), COALESCE(description,''), position_x, position_y, hidden, reveal_difficulty FROM pois ORDER BY system_id, name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = poiRows.Close() }()

	for poiRows.Next() {
		var systemID string
		var poi SystemPOI
		if err := poiRows.Scan(&systemID, &poi.ID, &poi.Name, &poi.Type, &poi.Class, &poi.Description, &poi.PositionX, &poi.PositionY, &poi.Hidden, &poi.RevealDifficulty); err != nil {
			return nil, err
		}
		if s, ok := systemMap[systemID]; ok {
			s.POIs = append(s.POIs, poi)
		}
	}
	if err := poiRows.Err(); err != nil {
		return nil, err
	}

	// Build POI lookup after all appends are done so pointers remain stable.
	poiLookup := make(map[string]*SystemPOI)
	for _, s := range systems {
		for i := range s.POIs {
			poiLookup[s.POIs[i].ID] = &s.POIs[i]
		}
	}

	// Load POI resources.
	resRows, err := db.Query(`
		SELECT pr.poi_id, pr.resource_id, COALESCE(i.name, pr.resource_id), pr.richness, pr.remaining
		FROM poi_resources pr
		LEFT JOIN items i ON pr.resource_id = i.id
		ORDER BY pr.poi_id, i.name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resRows.Close() }()

	for resRows.Next() {
		var poiID string
		var r POIResource
		if err := resRows.Scan(&poiID, &r.ResourceID, &r.ResourceName, &r.Richness, &r.Remaining); err != nil {
			return nil, err
		}
		if poi, ok := poiLookup[poiID]; ok {
			poi.Resources = append(poi.Resources, r)
		}
	}
	if err := resRows.Err(); err != nil {
		return nil, err
	}

	// Load bases (linked to POIs, which link to systems).
	baseRows, err := db.Query(`
		SELECT b.id, b.poi_id, b.name, COALESCE(b.description,''), COALESCE(b.story,''),
		       COALESCE(b.empire,''),
		       b.defense_level, b.has_drones, b.public_access,
		       COALESCE(b.condition,''), COALESCE(b.condition_text,''),
		       b.satisfaction_pct, b.satisfied_count, b.total_service_infra,
		       p.system_id
		FROM bases b
		JOIN pois p ON b.poi_id = p.id
		ORDER BY p.system_id, b.name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = baseRows.Close() }()

	for baseRows.Next() {
		var systemID string
		var base SystemBase
		if err := baseRows.Scan(&base.ID, &base.POIID, &base.Name, &base.Description, &base.Story,
			&base.Empire, &base.DefenseLevel, &base.HasDrones, &base.PublicAccess,
			&base.Condition, &base.ConditionText, &base.SatisfactionPct,
			&base.SatisfiedCount, &base.TotalServiceInfra, &systemID); err != nil {
			return nil, err
		}
		if s, ok := systemMap[systemID]; ok {
			s.Bases = append(s.Bases, base)
		}
	}
	if err := baseRows.Err(); err != nil {
		return nil, err
	}

	// Build base lookup after all appends are done so pointers remain stable.
	baseLookup := make(map[string]*SystemBase)
	for _, s := range systems {
		for i := range s.Bases {
			baseLookup[s.Bases[i].ID] = &s.Bases[i]
		}
	}

	// Load base services.
	svcRows, err := db.Query(`SELECT base_id, service_name, available FROM base_services ORDER BY base_id, service_name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = svcRows.Close() }()

	for svcRows.Next() {
		var baseID string
		var svc BaseService
		if err := svcRows.Scan(&baseID, &svc.Name, &svc.Available); err != nil {
			return nil, err
		}
		if base, ok := baseLookup[baseID]; ok {
			base.Services = append(base.Services, svc)
		}
	}
	if err := svcRows.Err(); err != nil {
		return nil, err
	}

	// Load base facilities.
	facRows, err := db.Query(`SELECT base_id, facility_name, COALESCE(category,'unknown'), level FROM base_facilities ORDER BY base_id, category, facility_name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = facRows.Close() }()

	for facRows.Next() {
		var baseID string
		var fac BaseFacility
		if err := facRows.Scan(&baseID, &fac.Name, &fac.Category, &fac.Level); err != nil {
			return nil, err
		}
		if base, ok := baseLookup[baseID]; ok {
			base.Facilities = append(base.Facilities, fac)
		}
	}
	if err := facRows.Err(); err != nil {
		return nil, err
	}

	return systems, nil
}

// systemTemplateFuncs returns the FuncMap used by system page templates.
// sysLookup must contain all systems for connection/map rendering.
func systemTemplateFuncs(sysLookup map[string]*System) htmltpl.FuncMap {
	return htmltpl.FuncMap{
		"titleCase":     titleCase,
		"securityClass": securityClass,
		"securityLabel": securityLabel,
		"fmtCoord":      func(f float64) string { return fmt.Sprintf("%.1f", f) },
		"poiIcon":       poiIcon,
		"hasResources":  func(pois []SystemPOI) bool { return poiHasResources(pois) },
		"resourcePOIs":  func(pois []SystemPOI) []SystemPOI { return filterResourcePOIs(pois) },
		"fmtRemaining":  fmtRemaining,
		"fmtRichness":   func(r float64) string { return fmt.Sprintf("%.0f", r) },
		"facilityBadge":      facilityBadge,
		"condenseFacilities": condenseFacilities,
		"conditionBadge":     conditionBadge,
		"titleCaseID":   titleCaseID,
		"sortPOIsByDist": func(pois []SystemPOI) []SystemPOI {
			sorted := make([]SystemPOI, len(pois))
			copy(sorted, pois)
			slices.SortFunc(sorted, func(a, b SystemPOI) int {
				da := math.Hypot(a.PositionX, a.PositionY)
				db := math.Hypot(b.PositionX, b.PositionY)
				return cmp.Compare(da, db)
			})
			return sorted
		},
		"poiDist": func(p SystemPOI) string {
			return fmt.Sprintf("%.1f", math.Hypot(p.PositionX, p.PositionY))
		},
		"poiPos": func(p SystemPOI) string {
			return fmt.Sprintf("(%.1f, %.1f)", p.PositionX, p.PositionY)
		},
		"poiDetailLink": func(p SystemPOI) string {
			if p.Type == "planet" {
				return fmt.Sprintf("planet_%s.html", sanitizeName(p.Name))
			}
			return ""
		},
		"systemMap": func(sys *System) htmltpl.HTML {
			allMap := make(map[string]*systemmap.System, len(sysLookup))
			for k, v := range sysLookup {
				allMap[k] = toMapSystem(v)
			}
			return htmltpl.HTML(systemmap.RenderSystemMap(toMapSystem(sys), allMap, false))
		},
		"isKnownEmpire": func(empire string) bool {
			for _, e := range empireOrder {
				if e == empire {
					return true
				}
			}
			return false
		},
		"formatEmpire": func(empire string, lastUpdatedTick int) string {
			if lastUpdatedTick == 0 {
				return "Unknown"
			}
			if empire == "" {
				return "Neutral"
			}
			return titleCase(empire)
		},
	}
}

func writeSystemPages(outDir string, systems []*System) error {
	sysLookup := make(map[string]*System, len(systems))
	for _, s := range systems {
		sysLookup[s.ID] = s
	}

	funcs := systemTemplateFuncs(sysLookup)
	indexTmpl := htmltpl.Must(htmltpl.New("idx").Funcs(funcs).Parse(systemIndexTemplate))
	detailTmpl := htmltpl.Must(htmltpl.New("detail").Funcs(funcs).Parse(systemDetailTemplate))

	// Clean generated HTML files, preserving CSS.
	entries, err := os.ReadDir(outDir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".html") {
				_ = os.Remove(filepath.Join(outDir, e.Name()))
			}
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// Build empire groups.
	empireMap := make(map[string][]*System)
	for _, s := range systems {
		key := strings.ToLower(strings.TrimSpace(s.Empire))
		if key == "" || key == "neutral" {
			continue
		}
		empireMap[key] = append(empireMap[key], s)
	}

	var empires []EmpireGroup
	for _, eid := range empireOrder {
		eSystems := empireMap[eid]
		if len(eSystems) == 0 {
			continue
		}
		slices.SortFunc(eSystems, func(a, b *System) int {
			return cmp.Compare(a.Name, b.Name)
		})
		color := empireColors[eid]
		eg := EmpireGroup{
			Name:    titleCase(eid),
			ID:      eid,
			Color:   color,
			Systems: eSystems,
		}
		eg.MapSVG = htmltpl.HTML(renderEmpireMap(eg, sysLookup))
		empires = append(empires, eg)
	}

	// Calculate exploration statistics.
	totalSystems := len(systems)
	exploredSystems := 0
	for _, s := range systems {
		if s.LastUpdatedTick > 0 {
			exploredSystems++
		}
	}
	explorationPct := 0.0
	if totalSystems > 0 {
		explorationPct = 100.0 * float64(exploredSystems) / float64(totalSystems)
	}

	indexData := SystemsIndexData{
		Systems:          systems,
		Empires:         empires,
		TotalSystems:    totalSystems,
		ExploredSystems: exploredSystems,
		ExplorationPct:  explorationPct,
	}

	// Index page.
	idxPath := filepath.Join(outDir, "index.html")
	f, err := os.Create(idxPath)
	if err != nil {
		return err
	}
	if err := indexTmpl.Execute(f, indexData); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	// Individual system pages (in subdirectories).
	for _, sys := range systems {
		sysDir := filepath.Join(outDir, sys.ID)
		if err := os.MkdirAll(sysDir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(sysDir, "index.html")
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		if err := detailTmpl.Execute(f, sys); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

// writeOneSystemPage regenerates a single system's detail page.
func writeOneSystemPage(outDir string, target *System, allSystems []*System) error {
	sysLookup := make(map[string]*System, len(allSystems))
	for _, s := range allSystems {
		sysLookup[s.ID] = s
	}

	funcs := systemTemplateFuncs(sysLookup)
	detailTmpl := htmltpl.Must(htmltpl.New("detail").Funcs(funcs).Parse(systemDetailTemplate))

	sysDir := filepath.Join(outDir, target.ID)
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(sysDir, "index.html")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := detailTmpl.Execute(f, target); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func toMapSystem(s *System) *systemmap.System {
	ms := &systemmap.System{
		ID:        s.ID,
		Name:      s.Name,
		PositionX: s.PositionX,
		PositionY: s.PositionY,
		Security:  s.PoliceLevel,
	}
	for _, c := range s.Connections {
		ms.Connections = append(ms.Connections, systemmap.Connection{
			SystemID: c.SystemID,
			Name:     c.Name,
			Distance: c.Distance,
		})
	}
	for _, p := range s.POIs {
		ms.POIs = append(ms.POIs, systemmap.POI{
			ID:          p.ID,
			Name:        p.Name,
			Type:        p.Type,
			Class:       p.Class,
			Description: p.Description,
			PositionX:   p.PositionX,
			PositionY:   p.PositionY,
		})
	}
	return ms
}

// renderEmpireMap generates an SVG showing the network of systems in an empire.
// Systems are rendered as dots with connections drawn between them, and the whole
// region is highlighted with the empire's color using an SVG metaball filter.
func renderEmpireMap(empire EmpireGroup, sysLookup map[string]*System) string {
	systems := empire.Systems
	if len(systems) == 0 {
		return ""
	}

	// Build a set of system IDs in this empire for fast lookup.
	inEmpire := make(map[string]bool, len(systems))
	for _, s := range systems {
		inEmpire[s.ID] = true
	}

	// Compute bounding box.
	minX, minY := systems[0].PositionX, systems[0].PositionY
	maxX, maxY := minX, minY
	for _, s := range systems[1:] {
		if s.PositionX < minX {
			minX = s.PositionX
		}
		if s.PositionX > maxX {
			maxX = s.PositionX
		}
		if s.PositionY < minY {
			minY = s.PositionY
		}
		if s.PositionY > maxY {
			maxY = s.PositionY
		}
	}

	// Add padding.
	padX := (maxX - minX) * 0.15
	padY := (maxY - minY) * 0.15
	if padX < 30 {
		padX = 30
	}
	if padY < 30 {
		padY = 30
	}
	minX -= padX
	minY -= padY
	maxX += padX
	maxY += padY

	rangeX := maxX - minX
	rangeY := maxY - minY
	if rangeX < 1 {
		rangeX = 1
	}
	if rangeY < 1 {
		rangeY = 1
	}

	// SVG dimensions — square panel.
	const svgSize = 500.0
	scale := svgSize / max(rangeX, rangeY)

	// Transform galaxy coords to SVG coords.
	tx := func(x float64) float64 {
		return (x - minX) * scale
	}
	ty := func(y float64) float64 {
		return (y - minY) * scale
	}
	svgW := rangeX * scale
	svgH := rangeY * scale

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %.0f %.0f" xmlns="http://www.w3.org/2000/svg" class="empire-map-svg">`, svgW, svgH))

	// Metaball filter for territory blob.
	filterID := "goo-" + empire.ID
	b.WriteString(fmt.Sprintf(`<defs><filter id="%s" x="-20%%" y="-20%%" width="140%%" height="140%%" colorInterpolationFilters="sRGB">`, filterID))
	b.WriteString(`<feGaussianBlur in="SourceGraphic" stdDeviation="18" result="blur"/>`)
	b.WriteString(`<feColorMatrix in="blur" type="matrix" values="1 0 0 0 0  0 1 0 0 0  0 0 1 0 0  0 0 0 30 -12" result="blob"/>`)
	b.WriteString(`<feComponentTransfer in="blob" result="fill"><feFuncA type="linear" slope="0.25" intercept="0"/></feComponentTransfer>`)
	b.WriteString(`</filter></defs>`)

	// Territory blob — circles at each system position plus thick connector
	// lines, all merged by the blur filter into one contiguous shape.
	blobR := 28.0 * (svgSize / 500.0)
	if blobR < 18 {
		blobR = 18
	}
	b.WriteString(fmt.Sprintf(`<g filter="url(#%s)">`, filterID))
	// Thick connection lines so the blob merges across edges.
	drawnBlob := make(map[string]bool)
	for _, s := range systems {
		for _, conn := range s.Connections {
			if !inEmpire[conn.SystemID] {
				continue
			}
			key := s.ID + "|" + conn.SystemID
			rev := conn.SystemID + "|" + s.ID
			if drawnBlob[key] || drawnBlob[rev] {
				continue
			}
			drawnBlob[key] = true
			target := sysLookup[conn.SystemID]
			if target == nil {
				continue
			}
			b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.0f"/>`,
				tx(s.PositionX), ty(s.PositionY), tx(target.PositionX), ty(target.PositionY), empire.Color, blobR*1.2))
		}
	}
	for _, s := range systems {
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.0f" fill="%s"/>`, tx(s.PositionX), ty(s.PositionY), blobR, empire.Color))
	}
	b.WriteString(`</g>`)

	// Connection lines (visible, on top of blob).
	b.WriteString(`<g stroke="#c8d0e0" stroke-width="1.5" opacity="0.6">`)
	drawn := make(map[string]bool)
	for _, s := range systems {
		for _, conn := range s.Connections {
			if !inEmpire[conn.SystemID] {
				continue
			}
			key := s.ID + "|" + conn.SystemID
			rev := conn.SystemID + "|" + s.ID
			if drawn[key] || drawn[rev] {
				continue
			}
			drawn[key] = true
			target := sysLookup[conn.SystemID]
			if target == nil {
				continue
			}
			b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
				tx(s.PositionX), ty(s.PositionY), tx(target.PositionX), ty(target.PositionY)))
		}
	}
	b.WriteString(`</g>`)

	// Outgoing connections to systems outside this empire (dashed).
	b.WriteString(`<g stroke="#c8d0e0" stroke-width="1.5" opacity="0.6" stroke-dasharray="6,4">`)
	for _, s := range systems {
		for _, conn := range s.Connections {
			if inEmpire[conn.SystemID] {
				continue
			}
			target := sysLookup[conn.SystemID]
			if target == nil {
				continue
			}
			b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
				tx(s.PositionX), ty(s.PositionY), tx(target.PositionX), ty(target.PositionY)))
		}
	}
	b.WriteString(`</g>`)

	// System dots and labels.
	capitalID := empireCapitals[empire.ID]
	for _, s := range systems {
		sx, sy := tx(s.PositionX), ty(s.PositionY)

		// Dot.
		dotColor := empire.Color
		if s.IsStronghold {
			dotColor = "#FF0000"
		}
		b.WriteString(fmt.Sprintf(`<a href="%s.html"><circle cx="%.1f" cy="%.1f" r="3.5" fill="%s" stroke="#000" stroke-width="0.5" class="empire-sys-dot"><title>%s</title></circle>`,
			s.ID, sx, sy, dotColor, s.Name))

		// Capital star overlay.
		if s.ID == capitalID {
			b.WriteString(renderFivePointStar(sx, sy, 10, empire.Color))
		}

		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="empire-sys-label" fill="#d8dee9">%s</text></a>`,
			sx+7, sy+5, s.Name))
	}

	b.WriteString(`</svg>`)
	return b.String()
}

// renderFivePointStar draws a 5-point star centered at (cx, cy) with the given outer radius.
func renderFivePointStar(cx, cy, r float64, color string) string {
	var b strings.Builder
	inner := r * 0.4
	b.WriteString(`<polygon points="`)
	for i := range 10 {
		angle := math.Pi/2 + float64(i)*math.Pi/5 // start at top
		rad := r
		if i%2 == 1 {
			rad = inner
		}
		px := cx + rad*math.Cos(angle)
		py := cy - rad*math.Sin(angle)
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(fmt.Sprintf("%.1f,%.1f", px, py))
	}
	b.WriteString(fmt.Sprintf(`" fill="%s" stroke="#000" stroke-width="0.5" opacity="0.9"/>`, color))
	return b.String()
}

func poiHasResources(pois []SystemPOI) bool {
	for _, poi := range pois {
		if len(poi.Resources) > 0 {
			return true
		}
	}
	return false
}

func condenseFacilities(facs []BaseFacility) []CondensedFacility {
	type key struct {
		Name     string
		Category string
		Level    int
	}
	counts := make(map[key]int)
	var order []key
	for _, f := range facs {
		k := key(f)
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k]++
	}
	result := make([]CondensedFacility, 0, len(order))
	for _, k := range order {
		result = append(result, CondensedFacility{
			Name:     k.Name,
			Category: k.Category,
			Level:    k.Level,
			Count:    counts[k],
		})
	}
	return result
}

func filterResourcePOIs(pois []SystemPOI) []SystemPOI {
	var result []SystemPOI
	for _, poi := range pois {
		if len(poi.Resources) > 0 {
			result = append(result, poi)
		}
	}
	return result
}

func fmtRemaining(r float64) string {
	if r <= 0 {
		return "depleted"
	}
	return humanize.Comma(int64(r))
}

func facilityBadge(category string) string {
	switch category {
	case "production":
		return "badge-orange"
	case "service":
		return "badge-frost"
	case "infrastructure":
		return ""
	default:
		return ""
	}
}

func conditionBadge(condition string) string {
	switch strings.ToLower(condition) {
	case "pristine", "excellent":
		return "badge-green"
	case "good", "operational":
		return "badge-frost"
	case "fair", "struggling":
		return "badge-yellow"
	case "poor", "critical":
		return "badge-red"
	case "degraded":
		return "badge-orange"
	default:
		return ""
	}
}

func titleCaseID(s string) string {
	return titleCase(strings.ReplaceAll(s, "_", " "))
}

func securityClass(policeLevel int) string {
	switch {
	case policeLevel >= 60:
		return "security-high"
	case policeLevel >= 30:
		return "security-med"
	default:
		return "security-low"
	}
}

func securityLabel(policeLevel int) string {
	switch {
	case policeLevel >= 60:
		return "High"
	case policeLevel >= 30:
		return "Medium"
	case policeLevel > 0:
		return "Low"
	default:
		return "Lawless"
	}
}

func poiIcon(poiType string) string {
	switch poiType {
	case "sun":
		return "\u2600" // ☀
	case "planet":
		return "\u25CF" // ●
	case "station":
		return "\u2B21" // ⬡
	case "asteroid_belt":
		return "\u25C8" // ◈
	case "gas_cloud":
		return "\u2601" // ☁
	case "ice_field":
		return "\u2744" // ❄
	case "relic":
		return "\u2726" // ✦
	default:
		return "\u25CB" // ○
	}
}

func loadRecipes(db *sql.DB) (map[string]*Recipe, error) {
	rows, err := db.Query(`SELECT id, name, COALESCE(description,''), COALESCE(category,''), crafting_time FROM recipes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	recipes := make(map[string]*Recipe)
	for rows.Next() {
		var r Recipe
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Category, &r.CraftingTime); err != nil {
			return nil, err
		}
		recipes[r.ID] = &r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load inputs.
	inputRows, err := db.Query(`
		SELECT ri.recipe_id, ri.item_id, COALESCE(i.name, ri.item_id), COALESCE(i.category,''), ri.quantity
		FROM recipe_inputs ri
		LEFT JOIN items i ON ri.item_id = i.id
		ORDER BY ri.recipe_id, COALESCE(i.name, ri.item_id)`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = inputRows.Close() }()

	for inputRows.Next() {
		var recipeID string
		var ri RecipeItem
		if err := inputRows.Scan(&recipeID, &ri.ItemID, &ri.ItemName, &ri.ItemCategory, &ri.Quantity); err != nil {
			return nil, err
		}
		if r, ok := recipes[recipeID]; ok {
			r.Inputs = append(r.Inputs, ri)
		}
	}
	if err := inputRows.Err(); err != nil {
		return nil, err
	}

	// Load outputs.
	outputRows, err := db.Query(`
		SELECT ro.recipe_id, ro.item_id, COALESCE(i.name, ro.item_id), COALESCE(i.category,''), ro.quantity
		FROM recipe_outputs ro
		LEFT JOIN items i ON ro.item_id = i.id
		ORDER BY ro.recipe_id, COALESCE(i.name, ro.item_id)`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = outputRows.Close() }()

	for outputRows.Next() {
		var recipeID string
		var ri RecipeItem
		if err := outputRows.Scan(&recipeID, &ri.ItemID, &ri.ItemName, &ri.ItemCategory, &ri.Quantity); err != nil {
			return nil, err
		}
		if r, ok := recipes[recipeID]; ok {
			r.Outputs = append(r.Outputs, ri)
		}
	}
	if err := outputRows.Err(); err != nil {
		return nil, err
	}

	return recipes, nil
}

func writeRecipePages(outDir string, categories []RecipeCategoryInfo) error {
	funcs := htmltpl.FuncMap{
		"titleCase": titleCase,
		"dirName":   dirName,
	}
	topTmpl := htmltpl.Must(htmltpl.New("top").Funcs(funcs).Parse(recipeTopTemplate))
	catTmpl := htmltpl.Must(htmltpl.New("cat").Funcs(funcs).Parse(recipeCatTemplate))
	detailTmpl := htmltpl.Must(htmltpl.New("detail").Funcs(funcs).Parse(recipeDetailTemplate))

	// Clean generated HTML files, preserving CSS.
	if err := cleanGeneratedFiles(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// Top-level index.html.
	topPath := filepath.Join(outDir, "index.html")
	f, err := os.Create(topPath)
	if err != nil {
		return err
	}
	if err := topTmpl.Execute(f, categories); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	// Per-category index.html.
	for _, cat := range categories {
		catDir := filepath.Join(outDir, cat.DirName)
		if err := os.MkdirAll(catDir, 0o755); err != nil {
			return err
		}
		catPath := filepath.Join(catDir, "index.html")
		f, err := os.Create(catPath)
		if err != nil {
			return err
		}
		if err := catTmpl.Execute(f, cat); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}

	// Per-recipe detail pages.
	for _, cat := range categories {
		for _, recipe := range cat.Recipes {
			path := filepath.Join(outDir, cat.DirName, recipe.ID+".html")
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			if err := detailTmpl.Execute(f, recipe); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func cleanGeneratedFiles(outDir string) error {
	entries, err := os.ReadDir(outDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		path := filepath.Join(outDir, e.Name())
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".html") {
			if err := os.Remove(path); err != nil {
				return err
			}
		} else if e.IsDir() && e.Name() != "images" {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeHTMLPages(outDir string, categories []CategoryInfo, items map[string]*Item) error {
	funcs := htmltpl.FuncMap{
		"yesno":          yesno,
		"fmtValue":       fmtValue,
		"rarityClass":    rarityClass,
		"titleCase":      titleCase,
		"dirName":        dirName,
		"resourceAnchor": resourceAnchor,
		"isResourceItem": isResourceItem,
		"hasBoM": func(b *bom.BoMResult) bool {
			return b != nil && len(b.BaseMaterials) > 0
		},
		"boMJSON": func(b *bom.BoMResult) string { return b.JSON() },
		"boMTable": func(bom *bom.BoMResult) htmltpl.HTML {
			if bom == nil || len(bom.BaseMaterials) == 0 {
				return ""
			}

			var sb strings.Builder
			sb.WriteString(`<div class="card" style="padding:0">`)
			sb.WriteString(`<div class="section-label">Construction</div>`)
			sb.WriteString(`<div class="bom-summary-table">`)
			sb.WriteString(`<table><thead><tr><th>Base Material</th><th>Quantity</th></tr></thead><tbody>`)

			for _, mat := range bom.BaseMaterials {
				item, ok := items[mat.ItemID]
				if !ok {
					sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td></tr>`, mat.ItemID, mat.Quantity))
					continue
				}

				var categoryLink string
				if item.Category == "ore" {
					categoryLink = "ore"
				} else if item.Category == "material" {
					categoryLink = "material"
				} else {
					categoryLink = item.Category
				}

				sb.WriteString(fmt.Sprintf(`<tr><td><a href="../%s/%s.html">%s</a></td><td>%d</td></tr>`,
					categoryLink, mat.ItemID, item.Name, mat.Quantity))
			}

			sb.WriteString(`</tbody></table></div>`)
			sb.WriteString(`</div>`)
			return htmltpl.HTML(sb.String())
		},
	}
	topTmpl := htmltpl.Must(htmltpl.New("top").Funcs(funcs).Parse(htmlTopTemplate))
	catTmpl := htmltpl.Must(htmltpl.New("cat").Funcs(funcs).Parse(htmlCatTemplate))
	itemHTMLTmpl := htmltpl.Must(htmltpl.New("item").Funcs(funcs).Parse(htmlItemTemplate))

	// Top-level index.html.
	topPath := filepath.Join(outDir, "index.html")
	f, err := os.Create(topPath)
	if err != nil {
		return err
	}
	if err := topTmpl.Execute(f, categories); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	// Per-category index.html.
	for _, cat := range categories {
		catDir := filepath.Join(outDir, cat.Name)
		if err := os.MkdirAll(catDir, 0o755); err != nil {
			return err
		}
		catPath := filepath.Join(catDir, "index.html")
		f, err := os.Create(catPath)
		if err != nil {
			return err
		}
		if err := catTmpl.Execute(f, cat); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}

	// Per-item HTML pages.
	for _, item := range items {
		path := filepath.Join(outDir, item.Category, item.ID+".html")
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		if err := itemHTMLTmpl.Execute(f, item); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Shared HTML fragments.
var siteHeader = `    <header class="site-header">
        <h1><a href="../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>
            <a href="../">Home</a>
            <a href="../systems/index.html">Systems</a>
            <a href="../items/index.html">Items</a>
            <a href="../recipes/index.html">Recipes</a>
            <a href="../skills/index.html">Skills</a>
            <a href="../ships/index.html">Ships</a>
            <a href="../facilities/index.html">Facilities</a>
            <a href="../resources/index.html">Resources</a>
            <a href="../missions/index.html">Missions</a>
            <button class="theme-toggle" id="theme-toggle" aria-label="Toggle theme">
                <svg class="icon-sun" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
                <svg class="icon-moon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
            </button>
        </nav>
    </header>`

// siteHeaderSub is the header for pages one level deeper (category/item pages).
var siteHeaderSub = `    <header class="site-header">
        <h1><a href="../../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>
            <a href="../../">Home</a>
            <a href="../../systems/index.html">Systems</a>
            <a href="../../items/index.html">Items</a>
            <a href="../../recipes/index.html">Recipes</a>
            <a href="../../skills/index.html">Skills</a>
            <a href="../../ships/index.html">Ships</a>
            <a href="../../facilities/index.html">Facilities</a>
            <a href="../../resources/index.html">Resources</a>
            <a href="../../missions/index.html">Missions</a>
            <button class="theme-toggle" id="theme-toggle" aria-label="Toggle theme">
                <svg class="icon-sun" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
                <svg class="icon-moon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
            </button>
        </nav>
    </header>`

var themeScript = `    <script>
    (function() {
        var toggle = document.getElementById('theme-toggle');
        var root = document.documentElement;
        var stored = localStorage.getItem('theme');
        if (stored === 'dark') root.classList.add('dark');
        toggle.addEventListener('click', function() {
            root.classList.toggle('dark');
            localStorage.setItem('theme', root.classList.contains('dark') ? 'dark' : 'light');
        });
    })();
    </script>`

var sortScript = `    <script>
    document.querySelectorAll("table.sortable").forEach(function(table) {
      var headers = table.querySelectorAll("th.sortable");
      var sortCol = -1, sortAsc = true;
      headers.forEach(function(th) {
        var idx = th.cellIndex;
        th.addEventListener("click", function() {
          if (sortCol === idx) { sortAsc = !sortAsc; } else { sortCol = idx; sortAsc = true; }
          table.querySelectorAll("th .sort-arrow").forEach(function(a) { a.remove(); });
          var arrow = document.createElement("span");
          arrow.className = "sort-arrow";
          arrow.textContent = sortAsc ? "\u25B2" : "\u25BC";
          th.appendChild(arrow);
          var tbody = table.querySelector("tbody");
          var rows = Array.from(tbody.querySelectorAll("tr"));
          rows.sort(function(a, b) {
            var at = a.cells[idx].getAttribute("data-sort") || a.cells[idx].textContent.trim();
            var bt = b.cells[idx].getAttribute("data-sort") || b.cells[idx].textContent.trim();
            var an = parseFloat(at), bn = parseFloat(bt);
            if (!isNaN(an) && !isNaN(bn)) return sortAsc ? an - bn : bn - an;
            return sortAsc ? at.localeCompare(bt) : bt.localeCompare(at);
          });
          rows.forEach(function(r) { tbody.appendChild(r); });
        });
      });
    });
    </script>`

var htmlTopTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Items - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../items/items.css">
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <h2>Items</h2>
        <p class="text-muted mt-1">{{len .}} categories of ore, components, modules, and trade goods.</p>
        <div class="item-categories">
{{- range .}}
            <a href="{{.Name}}/" class="item-cat-card">
                <div class="cat-count">{{.Count}} items</div>
                <div class="cat-name">{{titleCase .Name}}</div>
                <div class="cat-desc">{{.Description}}</div>
            </a>
{{- end}}
        </div>
    </main>
` + themeScript + `
</body>
</html>
`

var htmlCatTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{titleCase .Name}} - Items - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../items/items.css">
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Items</a> / {{titleCase .Name}}</div>
        <h2>{{titleCase .Name}}</h2>
        <p class="text-muted mt-1">{{.Description}}</p>
        <div class="card mt-3" style="padding:0">
        <table class="sortable">
        <thead>
        <tr><th class="sortable">Name</th><th></th><th class="sortable">Rarity</th><th class="sortable" style="text-align:right">Size</th><th class="sortable" style="text-align:right">Base Value</th><th>Description</th></tr>
        </thead>
        <tbody>
{{- range .Items}}
        <tr>
          <td><a href="{{.ID}}.html">{{.Name}}</a>{{if .Hazardous}} <span class="badge badge-hazardous" title="Hazardous">&#x2622;</span>{{end}}{{if .Hidden}} <span class="badge badge-hidden" title="Hidden">H</span>{{end}}</td>
          <td class="thumb">{{if .HasImage}}<img src="../images/{{.ID}}.png" alt="{{.Name}}">{{end}}</td>
          <td><span class="{{rarityClass .Rarity}}">{{.Rarity}}</span></td>
          <td class="size" data-sort="{{.Size}}">{{.Size}}</td>
          <td class="value" data-sort="{{.BaseValue}}">{{fmtValue .BaseValue}}</td>
          <td>{{.Description}}</td>
        </tr>
{{- end}}
        </tbody>
        </table>
        </div>
    </main>
` + sortScript + `
` + themeScript + `
</body>
</html>
`

var htmlItemTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} - Items - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../items/items.css">
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Items</a> / <a href="./">{{titleCase .Category}}</a> / {{.Name}}</div>
        <h2>{{.Name}}{{if .Hazardous}} <span class="badge badge-hazardous" title="Hazardous Material">&#x2622; Hazardous</span>{{end}}{{if .Hidden}} <span class="badge badge-hidden" title="Hidden Item">Hidden</span>{{end}}</h2>

        <div class="card mt-2" style="padding:0">
{{- if .HasImage}}
          <div class="item-image">
            <img src="../images/{{.ID}}.png" alt="{{.Name}}" height="200">
          </div>
{{- end}}
          <blockquote class="item-desc">{{.Description}}</blockquote>
          <div class="section-label">General</div>
          <table>
            <tr><td class="kv-label">Category</td><td><a href="./">{{titleCase .Category}}</a></td></tr>
            <tr><td class="kv-label">Rarity</td><td><span class="{{rarityClass .Rarity}}">{{.Rarity}}</span></td></tr>
            <tr><td class="kv-label">Size</td><td>{{.Size}}</td></tr>
            <tr><td class="kv-label">Stackable</td><td>{{yesno .Stackable}}</td></tr>
            <tr><td class="kv-label">Tradeable</td><td>{{yesno .Tradeable}}</td></tr>
{{- if (isResourceItem .Category)}}
            <tr><td class="kv-label">Resource Locations</td><td><a href="../../resources/index.html#{{resourceAnchor .ID}}">View systems with {{.Name}}</a></td></tr>
{{- end}}
{{- if .PowerBonus}}
            <tr><td class="kv-label">Power Bonus</td><td><span class="stat-positive">+{{.PowerBonus}}</span></td></tr>
{{- end}}
          </table>
          <div class="section-label">Market</div>
          <table>
            <tr><td class="kv-label">Base Value</td><td>{{fmtValue .BaseValue}}</td></tr>
          </table>
        </div>

{{- if or .ProducedBy .UsedIn .UsedInShips}}
        <div class="card" style="padding:0">
{{- if .ProducedBy}}
          <div class="section-label">Produced By</div>
          <table>
            <thead><tr><th>Recipe</th><th>Qty</th><th>Crafting Time</th></tr></thead>
            <tbody>
{{- range .ProducedBy}}
            <tr>
              <td><a href="../../recipes/{{dirName .RecipeCategory}}/{{.RecipeID}}.html">{{.RecipeName}}</a></td>
              <td>{{.Quantity}}</td>
              <td>{{.CraftingTime}} ticks</td>
            </tr>
{{- end}}
            </tbody>
          </table>
{{- end}}
{{- if .UsedIn}}
          <div class="section-label">Used In</div>
          <table>
            <thead><tr><th>Recipe</th><th>Qty</th><th>Produces</th></tr></thead>
            <tbody>
{{- range .UsedIn}}
            <tr>
              <td><a href="../../recipes/{{dirName .RecipeCategory}}/{{.RecipeID}}.html">{{.RecipeName}}</a></td>
              <td>{{.Quantity}}</td>
              <td><a href="../{{.OutputCategory}}/{{.OutputID}}.html">{{.OutputName}}</a></td>
            </tr>
{{- end}}
            </tbody>
          </table>
{{- end}}
{{- if .UsedInShips}}
          <div class="section-label">Used to Build Ships</div>
          <table>
            <thead><tr><th>Ship</th><th>Category</th><th>Qty</th></tr></thead>
            <tbody>
{{- range .UsedInShips}}
            <tr>
              <td><a href="../../ships/{{.ShipCategory}}/{{.ShipID}}.html">{{.ShipName}}</a></td>
              <td>{{.ShipCategory}}</td>
              <td>{{.Quantity}}</td>
            </tr>
{{- end}}
            </tbody>
          </table>
{{- end}}
        </div>
{{- end}}

{{- if hasBoM .BoM}}
        {{boMTable .BoM}}
        <details class="bom-json-details">
          <summary>View JSON Data</summary>
          <pre class="bom-json">{{boMJSON .BoM}}</pre>
        </details>
{{- end}}

    </main>
` + themeScript + `
</body>
</html>
`

// Recipe templates.

var recipeTopTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Recipes - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../recipes/recipes.css">
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <h2>Recipes</h2>
        <p class="text-muted mt-1">{{len .}} categories of crafting recipes.</p>
        <div class="item-categories">
{{- range .}}
            <a href="{{.DirName}}/" class="item-cat-card">
                <div class="cat-count">{{.Count}} recipes</div>
                <div class="cat-name">{{.Name}}</div>
                <div class="cat-desc">{{.Description}}</div>
            </a>
{{- end}}
        </div>
    </main>
` + themeScript + `
</body>
</html>
`

var recipeCatTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} - Recipes - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../recipes/recipes.css">
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Recipes</a> / {{.Name}}</div>
        <h2>{{.Name}}</h2>
        <p class="text-muted mt-1">{{.Description}}</p>
        <div class="card mt-3" style="padding:0">
        <table class="sortable">
        <thead>
        <tr><th class="sortable">Recipe</th><th class="sortable">Output</th><th>Inputs</th><th class="sortable" style="text-align:right">Time</th></tr>
        </thead>
        <tbody>
{{- range .Recipes}}
        <tr>
          <td><a href="{{.ID}}.html">{{.Name}}</a>{{if .Hidden}} <span class="badge badge-hidden" title="Hidden">H</span>{{end}}</td>
          <td>{{- range .Outputs}}<a href="../../items/{{.ItemCategory}}/{{.ItemID}}.html" class="recipe-item">{{if .HasImage}}<img src="../../items/images/{{.ItemID}}.png" alt="{{.ItemName}}" class="recipe-thumb">{{end}}{{.ItemName}}{{if gt .Quantity 1}} &times;{{.Quantity}}{{end}}</a>{{end}}</td>
          <td class="recipe-inputs">{{- range $i, $inp := .Inputs}}{{if $i}}, {{end}}<a href="../../items/{{$inp.ItemCategory}}/{{$inp.ItemID}}.html">{{$inp.ItemName}}</a>&nbsp;&times;{{$inp.Quantity}}{{end}}</td>
          <td class="time" data-sort="{{.CraftingTime}}">{{.CraftingTime}} ticks</td>
        </tr>
{{- end}}
        </tbody>
        </table>
        </div>
    </main>
` + sortScript + `
` + themeScript + `
</body>
</html>
`

var recipeDetailTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} - Recipes - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../recipes/recipes.css">
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Recipes</a> / <a href="./">{{.Category}}</a> / {{.Name}}</div>
        <h2>{{.Name}}{{if .Hidden}} <span class="badge badge-hidden" title="Hidden Recipe">Hidden</span>{{end}}</h2>

        <blockquote class="item-desc">{{.Description}}</blockquote>

        <div class="card mt-2" style="padding:0">
          <div class="section-label">Output</div>
          <table>
            <thead><tr><th></th><th>Item</th><th>Quantity</th></tr></thead>
            <tbody>
{{- range .Outputs}}
            <tr>
              <td class="thumb">{{if .HasImage}}<img src="../../items/images/{{.ItemID}}.png" alt="{{.ItemName}}">{{end}}</td>
              <td><a href="../../items/{{.ItemCategory}}/{{.ItemID}}.html">{{.ItemName}}</a></td>
              <td>{{.Quantity}}</td>
            </tr>
{{- end}}
            </tbody>
          </table>

          <div class="section-label">Inputs</div>
          <table>
            <thead><tr><th></th><th>Item</th><th>Quantity</th></tr></thead>
            <tbody>
{{- range .Inputs}}
            <tr>
              <td class="thumb">{{if .HasImage}}<img src="../../items/images/{{.ItemID}}.png" alt="{{.ItemName}}">{{end}}</td>
              <td><a href="../../items/{{.ItemCategory}}/{{.ItemID}}.html">{{.ItemName}}</a></td>
              <td>{{.Quantity}}</td>
            </tr>
{{- end}}
            </tbody>
          </table>

          <div class="section-label">Details</div>
          <table>
            <tr><td class="kv-label">Category</td><td><a href="./">{{.Category}}</a></td></tr>
            <tr><td class="kv-label">Crafting Time</td><td>{{.CraftingTime}} ticks</td></tr>
          </table>
        </div>
    </main>
` + themeScript + `
</body>
</html>
`

// System templates.

var systemIndexTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Systems - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../system.css">
    <link rel="stylesheet" href="../items/items.css">
    <style>
        .summary-cards { display: flex; gap: 16px; margin: 16px 0; flex-wrap: wrap; }
        .summary-card { background: var(--bg-card); border: 1px solid var(--border); border-radius: 8px; padding: 12px 20px; text-align: center; }
        .summary-card .num { font-size: 1.8em; font-weight: 700; }
        .summary-card .label { font-size: 0.8em; color: var(--text-muted); text-transform: uppercase; }
        .system-unexplored { font-style: italic; color: var(--text-muted); opacity: 0.7; }
        .system-unexplored:hover { opacity: 1; color: var(--link); }
    </style>
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <h2>Systems</h2>
        <p class="text-muted mt-1">{{len .Systems}} star systems in the galaxy.</p>

        <div class="summary-cards">
            <div class="summary-card">
                <div class="num">{{.TotalSystems}}</div>
                <div class="label">Total Systems</div>
            </div>
            <div class="summary-card">
                <div class="num">{{.ExploredSystems}}</div>
                <div class="label">Systems Explored</div>
            </div>
            <div class="summary-card">
                <div class="num">{{printf "%.1f" .ExplorationPct}}%</div>
                <div class="label">Galaxy Explored</div>
            </div>
        </div>

{{- if .Empires}}
        <nav class="empire-toc mt-3">
            <span class="label">Empires</span>
            <div class="empire-toc-links">
{{- range .Empires}}
                <a href="#empire-{{.ID}}" class="empire-toc-link" style="border-color: {{.Color}}; color: {{.Color}}">{{.Name}} <span class="text-muted">({{len .Systems}})</span></a>
{{- end}}
            </div>
        </nav>

{{- range .Empires}}
        <section id="empire-{{.ID}}" class="empire-section mt-3">
            <h3 class="empire-title" style="color: {{.Color}}">{{.Name}}</h3>
            <div class="empire-map-panel">
                {{.MapSVG}}
            </div>
            <div class="card mt-2" style="padding:0">
            <div class="section-label">{{.Name}} Systems ({{len .Systems}})</div>
            <table class="sortable">
            <thead>
            <tr><th class="sortable">System</th><th class="sortable">Security</th><th class="sortable" style="text-align:right">Connections</th><th class="sortable" style="text-align:right">POIs</th><th class="sortable">Stronghold</th></tr>
            </thead>
            <tbody>
{{- range .Systems}}
            <tr>
              <td>{{if eq .LastUpdatedTick 0}}<a href="{{.ID}}/" class="system-unexplored">{{.Name}}</a> <span class="badge badge-muted" style="font-size:0.65em;">Unexplored</span>{{else}}<a href="{{.ID}}/">{{.Name}}</a>{{end}}</td>
              <td>{{if ne .LastUpdatedTick 0}}<span class="{{securityClass .PoliceLevel}}">{{.PoliceLevel}} {{securityLabel .PoliceLevel}}</span>{{else}}<span class="text-muted">Unknown</span>{{end}}</td>
              <td style="text-align:right" data-sort="{{len .Connections}}">{{len .Connections}}</td>
              <td style="text-align:right" data-sort="{{len .POIs}}">{{len .POIs}}</td>
              <td>{{if .IsStronghold}}Yes{{end}}</td>
            </tr>
{{- end}}
            </tbody>
            </table>
            </div>
        </section>
{{- end}}
{{- end}}

        <h3 class="mt-3">All Systems</h3>
        <div class="card mt-2" style="padding:0">
        <table class="sortable">
        <thead>
        <tr><th class="sortable">System</th><th class="sortable">Empire</th><th class="sortable">Security</th><th class="sortable" style="text-align:right">Connections</th><th class="sortable" style="text-align:right">POIs</th><th class="sortable">Stronghold</th></tr>
        </thead>
        <tbody>
{{- range .Systems}}
        <tr>
          <td>{{if eq .LastUpdatedTick 0}}<a href="{{.ID}}/" class="system-unexplored">{{.Name}}</a> <span class="badge badge-muted" style="font-size:0.65em;">Unexplored</span>{{else}}<a href="{{.ID}}/">{{.Name}}</a>{{end}}</td>
          <td>{{if eq (formatEmpire .Empire .LastUpdatedTick) "Unknown"}}<span class="text-muted">Unknown</span>{{else}}{{formatEmpire .Empire .LastUpdatedTick}}{{end}}</td>
          <td>{{if ne .LastUpdatedTick 0}}<span class="{{securityClass .PoliceLevel}}">{{.PoliceLevel}} {{securityLabel .PoliceLevel}}</span>{{else}}<span class="text-muted">Unknown</span>{{end}}</td>
          <td style="text-align:right" data-sort="{{len .Connections}}">{{len .Connections}}</td>
          <td style="text-align:right" data-sort="{{len .POIs}}">{{len .POIs}}</td>
          <td>{{if .IsStronghold}}Yes{{end}}</td>
        </tr>
{{- end}}
        </tbody>
        </table>
        </div>
        <p class="text-muted" style="font-size:0.85em; margin-top:0.5rem;">
            {{.ExploredSystems}} systems explored / {{printf "%.1f" .ExplorationPct}}% explored · <a href="../galaxy-map.html" target="_blank">View galaxy map</a> ↗
        </p>
    </main>
` + sortScript + `
` + themeScript + `
</body>
</html>
`

var systemDetailTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} - Systems - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../system.css">
    <link rel="stylesheet" href="../../items/items.css">
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Systems</a> / {{.Name}}</div>

        <div class="sys-header">
            <div>
                <span class="label">System</span>
                <h2 class="sys-name">{{.Name}}</h2>
            </div>
            <div class="sys-meta">
                <div class="sys-meta-item">
                    <span class="label">Security</span>
                    <span class="stat {{securityClass .PoliceLevel}}">{{if ne .LastUpdatedTick 0}}{{.PoliceLevel}} <span class="security-label">{{securityLabel .PoliceLevel}}</span>{{else}}<span class="text-muted">Unknown</span>{{end}}</span>
                </div>
                <div class="sys-meta-item">
                    <span class="label">Empire</span>
                    <span class="stat">{{if eq (formatEmpire .Empire .LastUpdatedTick) "Unknown"}}<span class="text-muted">Unknown</span>{{else}}{{formatEmpire .Empire .LastUpdatedTick}}{{end}}</span>
                </div>
                <div class="sys-meta-item">
                    <span class="label">Position</span>
                    <span class="stat">{{fmtCoord .PositionX}}, {{fmtCoord .PositionY}}</span>
                </div>
{{- if .IsStronghold}}
                <div class="sys-meta-item">
                    <span class="label">Status</span>
                    <span class="stat security-high">Stronghold</span>
                </div>
{{- end}}
            </div>
        </div>

{{- if .Description}}
        <blockquote class="item-desc">{{.Description}}</blockquote>
{{- end}}

        {{systemMap .}}

{{- if .Connections}}
        <div class="card mt-2" style="padding:0">
          <div class="section-label">Jump Connections ({{len .Connections}})</div>
          <table>
            <thead><tr><th>System</th><th style="text-align:right">Distance</th></tr></thead>
            <tbody>
{{- range .Connections}}
            <tr>
              <td><a href="../{{.SystemID}}/">{{.Name}}</a></td>
              <td style="text-align:right">{{if .Distance}}{{.Distance}}{{else}}<span class="text-muted">—</span>{{end}}</td>
            </tr>
{{- end}}
            </tbody>
          </table>
        </div>
{{- end}}

{{- if .POIs}}
        <div class="card mt-2" style="padding:0">
          <div class="section-label">Points of Interest ({{len .POIs}})</div>
          <table>
            <thead><tr><th></th><th>Name</th><th>Type</th><th>Class</th><th style="text-align:right">Distance From Sun (AU)</th><th style="text-align:right;white-space:nowrap">Position</th><th>Hidden</th><th style="text-align:right">Reveal Difficulty</th><th>Description</th></tr></thead>
            <tbody>
{{- range sortPOIsByDist .POIs}}
            <tr>
              <td style="text-align:center;font-size:16px">{{poiIcon .Type}}</td>
              <td>{{- if poiDetailLink .}}<a href="{{poiDetailLink .}}">{{.Name}}</a>{{else}}{{.Name}}{{end}}</td>
              <td>{{titleCase .Type}}</td>
              <td>{{if .Class}}{{.Class}}{{else}}<span class="text-muted">—</span>{{end}}</td>
              <td style="text-align:right">{{poiDist .}}</td>
              <td style="text-align:right;white-space:nowrap">{{poiPos .}}</td>
              <td>{{if .Hidden}}<span class="badge badge-yellow">Yes</span>{{else}}<span class="text-muted">—</span>{{end}}</td>
              <td style="text-align:right">{{if .Hidden}}{{.RevealDifficulty}}{{else}}<span class="text-muted">—</span>{{end}}</td>
              <td>{{if .Description}}{{.Description}}{{else if $.LastUpdatedTick}}<span class="text-muted">—</span>{{else}}<span class="text-muted">Unexplored</span>{{end}}</td>
            </tr>
{{- end}}
            </tbody>
          </table>
        </div>
{{- end}}

{{- if hasResources .POIs}}
        <section class="sys-section">
            <div class="section-head">
                <h3>Resources</h3>
                <span class="badge badge-frost">{{len (resourcePOIs .POIs)}} locations</span>
            </div>
{{- range resourcePOIs .POIs}}
            <div class="resource-group">
                <h4>{{.Name}} <span class="badge badge-orange">{{.Type}}</span></h4>
                <table>
                    <thead><tr><th>Resource</th><th>Richness</th><th>Remaining</th></tr></thead>
                    <tbody>
{{- range .Resources}}
                        <tr>
                          <td>{{.ResourceName}}</td>
                          <td><span class="richness-bar" style="--r: {{fmtRichness .Richness}}%">{{fmtRichness .Richness}}</span></td>
                          <td>{{fmtRemaining .Remaining}}</td>
                        </tr>
{{- end}}
                    </tbody>
                </table>
            </div>
{{- end}}
        </section>
{{- end}}

{{- if .Bases}}
        <section class="sys-section">
            <div class="section-head">
                <h3>Bases &amp; Stations</h3>
                <span class="badge badge-frost">{{len .Bases}}</span>
            </div>
{{- range .Bases}}
            <div class="base-card">
                <div class="base-header">
                    <div>
                        <h4>{{.Name}}</h4>
{{- if .Empire}}
                        <span class="label">{{titleCase .Empire}}</span>
{{- end}}
                    </div>
                    <div class="base-stats">
{{- if .PublicAccess}}
                        <span class="badge badge-green">Public</span>
{{- else}}
                        <span class="badge badge-red">Private</span>
{{- end}}
{{- if .DefenseLevel}}
                        <span class="badge badge-frost">Defense {{.DefenseLevel}}</span>
{{- end}}
{{- if .HasDrones}}
                        <span class="badge badge-yellow">Drones</span>
{{- end}}
{{- if .Condition}}
                        <span class="badge {{conditionBadge .Condition}}">{{titleCase .Condition}}</span>
{{- end}}
                    </div>
                </div>
{{- if .ConditionText}}
                <p class="base-desc" style="font-style:italic; opacity:0.85">{{.ConditionText}}</p>
{{- end}}
{{- if .Description}}
                <p class="base-desc">{{.Description}}</p>
{{- end}}
{{- if .Story}}
                <details class="base-story"><summary>Station Log</summary><p>{{.Story}}</p></details>
{{- end}}
                <div class="base-meta">
{{- if .SatisfactionPct}}
                    <span class="label">Satisfaction: {{.SatisfactionPct}}% ({{.SatisfiedCount}}/{{.TotalServiceInfra}} infrastructure)</span>
{{- end}}
                </div>
                <div class="base-sections">
{{- if .Services}}
                    <div>
                        <span class="label">Services</span>
                        <div class="service-list">
{{- range .Services}}
                            <span class="service-tag{{if .Available}} available{{else}} unavailable{{end}}">{{titleCaseID .Name}}</span>
{{- end}}
                        </div>
                    </div>
{{- end}}
{{- if .Facilities}}
                    <div>
                        <span class="label">Facilities</span>
                        <table class="facility-table">
                            <thead><tr><th>Facility</th><th>Category</th><th>Level</th></tr></thead>
                            <tbody>
{{- range condenseFacilities .Facilities}}
                                <tr>
                                  <td>{{titleCaseID .Name}}{{if gt .Count 1}} <span class="badge badge-muted">x{{.Count}}</span>{{end}}</td>
                                  <td><span class="badge {{facilityBadge .Category}}">{{.Category}}</span></td>
                                  <td>{{.Level}}</td>
                                </tr>
{{- end}}
                            </tbody>
                        </table>
                    </div>
{{- end}}
                </div>
            </div>
{{- end}}
        </section>
{{- end}}

{{- if not (or .Connections .POIs .Description)}}
        <p class="text-muted mt-3">This system has not been explored yet. Data will appear as agents visit and scan.</p>
{{- end}}

        <p class="text-muted mt-3" style="font-size:0.85em">Last Updated Tick: {{if .LastUpdatedTick}}{{.LastUpdatedTick}}{{else}}—{{end}}</p>
    </main>
` + themeScript + `
</body>
</html>
`
