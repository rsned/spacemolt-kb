// Command generate-items-kb reads the crafting database and produces
// KB-styled HTML pages for all items, organized by category.
package main

import (
	"cmp"
	"database/sql"
	"fmt"
	htmltpl "html/template"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	humanize "github.com/dustin/go-humanize"
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

	ProducedBy []ProducedBy
	UsedIn     []UsedIn
}

// ProducedBy describes a recipe that produces this item.
type ProducedBy struct {
	RecipeID     string
	RecipeName   string
	Quantity     int
	CraftingTime int
	Skills       []SkillReq
}

// UsedIn describes a recipe that consumes this item and what it produces.
type UsedIn struct {
	RecipeID       string
	RecipeName     string
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

func main() {
	dbPath := "../../spacemolt-crafting-server/database/crafting.db"
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

	if err := loadProducedBy(db, items); err != nil {
		log.Fatalf("load produced-by: %v", err)
	}

	if err := loadUsedIn(db, items); err != nil {
		log.Fatalf("load used-in: %v", err)
	}

	// Clean generated HTML files, preserving images/ and items.css.
	if err := cleanGeneratedFiles(outDir); err != nil {
		log.Fatalf("clean output dir: %v", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
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
		SELECT ro.item_id, r.id, r.name, ro.quantity, r.crafting_time,
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
		var itemID, recipeID, recipeName, skillID, skillName string
		var qty, craftTime, skillLevel int
		if err := rows.Scan(&itemID, &recipeID, &recipeName, &qty, &craftTime, &skillID, &skillName, &skillLevel); err != nil {
			return err
		}
		k := key{itemID, recipeID}
		pb, ok := seen[k]
		if !ok {
			pb = &ProducedBy{
				RecipeID:     recipeID,
				RecipeName:   recipeName,
				Quantity:     qty,
				CraftingTime: craftTime,
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
		SELECT ri.item_id, r.id, r.name, ri.quantity, ro.item_id, oi.name, COALESCE(oi.category, '')
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
		if err := rows.Scan(&itemID, &u.RecipeID, &u.RecipeName, &u.Quantity, &u.OutputID, &u.OutputName, &u.OutputCategory); err != nil {
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
            <a href="../items/">Items</a>
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
            <a href="../../items/">Items</a>
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
          <td><a href="{{.ID}}.html">{{.Name}}</a></td>
          <td class="thumb"><img src="../images/{{.ID}}.png" alt="{{.Name}}"></td>
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
        <h2>{{.Name}}</h2>

        <div class="card mt-2" style="padding:0">
          <div class="item-image">
            <img src="../images/{{.ID}}.png" alt="{{.Name}}" height="200">
          </div>
          <div class="section-label">General</div>
          <table>
            <tr><td class="kv-label">Category</td><td><a href="./">{{titleCase .Category}}</a></td></tr>
            <tr><td class="kv-label">Rarity</td><td><span class="{{rarityClass .Rarity}}">{{.Rarity}}</span></td></tr>
            <tr><td class="kv-label">Size</td><td>{{.Size}}</td></tr>
            <tr><td class="kv-label">Stackable</td><td>{{yesno .Stackable}}</td></tr>
            <tr><td class="kv-label">Tradeable</td><td>{{yesno .Tradeable}}</td></tr>
          </table>
          <div class="section-label">Market</div>
          <table>
            <tr><td class="kv-label">Base Value</td><td>{{fmtValue .BaseValue}}</td></tr>
          </table>
        </div>

        <blockquote class="item-desc">{{.Description}}</blockquote>

{{- if or .ProducedBy .UsedIn}}
        <div class="card" style="padding:0">
{{- if .ProducedBy}}
          <div class="section-label">Produced By</div>
          <table>
            <thead><tr><th>Recipe</th><th>Qty</th><th>Crafting Time</th><th>Skills</th></tr></thead>
            <tbody>
{{- range .ProducedBy}}
            <tr>
              <td>{{.RecipeName}}</td>
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
              <td>{{.RecipeName}}</td>
              <td>{{.Quantity}}</td>
              <td><a href="../{{.OutputCategory}}/{{.OutputID}}.html">{{.OutputName}}</a></td>
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
