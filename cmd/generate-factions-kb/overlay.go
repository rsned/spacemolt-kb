package main

import (
	"bytes"
	"fmt"
	htmltpl "html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
)

// Overlay is contributor-authored content merged onto a faction or player page.
type Overlay struct {
	ImageFile string        // validated bare filename, or "" when absent/invalid
	ImageAlt  string        // alt text for the image
	Stats     []OverlayStat // ordered structured stats
	BodyHTML  htmltpl.HTML  // goldmark-rendered markdown body (safe), or ""
}

// OverlayStat is one label/value row of an overlay's structured stats.
type OverlayStat struct {
	Label string `yaml:"label"`
	Value string `yaml:"value"`
}

// goldmark with its default (safe) options: raw HTML is replaced with an
// "omitted" comment rather than passed through, so contributor markdown cannot
// inject scripts.
var markdown = goldmark.New()

// renderMarkdown converts a markdown body to safe HTML and hardens links with
// rel="nofollow noopener".
func renderMarkdown(src string) (htmltpl.HTML, error) {
	var buf bytes.Buffer
	if err := markdown.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	out := strings.ReplaceAll(buf.String(), "<a href=", `<a rel="nofollow noopener" href=`)
	return htmltpl.HTML(out), nil
}

var allowedImageExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true,
}

// validateImage checks that name is a bare filename (no separators, no ".."),
// has an allowed raster extension, and exists in dir. Returns the validated
// name, or an error describing why it was rejected. Empty name -> ("", nil).
func validateImage(dir, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("image %q must be a bare filename with no path", name)
	}
	ext := strings.ToLower(filepath.Ext(name))
	if !allowedImageExt[ext] {
		return "", fmt.Errorf("image %q has unsupported extension %q", name, ext)
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		return "", fmt.Errorf("image %q not found in overlay dir: %w", name, err)
	}
	return name, nil
}

// splitFrontmatter separates a leading "---\n...\n---\n" YAML block from the
// markdown body. Returns nil front when there is no well-formed frontmatter; in
// that case the entire (newline-normalized) input is the body.
func splitFrontmatter(content []byte) (front []byte, body string) {
	s := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return nil, s
	}
	rest := s[len("---\n"):]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		// Allow a closing fence at EOF with no trailing newline.
		if strings.HasSuffix(rest, "\n---") {
			return []byte(rest[:len(rest)-len("\n---")]), ""
		}
		return nil, s
	}
	return []byte(rest[:idx]), rest[idx+len("\n---\n"):]
}
