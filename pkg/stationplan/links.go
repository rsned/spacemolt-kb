package stationplan

import (
	"regexp"
	"slices"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

type linkTarget struct{ name, href string }

// Linker resolves facility and item ids to their KB page links. It never
// invents a link: an id it does not know resolves to nothing, so a guide
// cannot ship a broken href.
type Linker struct {
	targets map[string]linkTarget
}

// NewLinker indexes facilities and items. prefix is the relative path from
// the page being rendered to the kb root ("../" for kb/guides/). Facilities
// win when a facility and an item share an id.
func NewLinker(facs []catalog.Facility, items []catalog.Item, prefix string) *Linker {
	l := &Linker{targets: make(map[string]linkTarget, len(facs)+len(items))}
	for _, it := range items {
		if it.Category != "" {
			l.targets[it.ID] = linkTarget{it.Name, prefix + "items/" + it.Category + "/" + it.ID + ".html"}
		}
	}
	for _, f := range facs {
		if f.Category != "" {
			l.targets[f.ID] = linkTarget{f.Name, prefix + "facilities/" + f.Category + "/" + f.ID + ".html"}
		}
	}
	return l
}

// Href returns the KB page for id, facility first, then item.
func (l *Linker) Href(id string) (string, bool) {
	t, ok := l.targets[id]
	return t.href, ok
}

// Name returns id's display name, or id itself when unknown or unnamed.
func (l *Linker) Name(id string) string {
	if t, ok := l.targets[id]; ok && t.name != "" {
		return t.name
	}
	return id
}

var refPattern = regexp.MustCompile(`\[\[([a-z0-9_]+)\]\]`)

// ExpandRefs rewrites each [[id]] in markdown to a [Name](href) link, or to
// the bare id when it resolves to nothing. It returns the unresolved ids,
// de-duplicated in order of first appearance.
func (l *Linker) ExpandRefs(md string) (string, []string) {
	var unresolved []string
	out := refPattern.ReplaceAllStringFunc(md, func(m string) string {
		id := refPattern.FindStringSubmatch(m)[1]
		if href, ok := l.Href(id); ok {
			return "[" + l.Name(id) + "](" + href + ")"
		}
		if !slices.Contains(unresolved, id) {
			unresolved = append(unresolved, id)
		}
		return id
	})
	return out, unresolved
}
