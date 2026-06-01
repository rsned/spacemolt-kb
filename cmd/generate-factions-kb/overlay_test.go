package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	withFM := "---\nimage: logo.png\n---\n## Body\n\nHello\n"
	front, body := splitFrontmatter([]byte(withFM))
	if string(front) != "image: logo.png" {
		t.Errorf("front = %q", front)
	}
	if body != "## Body\n\nHello\n" {
		t.Errorf("body = %q", body)
	}

	// No frontmatter: everything is body.
	front, body = splitFrontmatter([]byte("just a body\n"))
	if front != nil {
		t.Errorf("expected nil front, got %q", front)
	}
	if body != "just a body\n" {
		t.Errorf("body = %q", body)
	}

	// Unterminated frontmatter: treat whole thing as body, no front.
	front, _ = splitFrontmatter([]byte("---\nimage: x\nno closing fence\n"))
	if front != nil {
		t.Errorf("expected nil front for unterminated, got %q", front)
	}

	// CRLF normalized.
	front, _ = splitFrontmatter([]byte("---\r\nimage: a.png\r\n---\r\nbody"))
	if string(front) != "image: a.png" {
		t.Errorf("CRLF front = %q", front)
	}
}

func TestRenderMarkdown(t *testing.T) {
	html, err := renderMarkdown("This is **bold** and a [link](http://example.com).")
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	if !strings.Contains(s, "<strong>bold</strong>") {
		t.Errorf("bold not rendered: %q", s)
	}
	if !strings.Contains(s, `rel="nofollow noopener"`) {
		t.Errorf("link not hardened with nofollow: %q", s)
	}

	// Raw HTML / scripts must be neutralized (goldmark safe mode).
	bad, err := renderMarkdown("ok <script>alert(1)</script> done")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bad), "<script>") {
		t.Errorf("script tag leaked through: %q", bad)
	}
}

func TestValidateImage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Empty name is fine (no image), returns "".
	if got, err := validateImage(dir, ""); err != nil || got != "" {
		t.Errorf("empty: got %q err %v", got, err)
	}
	// Valid file.
	if got, err := validateImage(dir, "logo.png"); err != nil || got != "logo.png" {
		t.Errorf("valid: got %q err %v", got, err)
	}
	// Bad extension.
	if _, err := validateImage(dir, "logo.svg"); err == nil {
		t.Error("expected error for .svg")
	}
	// Path traversal.
	if _, err := validateImage(dir, "../secret.png"); err == nil {
		t.Error("expected error for traversal")
	}
	if _, err := validateImage(dir, "sub/logo.png"); err == nil {
		t.Error("expected error for separator")
	}
	// Missing file.
	if _, err := validateImage(dir, "nope.png"); err == nil {
		t.Error("expected error for missing file")
	}
}

func writeOverlay(t *testing.T, dir, profile string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "profile.md"), []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadOverlay(t *testing.T) {
	// Missing profile.md -> (nil, nil).
	if ov, err := loadOverlay(t.TempDir()); err != nil || ov != nil {
		t.Errorf("missing: ov=%v err=%v", ov, err)
	}

	// Full overlay: frontmatter (image + ordered stats) + body.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeOverlay(t, dir, "---\nimage: logo.png\nimage_alt: Crest\nstats:\n  - label: Homeworld\n    value: Krynn\n  - label: Founded\n    value: 2387\n---\n## Bio\n\nWe **optimize**.\n")
	ov, err := loadOverlay(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ov.ImageFile != "logo.png" || ov.ImageAlt != "Crest" {
		t.Errorf("image fields: %+v", ov)
	}
	if len(ov.Stats) != 2 || ov.Stats[0].Label != "Homeworld" || ov.Stats[1].Value != "2387" {
		t.Errorf("stats: %+v", ov.Stats)
	}
	if !strings.Contains(string(ov.BodyHTML), "<strong>optimize</strong>") {
		t.Errorf("body: %q", ov.BodyHTML)
	}

	// Bad image extension: overlay still returned, image skipped (warn-not-fail).
	dir2 := t.TempDir()
	writeOverlay(t, dir2, "---\nimage: logo.svg\n---\nbody")
	ov2, err := loadOverlay(dir2)
	if err != nil {
		t.Fatal(err)
	}
	if ov2.ImageFile != "" {
		t.Errorf("expected image skipped, got %q", ov2.ImageFile)
	}

	// Body-only (no frontmatter).
	dir3 := t.TempDir()
	writeOverlay(t, dir3, "Just a bio.\n")
	ov3, err := loadOverlay(dir3)
	if err != nil {
		t.Fatal(err)
	}
	if ov3.BodyHTML == "" || len(ov3.Stats) != 0 || ov3.ImageFile != "" {
		t.Errorf("body-only: %+v", ov3)
	}
}
