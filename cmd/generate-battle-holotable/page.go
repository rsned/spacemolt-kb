package main

import (
	"bytes"
	"fmt"
	"html/template"
)

// pageTemplate is the whole page. It carries no ship data: the renderer fetches
// the replay and the hull pack beside it, which keeps holotable.js editable
// without regenerating anything.
const pageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Holotable — {{.SystemName}} ({{.BattleID}})</title>
<style>
  :root { color-scheme: dark; }
  body {
    margin: 0; background: #05080d; color: #9fd4e8;
    font: 13px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace;
    display: flex; flex-direction: column; height: 100vh; overflow: hidden;
  }
  header { padding: 10px 16px; border-bottom: 1px solid #123; }
  h1 { margin: 0; font-size: 15px; font-weight: 600; letter-spacing: .04em; }
  .meta { color: #4d7a8c; }
  /* #table takes whatever the flex column leaves it, instead of a fixed
     "100vh minus the header's height" guess that drifts every time the
     header's own CSS changes. */
  #table { display: block; width: 100%; flex: 1; min-height: 0; }
  /* order: -1 keeps a fetch failure visible above the header, in the flex
     flow rather than fixed/absolute, so it shrinks #table instead of
     overflowing the viewport — see initHolotable's catch in holotable.js.
     :not(:empty) means the div carries no padding (and so no height) while
     it has no text, so a successful load never shows a blank strip. */
  #status { color: #c86; order: -1; }
  #status:not(:empty) { padding: 16px; }
</style>
</head>
<body>
<header>
  <h1>{{.SystemName}}</h1>
  <div class="meta">battle {{.BattleID}} &middot; {{.TickCount}} ticks &middot; tick <span id="tick">—</span></div>
</header>
<canvas id="table"></canvas>
<div id="status"></div>
<script>
  window.HOLOTABLE = {
    replayURL: {{.ReplayURL}},
    hullsURL: {{.HullsURL}},
  };
</script>
<script src="holotable.js"></script>
</body>
</html>
`

// pageData is what the template renders against.
type pageData struct {
	BattleID   string
	SystemName string
	TickCount  int
	ReplayURL  string
	HullsURL   string
}

// RenderPage produces the holotable page for one battle. The page is thin by
// design: it names two data files and loads the shared renderer.
func RenderPage(rep Replay) ([]byte, error) {
	tmpl, err := template.New("holotable").Parse(pageTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	data := pageData{
		BattleID:   rep.BattleID,
		SystemName: rep.SystemName,
		TickCount:  rep.TickCount,
		ReplayURL:  rep.BattleID + ".json",
		HullsURL:   rep.BattleID + "-hulls.json",
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}
