package combatsim

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

type battleParticipant struct {
	PlayerID  string `json:"player_id"`
	Username  string `json:"username"`
	ShipClass string `json:"ship_class"`
	Modules   []struct {
		Name string `json:"name"`
	} `json:"modules"`
}

type rawAttack struct {
	AttackerID      string `json:"attacker_id"`
	TargetID        string `json:"target_id"`
	WeaponSkillPct  int    `json:"weapon_skill_pct"`
	ShieldResistPct int    `json:"shield_resist_pct"`
	Weapons         []struct {
		CritChance float64 `json:"crit_chance"`
	} `json:"weapons"`
}

// skillObs accumulates per-participant skill evidence from raw attack
// records. Fields hold the maximum observed value; zero means unobserved,
// which is indistinguishable from a true level 0 — both emit 0.
type skillObs struct {
	weapons        int // crit_chance × 100 (Weapons = 1%/level)
	weaponSkillPct int // Weapons + Gunnery
	shields        int // shield_resist_pct on attacks against this pilot
}

// warnf writes a best-effort warning; a failed write to the warning
// stream is deliberately ignored.
func warnf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// ExtractedFit is one participant's FitSpec plus the file name it belongs
// in: <battle_id>_<player_id>.json.
type ExtractedFit struct {
	Filename string
	Spec     FitSpec
}

// ExtractFits reads <battlesDir>/<battleID>.json and builds a FitSpec per
// participant. When <battleID>.raw.json sits next to it, skills are
// inferred from the attack records; otherwise they are zero and a warning
// is written. Armor never appears in battle logs and is always 0.
// A participant whose ship_class is not a catalog hull is skipped with a
// warning; a module name that maps to no catalog item is dropped with a
// warning. Warnings go to warn; only unreadable/unparsable input is an
// error.
func ExtractFits(battleID, battlesDir string, cat *Catalog, warn io.Writer) ([]ExtractedFit, error) {
	raw, err := os.ReadFile(filepath.Join(battlesDir, battleID+".json"))
	if err != nil {
		return nil, err
	}
	var battle struct {
		Participants []battleParticipant `json:"participants"`
	}
	if err := json.Unmarshal(raw, &battle); err != nil {
		return nil, fmt.Errorf("%s.json: %w", battleID, err)
	}

	obs, err := loadSkillObservations(filepath.Join(battlesDir, battleID+".raw.json"))
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		warnf(warn, "no %s.raw.json battle log: skills default to 0 for every participant\n", battleID)
	}

	nameToID := make(map[string]string, len(cat.Items))
	for id, it := range cat.Items {
		nameToID[it.Name] = id
	}

	var fits []ExtractedFit
	for _, p := range battle.Participants {
		if _, ok := cat.Ships[p.ShipClass]; !ok {
			warnf(warn, "skipping %s (%s): ship_class %q is not a catalog hull\n",
				p.Username, p.PlayerID, p.ShipClass)
			continue
		}
		modules := make([]string, 0, len(p.Modules))
		for _, m := range p.Modules {
			id, ok := nameToID[m.Name]
			if !ok {
				warnf(warn, "%s: dropping module %q: no catalog item with that name\n",
					p.Username, m.Name)
				continue
			}
			modules = append(modules, id)
		}
		o := obs[p.PlayerID]
		fits = append(fits, ExtractedFit{
			Filename: battleID + "_" + p.PlayerID + ".json",
			Spec: FitSpec{
				Name:    fmt.Sprintf("%s (%s)", p.Username, p.ShipClass),
				Hull:    p.ShipClass,
				Modules: modules,
				Skills: map[string]int{
					"weapons": o.weapons,
					"gunnery": max(0, o.weaponSkillPct-o.weapons),
					"shields": o.shields,
					"armor":   0,
				},
			},
		})
	}
	return fits, nil
}

// loadSkillObservations scans every attack record in a raw battle-log
// export for skill evidence. Weapons comes from per-weapon crit_chance
// (dev-confirmed 1%/level, rolled even on misses so it is always visible);
// weapon_skill_pct is Weapons + Gunnery; shield_resist_pct equals the
// TARGET's Shields level and is only logged while their shields hold, so
// taking the maximum over all observations is the correct estimator for
// every field.
func loadSkillObservations(path string) (map[string]skillObs, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pages []struct {
		Entries []struct {
			Attacks []rawAttack `json:"attacks"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &pages); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	obs := map[string]skillObs{}
	for _, page := range pages {
		for _, e := range page.Entries {
			for _, a := range e.Attacks {
				att := obs[a.AttackerID]
				att.weaponSkillPct = max(att.weaponSkillPct, a.WeaponSkillPct)
				for _, w := range a.Weapons {
					att.weapons = max(att.weapons, int(math.Round(w.CritChance*100)))
				}
				obs[a.AttackerID] = att
				tgt := obs[a.TargetID]
				tgt.shields = max(tgt.shields, a.ShieldResistPct)
				obs[a.TargetID] = tgt
			}
		}
	}
	return obs, nil
}
