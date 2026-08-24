package main

// filterScript drives the client-side filter bar shared by both comparison
// pages. Rows carry their filterable values in data- attributes so filtering
// never has to parse rendered text, and it composes with the existing sort
// script (which reorders the same <tr> nodes and ignores display state).
//
// The CPU/power budget inputs are the fitting-specific part: enter what the
// hull has free and the table hides what will not fit.
const filterScript = `    <script>
    (function () {
      var bar = document.querySelector(".module-filters");
      if (!bar) return;
      var table = document.querySelector("table.sortable");
      var rows = Array.from(table.querySelectorAll("tbody tr"));
      var count = document.querySelector(".filter-count");

      function activeChips(group) {
        return Array.from(bar.querySelectorAll('.chip[data-group="' + group + '"].on'))
                    .map(function (c) { return c.dataset.value; });
      }

      function apply() {
        var q = (bar.querySelector(".filter-search").value || "").toLowerCase().trim();
        var dts = activeChips("dtype");
        var ammo = bar.querySelector(".filter-ammo");
        var ammoVal = ammo ? ammo.value : "";
        var maxCPU = parseFloat(bar.querySelector(".filter-cpu").value);
        var maxPwr = parseFloat(bar.querySelector(".filter-power").value);
        var shown = 0;
        rows.forEach(function (tr) {
          var ok = true;
          if (q && tr.dataset.search.indexOf(q) === -1) ok = false;
          if (ok && dts.length) {
            // A weapon matches one damage type; a defense row matches any type
            // it actually resists.
            var have = (tr.dataset.dtype || "").split(" ").filter(Boolean);
            ok = dts.some(function (d) { return have.indexOf(d) !== -1; });
          }
          if (ok && ammoVal && tr.dataset.ammo !== ammoVal) ok = false;
          if (ok && !isNaN(maxCPU) && parseFloat(tr.dataset.cpu) > maxCPU) ok = false;
          if (ok && !isNaN(maxPwr) && parseFloat(tr.dataset.power) > maxPwr) ok = false;
          tr.style.display = ok ? "" : "none";
          if (ok) shown++;
        });
        if (count) count.textContent = shown + " of " + rows.length + " shown";
      }

      bar.querySelectorAll(".chip").forEach(function (c) {
        c.addEventListener("click", function () { c.classList.toggle("on"); apply(); });
      });
      bar.querySelectorAll("input, select").forEach(function (el) {
        el.addEventListener("input", apply);
        el.addEventListener("change", apply);
      });
      var reset = bar.querySelector(".filter-reset");
      if (reset) {
        reset.addEventListener("click", function () {
          bar.querySelectorAll(".chip.on").forEach(function (c) { c.classList.remove("on"); });
          bar.querySelectorAll("input").forEach(function (el) { el.value = ""; });
          bar.querySelectorAll("select").forEach(function (el) { el.selectedIndex = 0; });
          apply();
        });
      }
      apply();
    })();
    </script>`

