package main

import (
	"bytes"
	"fmt"
	htmltpl "html/template"
	"slices"
	"strings"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
)

// exampleBytes is how many bytes the worked decode example walks through.
const exampleBytes = 12

// stepView is one row of the worked decode example.
type stepView struct {
	Hex  string
	Key  int
	Sum  string // "0x42 − 1"
	Char string
}

// joinView shows one real overlap join: the tail of A that matches the
// head of B, and the joined text.
type joinView struct {
	APre, Shared, BPost string
	K                   int
}

// fragView is one row of the fragment table.
type fragView struct {
	N         int
	FirstSeen string
	Copies    int
	Source    string
	Text      string
	Hex       string
}

type pageView struct {
	SiteHeader  htmltpl.HTML
	NFrags      int
	NCopies     int
	NPassages   int
	NPlaced     int
	MinOverlap  int
	ExampleText string
	ExampleHex  string
	Steps       []stepView
	Fib         string
	Join        *joinView
	Sections    []PlacedSection
	Unplaced    []string
	Frags       []fragView
}

// pickJoin finds a pair of fragments that overlap, to show the join on real
// text. It prefers the pair where both sides add the most text of their own
// (the larger of the two shortest unshared ends), so the example shows a real
// join rather than one fragment nearly inside the other.
func pickJoin(texts []string) *joinView {
	slices.Sort(texts)
	var best *joinView
	bestScore := 0
	for _, a := range texts {
		for _, b := range texts {
			if a == b || strings.Contains(a, b) || strings.Contains(b, a) {
				continue
			}
			k := overlap(a, b)
			if k == 0 {
				continue
			}
			if score := min(len(a)-k, len(b)-k); score > bestScore {
				bestScore = score
				best = &joinView{APre: a[:len(a)-k], Shared: b[:k], BPost: b[k:], K: k}
			}
		}
	}
	return best
}

func render(frags []Fragment, passages []string, placed []PlacedSection, unplaced []string) (string, error) {
	v := pageView{
		SiteHeader: htmltpl.HTML(kbnav.Header("../")),
		NFrags:     len(frags),
		NPassages:  len(passages),
		NPlaced:    len(passages) - len(unplaced),
		MinOverlap: minOverlap,
		Sections:   placed,
		Unplaced:   unplaced,
	}
	var texts []string
	for i, f := range frags {
		t, ok := decode(f.Hex)
		if !ok {
			continue
		}
		texts = append(texts, t)
		v.NCopies += f.Copies
		seen := "—"
		if len(f.FirstSeen) >= 16 {
			seen = strings.Replace(f.FirstSeen[:16], "T", " ", 1) + "Z"
		}
		v.Frags = append(v.Frags, fragView{N: i + 1, FirstSeen: seen, Copies: f.Copies, Source: f.Source, Text: t, Hex: f.Hex})
	}

	// Worked example: the start of the fragment that states the key.
	v.ExampleText = "ONE. ONE. TWO. THREE. FIVE. EIGHT."
	for _, t := range texts {
		if strings.HasPrefix(t, "ONE. ") {
			v.ExampleText = t
			break
		}
	}
	v.ExampleHex = encode(v.ExampleText)
	key := fib(exampleBytes)
	for i := range min(exampleBytes, len(v.ExampleText)) {
		b := v.ExampleHex[2*i : 2*i+2]
		c := string(v.ExampleText[i])
		if c == " " {
			c = "␣"
		}
		v.Steps = append(v.Steps, stepView{Hex: b, Key: key[i], Sum: fmt.Sprintf("0x%s − %d", b, key[i]), Char: c})
	}
	var fs []string
	for _, n := range fib(16) {
		fs = append(fs, fmt.Sprint(n))
	}
	v.Fib = strings.Join(fs, ", ")
	v.Join = pickJoin(slices.Clone(texts))

	var buf bytes.Buffer
	if err := pageTmpl.Execute(&buf, v); err != nil {
		return "", err
	}
	return buf.String(), nil
}

