// Package apidiff compares dated OpenAPI snapshots of the SpaceMolt server
// and renders the struct-level changes as Go or TypeScript diff pages.
package apidiff

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// Snapshot is one dated spec's components.schemas.
type Snapshot struct {
	Date    string // YYYYMMDD
	Schemas map[string]any
}

var snapshotName = regexp.MustCompile(`^openapi\.(\d{8})\.json(\.gz)?$`)

// LoadSnapshots reads every openapi.YYYYMMDD.json in dir and every
// openapi.YYYYMMDD.json.gz in dir/archive, oldest first. Undated names, the
// v2 spec and editor files are skipped. A file that cannot be read fails the
// whole load with an error naming it.
func LoadSnapshots(dir string) ([]Snapshot, error) {
	var snaps []Snapshot
	seen := map[string]string{}
	for _, d := range []string{dir, filepath.Join(dir, "archive")} {
		entries, err := os.ReadDir(d)
		if err != nil {
			if d != dir && os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			m := snapshotName.FindStringSubmatch(e.Name())
			if m == nil || e.IsDir() {
				continue
			}
			path := filepath.Join(d, e.Name())
			if prev, dup := seen[m[1]]; dup {
				return nil, fmt.Errorf("snapshot %s appears twice: %s and %s", m[1], prev, path)
			}
			seen[m[1]] = path
			schemas, err := readSchemas(path, m[2] != "")
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			snaps = append(snaps, Snapshot{Date: m[1], Schemas: schemas})
		}
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].Date < snaps[j].Date })
	return snaps, nil
}

func readSchemas(path string, gz bool) (map[string]any, error) {
	f, err := os.Open(path) //nolint:gosec // dated spec files from a fixed docs directory
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only file
	var r io.Reader = f
	if gz {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer zr.Close() //nolint:errcheck // read-only stream
		r = zr
	}
	var doc struct {
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	dec := json.NewDecoder(r)
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = map[string]any{}
	}
	return doc.Components.Schemas, nil
}

// ISODate turns YYYYMMDD into YYYY-MM-DD.
func ISODate(d string) string {
	if len(d) != 8 {
		return d
	}
	return d[:4] + "-" + d[4:6] + "-" + d[6:]
}
