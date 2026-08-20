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
  /* The flex column is header / stage / transport / status. Nothing carries a
     "100vh minus the header's height" magic number, which drifts every time
     the header's own CSS changes. */
  #stage { flex: 1; display: flex; min-height: 0; }
  #table { display: block; flex: 1; min-width: 0; }
  #rail {
    width: 300px; flex: none; overflow-y: auto;
    border-left: 1px solid #123; padding: 8px 10px;
  }
  #chatter { margin: 0; padding: 0; list-style: none; }
  #chatter li { margin: 0 0 10px; }
  #chatter .tickno { color: #4d7a8c; }
  #chatter .count  { color: #7fb3c8; }
  #chatter .move   { color: #9fd4e8; }
  #chatter .kill   { color: #e08a6a; }
  #transport {
    display: flex; align-items: center; gap: 10px;
    padding: 8px 16px; border-top: 1px solid #123;
  }
  #transport button {
    background: #0b131c; color: #9fd4e8; border: 1px solid #1d3644;
    font: inherit; padding: 3px 10px; cursor: pointer;
  }
  #transport button:hover { background: #12212e; }
  #transport select {
    background: #0b131c; color: #9fd4e8; border: 1px solid #1d3644; font: inherit;
  }
  #scrub { flex: 1; min-width: 80px; }
  /* order: -1 keeps a fetch failure visible above the header, in the flex
     flow rather than fixed/absolute, so it shrinks the stage instead of
     overflowing the viewport — see initHolotable's catch in
     holotable-player.js. :not(:empty) means the div carries no padding (and
     so no height) while it has no text, so a successful load never shows a
     blank strip. */
  #status { color: #c86; order: -1; }
  #status:not(:empty) { padding: 16px; }
  /* Narrow viewports put the rail under the table rather than squeezing both. */
  @media (max-width: 900px) {
    #stage { flex-direction: column; }
    #table { min-height: 0; }
    #rail {
      width: auto; height: 180px;
      border-left: none; border-top: 1px solid #123;
    }
  }
</style>
</head>
<body>
<header>
  <h1>{{.SystemName}}</h1>
  <div class="meta">battle {{.BattleID}} &middot; {{.TickCount}} ticks &middot; tick <span id="tick">&mdash;</span></div>
</header>
<div id="stage">
  <canvas id="table"></canvas>
  <aside id="rail"><ol id="chatter"></ol></aside>
</div>
<div id="transport">
  <button id="stepBack" type="button" title="Step back (left arrow)">&#9664;&#9664;</button>
  <button id="playPause" type="button" title="Play / pause (space)">&#9654;</button>
  <button id="stepFwd" type="button" title="Step forward (right arrow)">&#9654;&#9654;</button>
  <input id="scrub" type="range" min="0" max="0" step="1" value="0" aria-label="Scrub through the battle">
  <label>speed <select id="speed"></select></label>
  <span id="readout" class="meta">&mdash;</span>
</div>
<div id="status"></div>
<script>
  window.HOLOTABLE = {
    replayURL: {{.ReplayURL}},
    hullsURL: {{.HullsURL}},
  };
</script>
<script src="holotable.js"></script>
<script src="holotable-player.js"></script>
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
