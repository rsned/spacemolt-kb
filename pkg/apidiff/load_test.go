package apidiff

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadSnapshotsFindsDatedFilesOnly(t *testing.T) {
	snaps, err := LoadSnapshots("testdata/docs")
	if err != nil {
		t.Fatal(err)
	}
	var dates []string
	for _, s := range snaps {
		dates = append(dates, s.Date)
		if _, ok := s.Schemas["A"]; !ok {
			t.Errorf("%s: schema A missing", s.Date)
		}
	}
	if want := []string{"20260215", "20260301"}; !slices.Equal(dates, want) {
		t.Errorf("dates = %v, want %v", dates, want)
	}
}

func TestLoadSnapshotsBadFileNamesIt(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "archive", "openapi.20260101.json.gz")
	if err := os.WriteFile(bad, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSnapshots(dir)
	if err == nil || !strings.Contains(err.Error(), "openapi.20260101.json.gz") {
		t.Fatalf("err = %v, want one naming the file", err)
	}
}

func TestISODate(t *testing.T) {
	if got := ISODate("20261107"); got != "2026-11-07" {
		t.Errorf("ISODate = %q", got)
	}
}
