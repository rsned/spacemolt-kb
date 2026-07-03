// Command generate-items-kb missions support.
package main

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"fmt"
	htmltpl "html/template"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Mission is a mission template loaded from mission_templates.
type Mission struct {
	ID             string
	Title          string
	Description    string
	Type           string
	Difficulty     int
	GiverName      string
	GiverTitle     string
	FactionID      string
	FactionName    string
	DialogOffer    string
	DialogAccept   string
	DialogDecline  string
	DialogComplete string
	ChainNext      string
	ChainNextTitle string
	ChainNextHref  string
	ChainPrev      string
	ChainPrevTitle string
	ChainPrevHref  string
	Repeatable     bool
	ExpiresInTicks int
	RewardsCredits int
	RewardsSkillXP []MissionReward
	RewardsItems   []MissionItemReward
	ProvidedItems  []MissionItemReward
	Requirements   map[string]any
	RequiredModules []string

	Objectives []MissionObjective
	Locations  []MissionLocation
}

// MissionReward is a skill XP reward entry.
type MissionReward struct {
	Name  string
	Value int
}

// MissionItemReward is an item reward or provided item.
type MissionItemReward struct {
	ItemID   string
	ItemName string
	Category string
	Quantity int
}

// MissionObjective is a single objective in a mission_objectives row.
type MissionObjective struct {
	SortOrder       int
	Type            string
	Description     string
	ItemID          string
	ItemName        string
	ItemCategory    string
	Quantity        int
	SystemID        string
	SystemName      string
	TargetBaseID    string
	TargetBaseName  string
}

// MissionLocation is a place the mission is offered.
type MissionLocation struct {
	BaseID        string
	BaseName      string
	SystemID      string
	SystemName    string
	FirstSeenAt   string
	LastSeenAt    string
}

// MissionCategoryInfo groups missions for page generation.
type MissionCategoryInfo struct {
	Name        string // internal key, e.g. "mining"
	Description string
	Count       int
	Missions    []*Mission
}

// chainHref returns a relative link from a mission detail page in fromType
// to the detail page for toID within toType. Pages live at
// kb/missions/<type>/<id>.html, so same-type links are local and cross-type
// links walk up one directory.
func chainHref(fromType, toType, toID string) string {
	if fromType == toType {
		return toID + ".html"
	}
	return "../" + toType + "/" + toID + ".html"
}

// missionTypeDescriptions maps mission types to their descriptions.
var missionTypeDescriptions = map[string]string{
	"combat":      "Bounty hunts, pirate takedowns, and other hostile engagements.",
	"crafting":    "Production runs and workshop assignments that put your recipes to use.",
	"delivery":    "Ferry goods across the galaxy on behalf of stations, factions, and researchers.",
	"equipment":   "Starter and onboarding missions that teach ship systems and basic gameplay.",
	"exploration": "Survey unknown systems, calibrate beacons, and chart the outer rim.",
	"mining":      "Extraction contracts for specific ores and refined minerals.",
	"trading":     "Market participation and commercial fulfilment assignments.",
}