// moduleFilterCSS styles the filter bar and the damage-type badges. Inlined
// rather than added to items.css so the comparison pages are self-contained.
const moduleFilterCSS = `    <style>
      /* smui.css exposes its palette as bare HSL triplets, so every colour
         here must go through hsl(var(--x)) — a bare var() resolves to the
         triplet and silently invalidates the declaration. */
      .module-filters { display:flex; flex-wrap:wrap; gap:16px; align-items:flex-end;
        margin:14px 0 10px; padding:12px 14px;
        background:hsl(var(--card)); border:1px solid hsl(var(--border)); }
      .module-filters .fgroup { display:flex; flex-direction:column; gap:6px; }
      .module-filters label { font-size:var(--text-label,12px); text-transform:uppercase;
        letter-spacing:.06em; color:hsl(var(--muted-foreground)); }
      .module-filters input, .module-filters select {
        background:hsl(var(--background)); color:hsl(var(--foreground));
        border:1px solid hsl(var(--border)); padding:5px 8px;
        font-family:var(--font-sans); font-size:var(--text-ui,13px); }
      .module-filters .filter-search { min-width:210px; }
      .module-filters .filter-cpu, .module-filters .filter-power { width:88px; }
      .filter-reset { cursor:pointer; background:hsl(var(--secondary));
        color:hsl(var(--secondary-foreground)); border:1px solid hsl(var(--border));
        padding:6px 12px; font-family:var(--font-sans); font-size:var(--text-ui,13px); }
      .filter-reset:hover { border-color:hsl(var(--smui-border-hover)); }
      .chips { display:flex; flex-wrap:wrap; gap:6px; }
      .chip { cursor:pointer; user-select:none; font-size:var(--text-label,12px);
        padding:4px 10px; border:1px solid hsl(var(--border));
        background:hsl(var(--background)); color:hsl(var(--muted-foreground)); }
      .chip:hover { border-color:hsl(var(--smui-border-hover)); color:hsl(var(--foreground)); }
      .chip.on { background:hsl(var(--primary)); border-color:hsl(var(--primary));
        color:hsl(var(--primary-foreground)); }
      .filter-count { font-size:var(--text-label,12px);
        color:hsl(var(--muted-foreground)); margin-left:auto; align-self:center; }
      /* Damage-type badges. Fixed colours rather than theme vars: the type is
         the same fact in either theme, and these stay legible on both. */
      .dt { font-size:11px; padding:2px 8px; white-space:nowrap; display:inline-block; }
      .dt-kinetic   { background:#4b5563; color:#f3f4f6; }
      .dt-thermal   { background:#9a3412; color:#ffedd5; }
      .dt-energy    { background:#1d4ed8; color:#dbeafe; }
      .dt-em        { background:#6d28d9; color:#ede9fe; }
      .dt-explosive { background:#b91c1c; color:#fee2e2; }
      .dt-void      { background:#1f2937; color:#c7d2fe; border:1px solid #4f46e5; }
      .specials { font-size:var(--text-label,12px);
        color:hsl(var(--muted-foreground)); max-width:280px; }
      .mod-table { font-size:var(--text-ui,13px); }
      .mod-table td.num, .mod-table th.num { text-align:right; }
      .mod-table .res0 { color:hsl(var(--muted-foreground)); opacity:.45; }
      /* Adaptive resistance is real coverage but different in kind from a fixed
         hardener, so it reads as a number you can sort on while still being
         visually distinguishable at a glance. */
      .mod-table td.adaptive, .adaptive-key { font-style:italic; opacity:.92; }
      .mod-table td.adaptive::after, .adaptive-key::after { content:"\00a0a"; font-size:9px;
        vertical-align:super; font-style:normal; opacity:.7; }
    </style>`

var weaponAllTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>All Weapons - Items - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../items/items.css">
` + moduleFilterCSS + `
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
      <div class="breadcrumb"><a href="../">Items</a> / <a href="./">Weapon</a> / All Weapons</div>
      <h2>All Weapons</h2>
      <p class="text-muted mt-1">All {{.Total}} weapon modules in one table, for fitting comparison across families.
        Click any column header to sort &mdash; click again to reverse.
        <a href="../defense/all.html">Defense modules &rarr;</a></p>

      <div class="module-filters">
        <div class="fgroup"><label>Search</label>
          <input type="text" class="filter-search" placeholder="name or special&hellip;"></div>
        <div class="fgroup"><label>Damage type</label>
          <div class="chips">
{{- range damageTypes}}
            <span class="chip" data-group="dtype" data-value="{{.}}">{{dtLabel .}}</span>
{{- end}}
          </div></div>
        <div class="fgroup"><label>Ammo</label>
          <select class="filter-ammo">
            <option value="">any</option>
{{- range .AmmoTypes}}
            <option value="{{.}}">{{titleCase .}}</option>
{{- end}}
          </select></div>
        <div class="fgroup"><label>Max CPU</label>
          <input type="number" class="filter-cpu" min="0" placeholder="free"></div>
        <div class="fgroup"><label>Max power</label>
          <input type="number" class="filter-power" min="0" placeholder="free"></div>
        <div class="fgroup"><label>&nbsp;</label>
          <button class="filter-reset">Reset</button></div>
        <span class="filter-count"></span>
      </div>

      <div class="card" style="padding:0; overflow-x:auto">
        <table class="sortable mod-table">
          <thead>
            <tr>
              <th class="sortable">Name</th>
              <th class="sortable">Dmg type</th>
              <th class="sortable num" title="Base damage per shot">Dmg</th>
              <th class="sortable num" title="Cooldown in ticks between shots">CD</th>
              <th class="sortable num" title="Damage per tick: damage divided by cooldown">DPT</th>
              <th class="sortable num" title="Effective DPT range across every round that fits this weapon">Eff DPT</th>
              <th class="sortable num" title="How far it can attack from">Reach</th>
              <th class="sortable num">CPU</th>
              <th class="sortable num">Power</th>
              <th class="sortable num" title="Damage per tick per point of CPU">DPT/CPU</th>
              <th class="sortable num" title="Damage per tick per point of power">DPT/Pwr</th>
              <th class="sortable">Ammo</th>
              <th class="sortable num" title="Magazine size">Mag</th>
              <th class="sortable num" title="Total damage in one magazine: damage x magazine size">Volley</th>
              <th class="sortable">Skill req</th>
              <th class="sortable num">Value</th>
              <th>Special</th>
            </tr>
          </thead>
          <tbody>
{{- range .Rows}}
            <tr data-search="{{lower .Name}} {{lower (join .Specials " ")}}"
                data-dtype="{{.DamageType}}" data-ammo="{{.AmmoType}}"
                data-cpu="{{.CPUUsage}}" data-power="{{.PowerUsage}}">
              <td><a href="{{.ID}}.html">{{.Name}}</a></td>
              <td data-sort="{{.DamageType}}"><span class="dt {{dtClass .DamageType}}">{{dtLabel .DamageType}}</span></td>
              <td class="num">{{.Damage}}</td>
              <td class="num">{{.Cooldown}}</td>
              <td class="num" data-sort="{{.DPT}}"><b>{{num1 .DPT}}</b></td>
              <td class="num" data-sort="{{.EffMaxDPT}}">{{effRange .}}</td>
              <td class="num">{{.Reach}}</td>
              <td class="num">{{.CPUUsage}}</td>
              <td class="num">{{.PowerUsage}}</td>
              <td class="num" data-sort="{{.DPTPerCPU}}">{{num2 .DPTPerCPU}}</td>
              <td class="num" data-sort="{{.DPTPerPwr}}">{{num2 .DPTPerPwr}}</td>
              <td>{{strOrDash (ammoLabel .AmmoType)}}</td>
              <td class="num">{{intOrDash .MagazineSize}}</td>
              <td class="num" data-sort="{{.Volley}}">{{intOrDash .Volley}}</td>
              <td>{{strOrDash .SkillReq}}</td>
              <td class="num value" data-sort="{{.BaseValue}}">{{fmtValue .BaseValue}}</td>
              <td class="specials">{{join .Specials " · "}}</td>
            </tr>
{{- end}}
          </tbody>
        </table>
      </div>
      <p class="text-muted mt-2" style="font-size:12px">
        <b>DPT</b> is damage &divide; cooldown ticks. <b>Eff DPT</b> spans the
        weakest to strongest round that fits the weapon's ammo type (ammo
        <code>damage_mod</code> runs &minus;50% to +100%), so a low-DPT launcher
        with good ammo can out-damage a high-DPT one with none.
        <b>DPT/CPU</b> and <b>DPT/Pwr</b> are what matter once the hull's
        budget, not the slot count, is the binding constraint.
      </p>
    </main>
