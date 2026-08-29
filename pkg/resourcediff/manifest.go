package resourcediff

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// ManifestEntry records one snapshot and, for baselines, the content patch
// it anchors.
type ManifestEntry struct {
	Date          string `json:"date"`
	ServerVersion string `json:"server_version,omitempty"`
	Source        string `json:"source"`
	Baseline      bool   `json:"baseline,omitempty"`
	// Baseline-only fields.
	ContentVersion     string   `json:"content_version,omitempty"`
	ContentReleaseDate string   `json:"content_release_date,omitempty"`
	ContentNotes       []string `json:"content_notes,omitempty"`
	Reason             string   `json:"reason,omitempty"`
}

// Manifest indexes the snapshot directory. Snapshots are kept sorted by date.
type Manifest struct {
	Latest    string          `json:"latest"`
	Baseline  string          `json:"baseline"`
	Snapshots []ManifestEntry `json:"snapshots"`
}

// SnapshotPath is where the snapshot for a date lives.
func SnapshotPath(dir, date string) string { return filepath.Join(dir, date+".json") }

// LoadSnapshot reads the snapshot for a date from dir.
func LoadSnapshot(dir, date string) (*Snapshot, error) {
	data, err := os.ReadFile(SnapshotPath(dir, date))
	if err != nil {
		return nil, err
	}
	return Unmarshal(data)
}

// LoadManifest reads dir/manifest.json; a missing file is an empty manifest.
func LoadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		return &Manifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Save writes the manifest to dir/manifest.json.
func (m *Manifest) Save(dir string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), append(data, '\n'), 0o644)
}

// PreviousBefore returns the newest snapshot dated strictly before date, or nil.
func (m *Manifest) PreviousBefore(date string) *ManifestEntry {
	var prev *ManifestEntry
	for i := range m.Snapshots {
		e := &m.Snapshots[i]
		if e.Date < date && (prev == nil || e.Date > prev.Date) {
			prev = e
		}
	}
	return prev
}

// BaselineFor returns the baseline in effect for a snapshot date: the newest
// baseline entry dated on or before it, or nil.
func (m *Manifest) BaselineFor(date string) *ManifestEntry {
	var base *ManifestEntry
	for i := range m.Snapshots {
		e := &m.Snapshots[i]
		if e.Baseline && e.Date <= date && (base == nil || e.Date > base.Date) {
			base = e
		}
	}
	return base
}