// loadMissions reads all mission templates, objectives, and offer locations
// from the knowledge database.
func loadMissions(db *sql.DB) ([]*Mission, error) {
	missionRows, err := db.Query(`
		SELECT id, title, COALESCE(description,''), COALESCE(type,''),
		       COALESCE(difficulty,0),
		       COALESCE(giver_name,''), COALESCE(giver_title,''),
		       COALESCE(faction_id,''), COALESCE(faction_name,''),
		       COALESCE(dialog_offer,''), COALESCE(dialog_accept,''),
		       COALESCE(dialog_decline,''), COALESCE(dialog_complete,''),
		       COALESCE(chain_next,''),
		       COALESCE(repeatable,0),
		       COALESCE(expires_in_ticks,0),
		       COALESCE(rewards_credits,0),
		       COALESCE(rewards_skill_xp,'{}'),
		       COALESCE(rewards_items,'{}'),
		       COALESCE(requirements,'{}'),
		       COALESCE(required_modules,'[]'),
		       COALESCE(provided_items,'{}')
		FROM mission_templates
		ORDER BY type, title
	`)
	if err != nil {
		return nil, fmt.Errorf("query mission_templates: %w", err)
	}
	defer func() { _ = missionRows.Close() }()

	missionMap := make(map[string]*Mission)
	var missions []*Mission
	for missionRows.Next() {
		var m Mission
		var repeatable int
		var rewardsXPJSON, rewardsItemsJSON, requirementsJSON, requiredModulesJSON, providedItemsJSON string
		if err := missionRows.Scan(
			&m.ID, &m.Title, &m.Description, &m.Type,
			&m.Difficulty,
			&m.GiverName, &m.GiverTitle,
			&m.FactionID, &m.FactionName,
			&m.DialogOffer, &m.DialogAccept, &m.DialogDecline, &m.DialogComplete,
			&m.ChainNext,
			&repeatable,
			&m.ExpiresInTicks,
			&m.RewardsCredits,
			&rewardsXPJSON, &rewardsItemsJSON,
			&requirementsJSON, &requiredModulesJSON, &providedItemsJSON,
		); err != nil {
			return nil, fmt.Errorf("scan mission: %w", err)
		}
		m.Repeatable = repeatable != 0

		m.RewardsSkillXP = decodeIntMap(rewardsXPJSON)
		m.RewardsItems = decodeItemMap(rewardsItemsJSON)
		m.ProvidedItems = decodeItemMap(providedItemsJSON)
		m.Requirements = decodeAnyMap(requirementsJSON)
		m.RequiredModules = decodeStringList(requiredModulesJSON)

		missionMap[m.ID] = &m
		missions = append(missions, &m)
	}
	if err := missionRows.Err(); err != nil {
		return nil, err
	}

	// Resolve chain_next titles and hrefs, plus build reverse chain_prev
	// links on the target missions.
	for _, m := range missions {
		if m.ChainNext == "" {
			continue
		}
		next, ok := missionMap[m.ChainNext]
		if !ok {
			continue
		}
		m.ChainNextTitle = next.Title
		m.ChainNextHref = chainHref(m.Type, next.Type, next.ID)
		next.ChainPrev = m.ID
		next.ChainPrevTitle = m.Title
		next.ChainPrevHref = chainHref(next.Type, m.Type, m.ID)
	}

	// Load objectives.
	objRows, err := db.Query(`
		SELECT mission_id, sort_order,
		       COALESCE(type,''), COALESCE(description,''),
		       COALESCE(item_id,''), COALESCE(quantity,0),
		       COALESCE(system_id,''), COALESCE(system_name,''),
		       COALESCE(target_base_id,''), COALESCE(target_base_name,'')
		FROM mission_objectives
		ORDER BY mission_id, sort_order
	`)
	if err != nil {
		return nil, fmt.Errorf("query mission_objectives: %w", err)
	}
	defer func() { _ = objRows.Close() }()

	for objRows.Next() {
		var missionID string
		var o MissionObjective
		if err := objRows.Scan(
			&missionID, &o.SortOrder,
			&o.Type, &o.Description,
			&o.ItemID, &o.Quantity,
			&o.SystemID, &o.SystemName,
			&o.TargetBaseID, &o.TargetBaseName,
		); err != nil {
			return nil, fmt.Errorf("scan objective: %w", err)
		}
		if m, ok := missionMap[missionID]; ok {
			m.Objectives = append(m.Objectives, o)
		}
	}
	if err := objRows.Err(); err != nil {
		return nil, err
	}

	// Load offer locations.
	locRows, err := db.Query(`
		SELECT mission_id, base_id, COALESCE(system_id,''),
		       COALESCE(first_seen_at,''), COALESCE(last_seen_at,'')
		FROM mission_template_locations
		ORDER BY mission_id, base_id
	`)
	if err != nil {
		return nil, fmt.Errorf("query mission_template_locations: %w", err)
	}
	defer func() { _ = locRows.Close() }()

	for locRows.Next() {
		var missionID string
		var l MissionLocation
		if err := locRows.Scan(
			&missionID, &l.BaseID, &l.SystemID,
			&l.FirstSeenAt, &l.LastSeenAt,
		); err != nil {
			return nil, fmt.Errorf("scan location: %w", err)
		}
		if m, ok := missionMap[missionID]; ok {
			m.Locations = append(m.Locations, l)
		}
	}
	if err := locRows.Err(); err != nil {
		return nil, err
	}

	return missions, nil
}

