// Command generate-items-kb reads the crafting database and produces
// KB-styled HTML pages for all items, organized by category.
package main

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"fmt"
	htmltpl "html/template"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	humanize "github.com/dustin/go-humanize"
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
}

// ShipBuildRef links an item to a ship that requires it as a build material.
type ShipBuildRef struct {
	ShipID       string
	ShipName     string
	ShipCategory string
	Quantity     int
}

// ProducedBy describes a recipe that produces this item.
type ProducedBy struct {
	RecipeID       string
	RecipeName     string
	RecipeCategory string
	Quantity       int
	CraftingTime   int
	Skills         []SkillReq
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

// SkillReq pairs a skill name with its required level.
type SkillReq struct {
	ID    string
	Name  string
	Level int
}

// CategoryInfo groups items for page generation.
type CategoryInfo struct {
	Name        string
	Description string
	Count       int
	Items       []*Item
}

// Recipe holds a crafting recipe with its inputs, outputs, and skill requirements.
type Recipe struct {
	ID              string
	Name            string
	Description     string
	Category        string
	CraftingTime    int
	BaseQuality     int
	SkillQualityMod int
	Hidden          bool
	Inputs          []RecipeItem
	Outputs         []RecipeItem
	Skills          []SkillReq
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
	ID           string
	POIID        string
	Name         string
	Description  string
	Empire       string
	DefenseLevel int
	HasDrones    bool
	PublicAccess bool
	Services     []BaseService
	Facilities   []BaseFacility
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
	Systems []*System
	Empires []EmpireGroup
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

func main() {
	dbPath := "../../spacemolt-crafting-server/database/crafting.db"
	catalogDir := "../spacemolt/data/game-api/craftsman-3"
	outDir := "kb/items"

	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}
	if len(os.Args) > 2 {
		outDir = os.Args[2]
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

	// Check which items have images.
	imgDir := filepath.Join(outDir, "images")
	for _, it := range items {
		_, err := os.Stat(filepath.Join(imgDir, it.ID+".png"))
		it.HasImage = err == nil
	}

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

	if err := writeHTMLPages(outDir, categories, items); err != nil {
		log.Fatalf("write HTML pages: %v", err)
	}

	fmt.Printf("Generated %d item pages + %d category pages in %s/\n", len(items), len(categories), outDir)

	// --- Recipe generation ---
	recipeOutDir := "kb/recipes"
	if len(os.Args) > 3 {
		recipeOutDir = os.Args[3]
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

	// Check which recipe output items have images.
	for _, r := range recipes {
		for i, out := range r.Outputs {
			_, err := os.Stat(filepath.Join(imgDir, out.ItemID+".png"))
			r.Outputs[i].HasImage = err == nil
		}
		for i, inp := range r.Inputs {
			_, err := os.Stat(filepath.Join(imgDir, inp.ItemID+".png"))
			r.Inputs[i].HasImage = err == nil
		}
	}

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
	knowledgeDBPath := "../spacemolt-knowledge.db"
	systemOutDir := "kb/systems"

	knowledgeDB, err := sql.Open("sqlite", knowledgeDBPath)
	if err != nil {
		log.Printf("warning: open knowledge database: %v (system pages will be skipped)", err)
	} else {
		defer func() { _ = knowledgeDB.Close() }()

		systems, err := loadSystems(knowledgeDB)
		if err != nil {
			log.Fatalf("load systems: %v", err)
		}

		if err := writeSystemPages(systemOutDir, systems); err != nil {
			log.Fatalf("write system pages: %v", err)
		}

		fmt.Printf("Generated %d system pages in %s/\n", len(systems), systemOutDir)
	}

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
	shipCatalog, err := loadShipCatalog(shipCatalogPath)
	if err != nil {
		log.Printf("warning: load ship catalog: %v (ship pages will be skipped)", err)
	} else {
		// Build recipe name lookup for passive recipe display.
		recipeNames := make(map[string]string)
		for _, r := range recipes {
			recipeNames[r.ID] = r.Name
		}
		if err := writeShipPages("kb/ships", shipCatalog, recipeNames); err != nil {
			log.Fatalf("write ship pages: %v", err)
		}
		fmt.Printf("Generated %d ship entries in kb/ships/\n", len(shipCatalog))
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
		SELECT ro.item_id, r.id, r.name, COALESCE(r.category,''), ro.quantity, r.crafting_time,
		       COALESCE(s.id, ''), COALESCE(s.name, ''), COALESCE(rs.level_required, 0)
		FROM recipe_outputs ro
		JOIN recipes r ON ro.recipe_id = r.id
		LEFT JOIN recipe_skills rs ON r.id = rs.recipe_id
		LEFT JOIN skills s ON rs.skill_id = s.id
		ORDER BY ro.item_id, r.id, s.name`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type key struct{ itemID, recipeID string }
	seen := make(map[key]*ProducedBy)
	var order []key

	for rows.Next() {
		var itemID, recipeID, recipeName, recipeCat, skillID, skillName string
		var qty, craftTime, skillLevel int
		if err := rows.Scan(&itemID, &recipeID, &recipeName, &recipeCat, &qty, &craftTime, &skillID, &skillName, &skillLevel); err != nil {
			return err
		}
		k := key{itemID, recipeID}
		pb, ok := seen[k]
		if !ok {
			pb = &ProducedBy{
				RecipeID:       recipeID,
				RecipeName:     recipeName,
				RecipeCategory: recipeCat,
				Quantity:       qty,
				CraftingTime:   craftTime,
			}
			seen[k] = pb
			order = append(order, k)
		}
		if skillName != "" {
			pb.Skills = append(pb.Skills, SkillReq{ID: skillID, Name: skillName, Level: skillLevel})
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, k := range order {
		if it, ok := items[k.itemID]; ok {
			it.ProducedBy = append(it.ProducedBy, *seen[k])
		}
	}
	return nil
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
	poiRows, err := db.Query(`SELECT system_id, id, name, type, COALESCE(class,''), COALESCE(description,''), position_x, position_y FROM pois ORDER BY system_id, name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = poiRows.Close() }()

	poiLookup := make(map[string]*SystemPOI) // poi ID → pointer for attaching resources
	for poiRows.Next() {
		var systemID string
		var poi SystemPOI
		if err := poiRows.Scan(&systemID, &poi.ID, &poi.Name, &poi.Type, &poi.Class, &poi.Description, &poi.PositionX, &poi.PositionY); err != nil {
			return nil, err
		}
		if s, ok := systemMap[systemID]; ok {
			s.POIs = append(s.POIs, poi)
			poiLookup[poi.ID] = &s.POIs[len(s.POIs)-1]
		}
	}
	if err := poiRows.Err(); err != nil {
		return nil, err
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
		SELECT b.id, b.poi_id, b.name, COALESCE(b.description,''), COALESCE(b.empire,''),
		       b.defense_level, b.has_drones, b.public_access, p.system_id
		FROM bases b
		JOIN pois p ON b.poi_id = p.id
		ORDER BY p.system_id, b.name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = baseRows.Close() }()

	baseLookup := make(map[string]*SystemBase)
	for baseRows.Next() {
		var systemID string
		var base SystemBase
		if err := baseRows.Scan(&base.ID, &base.POIID, &base.Name, &base.Description,
			&base.Empire, &base.DefenseLevel, &base.HasDrones, &base.PublicAccess, &systemID); err != nil {
			return nil, err
		}
		if s, ok := systemMap[systemID]; ok {
			s.Bases = append(s.Bases, base)
			baseLookup[base.ID] = &s.Bases[len(s.Bases)-1]
		}
	}
	if err := baseRows.Err(); err != nil {
		return nil, err
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

func writeSystemPages(outDir string, systems []*System) error {
	// Build lookup for galaxy-position of connected systems.
	sysLookup := make(map[string]*System, len(systems))
	for _, s := range systems {
		sysLookup[s.ID] = s
	}

	funcs := htmltpl.FuncMap{
		"titleCase":     titleCase,
		"securityClass": securityClass,
		"securityLabel": securityLabel,
		"fmtCoord":      func(f float64) string { return fmt.Sprintf("%.1f", f) },
		"poiIcon":       poiIcon,
		"hasResources":  func(pois []SystemPOI) bool { return poiHasResources(pois) },
		"resourcePOIs":  func(pois []SystemPOI) []SystemPOI { return filterResourcePOIs(pois) },
		"fmtRemaining":  fmtRemaining,
		"fmtRichness":   func(r float64) string { return fmt.Sprintf("%.0f", r) },
		"facilityBadge": facilityBadge,
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
		"systemMap": func(sys *System) htmltpl.HTML {
			allMap := make(map[string]*systemmap.System, len(sysLookup))
			for k, v := range sysLookup {
				allMap[k] = toMapSystem(v)
			}
			return htmltpl.HTML(systemmap.RenderSystemMap(toMapSystem(sys), allMap, false))
		},
	}
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

	indexData := SystemsIndexData{
		Systems: systems,
		Empires: empires,
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

	// Individual system pages.
	for _, sys := range systems {
		path := filepath.Join(outDir, sys.ID+".html")
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
	rows, err := db.Query(`SELECT id, name, COALESCE(description,''), COALESCE(category,''), crafting_time, base_quality, skill_quality_mod FROM recipes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	recipes := make(map[string]*Recipe)
	for rows.Next() {
		var r Recipe
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Category, &r.CraftingTime, &r.BaseQuality, &r.SkillQualityMod); err != nil {
			return nil, err
		}
		recipes[r.ID] = &r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load inputs.
	inputRows, err := db.Query(`
		SELECT ri.recipe_id, ri.item_id, COALESCE(i.name,''), COALESCE(i.category,''), ri.quantity
		FROM recipe_inputs ri
		LEFT JOIN items i ON ri.item_id = i.id
		ORDER BY ri.recipe_id, i.name`)
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
		SELECT ro.recipe_id, ro.item_id, COALESCE(i.name,''), COALESCE(i.category,''), ro.quantity
		FROM recipe_outputs ro
		LEFT JOIN items i ON ro.item_id = i.id
		ORDER BY ro.recipe_id, i.name`)
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

	// Load skills.
	skillRows, err := db.Query(`
		SELECT rs.recipe_id, rs.skill_id, COALESCE(s.name,''), rs.level_required
		FROM recipe_skills rs
		LEFT JOIN skills s ON rs.skill_id = s.id
		ORDER BY rs.recipe_id, s.name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = skillRows.Close() }()

	for skillRows.Next() {
		var recipeID string
		var sr SkillReq
		if err := skillRows.Scan(&recipeID, &sr.ID, &sr.Name, &sr.Level); err != nil {
			return nil, err
		}
		if r, ok := recipes[recipeID]; ok {
			r.Skills = append(r.Skills, sr)
		}
	}
	return recipes, skillRows.Err()
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
		"yesno":       yesno,
		"fmtValue":    fmtValue,
		"rarityClass": rarityClass,
		"titleCase":   titleCase,
		"dirName":     dirName,
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
            <a href="../systems/">Systems</a>
            <a href="../items/">Items</a>
            <a href="../recipes/">Recipes</a>
            <a href="../skills/">Skills</a>
            <a href="../ships/">Ships</a>
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
            <a href="../../systems/">Systems</a>
            <a href="../../items/">Items</a>
            <a href="../../recipes/">Recipes</a>
            <a href="../../skills/">Skills</a>
            <a href="../../ships/">Ships</a>
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
          <div class="section-label">General</div>
          <table>
            <tr><td class="kv-label">Category</td><td><a href="./">{{titleCase .Category}}</a></td></tr>
            <tr><td class="kv-label">Rarity</td><td><span class="{{rarityClass .Rarity}}">{{.Rarity}}</span></td></tr>
            <tr><td class="kv-label">Size</td><td>{{.Size}}</td></tr>
            <tr><td class="kv-label">Stackable</td><td>{{yesno .Stackable}}</td></tr>
            <tr><td class="kv-label">Tradeable</td><td>{{yesno .Tradeable}}</td></tr>
{{- if .PowerBonus}}
            <tr><td class="kv-label">Power Bonus</td><td><span class="stat-positive">+{{.PowerBonus}}</span></td></tr>
{{- end}}
          </table>
          <div class="section-label">Market</div>
          <table>
            <tr><td class="kv-label">Base Value</td><td>{{fmtValue .BaseValue}}</td></tr>
          </table>
        </div>

        <blockquote class="item-desc">{{.Description}}</blockquote>

{{- if or .ProducedBy .UsedIn .UsedInShips}}
        <div class="card" style="padding:0">
{{- if .ProducedBy}}
          <div class="section-label">Produced By</div>
          <table>
            <thead><tr><th>Recipe</th><th>Qty</th><th>Crafting Time</th><th>Skills</th></tr></thead>
            <tbody>
{{- range .ProducedBy}}
            <tr>
              <td><a href="../../recipes/{{dirName .RecipeCategory}}/{{.RecipeID}}.html">{{.RecipeName}}</a></td>
              <td>{{.Quantity}}</td>
              <td>{{.CraftingTime}} ticks</td>
              <td>{{- if .Skills}}{{range $i, $s := .Skills}}{{if $i}}, {{end}}<a href="../../skills/{{$s.ID}}.html">{{$s.Name}}</a> {{$s.Level}}{{end}}{{else}}None{{end}}</td>
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
              <td>{{.ShipName}}</td>
              <td>{{.ShipCategory}}</td>
              <td>{{.Quantity}}</td>
            </tr>
{{- end}}
            </tbody>
          </table>
{{- end}}
        </div>
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
        <tr><th class="sortable">Recipe</th><th class="sortable">Output</th><th>Inputs</th><th class="sortable" style="text-align:right">Time</th><th>Skills</th></tr>
        </thead>
        <tbody>
{{- range .Recipes}}
        <tr>
          <td><a href="{{.ID}}.html">{{.Name}}</a>{{if .Hidden}} <span class="badge badge-hidden" title="Hidden">H</span>{{end}}</td>
          <td>{{- range .Outputs}}<a href="../../items/{{.ItemCategory}}/{{.ItemID}}.html" class="recipe-item">{{if .HasImage}}<img src="../../items/images/{{.ItemID}}.png" alt="{{.ItemName}}" class="recipe-thumb">{{end}}{{.ItemName}}{{if gt .Quantity 1}} &times;{{.Quantity}}{{end}}</a>{{end}}</td>
          <td class="recipe-inputs">{{- range $i, $inp := .Inputs}}{{if $i}}, {{end}}<a href="../../items/{{$inp.ItemCategory}}/{{$inp.ItemID}}.html">{{$inp.ItemName}}</a>&nbsp;&times;{{$inp.Quantity}}{{end}}</td>
          <td class="time" data-sort="{{.CraftingTime}}">{{.CraftingTime}} ticks</td>
          <td>{{- if .Skills}}{{range $i, $s := .Skills}}{{if $i}}, {{end}}<a href="../../skills/{{$s.ID}}.html">{{$s.Name}}</a>&nbsp;{{$s.Level}}{{end}}{{else}}None{{end}}</td>
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
            <tr><td class="kv-label">Base Quality</td><td>{{.BaseQuality}}</td></tr>
            <tr><td class="kv-label">Skill Quality Mod</td><td>{{.SkillQualityMod}}</td></tr>
          </table>

{{- if .Skills}}
          <div class="section-label">Required Skills</div>
          <table>
            <thead><tr><th>Skill</th><th>Level</th></tr></thead>
            <tbody>
{{- range .Skills}}
            <tr>
              <td><a href="../../skills/{{.ID}}.html">{{.Name}}</a></td>
              <td>{{.Level}}</td>
            </tr>
{{- end}}
            </tbody>
          </table>
{{- end}}
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
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <h2>Systems</h2>
        <p class="text-muted mt-1">{{len .Systems}} star systems in the galaxy.</p>

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
              <td><a href="{{.ID}}.html">{{.Name}}</a></td>
              <td>{{if .PoliceLevel}}<span class="{{securityClass .PoliceLevel}}">{{.PoliceLevel}} {{securityLabel .PoliceLevel}}</span>{{else}}<span class="text-muted">Unknown</span>{{end}}</td>
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
          <td><a href="{{.ID}}.html">{{.Name}}</a></td>
          <td>{{if .Empire}}{{titleCase .Empire}}{{else}}<span class="text-muted">Unknown</span>{{end}}</td>
          <td>{{if .PoliceLevel}}<span class="{{securityClass .PoliceLevel}}">{{.PoliceLevel}} {{securityLabel .PoliceLevel}}</span>{{else}}<span class="text-muted">Unknown</span>{{end}}</td>
          <td style="text-align:right" data-sort="{{len .Connections}}">{{len .Connections}}</td>
          <td style="text-align:right" data-sort="{{len .POIs}}">{{len .POIs}}</td>
          <td>{{if .IsStronghold}}Yes{{end}}</td>
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

var systemDetailTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} - Systems - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../system.css">
    <link rel="stylesheet" href="../items/items.css">
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="./">Systems</a> / {{.Name}}</div>

        <div class="sys-header">
            <div>
                <span class="label">System</span>
                <h2 class="sys-name">{{.Name}}</h2>
            </div>
            <div class="sys-meta">
                <div class="sys-meta-item">
                    <span class="label">Security</span>
                    <span class="stat {{securityClass .PoliceLevel}}">{{if .PoliceLevel}}{{.PoliceLevel}} <span class="security-label">{{securityLabel .PoliceLevel}}</span>{{else}}<span class="text-muted">Unknown</span>{{end}}</span>
                </div>
                <div class="sys-meta-item">
                    <span class="label">Empire</span>
                    <span class="stat">{{if .Empire}}{{titleCase .Empire}}{{else}}<span class="text-muted">Unknown</span>{{end}}</span>
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
              <td><a href="{{.SystemID}}.html">{{.Name}}</a></td>
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
            <thead><tr><th></th><th>Name</th><th>Type</th><th>Class</th><th style="text-align:right">Distance (AU)</th><th>Description</th></tr></thead>
            <tbody>
{{- range sortPOIsByDist .POIs}}
            <tr>
              <td style="text-align:center;font-size:16px">{{poiIcon .Type}}</td>
              <td>{{.Name}}</td>
              <td>{{titleCase .Type}}</td>
              <td>{{if .Class}}{{.Class}}{{else}}<span class="text-muted">—</span>{{end}}</td>
              <td style="text-align:right">{{poiDist .}}</td>
              <td>{{if .Description}}{{.Description}}{{else}}<span class="text-muted">Unexplored</span>{{end}}</td>
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
                    </div>
                </div>
{{- if .Description}}
                <p class="base-desc">{{.Description}}</p>
{{- end}}
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
{{- range .Facilities}}
                                <tr>
                                  <td>{{titleCaseID .Name}}</td>
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
