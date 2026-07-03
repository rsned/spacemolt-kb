package main

import (
	"bytes"
	"errors"
	"fmt"
	htmltpl "html/template"
	"image"
	_ "image/gif"  // register GIF decoder for image.DecodeConfig
	_ "image/jpeg" // register JPEG decoder for image.DecodeConfig
	_ "image/png"  // register PNG decoder for image.DecodeConfig
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	_ "golang.org/x/image/webp" // register WebP decoder for image.DecodeConfig
	"gopkg.in/yaml.v3"
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
	// goldmark (safe mode) has already stripped raw HTML; the cast is valid.
	return htmltpl.HTML(out), nil
}

var allowedImageExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true,
}

// maxImageDim is the largest allowed width or height (px) for an overlay image.
// 512 accommodates SDXL-Turbo passenger portraits (generated at 512x512).
const maxImageDim = 512

// validateImage checks that name is a bare filename (no separators, no ".."),
// has an allowed raster extension, decodes as an image, and fits within
// maxImageDim on both sides. Returns the validated name, or an error describing
// why it was rejected. Empty name -> ("", nil).
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
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return "", fmt.Errorf("image %q not found in overlay dir: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return "", fmt.Errorf("image %q could not be decoded: %w", name, err)
	}
	if cfg.Width > maxImageDim || cfg.Height > maxImageDim {
		return "", fmt.Errorf("image %q is %dx%d px; max is %dx%d", name, cfg.Width, cfg.Height, maxImageDim, maxImageDim)
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
	head, tail, found := strings.Cut(rest, "\n---\n")
	if !found {
		// Allow a closing fence at EOF with no trailing newline.
		if strings.HasSuffix(rest, "\n---") {
			return []byte(rest[:len(rest)-len("\n---")]), ""
		}
		return nil, s
	}
	return []byte(head), tail
}

// overlaysRoot is the repo-root source directory holding contributor overlays.
const overlaysRoot = "overlays"

// copyOverlayImage copies name from srcDir to destDir. Empty name is a no-op.
// Failures are warnings (the page still renders without the image).
func copyOverlayImage(srcDir, name, destDir string) {
	if name == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(srcDir, name))
	if err != nil {
		log.Printf("warning: read overlay image %s: %v", name, err)
		return
	}
	if err := os.WriteFile(filepath.Join(destDir, name), data, 0o644); err != nil {
		log.Printf("warning: write overlay image %s: %v", name, err)
	}
}

// attachFactionOverlays loads overlays/factions/<id>/ for each faction and warns
// about overlay dirs that match no current faction.
func attachFactionOverlays(factions []*Faction, root string) {
	entityIDs := make(map[string]bool, len(factions))
	for _, f := range factions {
		entityIDs[f.ID] = true
		ov, err := loadOverlay(filepath.Join(root, "factions", f.ID))
		if err != nil {
			log.Printf("warning: faction overlay %s: %v", f.ID, err)
			continue
		}
		f.Overlay = ov
	}
	warnOrphanOverlays(filepath.Join(root, "factions"), entityIDs)
}

// attachPlayerOverlays loads overlays/players/<id>/ for each player and warns
// about overlay dirs that match no current player.
func attachPlayerOverlays(players []*Player, root string) {
	entityIDs := make(map[string]bool, len(players))
	for _, p := range players {
		entityIDs[p.ID] = true
		ov, err := loadOverlay(filepath.Join(root, "players", p.ID))
		if err != nil {
			log.Printf("warning: player overlay %s: %v", p.ID, err)
			continue
		}
		p.Overlay = ov
	}
	warnOrphanOverlays(filepath.Join(root, "players"), entityIDs)
}

// warnOrphanOverlays logs any subdirectory of dir whose name was not matched.
func warnOrphanOverlays(dir string, matched map[string]bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // no overlays of this kind yet
	}
	for _, e := range entries {
		if e.IsDir() && !matched[e.Name()] {
			log.Printf("warning: overlay %s matches no current entity", filepath.Join(dir, e.Name()))
		}
	}
}

// profileFront is the YAML frontmatter shape of a profile.md.
type profileFront struct {
	Image    string        `yaml:"image"`
	ImageAlt string        `yaml:"image_alt"`
	Stats    []OverlayStat `yaml:"stats"`
}

// loadOverlay reads <dir>/profile.md and returns the parsed Overlay. Returns
// (nil, nil) when no profile.md exists. A bad/missing image is a warning (the
// image is dropped, the rest still renders); malformed YAML or markdown is an
// error.
func loadOverlay(dir string) (*Overlay, error) {
	content, err := os.ReadFile(filepath.Join(dir, "profile.md"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	front, body := splitFrontmatter(content)
	var fm profileFront
	if len(front) > 0 {
		if err := yaml.Unmarshal(front, &fm); err != nil {
			return nil, fmt.Errorf("frontmatter: %w", err)
		}
	}
	ov := &Overlay{ImageAlt: fm.ImageAlt, Stats: fm.Stats}
	if img, err := validateImage(dir, fm.Image); err != nil {
		log.Printf("warning: overlay %s: %v (image skipped)", dir, err)
	} else {
		ov.ImageFile = img
	}
	if strings.TrimSpace(body) != "" {
		html, err := renderMarkdown(body)
		if err != nil {
			return nil, fmt.Errorf("markdown: %w", err)
		}
		ov.BodyHTML = html
	}
	return ov, nil
}