// enrichMissionItemLinks populates ItemName and Category fields on item rewards,
// provided items, and objectives so detail pages can link to item KB entries.
func enrichMissionItemLinks(missions []*Mission, items map[string]*Item) {
	resolve := func(id string) (string, string) {
		if id == "" {
			return "", ""
		}
		if it, ok := items[id]; ok {
			return it.Name, it.Category
		}
		return titleCase(id), ""
	}

	for _, m := range missions {
		for i := range m.RewardsItems {
			m.RewardsItems[i].ItemName, m.RewardsItems[i].Category = resolve(m.RewardsItems[i].ItemID)
		}
		for i := range m.ProvidedItems {
			m.ProvidedItems[i].ItemName, m.ProvidedItems[i].Category = resolve(m.ProvidedItems[i].ItemID)
		}
		for i := range m.Objectives {
			if m.Objectives[i].ItemID == "" {
				continue
			}
			m.Objectives[i].ItemName, m.Objectives[i].ItemCategory = resolve(m.Objectives[i].ItemID)
		}
	}
}

// enrichMissionLocationNames fills in BaseName and SystemName for location entries
// using data loaded from the pois and systems tables.
func enrichMissionLocationNames(db *sql.DB, missions []*Mission) error {
	baseNames := make(map[string]string)
	systemNames := make(map[string]string)

	poiRows, err := db.Query(`SELECT id, name, system_id FROM pois`)
	if err == nil {
		for poiRows.Next() {
			var id, name, systemID string
			if err := poiRows.Scan(&id, &name, &systemID); err == nil {
				baseNames[id] = name
			}
		}
		_ = poiRows.Close()
	}

	sysRows, err := db.Query(`SELECT id, name FROM systems`)
	if err == nil {
		for sysRows.Next() {
			var id, name string
			if err := sysRows.Scan(&id, &name); err == nil {
				systemNames[id] = name
			}
		}
		_ = sysRows.Close()
	}

	for _, m := range missions {
		for i := range m.Locations {
			if n, ok := baseNames[m.Locations[i].BaseID]; ok {
				m.Locations[i].BaseName = n
			} else {
				m.Locations[i].BaseName = titleCase(m.Locations[i].BaseID)
			}
			if m.Locations[i].SystemID != "" {
				if n, ok := systemNames[m.Locations[i].SystemID]; ok {
					m.Locations[i].SystemName = n
				}
			}
		}
	}
	return nil
}