` + sortScript + filterScript + themeScript + `
</body>
</html>
`

var defenseAllTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>All Defense Modules - Items - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../items/items.css">
` + moduleFilterCSS + `
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
      <div class="breadcrumb"><a href="../">Items</a> / <a href="./">Defense</a> / All Defense</div>
      <h2>All Defense Modules</h2>
      <p class="text-muted mt-1">All {{.Total}} defense modules in one table.
        The resistance columns use the same damage types as the weapons page, so the two read against each other.
        <a href="../weapon/all.html">Weapons &rarr;</a></p>

      <div class="module-filters">
        <div class="fgroup"><label>Search</label>
          <input type="text" class="filter-search" placeholder="name or special&hellip;"></div>
        <div class="fgroup"><label>Resists</label>
          <div class="chips">
{{- range .DamageTypes}}
            <span class="chip" data-group="dtype" data-value="{{.}}">{{dtLabel .}}</span>
{{- end}}
          </div></div>
        <div class="fgroup"><label>Max CPU</label>
          <input type="number" class="filter-cpu" min="0" placeholder="free"></div>
        <div class="fgroup"><label>Max power</label>
          <input type="number" class="filter-power" min="0" placeholder="free"></div>
        <div class="fgroup"><label>&nbsp;</label>
          <button class="filter-reset">Reset</button></div>
        <span class="filter-count"></span>
      </div>

      <div class="card" style="padding:0; overflow-x:auto">
        <table class="sortable mod-table">
          <thead>
            <tr>
              <th class="sortable">Name</th>
              <th class="sortable num">CPU</th>
              <th class="sortable num">Power</th>
              <th class="sortable num">Shield</th>
              <th class="sortable num" title="Shield recharge per tick">Shd regen</th>
              <th class="sortable num">Armor</th>
              <th class="sortable num" title="Armor repaired per tick">Arm repair</th>
              <th class="sortable num">Hull</th>
              <th class="sortable num" title="Flat damage reduction">Dmg red</th>
{{- range .DamageTypes}}
              <th class="sortable num" title="Resistance to {{.}} damage">{{dtLabel .}}</th>
{{- end}}
              <th class="sortable">Penalties</th>
              <th class="sortable">Skill req</th>
              <th class="sortable num">Value</th>
              <th>Special</th>
            </tr>
          </thead>
          <tbody>
{{- range .Rows}}
            <tr data-search="{{lower .Name}} {{lower (join .Specials " ")}}"
                data-dtype="{{resistedTypes .}}"
                data-cpu="{{.CPUUsage}}" data-power="{{.PowerUsage}}">
              <td><a href="{{.ID}}.html">{{.Name}}</a></td>
              <td class="num">{{.CPUUsage}}</td>
              <td class="num">{{.PowerUsage}}</td>
              <td class="num">{{intOrDash .ShieldBonus}}</td>
              <td class="num">{{intOrDash .ShieldRechargeBonus}}</td>
              <td class="num">{{intOrDash .ArmorBonus}}</td>
              <td class="num">{{intOrDash .ArmorRepairRate}}</td>
              <td class="num">{{intOrDash .HullBonus}}</td>
              <td class="num" data-sort="{{.DamageReduction}}">{{pctOrDash .DamageReduction}}</td>
{{- range .Resists}}
              <td class="num{{if not .Value}} res0{{end}}{{if .Adaptive}} adaptive{{end}}"
                  data-sort="{{pctSort .}}"
                  {{if .Adaptive}}title="Adaptive: applies to every damage type, ramping as it learns what is hitting you"{{end}}>{{strOrDash .Value}}</td>
{{- end}}
              <td>{{strOrDash (join .Penalties ", ")}}</td>
              <td>{{strOrDash .SkillReq}}</td>
              <td class="num value" data-sort="{{.BaseValue}}">{{fmtValue .BaseValue}}</td>
              <td class="specials">{{join .Specials " · "}}</td>
            </tr>
{{- end}}
          </tbody>
        </table>
      </div>
      <p class="text-muted mt-2" style="font-size:12px">
        Resistance columns are per damage type &mdash; match them against the damage type
        that is actually killing you on the <a href="../weapon/all.html">weapons page</a>.
        Values in <span class="adaptive-key">this style</span> come from an
        <b>adaptive</b> module: it carries no fixed per-type resistance, but applies
        that percentage against <i>every</i> damage type, ramping as it learns what is
        hitting you. So an Adaptive Shield III (35% across the board) trades raw
        per-type depth for coverage against a hardener&rsquo;s 30% in one.
        Penalties are real: several high-bulk modules cost speed, which is why
        they are a first-class column rather than buried in the description.
      </p>
    </main>
` + sortScript + filterScript + themeScript + `
</body>
</html>
`
