package main

import (
	"bytes"
	htmltpl "html/template"
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
