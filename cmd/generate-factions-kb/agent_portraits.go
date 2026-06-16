package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Agent portraits give the user's own bot fleet AI-generated faces on their KB
// player pages. Agents live as personality.json bios under the spacemolt repo;
// each agent's server player_id is recovered from daily-summary.db
// (status_raw.player.id in the latest snapshot). Only agents that are also real
// sighted players (present in the loaded players set) get a portrait, since the
// KB renders a page only for sighted players — generating for the rest would
// orphan the images.

// playerGeneratedDir is the cache directory for a player's generated portrait +
// prompt sidecar, mirroring passengerGeneratedDir but under generated/players.
func playerGeneratedDir(root, id string) string {
	return filepath.Join(root, "generated", "players", id)
}

// agentRoleArchetype maps an agent's personality.json role to one of the shared
// passenger archetypes (see archetypeGarment) so the bot fleet styles the same
// way the passenger gallery does. Roles absent here fall back to "spacer".
// Pirate/swashbuckler and the synthetic Assist role are handled separately in
// buildAgentPortraitPrompt.
var agentRoleArchetype = map[string]string{
	"craftsman":      "technician",
	"engineer":       "technician",
	"technician":     "technician",
	"dataservice":    "official",
	"detective":      "official",
	"agent":          "official",
	"explorer":       "pilot",
	"void navigator": "pilot",
	"miner":          "laborer",
	"salvager":       "laborer",
	"fighter":        "officer",
	"mercenary":      "outlaw",
	"bounty hunter":  "outlaw",
	"prophet":        "spiritual",
	"xenobiologist":  "medic",
	"merchant":       "merchant",
	"trader":         "merchant",
}

// agentRoleGarment overrides the shared archetypeGarment for a few agent roles
// where the passenger garment reads wrong on a combatant. Notably the "officer"
// garment ("formal military uniform") renders fighters as authoritarian
// junta-style officers; rugged tactical gear better fits a freelance space
// combatant. Roles absent here fall through to archetypeGarment via the
// archetype mapping.
var agentRoleGarment = map[string]string{
	"fighter":   "in practical tactical combat gear",
	"mercenary": "in practical tactical combat gear",
}

// syntheticAesthetic styles the Assist-role service drones as androids rather
// than humans; it replaces the human physical-traits descriptor entirely.
const syntheticAesthetic = "a humanoid service android, smooth synthetic facial features, softly glowing optical sensors, sleek matte robotic chassis, polished plating"

// isSyntheticRole reports whether a role denotes a non-human bot. Only the
// Assist drones qualify — names like "databot" belong to human archivists.
func isSyntheticRole(role string) bool {
	return strings.ToLower(strings.TrimSpace(role)) == "assist"
}

// buildAgentPortraitPrompt composes an image prompt for one agent, reusing the
// passenger styling helpers (portraitCue, bioGenderNoun, empireStyle,
// archetypeGarment, physicalTraits). Synthetics get an android cue and skip the
// human physical traits; pirates get the pirate aesthetic; everyone else gets an
// empire sensibility layered with a role-derived garment.
func buildAgentPortraitPrompt(playerID, bio, empire, role string, ov PortraitOverride) string {
	subject := ov.bioWithAppend(bio)
	if subject == "" {
		subject = "a seasoned spacer"
	}
	// A synthetic (Assist drone) stays an android unless an override re-bodies it
	// with an explicit appearance or species.
	if isSyntheticRole(role) && strings.TrimSpace(ov.Appearance) == "" && strings.TrimSpace(ov.Species) == "" {
		return portraitCue("android") + ", " + syntheticAesthetic + ", " + subject
	}
	emp := strings.ToLower(strings.TrimSpace(empire))
	r := strings.ToLower(strings.TrimSpace(role))
	return portraitCue(ov.gender(bio)) + ", " + agentAesthetic(bio, emp, r, ov) + ", " +
		ov.physical(playerID) + ", " + subject
}

// agentAesthetic picks the styling phrase for a non-synthetic agent: a performer
// bio wins, then pirate/swashbuckler roles, then an empire sensibility layered
// with a garment. An archetype override selects the garment directly; otherwise
// it derives from the role (generic fallbacks for unknown empire/role).
func agentAesthetic(bio, emp, role string, ov PortraitOverride) string {
	if hasPerformerCue(bio) {
		return performerAesthetic
	}
	if role == "pirate" || role == "swashbuckler" {
		return pirateAesthetic
	}
	empire, ok := empireStyle[emp]
	if !ok {
		empire = "practical spacer style"
	}
	var garment string
	if a := strings.ToLower(strings.TrimSpace(ov.Archetype)); a != "" {
		garment = archetypeGarment[a]
	}
	if garment == "" {
		garment, ok = agentRoleGarment[role]
		if !ok {
			garment, ok = archetypeGarment[agentRoleArchetype[role]]
			if !ok {
				garment = archetypeGarment["spacer"]
			}
		}
	}
	return empire + ", " + garment
}

