package resourcediff

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Patch is one server release from the get_version feed.
type Patch struct {
	Version     string   `json:"version"`
	ReleaseDate string   `json:"release_date"`
	Notes       []string `json:"notes"`
}

// Patch notes that talk about mineable resources. Strong words match in any
// case; the weaker ones only as lowercase prose, so that item names such as
// "Ore Compressor" or "Phase Crystal" in a modules list do not count.
var (
	resourceNoteStrong = regexp.MustCompile(`(?i)\b(mineable|resources?|deposits?|asteroids?|mining)\b`)
	resourceNoteWeak   = regexp.MustCompile(`\b(ores?|ices?|crystals?|nebulium)\b`)
)

func isResourceNote(n string) bool {
	return resourceNoteStrong.MatchString(n) || resourceNoteWeak.MatchString(n)
}

// ResourceNotes returns the patch's notes that mention resources, with
// markdown emphasis stripped.
func (p Patch) ResourceNotes() []string {
	var out []string
	for _, n := range p.Notes {
		if isResourceNote(n) {
			out = append(out, strings.ReplaceAll(n, "**", ""))
		}
	}
	return out
}

// VersionFeed is the union of every get_version.json scrape under a game-api
// directory. Each scrape lists only the most recent few patches, so the
// history has to be stitched together across scrapes.
type VersionFeed struct {
	Patches []Patch // newest first
	current string  // version reported by the newest scrape
}

// LoadVersionFeed reads <dir>/<YYYYMMDD>/get_version.json for every dated
// scrape directory. A missing directory yields an empty feed, not an error.
func LoadVersionFeed(dir string) (*VersionFeed, error) {
	feed := &VersionFeed{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return feed, nil
		}
		return nil, err
	}
	byVersion := make(map[string]Patch)
	newestScrape := ""
	for _, e := range entries {
		name := e.Name()
		if len(name) != 8 || !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name, "get_version.json"))
		if err != nil {
			continue
		}
		var top struct {
			Version  string  `json:"version"`
			Versions []Patch `json:"versions"`
		}
		if err := json.Unmarshal(data, &top); err != nil {
			return nil, fmt.Errorf("%s/get_version.json: %w", name, err)
		}
		if name > newestScrape && top.Version != "" {
			newestScrape = name
			feed.current = top.Version
		}
		for _, p := range top.Versions {
			if p.Version != "" {
				byVersion[p.Version] = p
			}
		}
	}
	for _, p := range byVersion {
		feed.Patches = append(feed.Patches, p)
	}
	slices.SortFunc(feed.Patches, func(a, b Patch) int { return CompareVersions(b.Version, a.Version) })
	return feed, nil
}

// Current is the server version reported by the newest scrape.
func (f *VersionFeed) Current() string { return f.current }

// Patch returns the feed entry for a version, or nil.
func (f *VersionFeed) Patch(version string) *Patch {
	for i := range f.Patches {
		if f.Patches[i].Version == version {
			return &f.Patches[i]
		}
	}
	return nil
}

// ResourcePatches returns the patches in (after, upto] whose notes mention
// resources, newest first. An empty after means no lower bound.
func (f *VersionFeed) ResourcePatches(after, upto string) []Patch {
	var out []Patch
	for _, p := range f.Patches {
		if after != "" && CompareVersions(p.Version, after) <= 0 {
			continue
		}
		if upto != "" && CompareVersions(p.Version, upto) > 0 {
			continue
		}
		if len(p.ResourceNotes()) > 0 {
			out = append(out, p)
		}
	}
	return out
}

// CompareVersions orders dotted numeric versions ("0.566.2"). Non-numeric
// components compare as strings; an empty version sorts first.
func CompareVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		var x, y string
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		xi, xerr := strconv.Atoi(x)
		yi, yerr := strconv.Atoi(y)
		var c int
		if xerr == nil && yerr == nil {
			c = cmp.Compare(xi, yi)
		} else {
			c = cmp.Compare(x, y)
		}
		if c != 0 {
			return c
		}
	}
	return 0
}
