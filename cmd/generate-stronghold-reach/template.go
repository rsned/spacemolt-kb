package main

const pageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Reach of the Nine Strongholds - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../items/items.css">
    <style>
        #reach-map { background:#0a0e1a; border:1px solid var(--border); border-radius:8px; overflow:hidden; }
        #reach-map .galaxy-map-svg { width:100%; height:auto; display:block; }
        .reach-controls { display:flex; align-items:center; gap:12px; margin:16px 0 8px; flex-wrap:wrap; }
        .reach-controls input[type=range] { flex:1; min-width:240px; }
        .reach-controls button { padding:4px 12px; cursor:pointer; }
        .reach-readout { font-weight:bold; font-size:1.05em; margin:0 0 12px; }
        .stat-hero { background:var(--bg-card); border:1px solid var(--border); border-left:4px solid #ff9500;
                     border-radius:8px; padding:20px; margin:20px 0; }
        .stat-hero .value { font-size:2em; font-weight:bold; color:#ff9500; }
        table.reach td.event { white-space:nowrap; }
        table.reach .tag-merge { color:#ff9500; font-weight:bold; }
        table.reach .tag-empire { display:inline-block; margin-left:6px; padding:1px 7px; border-radius:10px;
                                  background:rgba(255,149,0,0.12); border:1px solid rgba(255,149,0,0.35);
                                  font-size:0.85em; white-space:nowrap; }
        .holdouts { background:var(--bg-card); border:1px solid var(--border); border-left:4px solid #ff9500;
                    border-radius:8px; padding:14px 18px; margin:12px 0 4px; }
        .holdouts strong { color:#ff9500; }
{{.ReachCSS}}
    </style>
</head>
<body>
    <header class="site-header">
        <h1><a href="../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>
            <a href="../">Home</a>
            <a href="../systems/index.html">Systems</a>
            <a href="../items/index.html">Items</a>
            <a href="../recipes/index.html">Recipes</a>
            <a href="../skills/index.html">Skills</a>
            <a href="../ships/index.html">Ships</a>
            <a href="../facilities/index.html">Facilities</a>
            <a href="../resources/index.html">Resources</a>
            <a href="../missions/index.html">Missions</a>
            <a href="./">Did You Know?</a>
            <button class="theme-toggle" id="theme-toggle" aria-label="Toggle theme">
                <svg class="icon-sun" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
                <svg class="icon-moon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
            </button>
        </nav>
    </header>
    <main class="container page-content">
        <h2>Reach of the Nine Strongholds</h2>
        <p class="text-muted mt-1">Nobody in this galaxy lives very far from a pirate stronghold. A multi-source breadth-first search over the jump-gate network measures every system's distance to the nearest of the nine, then grows that reach one jump at a time.</p>

        <div class="stat-hero">
            <div class="value">{{.MaxRadius}} jumps</div>
            <div class="label">is all it takes to reach {{if .UnreachableCount}}{{.ReachableCount}} of the {{.TotalSystems}} known systems{{else}}every one of the {{.TotalSystems}} known systems{{end}} from the nearest stronghold</div>
        </div>

        <div class="holdouts">
            <strong>The last holdouts:</strong> {{.FarthestCount}} systems sit a full {{.MaxRadius}} jumps out — the deepest anyone in this galaxy gets from pirate territory. They are <strong>{{.FarthestNames}}</strong>.
        </div>

        <div class="reach-controls">
            <button id="prev" type="button">&larr;</button>
            <input type="range" id="radius" min="1" max="{{.MaxRadius}}" value="{{.DefaultRadius}}" step="1">
            <button id="next" type="button">&rarr;</button>
        </div>
        <p class="reach-readout" id="readout"></p>

        <div id="reach-map" data-r="{{.DefaultRadius}}">{{.MapSVG}}</div>
        <p class="text-muted mt-1">The five colored territories are the empires, each one a single unbroken run of systems. The pale wash spreading out of the orange stronghold dots is pirate reach at the selected number of jumps — drag the slider and watch it creep across empire space. Systems still out of reach keep their own color; once the wash covers them they turn white.</p>

        <h3 class="mt-3">Coverage at Every Radius</h3>
        <table class="reach">
            <thead><tr><th>Jumps</th><th>Systems in reach</th><th>% of galaxy</th><th>Separate blobs</th><th>Events</th></tr></thead>
            <tbody>
            {{range .Rows}}
                <tr>
                    <td>&le;{{.Radius}}</td>
                    <td>{{.Systems}}</td>
                    <td>{{printf "%.1f" .Percent}}%</td>
                    <td>{{.Blobs}}</td>
                    <td class="event">{{if .Merged}}<span class="tag-merge">merge</span>{{end}}{{range index $.EmpireArrivals .Radius}}<span class="tag-empire" title="first reached via {{.Via}}">{{.Empire}} reached</span>{{end}}</td>
                </tr>
            {{end}}
            </tbody>
        </table>

        <h3 class="mt-3">Which Stronghold Is Nearest?</h3>
        <p class="text-muted">Assigning every system to the stronghold it can reach in the fewest jumps carves the galaxy into nine territories.</p>
        <table class="reach">
            <thead><tr><th>Stronghold</th><th>Systems nearest to it</th></tr></thead>
            <tbody>
            {{range .Territory}}
                <tr><td><a href="../systems/{{.SystemID}}/">{{.Name}}</a></td><td>{{.Systems}}</td></tr>
            {{end}}
            </tbody>
        </table>

        <h3 class="mt-3">Analysis Notes</h3>
        <ul>
            <li><strong>Nine blobs become one.</strong> Each patch of reach always contains a stronghold, so the count can only fall: {{.MergeStory}}.</li>
            <li><strong>{{.TopTerritory}} dominates.</strong> It is the nearest stronghold for {{.TopTerritoryCount}} systems, far more than any other.</li>
            <li><strong>Empire territory is not far behind.</strong> The Events column marks the radius at which reach first touches each of the five empires — hover a tag for the border system it arrives through.</li>
            <li><strong>Jump gates only — wormholes are not counted.</strong> Every distance here is a count of jump-gate traversals. The galaxy's permanent wormhole fixtures are shortcuts that sidestep that network entirely, and they are not part of this calculation: a system shown {{.MaxRadius}} jumps out may in practice be a single wormhole transit from pirate space. Treat these numbers as the distance the gate network imposes, not the shortest possible trip.</li>
            {{if .UnreachableCount}}<li><strong>{{.UnreachableCount}} systems have no route to any stronghold</strong> and are drawn permanently dim.</li>{{end}}
        </ul>

        <h3 class="mt-3">Data Source</h3>
        <p class="text-muted">System positions, jump-gate connections, and stronghold flags come from the knowledge database ({{.TotalSystems}} systems, {{.EdgeCount}} jump gates). The nine strongholds are the systems flagged <code>is_stronghold</code>; all nine are neutral, with no empire and no police presence. Distance is the minimum number of jump-gate traversals to the nearest stronghold via a multi-source breadth-first search.</p>

        <p class="text-muted mt-3"><a href="./">&larr; Back to Did You Know?</a></p>
    </main>
    <script>
    (function() {
        var toggle = document.getElementById('theme-toggle');
        var root = document.documentElement;
        var stored = localStorage.getItem('theme');
        if (stored === 'dark') root.classList.add('dark');
        toggle.addEventListener('click', function() {
            root.classList.toggle('dark');
            localStorage.setItem('theme', root.classList.contains('dark') ? 'dark' : 'light');
        });
    })();
    (function() {
        var stats = {{.StatsJSON}};
        var map = document.getElementById('reach-map');
        var slider = document.getElementById('radius');
        var readout = document.getElementById('readout');
        function apply() {
            var r = parseInt(slider.value, 10);
            map.setAttribute('data-r', r);
            var s = stats[r];
            if (!s) { return; }
            readout.textContent = '≤' + r + ' jumps · ' + s.systems +
                ' systems · ' + s.percent + '% of the galaxy · ' +
                s.blobs + (s.blobs === 1 ? ' single blob' : ' separate blobs');
        }
        slider.addEventListener('input', apply);
        document.getElementById('prev').addEventListener('click', function() {
            slider.value = Math.max(parseInt(slider.min, 10), parseInt(slider.value, 10) - 1);
            apply();
        });
        document.getElementById('next').addEventListener('click', function() {
            slider.value = Math.min(parseInt(slider.max, 10), parseInt(slider.value, 10) + 1);
            apply();
        });
        apply();
    })();
    </script>
</body>
</html>
`