func decodeIntMap(s string) []MissionReward {
	if s == "" || s == "null" {
		return nil
	}
	m := make(map[string]int)
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	out := make([]MissionReward, 0, len(m))
	for k, v := range m {
		out = append(out, MissionReward{Name: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func decodeItemMap(s string) []MissionItemReward {
	if s == "" || s == "null" {
		return nil
	}
	m := make(map[string]int)
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	out := make([]MissionItemReward, 0, len(m))
	for k, v := range m {
		out = append(out, MissionItemReward{ItemID: k, Quantity: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ItemID < out[j].ItemID })
	return out
}

func decodeAnyMap(s string) map[string]any {
	if s == "" || s == "null" {
		return nil
	}
	m := make(map[string]any)
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func decodeStringList(s string) []string {
	if s == "" || s == "null" {
		return nil
	}
	var list []string
	if err := json.Unmarshal([]byte(s), &list); err != nil {
		return nil
	}
	return list
}

// writeMissionPages generates all mission HTML pages.
func writeMissionPages(outDir string, missions []*Mission) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create mission output dir: %w", err)
	}

	// Clean stale HTML at the top level, preserving missions.css.
	entries, _ := os.ReadDir(outDir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".html") {
			_ = os.Remove(filepath.Join(outDir, e.Name()))
		}
	}

	// Group by mission type.
	byType := make(map[string][]*Mission)
	for _, m := range missions {
		typ := m.Type
		if typ == "" {
			typ = "uncategorized"
		}
		byType[typ] = append(byType[typ], m)
	}
	for _, list := range byType {
		slices.SortFunc(list, func(a, b *Mission) int {
			if c := cmp.Compare(a.Difficulty, b.Difficulty); c != 0 {
				return c
			}
			return cmp.Compare(a.Title, b.Title)
		})
	}

	categories := make([]MissionCategoryInfo, 0, len(byType))
	for typ, list := range byType {
		categories = append(categories, MissionCategoryInfo{
			Name:        typ,
			Description: missionTypeDescriptions[typ],
			Count:       len(list),
			Missions:    list,
		})
	}
	slices.SortFunc(categories, func(a, b MissionCategoryInfo) int {
		return cmp.Compare(a.Name, b.Name)
	})

	funcs := htmltpl.FuncMap{
		"titleCase":   titleCase,
		"fmtValue":    fmtValue,
		"difficulty":  difficultyBadge,
		"lower":       strings.ToLower,
		"totalCount": func(cats []MissionCategoryInfo) int {
			n := 0
			for _, c := range cats {
				n += c.Count
			}
			return n
		},
	}

	topTmpl := htmltpl.Must(htmltpl.New("top").Funcs(funcs).Parse(htmlMissionsTopTemplate))
	catTmpl := htmltpl.Must(htmltpl.New("cat").Funcs(funcs).Parse(htmlMissionsCategoryTemplate))
	detTmpl := htmltpl.Must(htmltpl.New("det").Funcs(funcs).Parse(htmlMissionDetailTemplate))

	if err := writeTemplate(filepath.Join(outDir, "index.html"), topTmpl, categories); err != nil {
		return err
	}

	for _, cat := range categories {
		catDir := filepath.Join(outDir, cat.Name)
		if err := os.MkdirAll(catDir, 0o755); err != nil {
			return err
		}
		if err := writeTemplate(filepath.Join(catDir, "index.html"), catTmpl, cat); err != nil {
			return err
		}
		for _, m := range cat.Missions {
			path := filepath.Join(catDir, m.ID+".html")
			if err := writeTemplate(path, detTmpl, m); err != nil {
				return err
			}
		}
	}

	return nil
}

func difficultyBadge(d int) htmltpl.HTML {
	if d <= 0 {
		return htmltpl.HTML(`<span class="text-muted">—</span>`)
	}
	stars := strings.Repeat("\u2605", d)
	return htmltpl.HTML(fmt.Sprintf(`<span class="difficulty" title="Difficulty %d">%s</span>`, d, stars))
}

// --- Templates ---


var htmlMissionsTopTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Missions - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../missions/missions.css">
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <h2>Missions</h2>
        <p class="text-muted mt-1">{{totalCount .}} missions across {{len .}} categories.</p>
        <div class="item-categories">
{{- range .}}
            <a href="{{.Name}}/" class="item-cat-card">
                <div class="cat-count">{{.Count}} missions</div>
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

var htmlMissionsCategoryTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{titleCase .Name}} Missions - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../missions/missions.css">
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Missions</a> / {{titleCase .Name}}</div>
        <h2>{{titleCase .Name}} <span class="text-muted">{{.Count}} missions</span></h2>
        {{if .Description}}<p class="text-muted mt-1">{{.Description}}</p>{{end}}
        <div class="table-container">
            <table class="sortable">
                <thead>
                    <tr>
                        <th class="sortable">Title</th>
                        <th class="sortable">Difficulty</th>
                        <th class="sortable">Giver</th>
                        <th class="sortable">Faction</th>
                        <th class="sortable">Credits</th>
                        <th class="sortable">Objectives</th>
                        <th class="sortable">Chain</th>
                    </tr>
                </thead>
                <tbody>
{{- range .Missions}}
                    <tr>
                        <td><a href="{{.ID}}.html">{{.Title}}</a></td>
                        <td data-sort="{{.Difficulty}}">{{difficulty .Difficulty}}</td>
                        <td>{{if .GiverName}}{{.GiverName}}{{else}}<span class="text-muted">—</span>{{end}}</td>
                        <td>{{if .FactionName}}{{.FactionName}}{{else}}<span class="text-muted">—</span>{{end}}</td>
                        <td data-sort="{{.RewardsCredits}}" class="value">{{fmtValue .RewardsCredits}}</td>
                        <td data-sort="{{len .Objectives}}">{{len .Objectives}}</td>
                        <td>{{if .ChainNext}}{{if .ChainNextHref}}<a href="{{.ChainNextHref}}" title="Chains into {{.ChainNextTitle}}">{{.ChainNextTitle}}</a>{{else}}<span class="text-muted" title="Not yet discovered">{{.ChainNext}}</span>{{end}}{{else}}<span class="text-muted">—</span>{{end}}</td>
                    </tr>
{{- end}}
                </tbody>
            </table>
        </div>
    </main>
` + sortScript + themeScript + `
</body>
</html>
`

var htmlMissionDetailTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../missions/missions.css">
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Missions</a> / <a href="./">{{titleCase .Type}}</a> / {{.Title}}</div>

        {{if or .ChainPrevHref .ChainNextHref}}
        <nav class="chain-nav">
            {{if .ChainPrevHref}}<a class="chain-prev" href="{{.ChainPrevHref}}">&lt; Prev: {{.ChainPrevTitle}}</a>{{else}}<span></span>{{end}}
            {{if .ChainNextHref}}<a class="chain-next" href="{{.ChainNextHref}}">Next: {{.ChainNextTitle}} &gt;</a>{{else}}<span></span>{{end}}
        </nav>
        {{end}}

        <h2>{{.Title}} {{if gt .Difficulty 0}}{{difficulty .Difficulty}}{{end}}{{if .Repeatable}} <span class="badge badge-repeatable">Repeatable</span>{{end}}</h2>
        {{if .Description}}<p class="mission-desc">{{.Description}}</p>{{end}}

        {{if or .DialogOffer .DialogAccept .DialogDecline .DialogComplete}}
        <section class="detail-section">
            <h3>Dialog</h3>
            {{if .DialogOffer}}<p class="dialog-label">Offer</p><blockquote class="dialog">{{.DialogOffer}}</blockquote>{{end}}
            {{if .DialogAccept}}<p class="dialog-label">Accept</p><blockquote class="dialog">{{.DialogAccept}}</blockquote>{{end}}
            {{if .DialogDecline}}<p class="dialog-label">Decline</p><blockquote class="dialog">{{.DialogDecline}}</blockquote>{{end}}
            {{if .DialogComplete}}<p class="dialog-label">Complete</p><blockquote class="dialog">{{.DialogComplete}}</blockquote>{{end}}
        </section>
        {{end}}

        <section class="detail-section">
            <h3>Overview</h3>
            <table class="detail-table">
                <tbody>
                    <tr><td class="kv-label">Type</td><td><a href="./">{{titleCase .Type}}</a></td></tr>
                    {{if gt .Difficulty 0}}<tr><td class="kv-label">Difficulty</td><td>{{difficulty .Difficulty}}</td></tr>{{end}}
                    {{if .GiverName}}<tr><td class="kv-label">Giver</td><td>{{.GiverName}}{{if .GiverTitle}} <span class="text-muted">— {{.GiverTitle}}</span>{{end}}</td></tr>{{end}}
                    {{if .FactionName}}<tr><td class="kv-label">Faction</td><td>{{.FactionName}}</td></tr>{{end}}
                    <tr><td class="kv-label">Repeatable</td><td>{{if .Repeatable}}Yes{{else}}No{{end}}</td></tr>
                    {{if gt .ExpiresInTicks 0}}<tr><td class="kv-label">Expires In</td><td>{{.ExpiresInTicks}} ticks</td></tr>{{end}}
                    {{if .ChainNext}}<tr><td class="kv-label">Chains To</td><td>{{if .ChainNextHref}}<a href="{{.ChainNextHref}}">{{.ChainNextTitle}}</a>{{else}}<span class="text-muted" title="Not yet discovered">{{.ChainNext}}</span>{{end}}</td></tr>{{end}}
                    {{if .ChainPrev}}<tr><td class="kv-label">Chains From</td><td><a href="{{.ChainPrevHref}}">{{.ChainPrevTitle}}</a></td></tr>{{end}}
                </tbody>
            </table>
        </section>

        {{if .Objectives}}
        <section class="detail-section">
            <h3>Objectives</h3>
            <ol class="objectives">
            {{- range .Objectives}}
                <li>
                    <div class="obj-desc">{{.Description}}</div>
                    <div class="obj-meta">
                        <span class="obj-type">{{.Type}}</span>
                        {{if .ItemID}} · <a href="../../items/{{.ItemCategory}}/{{.ItemID}}.html">{{.ItemName}}</a>{{if gt .Quantity 0}} ×{{.Quantity}}{{end}}{{else if gt .Quantity 0}} · ×{{.Quantity}}{{end}}
                        {{if .SystemID}} · <a href="../../systems/{{.SystemID}}.html">{{if .SystemName}}{{.SystemName}}{{else}}{{.SystemID}}{{end}}</a>{{else if .SystemName}} · {{.SystemName}}{{end}}
                        {{if .TargetBaseName}} · {{.TargetBaseName}}{{end}}
                    </div>
                </li>
            {{- end}}
            </ol>
        </section>
        {{end}}

        <section class="detail-section">
            <h3>Rewards</h3>
            <table class="detail-table">
                <tbody>
                    <tr><td class="kv-label">Credits</td><td>{{fmtValue .RewardsCredits}}</td></tr>
                    {{if .RewardsSkillXP}}
                    <tr><td class="kv-label">Skill XP</td><td>
                        <ul class="reward-list">
                        {{- range .RewardsSkillXP}}
                            <li><a href="../../skills/{{.Name}}.html">{{titleCase .Name}}</a> <span class="text-muted">+{{.Value}}</span></li>
                        {{- end}}
                        </ul>
                    </td></tr>
                    {{end}}
                    {{if .RewardsItems}}
                    <tr><td class="kv-label">Items</td><td>
                        <ul class="reward-list">
                        {{- range .RewardsItems}}
                            <li>{{.Quantity}}x {{if .Category}}<a href="../../items/{{.Category}}/{{.ItemID}}.html">{{.ItemName}}</a>{{else}}{{.ItemName}}{{end}}</li>
                        {{- end}}
                        </ul>
                    </td></tr>
                    {{end}}
                </tbody>
            </table>
        </section>

        {{if .ProvidedItems}}
        <section class="detail-section">
            <h3>Provided Items</h3>
            <ul class="reward-list">
            {{- range .ProvidedItems}}
                <li>{{.Quantity}}x {{if .Category}}<a href="../../items/{{.Category}}/{{.ItemID}}.html">{{.ItemName}}</a>{{else}}{{.ItemName}}{{end}}</li>
            {{- end}}
            </ul>
        </section>
        {{end}}

        {{if .RequiredModules}}
        <section class="detail-section">
            <h3>Required Modules</h3>
            <ul class="reward-list">
            {{- range .RequiredModules}}
                <li>{{.}}</li>
            {{- end}}
            </ul>
        </section>
        {{end}}

        {{if .Locations}}
        <section class="detail-section">
            <h3>Offered At</h3>
            <table class="detail-table">
                <thead>
                    <tr>
                        <th>Base</th>
                        <th>System</th>
                    </tr>
                </thead>
                <tbody>
                {{- range .Locations}}
                    <tr>
                        <td>{{.BaseName}}</td>
                        <td>{{if .SystemID}}<a href="../../systems/{{.SystemID}}.html">{{if .SystemName}}{{.SystemName}}{{else}}{{.SystemID}}{{end}}</a>{{else}}<span class="text-muted">—</span>{{end}}</td>
                    </tr>
                {{- end}}
                </tbody>
            </table>
        </section>
        {{end}}

    </main>
` + themeScript + `
</body>
</html>
`
