// warp.js — dependency-free first-person "hyperspace warp" renderer for a
// Pathfinder Drive route. Flies a camera along origin->dest at the heading; real
// stars (from stars.json) stream past with parallax, colored by spectral class,
// over an ambient warp starfield. Page-agnostic: call playWarp(canvas, opts).
(function (global) {
  'use strict';

  // ---------- pure helpers (visual derivation, unit-testable) ----------

  function hashStr(s) {
    var h = 2166136261 >>> 0;
    for (var i = 0; i < s.length; i++) {
      h ^= s.charCodeAt(i);
      h = Math.imul(h, 16777619) >>> 0;
    }
    return h >>> 0;
  }

  // seededHeight: stable per-id vertical offset in [-1, 1] (the synthesized
  // depth dimension the 2-D galaxy lacks).
  function seededHeight(id) {
    return (hashStr(id) / 0xffffffff) * 2 - 1;
  }

  var CLUSTER_R = 55; // GU; multi-star cluster ring radius (inside the 100 GU bubble)

  // rosetteOffset: deterministic view-space [dperp, dz] for component i of n in
  // a multi-star cluster. When centered (a black hole headlines), component 0
  // sits at the center and the rest fan evenly on the ring; otherwise all fan on
  // the ring (so a binary reads as a symmetric pair). Seeded by id for variety.
  function rosetteOffset(id, i, n, centered) {
    if (centered && i === 0) return [0, 0];
    var ring = centered ? n - 1 : n;
    var k = centered ? i - 1 : i;
    var ang = (hashStr(id) % 360) * Math.PI / 180 + (k / ring) * 6.2832;
    return [CLUSTER_R * Math.cos(ang), CLUSTER_R * Math.sin(ang)];
  }

  var SPECTRAL = {
    O: '#9bb0ff', B: '#aabfff', A: '#cad8ff', F: '#f6f3ff',
    G: '#fff4e8', K: '#ffc07a', M: '#ff8a4d'
  };

  // classToColor: Morgan-Keenan class -> CSS color (blackbody-ish).
  function classToColor(cls) {
    if (!cls) return '#c8ccd8';
    if (cls === 'BH') return '#15102a';   // black hole
    if (cls.charAt(0) === 'D') return '#dfe9ff'; // white dwarf
    return SPECTRAL[cls.charAt(0)] || '#c8ccd8';
  }

  // centerColor: the star's hot-center dot. Hot stars (O/B/A and white dwarfs)
  // get a true white-hot core; cooler stars keep a warm tint so red/orange
  // giants don't wash out to yellow-white.
  function centerColor(cls) {
    if (!cls) return '#fff6ec';
    if (cls === 'BH') return '#000000';
    var c = cls.charAt(0);
    if (c === 'O' || c === 'B' || c === 'A' || c === 'D') return '#ffffff';
    if (c === 'F' || c === 'G') return '#fff2db';
    if (c === 'K') return '#ffcf95';
    if (c === 'M') return '#ffac74';
    return '#fff6ec';
  }

  // classToSize: luminosity-class size multiplier (V dwarf .. Ia supergiant).
  // Giant/supergiant tiers (I/II/III) are exaggerated (doubled) to dramatize
  // the size gap against dwarfs.
  function classToSize(cls) {
    if (!cls) return 1;
    if (cls === 'BH') return 2.4;
    if (cls.charAt(0) === 'D') return 0.6;
    var m = cls.match(/(Ia|Ib|III|II|IV|V|I)$/);
    var lum = m ? m[1] : 'V';
    if (lum === 'Ia' || lum === 'Ib' || lum === 'I') return 8;
    if (lum === 'II') return 6;
    if (lum === 'III') return 4;
    if (lum === 'IV') return 1.4;
    return 1;
  }

  // ---------- scene construction ----------

  var MARGIN = 100; // GU bubble radius (interruption test)

  function buildScene(origin, dest, stars, opt) {
    var dx = dest.x - origin.x, dy = dest.y - origin.y;
    var routeLen = Math.hypot(dx, dy);
    var ux = dx / routeLen, uy = dy / routeLen;

    var scene = [];
    var blockedAt = Infinity, blockedStar = null;
    for (var i = 0; i < stars.length; i++) {
      var s = stars[i];
      if (s.id === origin.id) continue;
      var rx = s.x - origin.x, ry = s.y - origin.y;
      var proj = rx * ux + ry * uy;        // along-track (GU)
      var perp = rx * uy - ry * ux;        // signed lateral (GU)
      var isDest = s.id === dest.id;
      // First system whose bubble the ray crosses before the destination.
      if (!isDest && proj > 0 && proj < routeLen && Math.abs(perp) <= MARGIN) {
        if (proj < blockedAt) { blockedAt = proj; blockedStar = s.id; }
      }
      scene.push({
        id: s.id, name: s.name, proj: proj, perp: perp,
        x: s.x, y: s.y,                       // world coords (for the top-down map)
        // The target sits on the route line (perp == 0); drop its z jitter too
        // so it arrives dead-center in the viewport.
        z: isDest ? 0 : seededHeight(s.id) * (opt.heightSpread || 600),
        color: classToColor(s.class), size: classToSize(s.class),
        center: centerColor(s.class),
        isDest: isDest, bh: s.class === 'BH',
        suns: (s.suns && s.suns.length > 1) ? s.suns : null  // multi-star cluster
      });
    }
    var endProj = isFinite(blockedAt) ? blockedAt : routeLen;
    return {
      ux: ux, uy: uy, routeLen: routeLen, stars: scene,
      endProj: endProj, blocked: isFinite(blockedAt),
      endStar: isFinite(blockedAt) ? blockedStar : dest.id
    };
  }

  // ambient warp starfield: faint filler that recycles as it passes, giving the
  // speed sensation the ~500 real stars are too sparse to provide alone.
  function makeAmbient(n, near, far, spread) {
    var a = [];
    for (var i = 0; i < n; i++) {
      a.push({
        perp: (Math.random() * 2 - 1) * spread,
        z: (Math.random() * 2 - 1) * spread,
        depth: near + Math.random() * (far - near)
      });
    }
    return a;
  }

  // ---------- player ----------

  function playWarp(canvas, opts) {
    opts = opts || {};
    var ctx = canvas.getContext('2d');
    var focal = opts.focal || 520;
    var near = opts.near || 8;
    var far = opts.far || 6000;
    var spread = opts.heightSpread || 600;  // synthesized vertical jitter (half the original for a flatter plane)
    var baseSpeed = opts.speed || 450;    // GU per second at 1x
    var speed = baseSpeed;                 // current speed (base * multiplier)
    var ambient = makeAmbient(opts.ambient || 320, near, far, spread * 2.2);

    var scene = buildScene(opts.origin, opts.dest, opts.stars, opts);
    var prev = {};                         // id -> last screen pos (for streaks)
    var t = 0, raf = null, last = 0, arrived = 0, running = false;

    function resize() {
      var dpr = Math.min(global.devicePixelRatio || 1, 2);
      var w = canvas.clientWidth || 640, h = canvas.clientHeight || 360;
      canvas.width = w * dpr; canvas.height = h * dpr;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    function W() { return canvas.clientWidth || 640; }
    function H() { return canvas.clientHeight || 360; }

    function project(perp, z, forward) {
      return { x: W() / 2 + focal * perp / forward, y: H() / 2 - focal * z / forward };
    }

    function drawStar(p, prevP, forward, color, size, glow, bh, center) {
      var sz = Math.max(0.8, size * focal / forward * 0.85);
      // brighter falloff: gentle curve holds luminosity far longer than linear.
      var alpha = Math.max(0, Math.min(1, 1.7 * Math.pow(1 - forward / far, 0.4)));
      if (bh) { drawBlackHole(p, sz, alpha); return; }
      ctx.fillStyle = color;
      ctx.strokeStyle = color;
      if (prevP) { // streak from last frame
        ctx.globalAlpha = alpha;
        ctx.lineWidth = Math.max(0.8, sz * 1.1);
        ctx.beginPath(); ctx.moveTo(prevP.x, prevP.y); ctx.lineTo(p.x, p.y); ctx.stroke();
      }
      // wide soft glow on every star: a radial gradient fading to nothing, so
      // the halo has no hard outer edge (which read as a second circle on big
      // giants). Additive 'lighter' is in effect.
      var glowR = sz * (glow ? 6 : 3.6);
      var grad = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, glowR);
      grad.addColorStop(0, color);
      grad.addColorStop(1, 'rgba(0,0,0,0)');
      ctx.fillStyle = grad;
      ctx.globalAlpha = alpha * (glow ? 0.9 : 0.7);
      ctx.beginPath(); ctx.arc(p.x, p.y, glowR, 0, 6.2832); ctx.fill();
      // colored core
      ctx.fillStyle = color;
      ctx.globalAlpha = Math.min(1, alpha);
      ctx.beginPath(); ctx.arc(p.x, p.y, sz, 0, 6.2832); ctx.fill();
      // hot center (white for hot stars, warm-tinted for cool ones)
      ctx.globalAlpha = Math.min(1, alpha);
      ctx.fillStyle = center || '#ffffff';
      ctx.beginPath(); ctx.arc(p.x, p.y, sz * 0.45, 0, 6.2832); ctx.fill();
      ctx.globalAlpha = 1;
    }

    // drawBlackHole renders the lone BH-class star: an actual dark event horizon
    // (source-over so it occludes the field) ringed by a bright accretion glow.
    function drawBlackHole(p, sz, alpha) {
      var r = sz * 1.6;
      // event horizon — a real dark disk that blocks what's behind it
      ctx.globalCompositeOperation = 'source-over';
      ctx.globalAlpha = Math.min(1, alpha * 1.2);
      ctx.fillStyle = '#000000';
      ctx.beginPath(); ctx.arc(p.x, p.y, r, 0, 6.2832); ctx.fill();
      // accretion ring + glow (additive)
      ctx.globalCompositeOperation = 'lighter';
      ctx.globalAlpha = alpha;
      ctx.lineWidth = Math.max(1, r * 0.45);
      ctx.strokeStyle = '#ffcf8a';
      ctx.beginPath(); ctx.arc(p.x, p.y, r * 1.5, 0, 6.2832); ctx.stroke();
      ctx.globalAlpha = alpha * 0.55;
      ctx.lineWidth = Math.max(1, r * 0.3);
      ctx.strokeStyle = '#9fc1ff';
      ctx.beginPath(); ctx.arc(p.x, p.y, r * 2.1, 0, 6.2832); ctx.stroke();
      ctx.globalAlpha = 1;
    }

    // drawBubble renders a star's 100 GU capture sphere as a translucent
    // light-gray shell: a faint fill plus a brighter rim so it reads as glass.
    function drawBubble(p, forward) {
      var r = focal * MARGIN / forward;
      if (r < 2) return;
      var fade = Math.max(0, Math.min(1, 1 - forward / far));
      ctx.fillStyle = 'rgba(208,213,226,' + (0.08 * fade) + ')';
      ctx.beginPath(); ctx.arc(p.x, p.y, r, 0, 6.2832); ctx.fill();
      ctx.strokeStyle = 'rgba(216,221,234,' + (0.293 * fade) + ')';
      ctx.lineWidth = 1;
      ctx.beginPath(); ctx.arc(p.x, p.y, r, 0, 6.2832); ctx.stroke();
    }

    // ---------- top-down picture-in-picture minimaps ----------

    function roundRect(x, y, w, h, rad) {
      ctx.beginPath();
      ctx.moveTo(x + rad, y);
      ctx.arcTo(x + w, y, x + w, y + h, rad);
      ctx.arcTo(x + w, y + h, x, y + h, rad);
      ctx.arcTo(x, y + h, x, y, rad);
      ctx.arcTo(x, y, x + w, y, rad);
      ctx.closePath();
    }

    // panelRect: a square inset 1/3 the canvas height in a bottom corner.
    function panelRect(corner) {
      var s = Math.round(H() / 3), pad = 12;
      var x = corner === 'right' ? W() - s - pad : pad;
      return { x: x, y: H() - s - pad, s: s };
    }

    // drawPanel: dark rounded backdrop, runs body() clipped inside, then frames
    // it and writes the caption on top.
    function drawPanel(r, title, body) {
      ctx.globalCompositeOperation = 'source-over';
      ctx.globalAlpha = 1;
      ctx.fillStyle = 'rgba(6,9,18,0.80)';
      roundRect(r.x, r.y, r.s, r.s, 6); ctx.fill();
      ctx.save();
      ctx.beginPath(); ctx.rect(r.x, r.y, r.s, r.s); ctx.clip();
      body(r);
      ctx.restore();
      ctx.strokeStyle = 'rgba(150,162,196,0.55)'; ctx.lineWidth = 1;
      roundRect(r.x, r.y, r.s, r.s, 6); ctx.stroke();
      ctx.fillStyle = 'rgba(190,200,224,0.85)'; ctx.font = '10px monospace';
      ctx.fillText(title, r.x + 8, r.y + 14);
    }

    // shipMarker: small filled triangle pointing along the unit direction (dx, dy).
    function shipMarker(cx, cy, dx, dy, a) {
      var nx = -dy, ny = dx; // perpendicular to travel
      ctx.fillStyle = '#ffffff';
      ctx.beginPath();
      ctx.moveTo(cx + dx * a, cy + dy * a);
      ctx.lineTo(cx - dx * a * 0.7 + nx * a * 0.7, cy - dy * a * 0.7 + ny * a * 0.7);
      ctx.lineTo(cx - dx * a * 0.7 - nx * a * 0.7, cy - dy * a * 0.7 - ny * a * 0.7);
      ctx.closePath(); ctx.fill();
    }

    // fullRouteBody: a galaxy-map-oriented overview of the whole route (equal
    // scale, +X right and +Y down to match the galaxy map). The route is drawn
    // at its true bearing through the local star field; a 100 GU bubble rings
    // every system, and a ship marker tracks along the route as you fly.
    function fullRouteBody(r) {
      var pad = 16, o = opts.origin, d = opts.dest;
      var minX = Math.min(o.x, d.x), maxX = Math.max(o.x, d.x);
      var minY = Math.min(o.y, d.y), maxY = Math.max(o.y, d.y);
      var mX = Math.max((maxX - minX) * 0.15, 280);     // GU margin (room for bubbles)
      var mY = Math.max((maxY - minY) * 0.15, 280);
      minX -= mX; maxX += mX; minY -= mY; maxY += mY;
      var rangeX = maxX - minX, rangeY = maxY - minY;
      var inner = r.s - 2 * pad;
      var scale = inner / Math.max(rangeX, rangeY);      // equal scale, galaxy-map style
      var offX = r.x + pad + (inner - rangeX * scale) / 2;
      var offY = r.y + pad + (inner - rangeY * scale) / 2;
      // +X right, +Y up: matches the in-game galaxy map and keeps left/right
      // consistent with the first-person view and radar (perp-based).
      function px(x, y) { return { x: offX + (x - minX) * scale, y: offY + (maxY - y) * scale }; }
      function inBox(x, y) { return x >= minX && x <= maxX && y >= minY && y <= maxY; }
      var br = Math.max(1.2, MARGIN * scale);            // 100 GU bubble radius in px
      // systems in view: 100 GU bubble + dot; near-passes glow orange
      for (var i = 0; i < scene.stars.length; i++) {
        var s = scene.stars[i];
        if (!inBox(s.x, s.y)) continue;
        var p = px(s.x, s.y);
        var near = Math.abs(s.perp) <= MARGIN && s.proj > 0 && s.proj < scene.routeLen;
        ctx.strokeStyle = near ? 'rgba(255,150,110,0.5)' : 'rgba(208,213,226,0.28)';
        ctx.lineWidth = 1;
        ctx.beginPath(); ctx.arc(p.x, p.y, br, 0, 6.2832); ctx.stroke();
        ctx.fillStyle = near ? '#ff9b6e' : s.color;
        ctx.beginPath(); ctx.arc(p.x, p.y, s.bh ? 3 : 1.8, 0, 6.2832); ctx.fill();
      }
      // route: flown portion solid, blocked remainder dashed
      var pO = px(o.x, o.y), pD = px(d.x, d.y);
      var pE = px(o.x + scene.ux * scene.endProj, o.y + scene.uy * scene.endProj);
      // origin's own bubble
      ctx.strokeStyle = 'rgba(208,213,226,0.28)'; ctx.lineWidth = 1;
      ctx.beginPath(); ctx.arc(pO.x, pO.y, br, 0, 6.2832); ctx.stroke();
      if (scene.blocked) {
        ctx.setLineDash([3, 3]); ctx.strokeStyle = 'rgba(120,150,210,0.4)'; ctx.lineWidth = 1;
        ctx.beginPath(); ctx.moveTo(pE.x, pE.y); ctx.lineTo(pD.x, pD.y); ctx.stroke();
        ctx.setLineDash([]);
      }
      ctx.strokeStyle = 'rgba(159,210,255,0.75)'; ctx.lineWidth = 1.5;
      ctx.beginPath(); ctx.moveTo(pO.x, pO.y); ctx.lineTo(pE.x, pE.y); ctx.stroke();
      // origin (green dot) and endpoint (blue dest, or red X where a route drops out)
      ctx.fillStyle = '#7fe0a0';
      ctx.beginPath(); ctx.arc(pO.x, pO.y, 3, 0, 6.2832); ctx.fill();
      if (scene.blocked) {
        ctx.strokeStyle = '#ff7a4d'; ctx.lineWidth = 1.4;
        ctx.beginPath();
        ctx.moveTo(pE.x - 3, pE.y - 3); ctx.lineTo(pE.x + 3, pE.y + 3);
        ctx.moveTo(pE.x + 3, pE.y - 3); ctx.lineTo(pE.x - 3, pE.y + 3); ctx.stroke();
      } else {
        ctx.fillStyle = '#9fd2ff';
        ctx.beginPath(); ctx.arc(pD.x, pD.y, 3, 0, 6.2832); ctx.fill();
      }
      // ship marker, pointing along travel (screen Y is flipped, so negate uy)
      var prog = Math.min(t, scene.endProj);
      var pS = px(o.x + scene.ux * prog, o.y + scene.uy * prog);
      shipMarker(pS.x, pS.y, scene.ux, -scene.uy, 5);
    }

    // radarBody: true-scale overhead view tracking the ship — the corridor just
    // ahead with 100 GU bubbles as real circles. Bubbles the lane threads
    // (|perp| <= 100 GU) glow orange. Ship sits near the bottom; the world
    // scrolls down past it, matching the first-person view above.
    function radarBody(r) {
      var pad = 14, spanGU = 2200, behindGU = 450;
      var cx = r.x + r.s / 2;
      var scale = (r.s - 2 * pad) / spanGU;             // px / GU, equal both axes
      var shipY = r.y + r.s - pad - behindGU * scale;
      function px(proj, perp) { return { x: cx + perp * scale, y: shipY - (proj - t) * scale }; }
      ctx.strokeStyle = 'rgba(120,150,210,0.4)'; ctx.lineWidth = 1;
      ctx.beginPath(); ctx.moveTo(cx, r.y); ctx.lineTo(cx, r.y + r.s); ctx.stroke();
      for (var i = 0; i < scene.stars.length; i++) {
        var s = scene.stars[i];
        var rel = s.proj - t;
        if (rel < -behindGU - 120 || rel > (spanGU - behindGU) + 120) continue;
        if (Math.abs(s.perp) * scale > r.s / 2) continue;
        var p = px(s.proj, s.perp);
        ctx.strokeStyle = Math.abs(s.perp) <= MARGIN ? 'rgba(255,150,110,0.55)' : 'rgba(208,213,226,0.3)';
        ctx.lineWidth = 1;
        ctx.beginPath(); ctx.arc(p.x, p.y, 100 * scale, 0, 6.2832); ctx.stroke();
        ctx.fillStyle = s.color;
        ctx.beginPath(); ctx.arc(p.x, p.y, s.bh ? 3 : 2.2, 0, 6.2832); ctx.fill();
      }
      shipMarker(cx, shipY, 0, -1, 6);
    }

    function frame(now) {
      if (!running) return;
      var dt = last ? Math.min(0.05, (now - last) / 1000) : 0;
      last = now;
      t += speed * dt;

      ctx.globalCompositeOperation = 'source-over';
      ctx.fillStyle = '#05060d';
      ctx.fillRect(0, 0, W(), H());
      ctx.globalCompositeOperation = 'lighter';

      // ambient field
      for (var i = 0; i < ambient.length; i++) {
        var a = ambient[i];
        a.depth -= speed * dt;
        if (a.depth < near) {
          a.depth += (far - near);
          a.perp = (Math.random() * 2 - 1) * spread * 2.2;
          a.z = (Math.random() * 2 - 1) * spread * 2.2;
        }
        var ap = project(a.perp, a.z, a.depth);
        if (ap.x < -50 || ap.x > W() + 50 || ap.y < -50 || ap.y > H() + 50) continue;
        var pa = project(a.perp, a.z, a.depth + speed * dt);
        ctx.strokeStyle = 'rgba(190,205,245,' + Math.max(0, 0.75 - a.depth / far) + ')';
        ctx.lineWidth = 1.1;
        ctx.beginPath(); ctx.moveTo(pa.x, pa.y); ctx.lineTo(ap.x, ap.y); ctx.stroke();
      }

      // Pass 1: the 100 GU bubbles, as translucent light-gray spheres behind
      // the stars (source-over so they read as glass, not additive glow).
      ctx.globalCompositeOperation = 'source-over';
      for (var b = 0; b < scene.stars.length; b++) {
        var sb = scene.stars[b];
        var fb = sb.proj - t;
        if (fb <= near || fb > far) continue;
        drawBubble(project(sb.perp, sb.z, fb), fb);
      }

      // Pass 2: the stars themselves, additive and bright. Multi-star systems
      // expand into a small cluster, each component drawn by its own class.
      ctx.globalCompositeOperation = 'lighter';
      for (var j = 0; j < scene.stars.length; j++) {
        var s = scene.stars[j];
        var forward = s.proj - t;
        if (forward <= near || forward > far) { prev[s.id] = null; continue; }
        var headlit = s.isDest || s.id === scene.endStar;
        if (s.suns) {
          var centered = s.suns[0].class === 'BH';
          for (var c = 0; c < s.suns.length; c++) {
            var comp = s.suns[c];
            var off = rosetteOffset(s.id, c, s.suns.length, centered);
            var pc = project(s.perp + off[0], s.z + off[1], forward);
            drawStar(pc, null, forward, classToColor(comp.class), classToSize(comp.class),
              headlit, comp.class === 'BH', centerColor(comp.class));
          }
          prev[s.id] = null;
        } else {
          var p = project(s.perp, s.z, forward);
          drawStar(p, prev[s.id], forward, s.color, s.size, headlit, s.bh, s.center);
          prev[s.id] = p;
        }
      }

      // Pass 3: labels for stars close ahead and near the flight line. A dark
      // outline keeps them legible against the bright starfield.
      ctx.globalCompositeOperation = 'source-over';
      ctx.font = 'bold 16px monospace';
      ctx.lineJoin = 'round';
      for (var k = 0; k < scene.stars.length; k++) {
        var sl = scene.stars[k];
        var fl = sl.proj - t;
        if (fl <= near || fl > far) continue;
        if (fl < 1100 && Math.abs(sl.perp) < 800) {
          var la = Math.max(0, Math.min(1, 1 - fl / 1100));
          ctx.lineWidth = 3.5;
          if (sl.suns) {
            // label each component at its cluster position
            var centeredL = sl.suns[0].class === 'BH';
            for (var c2 = 0; c2 < sl.suns.length; c2++) {
              var offL = rosetteOffset(sl.id, c2, sl.suns.length, centeredL);
              var plc = project(sl.perp + offL[0], sl.z + offL[1], fl);
              if (plc.x < 0 || plc.x > W()) continue;
              ctx.globalAlpha = la;
              ctx.strokeStyle = 'rgba(4,5,12,0.9)';
              ctx.strokeText(sl.suns[c2].name, plc.x + 10, plc.y - 8);
              ctx.fillStyle = '#e2e7f4';
              ctx.fillText(sl.suns[c2].name, plc.x + 10, plc.y - 8);
            }
            ctx.globalAlpha = 1;
          } else {
            var pl = project(sl.perp, sl.z, fl);
            if (pl.x >= 0 && pl.x <= W()) {
              ctx.globalAlpha = la;
              ctx.strokeStyle = 'rgba(4,5,12,0.9)';
              ctx.strokeText(sl.name, pl.x + 10, pl.y - 8);
              ctx.fillStyle = '#e2e7f4';
              ctx.fillText(sl.name, pl.x + 10, pl.y - 8);
              ctx.globalAlpha = 1;
            }
          }
        }
      }

      // arrival flash when the camera reaches the endpoint star
      if (t >= scene.endProj - near) {
        arrived += dt;
        var r = arrived * 900;
        ctx.globalAlpha = Math.max(0, 1 - arrived * 1.2);
        ctx.fillStyle = scene.blocked ? '#ff7a4d' : '#9fd2ff';
        ctx.beginPath(); ctx.arc(W() / 2, H() / 2, r, 0, 6.2832); ctx.fill();
        ctx.globalAlpha = 1;
        if (arrived > 1.1) {
          running = false;
          if (opts.onArrive) opts.onArrive(scene);
          return;
        }
      }

      // HUD (top-left, clear of the bottom-corner minimaps)
      ctx.globalCompositeOperation = 'source-over';
      ctx.fillStyle = '#cdd3e0'; ctx.font = '12px monospace';
      var pct = Math.min(100, (t / scene.endProj) * 100);
      ctx.fillText(Math.round(t) + ' / ' + Math.round(scene.endProj) + ' GU  (' +
        Math.round(pct) + '%)', 10, 20);

      // top-down picture-in-picture: full route (left) + tracking radar (right)
      if (opts.minimaps !== false) {
        drawPanel(panelRect('left'), 'FULL ROUTE', fullRouteBody);
        drawPanel(panelRect('right'), 'RADAR', radarBody);
      }

      raf = global.requestAnimationFrame(frame);
    }

    function play() { if (!running) { running = true; last = 0; raf = global.requestAnimationFrame(frame); } }
    function pause() { running = false; if (raf) global.cancelAnimationFrame(raf); }
    function replay() { pause(); t = 0; arrived = 0; prev = {}; play(); }
    function setSpeedMul(m) { speed = baseSpeed * m; } // live speed adjust
    function destroy() { pause(); global.removeEventListener('resize', resize); }

    resize();
    global.addEventListener('resize', resize);
    return {
      play: play, pause: pause, replay: replay,
      setSpeedMul: setSpeedMul, destroy: destroy, scene: scene
    };
  }

  var api = {
    playWarp: playWarp,
    // exposed for inspection/testing:
    hashStr: hashStr, seededHeight: seededHeight,
    classToColor: classToColor, classToSize: classToSize, buildScene: buildScene
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  global.Warp = api;
})(typeof window !== 'undefined' ? window : this);
