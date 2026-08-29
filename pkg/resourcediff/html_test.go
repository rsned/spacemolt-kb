package resourcediff

import (
	"testing"
)

// fixturePage mirrors the structure that cmd/generate-items-kb/resources.go
// emits: summary cards, then one section per resource type. Two discovered
// types (one with a hidden POI, one with a low tick rendered as "-") and one
// undiscovered type.
const fixturePage = `<!DOCTYPE html>
<html lang="en">
<body>
    <main class="container page-content">
        <div class="summary-cards">
            <div class="summary-card">
                <div class="num">3</div>
                <div class="label">Resource Types</div>
            </div>
            <div class="summary-card">
                <div class="num">3</div>
                <div class="label">Total Deposits</div>
            </div>
        </div>

        <div class="summary-cards">
            <div class="summary-card">
                <div class="num">505</div>
                <div class="label">Star Systems</div>
            </div>
            <div class="summary-card">
                <div class="num">500</div>
                <div class="label">Systems Explored</div>
            </div>
            <div class="summary-card">
                <div class="num">99.0%</div>
                <div class="label">Galaxy Explored</div>
            </div>
        </div>
        <div id="adamantite-ore" class="resource-section">
            <h3>Adamantite Ore <span class="badge" style="font-size:0.7em; vertical-align:middle;">2 deposits</span> <small style="font-size:0.8em; font-weight:normal;"><a href="../items/ore/adamantite_ore.html">Details</a></small> <a href="#" class="back-top">[top]</a></h3>
            <table class="sortable">
                <thead>
                    <tr><th>System</th></tr>
                </thead>
                <tbody>
                    <tr>
                        <td><a href="../systems/embervale/index.html">Embervale</a></td>
                        <td><code>embervale</code></td>
                        <td><span class="text-muted">—</span></td>
                        <td>Adamantite Core</td>
                        <td><code>adamantite_core</code></td>
                        <td><span class="badge badge-yellow">Yes</span></td>
                        <td><code>adamantite_ore</code></td>
                        <td>15</td>
                        <td>500</td>
                        <td>300</td>
                        <td><span class="depletion medium">40%</span></td>
                        <td>603343</td>
                    </tr>
                    <tr>
                        <td><a href="../systems/sol/index.html">Sol</a></td>
                        <td><code>sol</code></td>
                        <td><span class="badge badge-green">✓</span></td>
                        <td>Ol&#39; Belt</td>
                        <td><code>sol_belt</code></td>
                        <td><span class="text-muted">—</span></td>
                        <td><code>adamantite_ore</code></td>
                        <td>8</td>
                        <td>500</td>
                        <td>1,250</td>
                        <td><span class="depletion low">0%</span></td>
                        <td>-</td>
                    </tr>
                </tbody>
            </table>
        </div>
        <div id="void-crystal" class="resource-section" hidden>
            <h3>Void Crystal <span class="badge" style="font-size:0.7em; vertical-align:middle;">Undiscovered</span> <small style="font-size:0.8em; font-weight:normal;"><a href="../items/material/void_crystal.html">Details</a></small> <a href="#" class="back-top">[top]</a></h3>
            <div class="undiscovered">
                <h4>Not Yet Discovered</h4>
            </div>
        </div>
        <div id="zinc-ore" class="resource-section" hidden>
            <h3>Zinc Ore <span class="badge" style="font-size:0.7em; vertical-align:middle;">1 deposits</span> <small style="font-size:0.8em; font-weight:normal;"><a href="../items/ore/zinc_ore.html">Details</a></small> <a href="#" class="back-top">[top]</a></h3>
            <table class="sortable">
                <tbody>
                    <tr>
                        <td><a href="../systems/haven/index.html">Haven</a></td>
                        <td><code>haven</code></td>
                        <td><span class="badge badge-green">✓</span></td>
                        <td>Zinc Field</td>
                        <td><code>haven_zinc</code></td>
                        <td><span class="text-muted">—</span></td>
                        <td><code>zinc_ore</code></td>
                        <td>3</td>
                        <td class="cap">1,200</td>
                        <td>90</td>
                        <td><span class="depletion low">0%</span></td>
                        <td>4</td>
                        <td>610000</td>
                    </tr>
                </tbody>
            </table>
        </div>
    </main>
</body>
</html>
`

func TestFromHTML(t *testing.T) {
	snap, err := FromHTML([]byte(fixturePage))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Source != SourceHTML {
		t.Errorf("source = %q, want %q", snap.Source, SourceHTML)
	}
	want := Summary{Types: 3, Deposits: 3, Systems: 505, Explored: 500}
	if snap.Summary != want {
		t.Errorf("summary = %+v, want %+v", snap.Summary, want)
	}
	if len(snap.Types) != 3 {
		t.Fatalf("types = %d, want 3", len(snap.Types))
	}
	// Types are sorted by ID.
	if snap.Types[0].ID != "adamantite_ore" || snap.Types[0].Category != "ore" || snap.Types[0].Name != "Adamantite Ore" {
		t.Errorf("types[0] = %+v", snap.Types[0])
	}
	if snap.Types[1].ID != "void_crystal" || snap.Types[1].Category != "material" {
		t.Errorf("types[1] = %+v", snap.Types[1])
	}
	if len(snap.Deposits) != 3 {
		t.Fatalf("deposits = %d, want 3", len(snap.Deposits))
	}
	// Deposits are sorted by (resource, system, poi).
	d := snap.Deposits[0]
	if d.ResourceID != "adamantite_ore" || d.SystemID != "embervale" || d.SystemName != "Embervale" ||
		d.POIID != "adamantite_core" || d.POIName != "Adamantite Core" || !d.Hidden || d.Station ||
		d.Richness != 15 || d.Remaining != 300 || d.LastTick != 603343 {
		t.Errorf("deposits[0] = %+v", d)
	}
	d = snap.Deposits[1]
	if d.SystemID != "sol" || d.POIName != "Ol' Belt" || d.Hidden || !d.Station ||
		d.Richness != 8 || d.Remaining != 1250 || d.LastTick != 0 {
		t.Errorf("deposits[1] = %+v", d)
	}
	d = snap.Deposits[2]
	if d.ResourceID != "zinc_ore" || d.MaxRemaining != 1200 || d.Remaining != 90 || d.LastTick != 610000 {
		t.Errorf("deposits[2] = %+v", d)
	}
	// Legacy 12-column rows carry the estimate, not a capacity.
	if snap.Deposits[0].MaxRemaining != 0 {
		t.Errorf("legacy row should have unknown capacity: %+v", snap.Deposits[0])
	}
}

func TestFromHTML_RejectsUnrecognisedPage(t *testing.T) {
	if _, err := FromHTML([]byte("<html><body>nothing here</body></html>")); err == nil {
		t.Fatal("want error for a page with no resource sections")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	snap, err := FromHTML([]byte(fixturePage))
	if err != nil {
		t.Fatal(err)
	}
	snap.Date = "2026-08-29"
	snap.ServerVersion = "0.566.2"
	data, err := snap.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if back.Date != snap.Date || back.ServerVersion != snap.ServerVersion || back.Summary != snap.Summary {
		t.Errorf("header mismatch: %+v vs %+v", back, snap)
	}
	if len(back.Deposits) != len(snap.Deposits) || back.Deposits[1] != snap.Deposits[1] {
		t.Errorf("deposits mismatch")
	}
	if len(back.Types) != len(snap.Types) {
		t.Errorf("types mismatch")
	}
}
