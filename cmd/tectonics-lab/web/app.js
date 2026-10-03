// Tectonics Lab viewer: scrubs keyframe bundles produced by tectonics-lab run.
(function () {
  'use strict';
  const $ = (id) => document.getElementById(id);

  // ---- GLSL: one colouring function shared by the sphere and the cross ----
  const VS = `
    attribute vec2 aPos; varying vec2 vUV;
    void main(){ vUV = aPos*0.5+0.5; gl_Position = vec4(aPos,0.0,1.0); }`;
  const FS = `
    precision highp float;
    varying vec2 vUV;
    uniform sampler2D uTh, uPl, uFe;
    uniform int uMode;      // 0 sphere, 1 flat cross
    uniform int uLayer;     // 0 plates, 1 thickness, 2 hypsometric, 3 features
    uniform mat3 uView;     // view rotation (screen -> world)
    uniform vec3 uSun;

    // Mirrors cubemap.DirToFaceUV + crossCells: returns cross-texture coords.
    vec2 crossUV(vec3 d){
      vec3 a = abs(d); float sc, tc, ma; vec2 cell;
      if (a.x >= a.y && a.x >= a.z) { ma = a.x;
        if (d.x >= 0.0) { sc = -d.z; tc = -d.y; cell = vec2(2.0,1.0); } else { sc = d.z; tc = -d.y; cell = vec2(0.0,1.0); }
      } else if (a.y >= a.z) { ma = a.y;
        if (d.y >= 0.0) { sc = d.x; tc = d.z; cell = vec2(1.0,0.0); } else { sc = d.x; tc = -d.z; cell = vec2(1.0,2.0); }
      } else { ma = a.z;
        if (d.z >= 0.0) { sc = d.x; tc = -d.y; cell = vec2(1.0,1.0); } else { sc = -d.x; tc = -d.y; cell = vec2(3.0,1.0); }
      }
      float u = 0.5*(sc/ma+1.0), v = 0.5*(tc/ma+1.0);
      return vec2((cell.x+u)/4.0, (cell.y+v)/3.0);
    }
    vec3 hsv(float h, float s, float v){
      vec3 k = fract(vec3(h, h+2.0/3.0, h+1.0/3.0))*6.0;
      return v * mix(vec3(1.0), clamp(abs(k-3.0)-1.0, 0.0, 1.0), s);
    }
    vec3 colour(vec2 uv){
      float th = texture2D(uTh, uv).r;
      float id = floor(texture2D(uPl, uv).r*255.0+0.5);
      float fb = floor(texture2D(uFe, uv).r*255.0+0.5);
      float ftype = floor(fb/64.0), fage = fb - ftype*64.0;
      vec3 c;
      if (uLayer == 0) {
        c = hsv(fract(id*0.618034), 0.55, 0.45+0.45*th);
      } else if (uLayer == 1) {
        c = vec3(th);
      } else if (uLayer == 2) {
        c = th < 0.5 ? mix(vec3(0.05,0.15,0.45), vec3(0.2,0.55,0.8), th*2.0)
                     : mix(vec3(0.25,0.5,0.2), vec3(0.95,0.95,0.95), (th-0.5)*2.0);
      } else {
        c = vec3(0.25+0.5*th);
      }
      if (uLayer == 0 || uLayer == 3) {
        float fade = 1.0 - fage/63.0;
        if (ftype == 1.0) c = mix(c, vec3(0.2,0.5,1.0), 0.4+0.6*fade);
        if (ftype == 2.0) c = mix(c, mix(vec3(0.55,0.3,0.15), vec3(1.0,0.2,0.1), fade), 0.5+0.5*fade);
        if (ftype == 3.0) c = mix(c, vec3(1.0,0.9,0.2), 0.3+0.7*fade);
      }
      return c;
    }
    void main(){
      if (uMode == 1) {
        gl_FragColor = vec4(colour(vec2(vUV.x, 1.0-vUV.y)), 1.0);
        return;
      }
      vec2 p = vUV*2.0-1.0;
      float r2 = dot(p,p);
      if (r2 > 1.0) { gl_FragColor = vec4(0.02,0.027,0.04,1.0); return; }
      vec3 n = vec3(p.x, p.y, sqrt(1.0-r2));
      vec3 d = uView * n;
      float light = 0.25 + 0.75*max(0.0, dot(n, normalize(uSun)));
      gl_FragColor = vec4(colour(crossUV(d))*light, 1.0);
    }`;

  function makeGL(canvas) {
    const gl = canvas.getContext('webgl');
    if (!gl) { throw new Error('WebGL unavailable'); }
    const sh = (type, src) => {
      const s = gl.createShader(type); gl.shaderSource(s, src); gl.compileShader(s);
      if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) { throw new Error(gl.getShaderInfoLog(s)); }
      return s;
    };
    const prog = gl.createProgram();
    gl.attachShader(prog, sh(gl.VERTEX_SHADER, VS)); gl.attachShader(prog, sh(gl.FRAGMENT_SHADER, FS));
    gl.linkProgram(prog); gl.useProgram(prog);
    const buf = gl.createBuffer(); gl.bindBuffer(gl.ARRAY_BUFFER, buf);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1,-1, 1,-1, -1,1, 1,1]), gl.STATIC_DRAW);
    const aPos = gl.getAttribLocation(prog, 'aPos'); gl.enableVertexAttribArray(aPos); gl.vertexAttribPointer(aPos, 2, gl.FLOAT, false, 0, 0);
    const u = {}; ['uTh','uPl','uFe','uMode','uLayer','uView','uSun'].forEach((n) => { u[n] = gl.getUniformLocation(prog, n); });
    gl.uniform1i(u.uTh, 0); gl.uniform1i(u.uPl, 1); gl.uniform1i(u.uFe, 2);
    const tex = [0,1,2].map(() => {
      const t = gl.createTexture(); gl.bindTexture(gl.TEXTURE_2D, t);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST); gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE); gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      return t;
    });
    return { gl, u, tex };
  }

  function upload(ctx, images) {
    const gl = ctx.gl;
    images.forEach((img, i) => {
      gl.activeTexture(gl.TEXTURE0 + i); gl.bindTexture(gl.TEXTURE_2D, ctx.tex[i]);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, img);
    });
  }

  // ---- view rotation (yaw/pitch by drag) ----
  let yaw = 0.6, pitch = 0.2;
  function viewMatrix() {
    const cy = Math.cos(yaw), sy = Math.sin(yaw), cp = Math.cos(pitch), sp = Math.sin(pitch);
    // column-major mat3 = Ry(yaw) * Rx(pitch)
    return new Float32Array([cy, 0, -sy,  sy*sp, cp, cy*sp,  sy*cp, -sp, cy*cp]);
  }
  function worldToScreen(v) { // inverse of uView applied in the shader
    const m = viewMatrix();
    return [m[0]*v[0]+m[1]*v[1]+m[2]*v[2], m[3]*v[0]+m[4]*v[1]+m[5]*v[2], m[6]*v[0]+m[7]*v[1]+m[8]*v[2]];
  }

  // ---- state ----
  const sphere = makeGL($('sphere')), cross = makeGL($('cross'));
  const arrows = $('arrows').getContext('2d');
  let manifest = null, slug = '', frameCache = new Map(), current = 0, playing = false, timer = null;
  const layer = () => parseInt(document.querySelector('input[name=layer]:checked').value, 10);

  function loadImage(url) {
    return new Promise((res, rej) => { const img = new Image(); img.onload = () => res(img); img.onerror = () => rej(new Error('load ' + url)); img.src = url; });
  }
  async function frameImages(i) {
    if (frameCache.has(i)) { return frameCache.get(i); }
    const dir = `/bundles/${slug}/${manifest.frames[i].dir}`;
    const p = Promise.all(['thickness', 'plate', 'feature'].map((n) => loadImage(`${dir}/${n}.png`)));
    frameCache.set(i, p);
    return p;
  }

  function drawArrows(frame) {
    const c = arrows, W = c.canvas.width, H = c.canvas.height;
    c.clearRect(0, 0, W, H);
    if (!$('showArrows').checked) { return; }
    const R = Math.min(W, H) / 2;
    for (const pl of frame.plates) {
      if (pl.retired) { continue; }
      const s = worldToScreen(pl.centroid);
      if (s[2] < 0.05) { continue; }
      const w = pl.pole.map((x) => x * pl.speed_cm_yr);
      const v = [w[1]*pl.centroid[2]-w[2]*pl.centroid[1], w[2]*pl.centroid[0]-w[0]*pl.centroid[2], w[0]*pl.centroid[1]-w[1]*pl.centroid[0]];
      const sv = worldToScreen(v);
      const x = W/2 + s[0]*R, y = H/2 - s[1]*R, k = 6 * R / 100;
      const dx = sv[0]*k, dy = -sv[1]*k;
      c.strokeStyle = pl.major ? '#ffffff' : '#b0b8c4'; c.lineWidth = pl.major ? 2 : 1;
      c.beginPath(); c.moveTo(x, y); c.lineTo(x+dx, y+dy); c.stroke();
      c.beginPath(); c.arc(x+dx, y+dy, pl.major ? 3 : 2, 0, Math.PI*2); c.fillStyle = c.strokeStyle; c.fill();
      c.fillStyle = '#e8ecf2'; c.font = '11px system-ui'; c.fillText(String(pl.id), x+4, y-4);
    }
  }

  async function render() {
    if (!manifest) { return; }
    const i = current, images = await frameImages(i);
    if (i !== current) { return; }
    for (const [ctx, mode] of [[sphere, 0], [cross, 1]]) {
      const gl = ctx.gl;
      upload(ctx, images);
      gl.viewport(0, 0, gl.canvas.width, gl.canvas.height);
      gl.uniform1i(ctx.u.uMode, mode); gl.uniform1i(ctx.u.uLayer, layer());
      gl.uniformMatrix3fv(ctx.u.uView, false, viewMatrix()); gl.uniform3f(ctx.u.uSun, -0.5, 0.4, 0.8);
      gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
    }
    drawArrows(manifest.frames[i]);
    $('myr').textContent = `${manifest.frames[i].myr.toFixed(0)} Myr  (step ${manifest.frames[i].step}/${manifest.steps})`;
  }

  function setFrame(i) { current = Math.max(0, Math.min(manifest.frames.length - 1, i)); $('slider').value = current; render(); }

  async function loadBundle(name) {
    slug = name; frameCache = new Map();
    manifest = await (await fetch(`/bundles/${slug}/manifest.json`)).json();
    $('slider').max = manifest.frames.length - 1;
    $('status').textContent = `${slug}: ${manifest.archetype}, face ${manifest.face}, ${manifest.frames.length} frames, ${manifest.frames[0].plates.length} plates`;
    setFrame(0);
    for (let i = 1; i < manifest.frames.length; i++) { frameImages(i); } // warm the cache in order
  }

  async function refreshBundles(select) {
    const list = await (await fetch('/api/bundles')).json();
    const sel = $('bundle'); sel.innerHTML = '';
    for (const b of list) { const o = document.createElement('option'); o.value = b.slug; o.textContent = `${b.slug} (${b.frames} frames)`; sel.appendChild(o); }
    if (select && list.some((b) => b.slug === select)) { sel.value = select; }
    if (sel.value) { loadBundle(sel.value); } else { $('status').textContent = 'no bundles yet — start a run'; }
  }

  async function startRun() {
    const body = { planet: $('planet').value.trim(), seed: parseInt($('seed').value, 10) || 0, archetype: $('archetype').value,
      face: parseInt($('face').value, 10) || 0, steps: parseInt($('steps').value, 10) || 0,
      sets: $('sets').value.split(/\s+/).filter(Boolean) };
    const res = await fetch('/api/run', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    const j = await res.json();
    if (!res.ok) { $('status').textContent = 'run refused: ' + j.error; return; }
    const poll = async () => {
      const st = await (await fetch('/api/jobs/' + j.job)).json();
      if (st.state === 'running') { $('status').textContent = `running: step ${st.step}/${st.steps}`; setTimeout(poll, 1000); return; }
      if (st.state === 'error') { $('status').textContent = 'run failed: ' + st.error; return; }
      refreshBundles(st.slug);
    };
    poll();
  }

  // ---- wiring ----
  $('bundle').addEventListener('change', (e) => loadBundle(e.target.value));
  $('run').addEventListener('click', startRun);
  $('slider').addEventListener('input', (e) => setFrame(parseInt(e.target.value, 10)));
  document.querySelectorAll('input[name=layer], #showArrows').forEach((el) => el.addEventListener('change', render));
  $('play').addEventListener('click', () => {
    playing = !playing; $('play').textContent = playing ? '❚❚' : '▶';
    if (playing) { timer = setInterval(() => setFrame(current + 1 >= manifest.frames.length ? 0 : current + 1), 120); } else { clearInterval(timer); }
  });
  let drag = null;
  $('arrows').style.pointerEvents = 'none';
  $('sphere').addEventListener('mousedown', (e) => { drag = [e.clientX, e.clientY]; });
  window.addEventListener('mousemove', (e) => {
    if (!drag) { return; }
    yaw += (e.clientX - drag[0]) * 0.01; pitch = Math.max(-1.4, Math.min(1.4, pitch + (e.clientY - drag[1]) * 0.01));
    drag = [e.clientX, e.clientY]; render();
  });
  window.addEventListener('mouseup', () => { drag = null; });
  window.addEventListener('keydown', (e) => {
    if (!manifest) { return; }
    if (e.key === 'ArrowRight') { setFrame(current + 1); } else if (e.key === 'ArrowLeft') { setFrame(current - 1); }
  });

  fetch('/api/archetypes').then((r) => r.json()).then((names) => {
    const sel = $('archetype');
    for (const n of names) { const o = document.createElement('option'); o.value = n; o.textContent = n; sel.appendChild(o); }
    sel.value = 'terran';
  });
  refreshBundles();
})();
