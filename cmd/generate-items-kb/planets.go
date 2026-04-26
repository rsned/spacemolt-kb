package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	htmltpl "html/template"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/kbdb"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/stats"
)

// writePlanetPages generates surface map images and detail pages for all planets.
// explorerNotesFile matches the JSON structure from generate-explorer-notes.
type explorerNotesFile struct {
	Explorer string `json:"explorer"`
	Notes    []struct {
		POIID string `json:"poi_id"`
		Note  string `json:"note"`
	} `json:"notes"`
}

func loadExplorerNotes(path string) (map[string]string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("No explorer notes file found at %s", path)
		return nil, ""
	}
	var nf explorerNotesFile
	if err := json.Unmarshal(data, &nf); err != nil {
		log.Printf("WARNING: could not parse explorer notes: %v", err)
		return nil, ""
	}
	notes := make(map[string]string, len(nf.Notes))
	for _, n := range nf.Notes {
		notes[n.POIID] = n.Note
	}
	log.Printf("Loaded %d explorer notes", len(notes))
	return notes, nf.Explorer
}

func writePlanetPages(db *sql.DB, outDir string, systems []*System) error {
	tmpl := htmltpl.Must(htmltpl.New("planet").Parse(planetDetailTemplate))

	// Load explorer notes
	explorerNotes, explorerName := loadExplorerNotes("data/explorer-notes.json")

	var generated, persisted int
	for _, sys := range systems {
		for _, poi := range sys.POIs {
			if poi.Type != "planet" {
				continue
			}

			planetClass := poi.Class
			if planetClass == "" {
				planetClass = "unknown"
			}

			sanitizedSys := sanitizeName(sys.ID)
			sanitizedPlanet := sanitizeName(poi.Name)
			imgFilename := fmt.Sprintf("%s_%s.png", sanitizedSys, sanitizedPlanet)

			// Generate stats and persist to metadata table.
			orbitalDist := math.Hypot(poi.PositionX, poi.PositionY)
			ps := stats.GenerateStats(planetClass, poi.Name, orbitalDist)

			meta := kbdb.FromPlanetStats(poi.ID, poi.Name, ps)
			if err := kbdb.UpsertPlanet(db, meta); err != nil {
				log.Printf("WARNING: failed to persist planet metadata for %s: %v", poi.Name, err)
			} else {
				persisted++
			}

			// Determine ring config for template
			hasRings := planetClass == "jovian" || planetClass == "ice_giant"

			// Write detail page
			sysDir := filepath.Join(outDir, sys.ID)
			if err := os.MkdirAll(sysDir, 0o755); err != nil {
				return err
			}

			pageFilename := fmt.Sprintf("planet_%s.html", sanitizedPlanet)
			pagePath := filepath.Join(sysDir, pageFilename)

			data := planetPageData{
				PlanetName:   poi.Name,
				PlanetClass:  planetClass,
				SystemName:   sys.Name,
				SystemID:     sys.ID,
				Description:  poi.Description,
				Stats:        ps,
				ImagePath:    "../../images/planets/" + imgFilename,
				HasRings:     hasRings,
				RingType:     planetClass,
				Resources:    poi.Resources,
				ExplorerNote: explorerNotes[poi.ID],
				ExplorerName: explorerName,
			}

			f, err := os.Create(pagePath)
			if err != nil {
				return err
			}
			if err := tmpl.Execute(f, data); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}

			generated++
			if generated%100 == 0 {
				log.Printf("  Generated %d planet pages...", generated)
			}
		}
	}

	log.Printf("Planet pages: %d generated, %d persisted", generated, persisted)
	return nil
}

// persistStarMetadata iterates all sun POIs and persists their derived metadata.
func persistStarMetadata(db *sql.DB, systems []*System) error {
	var persisted int
	for _, sys := range systems {
		for _, poi := range sys.POIs {
			if poi.Type != "sun" {
				continue
			}
			if poi.Class == "" {
				continue
			}
			meta, err := kbdb.FromStarClass(poi.ID, poi.Name, poi.Class)
			if err != nil {
				log.Printf("WARNING: failed to parse star class for %s (%s): %v", poi.Name, poi.Class, err)
				continue
			}
			if err := kbdb.UpsertStar(db, *meta); err != nil {
				log.Printf("WARNING: failed to persist star metadata for %s: %v", poi.Name, err)
				continue
			}
			persisted++
		}
	}
	log.Printf("Star metadata: %d persisted", persisted)
	return nil
}

