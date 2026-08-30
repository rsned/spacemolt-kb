package wildlife

import (
	"regexp"
	"strings"
)

// LoreEntry is one species' field-guide prose: what changed from the
// terrestrial baseline, how it feeds, and how it defends itself.
type LoreEntry struct {
	Name    string
	Tags    []string // habitat · role · ranchable, as written in the doc
	Intro   string
	Codex   string // the official scan description quoted in the entry, if any
	Changed string
	Feeds   string
	Defends string
}

// Lore is the parsed roster, keyed by normalised name.
type Lore map[string]LoreEntry

// Lookup finds an entry by species name, ignoring case and punctuation.
func (l Lore) Lookup(name string) (LoreEntry, bool) {
	e, ok := l[normName(name)]
	return e, ok
}

var (
	reEntryHead = regexp.MustCompile(`^\*\*(.+?)\*\*\s*\*\((.*?)\)\*\s*$`)
	reBullet    = regexp.MustCompile(`^- \*\*(Changed|Feeds|Defends):\*\*\s*(.*)$`)
	reNonAlnum  = regexp.MustCompile(`[^a-z0-9]+`)
)

func normName(s string) string {
	return strings.Trim(reNonAlnum.ReplaceAllString(strings.ToLower(s), " "), " ")
}

// ParseLore reads the "Part 1" roster of the wildlife lore document. Each
// entry is a `**Name** *(tags)*` heading, an intro paragraph, and
// Changed/Feeds/Defends bullets; wrapped lines are joined. Later parts of
// the document (exotic-form hypotheses) are ignored.
func ParseLore(md []byte) Lore {
	lines := strings.Split(string(md), "\n")
	out := Lore{}
	inPart1 := false
	var cur *LoreEntry
	field := "" // "intro", "codex", "Changed", "Feeds", "Defends"
	flush := func() {
		if cur != nil {
			out[normName(cur.Name)] = *cur
		}
		cur, field = nil, ""
	}
	appendText := func(text string) {
		if cur == nil {
			return
		}
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		join := func(dst *string) {
			switch {
			case *dst == "":
				*dst = text
			case strings.HasSuffix(*dst, "-"):
				// A line broken at a hyphen ("light-\nfracturing") rejoins
				// without a space.
				*dst += text
			default:
				*dst += " " + text
			}
		}
		switch field {
		case "intro":
			join(&cur.Intro)
		case "codex":
			join(&cur.Codex)
		case "Changed":
			join(&cur.Changed)
		case "Feeds":
			join(&cur.Feeds)
		case "Defends":
			join(&cur.Defends)
		}
	}
	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		if strings.HasPrefix(line, "## ") {
			flush()
			inPart1 = strings.Contains(line, "Part 1")
			continue
		}
		if !inPart1 {
			continue
		}
		if strings.TrimSpace(line) == "---" {
			flush()
			continue
		}
		if m := reEntryHead.FindStringSubmatch(line); m != nil {
			flush()
			cur = &LoreEntry{Name: strings.TrimSpace(m[1])}
			for _, t := range strings.Split(m[2], "·") {
				if t = strings.TrimSpace(t); t != "" {
					cur.Tags = append(cur.Tags, t)
				}
			}
			field = "intro"
			continue
		}
		if cur == nil {
			continue
		}
		if m := reBullet.FindStringSubmatch(line); m != nil {
			field = m[1]
			appendText(m[2])
			continue
		}
		// An italic "*Codex (…): "…"*" block quotes the official scan entry;
		// it runs until the line that closes the quote and is kept apart from
		// the field notes.
		if strings.HasPrefix(strings.TrimSpace(line), "*Codex") {
			field = "codex"
			text := strings.TrimPrefix(strings.TrimSpace(line), "*")
			if i := strings.Index(text, "\""); i >= 0 {
				text = text[i:]
			}
			appendText(text)
			if strings.HasSuffix(strings.TrimSpace(line), "\"*") {
				cur.Codex = strings.Trim(strings.TrimSuffix(cur.Codex, "*"), "\" ")
				field = "intro"
			}
			continue
		}
		if field == "codex" {
			appendText(line)
			if strings.HasSuffix(strings.TrimSpace(line), "\"*") {
				cur.Codex = strings.Trim(strings.TrimSuffix(cur.Codex, "*"), "\" ")
				field = "intro"
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			// A blank line ends the entry's prose; bullets follow the intro
			// directly, so a blank line means the entry is complete.
			flush()
			continue
		}
		appendText(line)
	}
	flush()
	return out
}