// agentPersona is the subset of an agent's personality.json needed to build a
// portrait, paired with its resolved server player_id.
type agentPersona struct {
	AgentID  string
	PlayerID string
	Name     string
	Bio      string
	Empire   string
	Role     string
}

// loadAgentPlayerIDs reads daily-summary.db and returns agent_id -> player_id
// from each agent's most recent snapshot (status_raw.player.id).
func loadAgentPlayerIDs(dailySummaryPath string) (map[string]string, error) {
	db, err := sql.Open("sqlite", dailySummaryPath)
	if err != nil {
		return nil, fmt.Errorf("open daily-summary: %w", err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query(`SELECT agent_id, state_json FROM snapshots
		GROUP BY agent_id HAVING captured_at = MAX(captured_at)`)
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]string{}
	for rows.Next() {
		var agentID, stateJSON string
		if err := rows.Scan(&agentID, &stateJSON); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		var st struct {
			StatusRaw struct {
				Player struct {
					ID string `json:"id"`
				} `json:"player"`
			} `json:"status_raw"`
		}
		if err := json.Unmarshal([]byte(stateJSON), &st); err != nil {
			continue
		}
		if id := st.StatusRaw.Player.ID; id != "" {
			out[agentID] = id
		}
	}
	return out, rows.Err()
}

// personalityFile is the subset of personality.json this package reads.
type personalityFile struct {
	Name      string `json:"name"`
	Biography string `json:"biography"`
	Empire    string `json:"empire"`
	Role      string `json:"role"`
}

// loadAgentPersonas builds the persona list for every agent whose resolved
// player_id is in live (the set of sighted players that have KB pages). agentsDir
// holds one subdirectory per agent, each with a personality.json.
func loadAgentPersonas(agentsDir, dailySummaryPath string, live map[string]bool) ([]agentPersona, error) {
	ids, err := loadAgentPlayerIDs(dailySummaryPath)
	if err != nil {
		return nil, err
	}
	var personas []agentPersona
	for agentID, playerID := range ids {
		if !live[playerID] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(agentsDir, agentID, "personality.json"))
		if err != nil {
			log.Printf("warning: agent %s personality.json: %v", agentID, err)
			continue
		}
		var pf personalityFile
		if err := json.Unmarshal(data, &pf); err != nil {
			log.Printf("warning: agent %s personality.json parse: %v", agentID, err)
			continue
		}
		personas = append(personas, agentPersona{
			AgentID:  agentID,
			PlayerID: playerID,
			Name:     pf.Name,
			Bio:      pf.Biography,
			Empire:   pf.Empire,
			Role:     pf.Role,
		})
	}
	return personas, nil
}

// generateAgentPortraits builds a prompt for each agent persona and invokes the
// configured CLI for any whose cached portrait is missing or whose prompt
// changed, writing into playerGeneratedDir. Empty cmdLine is a no-op. A positive
// limit restricts generation to the first limit personas.
func generateAgentPortraits(personas []agentPersona, root, cmdLine string, limit int) {
	if strings.TrimSpace(cmdLine) == "" {
		log.Printf("agent portrait generation skipped: no command configured (set SMKB_PORTRAIT_CMD)")
		return
	}
	if limit > 0 && limit < len(personas) {
		log.Printf("agent portrait generation limited to first %d of %d agents", limit, len(personas))
		personas = personas[:limit]
	}
	for _, a := range personas {
		ov := loadPortraitOverride(filepath.Join(root, "players", a.PlayerID))
		prompt := buildAgentPortraitPrompt(a.PlayerID, a.Bio, a.Empire, a.Role, ov)
		hash := promptHash(prompt)
		dir := playerGeneratedDir(root, a.PlayerID)
		if portraitExists(dir) && cachedHashMatches(dir, hash) {
			continue
		}
		out := filepath.Join(dir, generatedPortraitName)
		if err := runPortraitCmd(cmdLine, prompt, a.PlayerID, out); err != nil {
			log.Printf("warning: agent portrait %s (%s): %v", a.AgentID, a.PlayerID, err)
			continue
		}
		writePromptSidecar(dir, prompt, hash)
		log.Printf("generated portrait for agent %s -> %s", a.AgentID, a.PlayerID)
	}
}

// attachPlayerPortraits sets PortraitFile for each player whose generated
// portrait exists and validates. A missing file is silent; only real validation
// failures warn.
func attachPlayerPortraits(players []*Player, root string) {
	for _, p := range players {
		dir := playerGeneratedDir(root, p.ID)
		name, err := validateImage(dir, generatedPortraitName)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				log.Printf("warning: generated player portrait %s: %v", p.ID, err)
			}
			continue
		}
		p.PortraitFile = name
	}
}