func sanitizeName(name string) string {
	r := strings.NewReplacer(" ", "_", "'", "", "/", "_", "\\", "_")
	return strings.ToLower(r.Replace(name))
}

type planetPageData struct {
	PlanetName   string
	PlanetClass  string
	SystemName   string
	SystemID     string
	Description  string
	Stats        stats.PlanetStats
	ImagePath    string
	HasRings     bool
	RingType     string
	Resources    []POIResource
	ExplorerNote string
	ExplorerName string
}

var planetDetailTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.PlanetName}} - {{.SystemName}} - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../system.css">
    <link rel="stylesheet" href="../../items/items.css">
    <style>
        .planet-sphere { display: flex; justify-content: center; margin: 20px 0; }
        .planet-sphere canvas { cursor: grab; border-radius: 8px; }
        .planet-sphere canvas:active { cursor: grabbing; }
        .sphere-hint { text-align: center; color: var(--text-muted); font-size: 0.8em; margin-top: 4px; }
        .stats-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px 24px; padding: 16px; }
        .stat-item { display: flex; flex-direction: column; }
        .stat-item .label { font-size: 0.75em; text-transform: uppercase; color: var(--text-muted); margin-bottom: 2px; }
        .stat-item .value { font-size: 1.05em; }
        .planet-header { display: flex; align-items: center; gap: 12px; margin-bottom: 8px; }
        .planet-header .badge-type { padding: 3px 10px; border-radius: 4px; font-size: 0.8em; background: var(--bg-card); border: 1px solid var(--border); }
        .explorer-note { border-left: 3px solid #6a8a5a; padding: 12px 16px; margin: 16px 0; background: var(--bg-card); border-radius: 0 6px 6px 0; font-style: italic; line-height: 1.6; }
        .explorer-note .attribution { font-style: normal; font-size: 0.8em; color: var(--text-muted); margin-top: 8px; }
    </style>
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Systems</a> / <a href="./">{{.SystemName}}</a> / {{.PlanetName}}</div>

        <div class="planet-header">
            <h2>{{.PlanetName}}</h2>
            <span class="badge-type">{{.Stats.TypeName}}</span>
        </div>

{{- if .Description}}
        <blockquote class="item-desc">{{.Description}}</blockquote>
{{- else}}
        <blockquote class="item-desc" style="opacity:0.5">No survey data available. This world remains unexplored.</blockquote>
{{- end}}

{{- if .ExplorerNote}}
        <div class="explorer-note">
            {{.ExplorerNote}}
            <div class="attribution">&mdash; {{.ExplorerName}}, Explorer's Guild Field Journal</div>
        </div>
{{- end}}

        <div class="planet-sphere">
            <div>
                <canvas id="sphere-canvas" width="400" height="400"></canvas>
                <div class="sphere-hint">Click and drag to rotate</div>
            </div>
        </div>

        <div class="card" style="padding:0">
            <div class="section-label">Physical Properties</div>
            <div class="stats-grid">
                <div class="stat-item">
                    <span class="label">Radius</span>
                    <span class="value">{{.Stats.FormatRadius}}</span>
                </div>
                <div class="stat-item">
                    <span class="label">Mass</span>
                    <span class="value">{{.Stats.FormatMass}}</span>
                </div>
                <div class="stat-item">
                    <span class="label">Surface Gravity</span>
                    <span class="value">{{.Stats.FormatGravity}}</span>
                </div>
                <div class="stat-item">
                    <span class="label">Temperature</span>
                    <span class="value">{{.Stats.FormatTemperature}}</span>
                </div>
                <div class="stat-item">
                    <span class="label">Atmosphere</span>
                    <span class="value">{{.Stats.FormatPressure}} — {{.Stats.AtmDescription}}</span>
                </div>
                <div class="stat-item">
                    <span class="label">Day Length</span>
                    <span class="value">{{.Stats.FormatDayLength}}</span>
                </div>
                <div class="stat-item">
                    <span class="label">Orbital Period</span>
                    <span class="value">{{.Stats.FormatOrbitalPeriod}}</span>
                </div>
                <div class="stat-item">
                    <span class="label">Orbital Distance</span>
                    <span class="value">{{.Stats.OrbitalDistanceAU}} AU</span>
                </div>
            </div>
        </div>

{{- if .Resources}}
        <div class="card mt-2" style="padding:0">
            <div class="section-label">Resources</div>
            <table>
                <thead><tr><th>Resource</th><th>Richness</th><th>Remaining</th></tr></thead>
                <tbody>
{{- range .Resources}}
                <tr>
                    <td>{{.ResourceName}}</td>
                    <td>{{printf "%.0f" .Richness}}</td>
                    <td>{{printf "%.0f" .Remaining}}</td>
                </tr>
{{- end}}
                </tbody>
            </table>
        </div>
{{- end}}
    </main>

<script>
(function() {
const canvas = document.getElementById('sphere-canvas');
const ctx = canvas.getContext('2d');
const W = canvas.width, H = canvas.height;
const CX = W/2, CY = H/2, RADIUS = {{if .HasRings}}160{{else}}180{{end}};

let textureData = null, texW = 0, texH = 0;
let rotY = 0.4, rotX = {{if .HasRings}}0.35{{else}}0.15{{end}};
let dragging = false, lastMX = 0, lastMY = 0;
let autoRotate = true;

const img = new Image();
img.crossOrigin = 'anonymous';
img.onload = function() {
    const tc = document.createElement('canvas');
    tc.width = img.width; tc.height = img.height;
    const tctx = tc.getContext('2d');
    tctx.drawImage(img, 0, 0);
    textureData = tctx.getImageData(0, 0, img.width, img.height).data;
    texW = img.width; texH = img.height;
};
img.src = '{{.ImagePath}}';

function sampleTex(u, v) {
    if (!textureData) return [80,80,80];
    let px = Math.floor(u * texW) % texW;
    let py = Math.floor(v * texH);
    if (px < 0) px += texW;
    py = Math.max(0, Math.min(texH-1, py));
    const i = (py * texW + px) * 4;
    return [textureData[i], textureData[i+1], textureData[i+2]];
}

{{if .HasRings}}
const ringCfg = {{if eq .RingType "jovian"}}{
    inner: 1.3*RADIUS, outer: 2.2*RADIUS,
    colors: [
        {pos:0,r:180,g:150,b:110,a:0},{pos:0.08,r:180,g:150,b:110,a:0.4},
        {pos:0.2,r:160,g:130,b:95,a:0.55},{pos:0.35,r:170,g:145,b:105,a:0.3},
        {pos:0.45,r:155,g:125,b:90,a:0.6},{pos:0.6,r:175,g:150,b:115,a:0.65},
        {pos:0.75,r:145,g:120,b:85,a:0.45},{pos:0.9,r:160,g:135,b:100,a:0.2},
        {pos:1,r:150,g:130,b:95,a:0}
    ]
}{{else}}{
    inner: 1.25*RADIUS, outer: 1.75*RADIUS,
    colors: [
        {pos:0,r:150,g:180,b:210,a:0},{pos:0.1,r:150,g:180,b:210,a:0.3},
        {pos:0.3,r:140,g:170,b:200,a:0.4},{pos:0.5,r:160,g:185,b:215,a:0.35},
        {pos:0.7,r:145,g:175,b:205,a:0.3},{pos:0.9,r:155,g:180,b:210,a:0.15},
        {pos:1,r:150,g:175,b:205,a:0}
    ]
}{{end}};

function sampleRing(colors, t) {
    t = Math.max(0, Math.min(1, t));
    for (let i=1; i<colors.length; i++) {
        if (t <= colors[i].pos) {
            const lt = (t-colors[i-1].pos)/(colors[i].pos-colors[i-1].pos);
            const a=colors[i-1], b=colors[i];
            return {r:a.r+(b.r-a.r)*lt, g:a.g+(b.g-a.g)*lt, b:a.b+(b.b-a.b)*lt, a:a.a+(b.a-a.a)*lt};
        }
    }
    return colors[colors.length-1];
}
{{end}}

function render() {
    const id = ctx.createImageData(W, H);
    const d = id.data;
    const cRX=Math.cos(rotX), sRX=Math.sin(rotX), cRY=Math.cos(rotY), sRY=Math.sin(rotY);
    const lx=-0.4, ly=0.5, lz=0.7, ll=Math.sqrt(lx*lx+ly*ly+lz*lz);
    {{if .HasRings}}const rNY=cRX, rNZ=-sRX;{{end}}

    for (let py=0; py<H; py++) {
        for (let px=0; px<W; px++) {
            const dx=px-CX, dy=py-CY, dSq=dx*dx+dy*dy;
            const idx=(py*W+px)*4;
            let pR=17, pG=17, pB=17, onP=false;

            if (dSq <= RADIUS*RADIUS) {
                onP = true;
                const nx=dx/RADIUS, ny=-dy/RADIUS, nz=Math.sqrt(Math.max(0,1-nx*nx-ny*ny));
                const y1=ny*cRX-nz*sRX, z1=ny*sRX+nz*cRX;
                const x2=nx*cRY+z1*sRY, z2=-nx*sRY+z1*cRY;
                const lat=Math.asin(Math.max(-1,Math.min(1,y1)));
                const lon=Math.atan2(x2,z2);
                const [tr,tg,tb] = sampleTex((lon/(2*Math.PI))+0.5, 0.5-lat/Math.PI);
                const dot=(nx*lx+(-ny)*ly+nz*lz)/ll;
                const lit=Math.max(0.08, dot*0.7+0.3);
                const rim=1-nz, rg=rim*rim*rim*0.3;
                pR=Math.min(255,Math.floor(tr*lit+rg*80));
                pG=Math.min(255,Math.floor(tg*lit+rg*120));
                pB=Math.min(255,Math.floor(tb*lit+rg*180));
            }

            {{if .HasRings}}
            let ringA=0, ringR=0, ringG=0, ringB=0, ringZ=0;
            if (Math.abs(rNZ) > 0.001) {
                const t = (dy*rNY)/rNZ;
                ringZ = t;
                const ix=dx, iy=-dy, iz=t;
                const dist3d = Math.sqrt(ix*ix+iy*iy+iz*iz);
                if (dist3d >= ringCfg.inner && dist3d <= ringCfg.outer) {
                    const rt = (dist3d-ringCfg.inner)/(ringCfg.outer-ringCfg.inner);
                    const rc = sampleRing(ringCfg.colors, rt);
                    const rLit = 0.6+0.4*Math.max(0,(ly*rNY+lz*rNZ)/ll);
                    ringR=rc.r*rLit; ringG=rc.g*rLit; ringB=rc.b*rLit; ringA=rc.a;
                    if (onP && t < 0) ringA=0;
                }
            }
            if (onP && ringA > 0) {
                if (ringZ > 0) { d[idx]=Math.floor(pR*(1-ringA)+ringR*ringA); d[idx+1]=Math.floor(pG*(1-ringA)+ringG*ringA); d[idx+2]=Math.floor(pB*(1-ringA)+ringB*ringA); }
                else { d[idx]=pR; d[idx+1]=pG; d[idx+2]=pB; }
            } else if (onP) { d[idx]=pR; d[idx+1]=pG; d[idx+2]=pB; }
            else if (ringA > 0) { d[idx]=Math.floor(17*(1-ringA)+ringR*ringA); d[idx+1]=Math.floor(17*(1-ringA)+ringG*ringA); d[idx+2]=Math.floor(17*(1-ringA)+ringB*ringA); }
            else { d[idx]=17; d[idx+1]=17; d[idx+2]=17; }
            {{else}}
            d[idx]=pR; d[idx+1]=pG; d[idx+2]=pB;
            {{end}}
            d[idx+3]=255;
        }
    }
    ctx.putImageData(id, 0, 0);
}

function animate() {
    if (autoRotate && !dragging) rotY += 0.003;
    render();
    requestAnimationFrame(animate);
}

canvas.addEventListener('mousedown', e => { dragging=true; autoRotate=false; lastMX=e.clientX; lastMY=e.clientY; });
window.addEventListener('mousemove', e => { if(!dragging)return; rotY+=(e.clientX-lastMX)*0.01; rotX+=(e.clientY-lastMY)*0.01; rotX=Math.max(-Math.PI/2,Math.min(Math.PI/2,rotX)); lastMX=e.clientX; lastMY=e.clientY; });
window.addEventListener('mouseup', () => { if(dragging){dragging=false; setTimeout(()=>{if(!dragging)autoRotate=true;},3000);} });
animate();
})();
</script>
` + themeScript + `
</body>
</html>
`
