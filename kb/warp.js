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

  var SPECTRAL = {
    O: '#9bb0ff', B: '#aabfff', A: '#cad8ff', F: '#f6f3ff',
    G: '#fff4e8', K: '#ffd6a5', M: '#ffb86b'
  };

  // classToColor: Morgan-Keenan class -> CSS color (blackbody-ish).
  function classToColor(cls) {
    if (!cls) return '#c8ccd8';
    if (cls === 'BH') return '#15102a';   // black hole
    if (cls.charAt(0) === 'D') return '#dfe9ff'; // white dwarf
    return SPECTRAL[cls.charAt(0)] || '#c8ccd8';
  }

  // classToSize: luminosity-class size multiplier (V dwarf .. Ia supergiant).
  function classToSize(cls) {
    if (!cls) return 1;
    if (cls === 'BH') return 2.4;
    if (cls.charAt(0) === 'D') return 0.6;
    var m = cls.match(/(Ia|Ib|III|II|IV|V|I)$/);
    var lum = m ? m[1] : 'V';
    if (lum === 'Ia' || lum === 'Ib' || lum === 'I') return 4;
    if (lum === 'II') return 3;
    if (lum === 'III') return 2;
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
        // The target sits on the route line (perp == 0); drop its z jitter too
        // so it arrives dead-center in the viewport.
        z: isDest ? 0 : seededHeight(s.id) * (opt.heightSpread || 1200),
        color: classToColor(s.class), size: classToSize(s.class),
        isDest: isDest
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
    var spread = opts.heightSpread || 1200;
    var speed = opts.speed || 450;        // GU per second of animation
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

    function drawStar(p, prevP, forward, color, size, glow) {
      var sz = Math.max(0.8, size * focal / forward * 0.85);
      // brighter falloff: gentle curve holds luminosity far longer than linear.
      var alpha = Math.max(0, Math.min(1, 1.7 * Math.pow(1 - forward / far, 0.4)));
      ctx.fillStyle = color;
      ctx.strokeStyle = color;
      if (prevP) { // streak from last frame
        ctx.globalAlpha = alpha;
        ctx.lineWidth = Math.max(0.8, sz * 1.1);
        ctx.beginPath(); ctx.moveTo(prevP.x, prevP.y); ctx.lineTo(p.x, p.y); ctx.stroke();
      }
      // wide soft glow on every star (additive 'lighter' is in effect)
      ctx.globalAlpha = alpha * (glow ? 0.55 : 0.4);
      ctx.beginPath(); ctx.arc(p.x, p.y, sz * (glow ? 6 : 3.6), 0, 6.2832); ctx.fill();
      // colored core
      ctx.globalAlpha = Math.min(1, alpha);
      ctx.beginPath(); ctx.arc(p.x, p.y, sz, 0, 6.2832); ctx.fill();
      // white-hot center
      ctx.globalAlpha = Math.min(1, alpha);
      ctx.fillStyle = '#ffffff';
      ctx.beginPath(); ctx.arc(p.x, p.y, sz * 0.45, 0, 6.2832); ctx.fill();
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

      // Pass 2: the stars themselves, additive and bright.
      ctx.globalCompositeOperation = 'lighter';
      for (var j = 0; j < scene.stars.length; j++) {
        var s = scene.stars[j];
        var forward = s.proj - t;
        if (forward <= near || forward > far) { prev[s.id] = null; continue; }
        var p = project(s.perp, s.z, forward);
        drawStar(p, prev[s.id], forward, s.color, s.size, s.isDest || s.id === scene.endStar);
        prev[s.id] = p;
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
          var pl = project(sl.perp, sl.z, fl);
          if (pl.x < 0 || pl.x > W()) continue;
          ctx.globalAlpha = Math.max(0, Math.min(1, 1 - fl / 1100));
          ctx.lineWidth = 3.5;
          ctx.strokeStyle = 'rgba(4,5,12,0.9)';
          ctx.strokeText(sl.name, pl.x + 10, pl.y - 8);
          ctx.fillStyle = '#e2e7f4';
          ctx.fillText(sl.name, pl.x + 10, pl.y - 8);
          ctx.globalAlpha = 1;
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

      // HUD
      ctx.globalCompositeOperation = 'source-over';
      ctx.fillStyle = '#cdd3e0'; ctx.font = '12px monospace';
      var pct = Math.min(100, (t / scene.endProj) * 100);
      ctx.fillText(Math.round(t) + ' / ' + Math.round(scene.endProj) + ' GU  (' +
        Math.round(pct) + '%)', 10, H() - 12);

      raf = global.requestAnimationFrame(frame);
    }

    function play() { if (!running) { running = true; last = 0; raf = global.requestAnimationFrame(frame); } }
    function pause() { running = false; if (raf) global.cancelAnimationFrame(raf); }
    function replay() { pause(); t = 0; arrived = 0; prev = {}; play(); }

    resize();
    global.addEventListener('resize', resize);
    return { play: play, pause: pause, replay: replay, scene: scene };
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
