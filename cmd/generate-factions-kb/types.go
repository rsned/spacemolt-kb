// Command generate-factions-kb reads the knowledge database and produces
// KB-styled HTML pages for player factions and the players sighted in them.
package main

// Faction is a player faction with its reconstructed roster and related records.
type Faction struct {
	ID                  string
	Name                string
	Tag                 string
	Slug                string
	LeaderName          string
	LeaderSlug          string // player_id of the leader when they have a page, else ""
	Treasury            int64
	OwnedBases          int
	Description         string
	Charter             string
	Emblem              string
	PrimaryColor        string
	SecondaryColor      string
	FoundedUTC          string
	OfficialMemberCount int // factions.member_count (usually 0, hidden by the API)

	Members    []*Member
	Bases      []Base
	Relations  []Relation
	Facilities []Facility

	Overlay *Overlay // contributor-authored content, nil when none
}

// MemberCount is the reconstructed roster size (distinct sighted players).
func (f *Faction) MemberCount() int { return len(f.Members) }

// Member is one player on a faction's reconstructed roster.
type Member struct {
	PlayerID    string
	Username    string
	Slug        string
	Role        string // from faction_members overlay; "" when unknown
	IsOnline    bool
	LastSeenUTC string
	Ships       []string // distinct ship class names sighted
}

// Base is a faction-owned base.
type Base struct {
	Name       string
	SystemName string
	Services   string
}

// Relation is a diplomatic relation to another faction.
type Relation struct {
	Kind       string
	TargetName string
	TargetTag  string
	Reason     string
	OurKills   int
	TheirKills int
}

// Facility is a faction facility at a base.
type Facility struct {
	Type     string
	Category string
	Level    int
	Status   string
}

// Player is a tracked player with sighting-derived history.
type Player struct {
	ID             string
	Username       string
	Slug           string
	FactionID      string
	FactionTag     string
	FactionSlug    string // "" when the faction is unknown/untracked
	ClanTag        string
	PrimaryColor   string
	SecondaryColor string
	StatusMessage  string
	FirstSeenUTC   string
	LastSeenUTC    string

	Ships     []ShipSeen
	Sightings []Sighting

	Overlay      *Overlay // contributor-authored content, nil when none
	PortraitFile string   // generated AI portrait filename, "" when none
}

// ShipSeen is a ship class a player was observed flying.
type ShipSeen struct {
	Class        string
	FirstSeenUTC string
	LastSeenUTC  string
}

// Sighting is a grouped where/when observation of a player.
type Sighting struct {
	SystemID    string
	SystemSlug  string // "" when no system page exists
	POIID       string
	ShipClass   string
	InCombat    bool
	LastSeenUTC string
}

// Passenger is a ship passenger (citizen) sighted in the game world.
type Passenger struct {
	ID            string
	Slug          string // == ID (citizen_id is already URL-safe)
	Name          string
	Citizenship   string
	EmpireColor   string // resolved from citizenship; "" when unknown
	Bio           string
	Class         string // travel class: "first" / "business"
	FirstSeenUTC  string
	LastSeenUTC   string
	SightingCount int

	Overlay        *Overlay // contributor-authored content, nil when none
	PortraitFile   string   // generated AI portrait filename, "" when none
	PortraitPrompt string   // prompt used to generate PortraitFile, "" when none
}
