// Command generate-repeater-messages writes the Did You Know page on the
// repeaters' coded broadcasts: how the Fibonacci cipher works, the decoded
// fragments joined into longer passages and laid out in our best reading
// order, and every distinct fragment with its decode.
//
// Fragments are kept in data/repeater/messages.json. Each run first merges
// in any new repeater mail from the agent mailboxes (when they are present),
// so the page can also be rebuilt from the committed file alone. Run from
// the kb repo root:
//
//	go run ./cmd/generate-repeater-messages
package main

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	_ "modernc.org/sqlite"
)

// Fragment is one distinct broadcast: the hex as sent and where we saw it.
type Fragment struct {
	Hex       string `json:"hex"`
	FirstSeen string `json:"first_seen,omitempty"` // UTC timestamp, from mailboxes
	Copies    int    `json:"copies"`               // mailbox copies across all agents
	Source    string `json:"source"`               // "mailbox", "player list" or both
}

func main() {
	dataPath := flag.String("data", "data/repeater/messages.json", "fragment store")
	orderPath := flag.String("order", "data/repeater/order.json", "reading order of the assembled passages")
	mboxGlob := flag.String("mbox", "../spacemolt/data/agents/*/mbox.db", "agent mailboxes to merge new repeater mail from")
	out := flag.String("out", "kb/did_you_know/repeater_messages.html", "output page")
	flag.Parse()

	frags, err := loadFragments(*dataPath)
	if err != nil {
		log.Fatal(err)
	}
	boxes, _ := filepath.Glob(*mboxGlob)
	if len(boxes) > 0 {
		mail, err := readMailboxes(boxes)
		if err != nil {
			log.Fatal(err)
		}
		before := len(frags)
		frags = mergeMail(frags, mail)
		log.Printf("mailboxes: %d boxes, %d distinct repeater messages; %d new", len(boxes), len(mail), len(frags)-before)
		if err := saveFragments(*dataPath, frags); err != nil {
			log.Fatal(err)
		}
	}

	var sections []Section
	data, err := os.ReadFile(*orderPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(data, &sections); err != nil {
		log.Fatalf("%s: %v", *orderPath, err)
	}

	var texts []string
	for _, f := range frags {
		if t, ok := decode(f.Hex); ok {
			texts = append(texts, t)
		}
	}
	passages := assemble(texts)
	placed, unplaced, stale := place(passages, sections)
	for _, a := range stale {
		log.Printf("order: anchor matches no passage: %q", a)
	}
	page, err := render(frags, passages, placed, unplaced)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, []byte(page), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d fragments -> %d passages (%d placed in %d sections, %d unplaced); wrote %s\n",
		len(frags), len(passages), len(passages)-len(unplaced), len(placed), len(unplaced), *out)
}

func loadFragments(path string) ([]Fragment, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f []Fragment
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

func saveFragments(path string, frags []Fragment) error {
	data, err := json.MarshalIndent(frags, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// mailItem is one distinct repeater message summed over mailboxes.
type mailItem struct {
	first  string
	copies int
}

// readMailboxes collects the coded repeater mail (sender repeater_npc,
// content all hex) from every mailbox. Plain-text repeater events are skipped.
func readMailboxes(paths []string) (map[string]mailItem, error) {
	out := map[string]mailItem{}
	for _, p := range paths {
		db, err := sql.Open("sqlite", "file:"+p+"?mode=ro")
		if err != nil {
			return nil, err
		}
		rows, err := db.Query(`SELECT content, MIN(timestamp_utc), COUNT(*) FROM messages
			WHERE sender_id = 'repeater_npc' GROUP BY content`)
		if err != nil {
			_ = db.Close()
			continue // a mailbox without the messages table
		}
		for rows.Next() {
			var content, first string
			var n int
			if err := rows.Scan(&content, &first, &n); err != nil {
				_ = rows.Close()
				_ = db.Close()
				return nil, err
			}
			h := strings.ToLower(strings.TrimSpace(content))
			if _, ok := decode(h); !ok {
				continue
			}
			m := out[h]
			if m.first == "" || first < m.first {
				m.first = first
			}
			m.copies += n
			out[h] = m
		}
		_ = rows.Close()
		_ = db.Close()
	}
	return out, nil
}

// mergeMail folds mailbox counts into the store: known fragments get their
// copies and first sighting updated, new ones are added. The store is kept
// sorted by first sighting (unknown last), then hex.
func mergeMail(frags []Fragment, mail map[string]mailItem) []Fragment {
	idx := map[string]int{}
	for i, f := range frags {
		idx[f.Hex] = i
	}
	for h, m := range mail {
		i, ok := idx[h]
		if !ok {
			frags = append(frags, Fragment{Hex: h, Source: "mailbox"})
			i = len(frags) - 1
		}
		f := &frags[i]
		if f.FirstSeen == "" || m.first < f.FirstSeen {
			f.FirstSeen = m.first
		}
		f.Copies = max(f.Copies, m.copies)
		if !strings.Contains(f.Source, "mailbox") {
			f.Source = "mailbox, " + f.Source
		}
	}
	slices.SortFunc(frags, func(a, b Fragment) int {
		if (a.FirstSeen == "") != (b.FirstSeen == "") {
			if a.FirstSeen == "" {
				return 1
			}
			return -1
		}
		return cmp.Or(cmp.Compare(a.FirstSeen, b.FirstSeen), cmp.Compare(a.Hex, b.Hex))
	})
	return frags
}
