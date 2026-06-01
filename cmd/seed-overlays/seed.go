// Command seed-overlays scaffolds overlay stubs from agent personality.json data.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type stubStat struct{ Label, Value string }

// normalizeName lowercases and trims a display name for matching.
func normalizeName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// normalizeOrg normalizes an organization/faction name for matching by
// lowercasing, trimming, and dropping a leading "the ".
func normalizeOrg(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.TrimPrefix(s, "the ")
}

// renderStub builds a profile.md body from a biography and structured stats.
func renderStub(biography string, stats []stubStat) string {
	var b strings.Builder
	b.WriteString("---\n")
	if len(stats) > 0 {
		b.WriteString("stats:\n")
		for _, s := range stats {
			fmt.Fprintf(&b, "  - label: %q\n    value: %q\n", s.Label, s.Value)
		}
	}
	b.WriteString("---\n\n")
	b.WriteString("## Biography\n\n")
	b.WriteString(strings.TrimSpace(biography))
	b.WriteString("\n")
	return b.String()
}

// writeStub writes content to path unless the file already exists. Returns
// whether it (would) write. In dryRun mode it never touches the filesystem.
func writeStub(path, content string, dryRun bool) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil // exists -> never overwrite
	}
	if dryRun {
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