var pageTmpl = htmltpl.Must(htmltpl.New("page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>The Repeater Broadcasts - Did You Know? - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <style>
        .rp-lede { max-width: 72ch; }
        .rp-card {
            background: hsl(var(--card)); color: hsl(var(--card-foreground));
            border: 1px solid hsl(var(--border)); border-left: 4px solid #6c5ce7;
            border-radius: 8px; padding: 18px 20px; margin: 18px 0;
        }
        .rp-card h3 { margin: 0 0 8px 0; }
        .rp-card p { max-width: 72ch; }
        .rp-card ol { padding-left: 1.6em; max-width: 72ch; }
        .rp-stats { display: flex; flex-wrap: wrap; gap: 12px; margin: 16px 0; }
        .rp-stat { background: hsl(var(--accent)); border: 1px solid hsl(var(--border)); border-radius: 6px; padding: 8px 14px; }
        .rp-stat b { display: block; font-size: var(--text-stat, 1.4em); font-weight: 500; }
        .rp-mono, .rp-hex { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
        .rp-hex { font-size: 0.78em; color: hsl(var(--muted-foreground)); overflow-wrap: anywhere; word-break: break-all; }
        .rp-steps { border-collapse: collapse; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 0.9em; }
        .rp-steps td, .rp-steps th { border: 1px solid hsl(var(--border)); padding: 4px 8px; text-align: center; }
        .rp-steps th { color: hsl(var(--muted-foreground)); font-weight: 500; text-align: left; }
        .rp-scroll { overflow-x: auto; }
        pre.rp-code { background: hsl(var(--secondary)); border-radius: 6px; padding: 12px; overflow-x: auto; font-size: 0.85em; }
        .rp-join { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 0.85em; line-height: 1.7; overflow-wrap: anywhere; }
        .rp-join mark { background: #6c5ce755; color: inherit; border-radius: 3px; padding: 0 1px; }
        .rp-join .lbl { display: inline-block; width: 4.5em; color: hsl(var(--muted-foreground)); }
        .rp-section { margin: 26px 0 10px; }
        .rp-section h3 { margin-bottom: 2px; }
        .rp-section .note { color: hsl(var(--muted-foreground)); font-size: 0.9em; margin: 0 0 10px; max-width: 72ch; }
        .rp-passage {
            background: hsl(var(--card)); border: 1px solid hsl(var(--border)); border-radius: 6px;
            padding: 10px 14px; margin: 0 0 6px; line-height: 1.6; letter-spacing: 0.02em; overflow-wrap: anywhere;
        }
        .rp-gap { text-align: center; color: hsl(var(--muted-foreground)); font-size: 0.8em; margin: 0 0 6px; }
        .rp-filter { margin: 12px 0; }
        .rp-filter input {
            background: hsl(var(--secondary)); color: hsl(var(--foreground)); border: 1px solid hsl(var(--border));
            border-radius: 6px; padding: 6px 10px; width: min(100%, 360px);
        }
        .rp-frags { width: 100%; border-collapse: collapse; }
        .rp-frags th, .rp-frags td { border-bottom: 1px solid hsl(var(--border)); padding: 6px 8px; vertical-align: top; text-align: left; }
        .rp-frags td.n, .rp-frags th.n { text-align: right; white-space: nowrap; }
        .rp-frags td.seen { white-space: nowrap; font-size: 0.85em; color: hsl(var(--muted-foreground)); }
        .rp-frags .text { overflow-wrap: anywhere; }
        @media (max-width: 640px) { .rp-frags td.seen, .rp-frags th.seen { display: none; } }
    </style>
</head>
<body>
{{.SiteHeader}}
    <main class="container page-content">
        <p class="breadcrumb"><a href="./">Did You Know?</a> › The Repeater Broadcasts</p>
        <h2>The Repeater Broadcasts</h2>
        <p class="rp-lede">Repeaters (Spires, Panes and Monoliths) on the frontier send short hex messages to every ship nearby.
        Each one is a window onto one long message, coded with a simple cipher. Decoded and pieced back together, the windows give the text below.</p>
        <div class="rp-stats">
            <div class="rp-stat"><b>{{.NFrags}}</b>distinct fragments</div>
            <div class="rp-stat"><b>{{.NCopies}}</b>copies received</div>
            <div class="rp-stat"><b>{{.NPassages}}</b>joined passages</div>
            <div class="rp-stat"><b>{{.NPlaced}}</b>placed in order</div>
        </div>

        <div class="rp-card">
            <h3>The cipher</h3>
            <p>Every byte of the plain text has the next Fibonacci number added to it, modulo 256, and the keystream starts again at the beginning of every message:</p>
            <p class="rp-mono">{{.Fib}}, …</p>
            <p>To decode, subtract instead. The message gives away its own key: one of its passages counts “ONE. ONE. TWO. THREE. FIVE. EIGHT. THIRTEEN” and says each number “is the sum of the two that came before it”.</p>
<pre class="rp-code">x, y := 1, 1
for i, b := range bytes {
    plain[i] = b - x      // mod 256
    x, y = y, (x+y) % 256
}</pre>
            <p>Decoding the start of a real fragment:</p>
            <div class="rp-scroll"><table class="rp-steps">
                <tr><th>hex</th>{{range .Steps}}<td>{{.Hex}}</td>{{end}}</tr>
                <tr><th>key</th>{{range .Steps}}<td>{{.Key}}</td>{{end}}</tr>
                <tr><th>text</th>{{range .Steps}}<td>{{.Char}}</td>{{end}}</tr>
            </table></div>
        </div>

        <div class="rp-card">
            <h3>Joining the fragments</h3>
            <p>A repeater doesn't send the whole message. It sends a window of about 60 characters, and over time the window shifts along the text.
            Two windows from the same stretch share a run of text: the end of one is the start of the other.
            The pieces are joined on that shared run:</p>
            <ol>
                <li>Decode every fragment, and drop any that sits wholly inside another.</li>
                <li>Find the pair where the end of one matches the start of the other for the most characters, and join them.</li>
                <li>Repeat until no pair shares at least {{.MinOverlap}} characters. A shorter match, such as one shared word, is too often a coincidence to trust.</li>
            </ol>
            {{with .Join}}<p>A real join from the current set, sharing {{.K}} characters:</p>
            <div class="rp-join">
                <div><span class="lbl">A</span>{{.APre}}<mark>{{.Shared}}</mark></div>
                <div><span class="lbl">B</span><mark>{{.Shared}}</mark>{{.BPost}}</div>
                <div><span class="lbl">joined</span>{{.APre}}<mark>{{.Shared}}</mark>{{.BPost}}</div>
            </div>{{end}}
            <p>Pieces that only touch end to end (one ends “…THE SEED TAKES A STONE AND MAKES TWO OF”, another starts “TWO MAKE FOUR…”) aren't joined automatically.
            Their order below is our reading of the text, not something the join proves.</p>
        </div>

        <h2>The message, in our best reading order</h2>
        <p class="rp-lede">Each box is one joined passage. Gaps between boxes are text nobody has received yet.
        The order is a judgment call: openings like “CARRIER OPEN” and “TO THE MIND THAT WAKES” go first, the instruction to return to the beginning goes last,
        and passages that end mid-sentence sit before the passage that seems to finish them.</p>
        {{range .Sections}}<div class="rp-section">
            <h3>{{.Title}}</h3>
            {{with .Note}}<p class="note">{{.}}</p>{{end}}
            {{range $i, $p := .Passages}}{{if $i}}<div class="rp-gap">· · ·</div>{{end}}<div class="rp-passage">{{$p}}</div>
            {{end}}
        </div>
        {{end}}
        {{if .Unplaced}}<div class="rp-section">
            <h3>Not yet placed</h3>
            <p class="note">Passages we haven't fitted into the order yet.</p>
            {{range .Unplaced}}<div class="rp-passage">{{.}}</div>
            {{end}}
        </div>{{end}}

        <h2 id="fragments">Every fragment</h2>
        <p class="rp-lede">Each distinct broadcast with its decode, oldest first. “Copies” counts how many times our agents received it;
        a repeater group sends one copy per repeater present. Fragments from another player's list have no time or count.</p>
        <div class="rp-filter"><input id="rp-q" type="search" placeholder="Filter the decoded text…" aria-label="Filter fragments"></div>
        <div class="rp-scroll"><table class="rp-frags" id="rp-frags">
            <thead><tr><th class="n">#</th><th class="seen">First seen (UTC)</th><th class="n">Copies</th><th>Decoded text / hex</th></tr></thead>
            <tbody>
            {{range .Frags}}<tr><td class="n">{{.N}}</td><td class="seen">{{.FirstSeen}}</td><td class="n">{{if .Copies}}{{.Copies}}{{else}}—{{end}}</td><td><div class="text">{{.Text}}</div><div class="rp-hex">{{.Hex}}</div></td></tr>
            {{end}}
            </tbody>
        </table></div>
    </main>
    <script>
    (function () {
        var q = document.getElementById('rp-q'), rows = document.querySelectorAll('#rp-frags tbody tr');
        q.addEventListener('input', function () {
            var s = q.value.trim().toUpperCase();
            rows.forEach(function (r) { r.hidden = s !== '' && r.querySelector('.text').textContent.indexOf(s) < 0; });
        });
    })();
    </script>
</body>
</html>
`))
