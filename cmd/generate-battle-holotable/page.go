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
    overflow: hidden;
  }
  header { padding: 10px 16px; border-bottom: 1px solid #123; }
  h1 { margin: 0; font-size: 15px; font-weight: 600; letter-spacing: .04em; }
  .meta { color: #4d7a8c; }
  #table { display: block; width: 100%; height: calc(100vh - 46px); }
  #status { padding: 16px; color: #c86; }
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
