// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 06: The photo editor (Crop, Adjust, Filters, Markup).
'use strict';

// ---------- the photo editor ----------
// Laid out like the iPhone Photos editor: Crop, Adjust, Filters, Markup.
// Open-source helpers in web/, loaded only when the editor opens:
//   Cropper.js (vendor): cropping, zooming, turning, flipping, straightening
//   Fabric.js (vendor):  everything added on Markup stays an object you can tap later to move, resize, turn,
//                        recolour or delete
//   develop.js (ours):   the Adjust sliders, filters and perspective, on raw pixels
// Added things are kept in "frame units" (screen pixels of the crop frame, from its top-left corner). When the
// crop, turn or flip changes, they are moved along with the picture, so they stay stuck to the same spot.
const ED_FULL = 3508;      // long side when saving: A4 at 300 dpi
const ED_COLORS = ['#111111', '#ffffff', '#e53935', '#fdd835', '#2e9e4f', '#1e63d6'];
const ED_RATIOS = [['free', 'Free'], ['orig', 'Original'], ['sq', 'Square'], ['a4', 'A4'], ['4:3', '4:3'], ['3:2', '3:2'], ['16:9', '16:9']];
const ED_ADD = [['text', 'text', 'Text'], ['sign', 'sign', 'Signature'], ['mag', 'mag', 'Magnifier'], ['rect', 'rect', 'Square'], ['circle', 'circle', 'Circle'],
  ['bubble', 'bubble', 'Speech bubble'], ['arrow', 'arrow', 'Arrow'], ['line', 'line', 'Line'], ['star', 'star', 'Star']];

const scriptLoads = {};
function loadScript(src, name) {
  if (window[name]) return Promise.resolve(window[name]);
  return scriptLoads[src] = scriptLoads[src] || new Promise((res, rej) => {
    const s = document.createElement('script');
    s.src = src;
    s.onload = () => res(window[name]);
    s.onerror = () => { delete scriptLoads[src]; s.remove(); rej(new Error('Couldn\'t open the editor, try again')); };
    document.head.appendChild(s);
  });
}

// 2D matrices [a, b, c, d, e, f] like CSS matrix(): x' = a·x + c·y + e, y' = b·x + d·y + f
const mMul = (m, n) => [m[0] * n[0] + m[2] * n[1], m[1] * n[0] + m[3] * n[1], m[0] * n[2] + m[2] * n[3], m[1] * n[2] + m[3] * n[3],
  m[0] * n[4] + m[2] * n[5] + m[4], m[1] * n[4] + m[3] * n[5] + m[5]];
const mInv = ([a, b, c, d, e, f]) => { const k = a * d - b * c; return [d / k, -b / k, -c / k, a / k, (c * f - d * e) / k, (b * e - a * f) / k]; };
const mAt = (m, [x, y]) => [m[0] * x + m[2] * y + m[4], m[1] * x + m[3] * y + m[5]];
const mMove = (x, y) => [1, 0, 0, 1, x, y];
const mTurn = r => [Math.cos(r), Math.sin(r), -Math.sin(r), Math.cos(r), 0, 0];
const mSize = (k, l = k) => [k, 0, 0, l, 0, 0];
// Cropper.js turns the picture around its middle: canvas = move(middle + e,f) · linear · move(-middle)
const picToCanvas = (tf, w, h) => mMul(mMove(w / 2 + tf[4], h / 2 + tf[5]), mMul([tf[0], tf[1], tf[2], tf[3], 0, 0], mMove(-w / 2, -h / 2)));
const canvasToTf = (m, w, h) => { const [cx, cy] = mAt([m[0], m[1], m[2], m[3], 0, 0], [w / 2, h / 2]); return [m[0], m[1], m[2], m[3], m[4] + cx - w / 2, m[5] + cy - h / 2]; };

// colours: '#rrggbb' or 'rgba(r,g,b,a)'
function rgba(c) {
  let m = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(c);
  if (m) return [parseInt(m[1], 16), parseInt(m[2], 16), parseInt(m[3], 16), 1];
  m = /rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)\s*(?:,\s*([\d.]+))?\s*\)/i.exec(c || '');
  return m ? [+m[1], +m[2], +m[3], m[4] === undefined ? 1 : +m[4]] : [0, 0, 0, 1];
}
const cssOf = ([r, g, b, a = 1]) => a >= 1 ? '#' + [r, g, b].map(v => Math.round(v).toString(16).padStart(2, '0')).join('') : `rgba(${Math.round(r)},${Math.round(g)},${Math.round(b)},${+a.toFixed(2)})`;
const isLight = c => { const [r, g, b] = rgba(c); return 0.299 * r + 0.587 * g + 0.114 * b > 170; };
function hsl(h, s, l) {   // 0..360, 0..1, 0..1 -> '#rrggbb'
  const k = n => (n + h / 30) % 12, a = s * Math.min(l, 1 - l);
  const f = n => l - a * Math.max(-1, Math.min(k(n) - 3, 9 - k(n), 1));
  return cssOf([f(0) * 255, f(8) * 255, f(4) * 255]);
}

// shapes, as paths in a w×h box around (0,0), so they recolour and resize like any line
function arrowPath(x1, y1, x2, y2, w) {
  const a = Math.atan2(y2 - y1, x2 - x1), h = Math.max(w * 3.2, Math.hypot(x2 - x1, y2 - y1) * 0.18), s = 0.5;
  const p = (r) => `${x2 - h * Math.cos(a + r)} ${y2 - h * Math.sin(a + r)}`;
  return `M ${x1} ${y1} L ${x2} ${y2} M ${p(s)} L ${x2} ${y2} L ${p(-s)}`;
}
function starPath(r) {
  let d = '';
  for (let i = 0; i < 10; i++) { const a = -Math.PI / 2 + i * Math.PI / 5, k = i % 2 ? r * 0.45 : r; d += `${i ? 'L' : 'M'} ${k * Math.cos(a)} ${k * Math.sin(a)} `; }
  return d + 'Z';
}
function bubblePath(w, h) {   // a rounded box with a little tail at the bottom left, like a cartoon speech bubble
  const r = Math.min(w, h) * 0.22, x0 = -w / 2, y0 = -h / 2, x1 = w / 2, y1 = h / 2;
  return `M ${x0 + r} ${y0} L ${x1 - r} ${y0} Q ${x1} ${y0} ${x1} ${y0 + r} L ${x1} ${y1 - r} Q ${x1} ${y1} ${x1 - r} ${y1}
    L ${x0 + w * 0.42} ${y1} L ${x0 + w * 0.14} ${y1 + h * 0.34} L ${x0 + w * 0.3} ${y1} L ${x0 + r} ${y1} Q ${x0} ${y1} ${x0} ${y1 - r}
    L ${x0} ${y0 + r} Q ${x0} ${y0} ${x0 + r} ${y0} Z`.replace(/\s+/g, ' ');
}

// the colour sheet: grid, spectrum, see-through slider, recent colours, and a picker to take a colour from the photo
const recentColours = () => { try { return JSON.parse(localStorage.getItem('sakuraRecentColours') || '[]'); } catch (_) { return []; } };
function rememberColour(c) { try { localStorage.setItem('sakuraRecentColours', JSON.stringify([c, ...recentColours().filter(x => x !== c)].slice(0, 10))); } catch (_) {} }
function colourSheet(cur, canPick) {
  return new Promise(res => {
    let [r, g, b, a] = rgba(cur), tab = 'grid';
    const grid = () => {
      const rows = [Array.from({ length: 12 }, (_, i) => cssOf([255 - i * 23.18, 255 - i * 23.18, 255 - i * 23.18]))];
      for (const l of [0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.88]) rows.push(Array.from({ length: 12 }, (_, i) => hsl(i * 30, l > 0.8 ? 0.7 : 0.85, l)));
      return `<div class="cgrid">${rows.flat().map(c => `<button data-c="${c}" style="background:${c}" aria-label="${c}"></button>`).join('')}</div>`;
    };
    const q = modal(`<div class="csheet"><div class="chead"><b>Colour</b><button class="btn" data-x="done">Done</button></div>
      <div class="seg mini ctabs"><button data-tab="grid" class="on">Grid</button><button data-tab="spec">Spectrum</button></div>
      <div id="cbody"></div>
      <label class="edslide cop"><span>See-through</span><input type="range" id="copa" min="10" max="100" value="${Math.round(a * 100)}"></label>
      <div class="crow"><span class="cnow" id="cnow"></span><div class="crecent">${recentColours().map(c => `<button data-c="${c}" style="background:${c}"></button>`).join('')}</div>
        ${canPick ? `<button class="btn" data-x="pick">${ic('drop')} From photo</button>` : ''}</div></div>`);
    q.el.classList.add('sheetmodal');
    const $q = s => q.el.querySelector(s);
    const now = () => cssOf([r, g, b, a]);
    const show = () => { $q('#cnow').style.background = now(); q.el.querySelectorAll('.cgrid [data-c]').forEach(x => x.classList.toggle('on', x.dataset.c === cssOf([r, g, b]))); };
    function body() {
      q.el.querySelectorAll('[data-tab]').forEach(x => x.classList.toggle('on', x.dataset.tab === tab));
      if (tab === 'grid') { $q('#cbody').innerHTML = grid(); }
      else {
        $q('#cbody').innerHTML = '<canvas id="cspec" class="cspec" width="360" height="200"></canvas>';
        const c = $q('#cspec'), gg = c.getContext('2d');
        for (let x = 0; x < 360; x++) { const grd = gg.createLinearGradient(0, 0, 0, 200); grd.addColorStop(0, hsl(x, 1, 0.95)); grd.addColorStop(0.5, hsl(x, 1, 0.5)); grd.addColorStop(1, hsl(x, 1, 0.08)); gg.fillStyle = grd; gg.fillRect(x, 0, 1, 200); }
        const pickAt = e => { const bx = c.getBoundingClientRect(), x = Math.max(0, Math.min(359, (e.clientX - bx.left) / bx.width * 360)), y = Math.max(0, Math.min(199, (e.clientY - bx.top) / bx.height * 200));
          [r, g, b] = gg.getImageData(x | 0, y | 0, 1, 1).data; show(); };
        c.onpointerdown = e => { c.setPointerCapture(e.pointerId); pickAt(e); c.onpointermove = pickAt; };
        c.onpointerup = () => { c.onpointermove = null; };
      }
      q.el.querySelectorAll('#cbody [data-c]').forEach(x => x.onclick = () => { [r, g, b] = rgba(x.dataset.c); show(); });
      show();
    }
    q.el.querySelectorAll('.crecent [data-c]').forEach(x => x.onclick = () => { [r, g, b, a] = rgba(x.dataset.c); $q('#copa').value = Math.round(a * 100); show(); });
    q.el.querySelectorAll('[data-tab]').forEach(x => x.onclick = () => { tab = x.dataset.tab; body(); });
    $q('#copa').oninput = e => { a = +e.target.value / 100; show(); };
    $q('[data-x=done]').onclick = () => { const c = now(); rememberColour(c); q.close(); res(c); };
    const pk = $q('[data-x=pick]'); if (pk) pk.onclick = () => { q.close(); res({ pick: true, alpha: a }); };
    q.el.addEventListener('click', e => { if (e.target === q.el) { q.close(); res(null); } });
    body();
  });
}

// the signature pad: drawn once, remembered on this phone, put on any page with one tap
const savedSignature = () => { try { return JSON.parse(localStorage.getItem('sakuraSignature') || 'null'); } catch (_) { return null; } };
function signatureSheet() {
  return new Promise(res => {
    const old = savedSignature();
    const q = modal(`<div class="csheet"><div class="chead"><b>Signature</b><button class="btn" data-x="cancel">Cancel</button></div>
      <p class="sub" style="margin:0 0 8px">${old ? 'Use your saved signature, or sign again below.' : 'Sign with your finger. It\'s remembered on this phone only.'}</p>
      <canvas class="signpad" id="pad" width="720" height="300"></canvas>
      <div class="foot"><button class="btn" data-x="clear">Clear</button><button class="btn main" data-x="use">Use it</button></div></div>`);
    q.el.classList.add('sheetmodal');
    const c = q.el.querySelector('#pad'), g = c.getContext('2d');
    let strokes = old ? old.map(s => s.slice()) : [];
    const draw = () => {
      g.clearRect(0, 0, 720, 300); g.strokeStyle = '#d7cdd2'; g.lineWidth = 2; g.beginPath(); g.moveTo(40, 230); g.lineTo(680, 230); g.stroke();
      g.strokeStyle = '#111'; g.lineWidth = 6; g.lineCap = g.lineJoin = 'round';
      for (const s of strokes) { g.beginPath(); s.forEach(([x, y], i) => i ? g.lineTo(x * 720, y * 300) : g.moveTo(x * 720, y * 300)); if (s.length === 1) g.lineTo(s[0][0] * 720 + 0.5, s[0][1] * 300); g.stroke(); }
    };
    const at = e => { const b = c.getBoundingClientRect(); return [(e.clientX - b.left) / b.width, (e.clientY - b.top) / b.height]; };
    c.onpointerdown = e => { e.preventDefault(); c.setPointerCapture(e.pointerId); if (old && strokes === old) strokes = []; strokes.push([at(e)]); draw();
      c.onpointermove = ev => { strokes[strokes.length - 1].push(at(ev)); draw(); }; };
    c.onpointerup = c.onpointercancel = () => { c.onpointermove = null; };
    q.el.querySelector('[data-x=clear]').onclick = () => { strokes = []; draw(); };
    q.el.querySelector('[data-x=cancel]').onclick = () => { q.close(); res(null); };
    q.el.querySelector('[data-x=use]').onclick = () => {
      if (!strokes.length) return toast('Sign in the box first');
      try { localStorage.setItem('sakuraSignature', JSON.stringify(strokes.map(s => s.map(([x, y]) => [+x.toFixed(4), +y.toFixed(4)])))); } catch (_) {}
      q.close(); res(strokes);
    };
    draw();
  });
}

function openEditor(file, onSave) {
  const D = () => window.SakuraDevelop;
  const E = { tab: 'crop', sub: 'pen', color: '#e53935', size: 4, ratio: 'free', angle: 0, pv: 0, ph: 0, straight: 'angle',
    adj: {}, param: 'auto', filter: 'none', famt: 100 };
  const tabs = [['crop', 'crop', 'Crop'], ['adjust', 'dial', 'Adjust'], ['filters', 'filters', 'Filters'], ['markup', 'pen', 'Markup']];
  const m = modal(`<div class="ed">
      <div class="edtop"><button class="btn" data-x="cancel">Cancel</button><b>Edit</b>
        <button class="btn edundo" data-x="undo" aria-label="Undo">${ic('undo')}</button><button class="btn edundo" data-x="redo" aria-label="Redo">${ic('redo')}</button>
        <button class="btn main" data-x="save">Done</button></div>
      <div class="edstage" id="edstage"><div class="edcrop" id="edcrop"></div>
        <div class="edview" id="edview" hidden><canvas id="edbg"></canvas><canvas id="edmag"></canvas><canvas id="edfab"></canvas></div><div class="spin" id="edspin"></div></div>
      <div class="edopts" id="edopts"></div>
      <div class="edtools">${tabs.map(([k, n, t]) => `<button data-tab="${k}">${icon(n)}${t}</button>`).join('')}</div></div>`);
  m.el.classList.add('edmodal');
  const $e = s => m.el.querySelector(s), stageEl = $e('#edstage'), cropEl = $e('#edcrop'), viewEl = $e('#edview'), bg = $e('#edbg'), magC = $e('#edmag');
  let F, cc, ci, cs, fc, img, W = 1, H = 1, ro = null, closed = false, fit = 1, picking = null;
  let src = file, resume = null;   // src: the picture the edits apply to (the original, if this one was edited before)
  let geo = null;            // the picture position + crop frame the added things currently line up with
  const hist = []; let at = -1, quiet = 0, commitT = 0;

  // settle: a change joins the history (and the autosave) a moment after it's made; Save, Undo and closing act on
  // it straight away, so a line drawn just before them is never lost (found on a busy computer)
  const settle = () => { if (ci && !closed && !quiet) { commit(true); if (saveT) autosave(true); } };
  const close = () => { settle(); closed = true; if (ro) ro.disconnect(); document.removeEventListener('keydown', onKey); if (fc) fc.dispose(); m.close(); };
  $e('[data-x=cancel]').onclick = close;

  (async () => {
    try {
      const [C, fab, , saved] = await Promise.all([loadScript('vendor/cropper.min.js', 'Cropper'), loadScript('vendor/fabric.min.js', 'fabric'), loadScript('develop.js', 'SakuraDevelop'),
        api('/api/edit/' + file.id).catch(() => ({}))]);
      F = fab;
      if (saved && saved.state && saved.source) { src = saved.source; resume = saved.state; }
      img = new Image();
      img.src = '/api/file/' + src.id;
      await img.decode();
      if (closed) return;
      W = img.naturalWidth; H = img.naturalHeight;
      new C.default(img, { container: cropEl, template: `<cropper-canvas>
        <cropper-image rotatable scalable translatable initial-fit="none"></cropper-image>
        <cropper-shade theme-color="rgba(20,16,18,.62)"></cropper-shade>
        <cropper-handle action="move" plain></cropper-handle>
        <cropper-selection movable resizable outlined precise theme-color="#ffffff">
          <cropper-grid role="grid" covered theme-color="rgba(255,255,255,.35)"></cropper-grid>
          <cropper-handle action="move" theme-color="rgba(255,255,255,0)"></cropper-handle>
          ${['n', 'e', 's', 'w', 'ne', 'nw', 'se', 'sw'].map(d => `<cropper-handle action="${d}-resize" theme-color="transparent"></cropper-handle>`).join('')}
        </cropper-selection></cropper-canvas>` });
      cc = cropEl.querySelector('cropper-canvas'); ci = cropEl.querySelector('cropper-image'); cs = cropEl.querySelector('cropper-selection');
      await ci.$ready();
      await new Promise(r => requestAnimationFrame(r));
      fitAll();
      geo = { pic: pic2c(), sel: sel() };
      setupFabric();
      cc.addEventListener('actionend', () => commit());
      ro = new ResizeObserver(() => { if (showsView()) showView(); });
      ro.observe(stageEl);
      $e('#edspin').remove();
      if (resume && Array.isArray(resume.hist) && resume.hist.length) {   // pick up where you left off
        hist.push(...resume.hist);
        await restore(Math.max(0, Math.min(resume.at, hist.length - 1)), true);
        banner();
      } else commit(true);
      opts();
    } catch (e) { close(); toast(e.message || 'Couldn\'t open that picture', true); }
  })();

  // ---- geometry: where the picture and the crop frame are, in Cropper's screen pixels ----
  const tf = () => ci.$getTransform();
  const pic2c = () => picToCanvas(tf(), W, H);
  const sel = () => ({ x: cs.x, y: cs.y, w: cs.width, h: cs.height });
  const box = () => { const r = cropEl.getBoundingClientRect(); return { w: r.width, h: r.height }; };
  const corners = () => [[0, 0], [W, 0], [W, H], [0, H]].map(p => mAt(pic2c(), p));
  const bounds = pts => { const xs = pts.map(p => p[0]), ys = pts.map(p => p[1]); return { x: Math.min(...xs), y: Math.min(...ys), w: Math.max(...xs) - Math.min(...xs), h: Math.max(...ys) - Math.min(...ys) }; };
  const setPic = mat => ci.$setTransform(canvasToTf(mat, W, H));
  const setSel = r => { cs.aspectRatio = NaN; cs.$change(r.x, r.y, r.w, r.h); applyRatio(false); };
  const scaleOf = () => Math.hypot(tf()[0], tf()[1]);   // screen pixels per picture pixel
  const unit = () => Math.min(sel().w, sel().h);        // the frame's short side, in frame units
  const showsView = () => E.tab !== 'crop' || E.straight !== 'angle';   // the finished picture, not the crop handles

  function fitAll(pic = pic2c(), frame = null) {
    const b = box(), pad = 18;
    const bb = bounds([[0, 0], [W, 0], [W, H], [0, H]].map(p => mAt(pic, p)));
    const k = Math.min((b.w - 2 * pad) / bb.w, (b.h - 2 * pad) / bb.h);
    const g = mMul(mMove(b.w / 2, b.h / 2), mMul(mSize(k), mMove(-(bb.x + bb.w / 2), -(bb.y + bb.h / 2))));
    setPic(mMul(g, pic));
    const f = frame || bb, [p, q] = [mAt(g, [f.x, f.y]), mAt(g, [f.x + f.w, f.y + f.h])];
    setSel({ x: Math.min(p[0], q[0]), y: Math.min(p[1], q[1]), w: Math.abs(q[0] - p[0]), h: Math.abs(q[1] - p[1]) });
  }
  function coverFrame() {   // zoom in just enough that a straightened picture leaves no empty corners
    const s = sel(), c = [s.x + s.w / 2, s.y + s.h / 2];
    const inside = () => [[s.x, s.y], [s.x + s.w, s.y], [s.x + s.w, s.y + s.h], [s.x, s.y + s.h]]
      .every(p => { const [x, y] = mAt(mInv(pic2c()), p); return x >= -0.5 && y >= -0.5 && x <= W + 0.5 && y <= H + 0.5; });
    for (let i = 0; i < 80 && !inside(); i++) setPic(mMul(mMul(mMove(c[0], c[1]), mMul(mSize(1.015), mMove(-c[0], -c[1]))), pic2c()));
  }
  function ratioOf(k) {
    if (k === 'free') return NaN;
    const land = sel().w >= sel().h;
    const r = k === 'orig' ? W / H : k === 'sq' ? 1 : k === 'a4' ? 297 / 210 : +k.split(':')[0] / +k.split(':')[1];
    return k === 'orig' || k === 'sq' ? r : land ? r : 1 / r;
  }
  function applyRatio(set = true) {
    const ar = ratioOf(E.ratio);
    if (!set) { cs.aspectRatio = ar; return; }
    const bb = bounds(corners()), b = box();
    const area = { x: Math.max(0, bb.x), y: Math.max(0, bb.y) }; area.w = Math.min(b.w, bb.x + bb.w) - area.x; area.h = Math.min(b.h, bb.y + bb.h) - area.y;
    const r = { ...area };
    if (ar) { if (area.w / area.h > ar) { r.w = area.h * ar; r.x += (area.w - r.w) / 2; } else { r.h = area.w / ar; r.y += (area.h - r.h) / 2; } }
    cs.aspectRatio = NaN; cs.$change(r.x, r.y, r.w, r.h); cs.aspectRatio = ar;
  }
  function turn() {   // a quarter turn clockwise; the crop frame and everything added turn with the picture
    const s = sel(), c = [s.x + s.w / 2, s.y + s.h / 2], r = mMul(mMove(c[0], c[1]), mMul(mTurn(Math.PI / 2), mMove(-c[0], -c[1])));
    fitAll(mMul(r, pic2c()), bounds([[s.x, s.y], [s.x + s.w, s.y + s.h]].map(p => mAt(r, p))));
    if (showsView()) showView();
    commit();
  }
  function flip() {   // mirror left-right around the middle of the frame
    const s = sel(), c = s.x + s.w / 2;
    setPic(mMul(mMul(mMove(c, 0), mMul(mSize(-1, 1), mMove(-c, 0))), pic2c()));
    if (showsView()) showView();
    commit();
  }

  // added things line up with `geo`; after cropping, turning or flipping, move them along with the picture
  function follow() {
    if (!fc || !geo) return;
    const s = sel(), g = geo;
    const t = mMul(mMove(-s.x, -s.y), mMul(pic2c(), mMul(mInv(g.pic), mMove(g.sel.x, g.sel.y))));
    geo = { pic: pic2c(), sel: s };
    if (t.every((v, i) => Math.abs(v - [1, 0, 0, 1, 0, 0][i]) < 1e-9)) return;
    const mirrored = t[0] * t[3] - t[1] * t[2] < 0;
    fc.getObjects().forEach(o => {
      F.util.addTransformToObject(o, t);
      if (mirrored && ['text', 'bubble', 'sign'].includes(o.kind)) o.set({ flipX: !o.flipX });   // words stay readable
      o.setCoords();
    });
  }

  // ---- the finished picture: crop, perspective, Adjust, filter ----
  // the finished picture: crop and perspective, then the Adjust sliders and the filter on its pixels.
  // renderPicture is for the screen; renderFull (saving) spreads the pixel work over all the phone's cores
  function drawCropped(c, k) {   // k = output pixels per frame unit
    const s = sel();
    c.width = Math.max(1, Math.round(s.w * k)); c.height = Math.max(1, Math.round(s.h * k));
    const g = c.getContext('2d', { willReadFrequently: true });
    g.fillStyle = '#fff'; g.fillRect(0, 0, c.width, c.height);
    g.save(); g.setTransform(...mMul(mSize(k), mMul(mMove(-s.x, -s.y), pic2c()))); g.drawImage(img, 0, 0, W, H); g.restore();
    if (E.pv || E.ph) { const tmp = document.createElement('canvas'); tmp.width = c.width; tmp.height = c.height; tmp.getContext('2d').drawImage(c, 0, 0); D().warp(tmp, c, E.pv, E.ph, k > 3 ? 24 : 16); }
    return g;
  }
  const developed = () => Object.values(E.adj).some(v => v) || E.filter !== 'none';
  function renderPicture(c, k) {
    const g = drawCropped(c, k);
    if (developed()) { const d = g.getImageData(0, 0, c.width, c.height); D().develop(d, E.adj, E.filter, E.famt); g.putImageData(d, 0, 0); }
    return c;
  }
  async function renderFull(c, k) {
    const g = drawCropped(c, k);
    if (developed()) { const d = g.getImageData(0, 0, c.width, c.height); await D().developFast(d, E.adj, E.filter, E.famt); g.putImageData(d, 0, 0); }
    return c;
  }
  // the magnifier: a circle on the Markup layer that shows the picture under it, bigger
  function paintMagnifiers(g, pic, k) {   // pic: the finished picture at k output pixels per frame unit
    for (const o of fc.getObjects()) {
      if (o.kind !== 'mag') continue;
      const c = o.getCenterPoint(), r = o.radius * o.scaleX, z = o.zoom || 2;
      g.save(); g.beginPath(); g.arc(c.x * k, c.y * k, r * k, 0, Math.PI * 2); g.clip();
      g.drawImage(pic, (c.x - r / z) * k, (c.y - r / z) * k, 2 * r / z * k, 2 * r / z * k, (c.x - r) * k, (c.y - r) * k, 2 * r * k, 2 * r * k);
      g.restore();
    }
  }
  function drawMagLayer() {
    if (!fc || viewEl.hidden) return;
    const g = magC.getContext('2d');
    magC.width = bg.width; magC.height = bg.height;
    g.clearRect(0, 0, magC.width, magC.height);
    paintMagnifiers(g, bg, bg.width / sel().w);
  }
  // quick: while a finger is on a slider, draw at plain screen resolution (several times less work),
  // then sharp again when it lets go
  let raf = 0;
  function showView(quick) {
    cancelAnimationFrame(raf);
    raf = requestAnimationFrame(() => {
      follow();
      const r = stageEl.getBoundingClientRect(), s = sel(), dpr = quick ? 1 : Math.min(window.devicePixelRatio || 1, 2.5);
      fit = Math.min((r.width - 24) / s.w, (r.height - 24) / s.h);
      const w = s.w * fit, h = s.h * fit;
      renderPicture(bg, fit * dpr);
      for (const c of [bg, magC]) { c.style.width = w + 'px'; c.style.height = h + 'px'; }
      viewEl.style.width = w + 'px'; viewEl.style.height = h + 'px';
      fc.setDimensions({ width: w, height: h });
      fc.setViewportTransform([fit, 0, 0, fit, 0, 0]);
      fc.calcOffset();   // Fabric remembers where its canvas is on screen; tell it after anything moved
      fc.requestRenderAll();
      if (E.tab === 'filters') filterThumbs();
    });
  }

  // ---- Markup (Fabric) ----
  const look = { transparentCorners: false, cornerColor: '#ffffff', cornerStrokeColor: '#9c4a61', borderColor: '#9c4a61', cornerStyle: 'circle',
    cornerSize: 14, touchCornerSize: 34, padding: 16, borderScaleFactor: 2 };
  const lineW = () => unit() * 0.004 * E.size * (E.sub === 'marker' ? 3 : 1);
  const fontSize = () => unit() * 0.012 * (E.size + 3);
  const markerCol = c => { const [r, g, b, a] = rgba(c); return cssOf([r, g, b, 0.38 * a]); };
  function setupFabric() {
    fc = new F.Canvas($e('#edfab'), { preserveObjectStacking: true, selection: false, allowTouchScrolling: false, enableRetinaScaling: true, uniformScaling: true });
    viewEl.fc = fc;   // lets the automated editor test look inside
    // corners resize without squashing, the top handle turns; no side handles: on a thin line they'd sit
    // right where your finger grabs it, so dragging the line would stretch it instead of moving it
    fc.on('object:added', e => { e.target.set(look); e.target.setControlsVisibility({ mt: false, mb: false, ml: false, mr: false }); applyMode(e.target); commit(); });
    fc.on('object:modified', () => commit());
    fc.on('object:removed', () => commit());
    fc.on('text:changed', () => commit());
    fc.on('selection:created', () => opts());
    fc.on('selection:updated', () => opts());
    fc.on('selection:cleared', () => opts());
    fc.on('path:created', e => { (e.path || e.target).set({ kind: E.sub === 'marker' ? 'marker' : 'pen' }); commit(); });
    fc.on('after:render', drawMagLayer);
    fc.on('mouse:down', onDown);
    fc.on('mouse:move', e => { if (erasing && e.target) fc.remove(e.target); });
    fc.on('mouse:up', () => { erasing = false; });
  }
  let erasing = false;
  // only Select can select: while drawing, erasing or adding text, a tap never grabs what's already there
  function applyMode(o) {
    const sel = E.tab === 'markup' && E.sub === 'select', erase = E.tab === 'markup' && E.sub === 'eraser';
    o.selectable = sel; o.evented = sel || erase;
    o.hoverCursor = erase ? 'crosshair' : 'move';
  }
  function onDown(e) {
    if (picking) { const p = e.scenePoint; picking(p); return; }
    if (E.tab !== 'markup') return;
    if (E.sub === 'eraser') { erasing = true; if (e.target) fc.remove(e.target); return; }
    if (E.sub === 'text') return addText(e.scenePoint);
  }
  function place(o) {   // put a new thing on the picture, selected, ready to move and resize
    fc.add(o);
    E.sub = 'select'; fc.getObjects().forEach(applyMode);
    fc.setActiveObject(o); fc.requestRenderAll(); opts();
  }
  async function addText(p) {
    const t = await askText(''); if (!t) return;
    place(new F.IText(t, textLook({ left: p.x, top: p.y })));
  }
  const textLook = (o = {}) => ({ fill: E.color, fontSize: fontSize(), fontWeight: 'bold', fontFamily: 'system-ui, sans-serif',
    stroke: isLight(E.color) ? '#111111' : '#ffffff', strokeWidth: fontSize() * 0.06, paintFirst: 'stroke', kind: 'text', ...o });
  // a speech bubble: the bubble and its words as one thing (Fabric groups sit where their parts are, so the
  // parts are made in place). The tail hangs below the box, so the box's middle is above the shape's middle.
  function makeBubble(text, colour, cx, cy, u) {
    const t = new F.Textbox(text, { width: u * 0.5, fontSize: u * 0.06, fontWeight: 'bold', fontFamily: 'system-ui, sans-serif', fill: colour, textAlign: 'center', originX: 'center', originY: 'center', left: cx, top: cy });
    const w = u * 0.6, h = Math.max(u * 0.22, t.height + u * 0.1);
    const b = new F.Path(bubblePath(w, h), { fill: '#ffffff', stroke: colour, strokeWidth: u * 0.012, strokeLineJoin: 'round', originX: 'center', originY: 'center', left: cx, top: cy + h * 0.17 });
    return new F.Group([b, t], { kind: 'bubble' });
  }
  async function addThing(k) {
    // each new thing lands a little down and to the right of the last one, so they don't hide each other
    const s = sel(), u = unit(), step = (fc.getObjects().length % 5 - 2) * u * 0.07, cx = s.w / 2 + step, cy = s.h / 2 + step, o = { stroke: E.color, strokeWidth: u * 0.012, fill: '', strokeLineCap: 'round', strokeLineJoin: 'round', left: cx, top: cy };
    if (k === 'text') return addText({ x: cx, y: cy });
    if (k === 'bubble') { const t = await askText('Hello!'); if (t) place(makeBubble(t, E.color, cx, cy, u)); return; }
    if (k === 'sign') {
      const st = await signatureSheet(); if (!st) return;
      const w = u * 0.55, h = w * 300 / 720;
      const ox = cx - w / 2, oy = cy - h / 2;
      const paths = st.filter(x => x.length).map(x => new F.Path('M ' + x.map(([a, b]) => `${ox + a * w} ${oy + b * h}`).join(' L ') + (x.length === 1 ? ` L ${ox + x[0][0] * w + 0.5} ${oy + x[0][1] * h}` : ''),
        { stroke: isLight(E.color) ? '#111111' : E.color, strokeWidth: w * 0.012, fill: '', strokeLineCap: 'round', strokeLineJoin: 'round' }));
      return place(new F.Group(paths, { kind: 'sign' }));
    }
    if (k === 'mag') return place(new F.Circle({ radius: u * 0.16, left: cx, top: cy, fill: 'rgba(0,0,0,0)', stroke: '#ffffff', strokeWidth: u * 0.012, zoom: 2, kind: 'mag',
      shadow: new F.Shadow({ color: 'rgba(0,0,0,.45)', blur: u * 0.02 }) }));
    const L = u * 0.36;
    const shape = k === 'rect' ? new F.Rect({ ...o, width: L, height: L * 0.75, kind: 'rect' })
      : k === 'circle' ? new F.Ellipse({ ...o, rx: L / 2, ry: L / 2, kind: 'circle' })
      : k === 'arrow' ? new F.Path(arrowPath(-L / 2, L / 4, L / 2, -L / 4, o.strokeWidth), { ...o, kind: 'arrow' })
      : k === 'line' ? new F.Path(`M ${-L / 2} 0 L ${L / 2} 0`, { ...o, kind: 'line' })
      : new F.Path(starPath(L / 2), { ...o, kind: 'star' });
    place(shape);
  }
  function addSheet() {
    return new Promise(res => {
      const q = modal(`<div class="csheet"><div class="chead"><b>Add</b><button class="btn" data-x="cancel">Cancel</button></div>
        <div class="addgrid">${ED_ADD.map(([k, n, l]) => `<button data-k="${k}">${icon(n)}<span>${l}</span></button>`).join('')}</div></div>`);
      q.el.classList.add('sheetmodal');
      q.el.querySelectorAll('[data-k]').forEach(b => b.onclick = () => { q.close(); res(b.dataset.k); });
      q.el.querySelector('[data-x=cancel]').onclick = () => { q.close(); res(null); };
    });
  }
  function askText(val) {
    return new Promise(res => {
      const q = modal(`<h2>Type the text</h2><textarea class="big" id="edtxt" rows="3" maxlength="400" placeholder="Your text">${esc(val)}</textarea>
        <div class="foot"><button class="btn" data-c="0">Cancel</button><button class="btn main" data-c="1">${val ? 'OK' : 'Add'}</button></div>`);
      const inp = q.el.querySelector('#edtxt'); setTimeout(() => { inp.focus(); inp.select(); }, 50);
      const done = ok => { const v = inp.value.replace(/\s+$/, ''); q.close(); res(ok && v.trim() ? v : null); };
      q.el.querySelectorAll('[data-c]').forEach(b => b.onclick = () => done(b.dataset.c === '1'));
    });
  }
  const active = () => fc && fc.getActiveObject();
  const parts = o => o.kind === 'bubble' || o.kind === 'sign' ? o.getObjects() : [o];
  function recolour(o, c) {
    if (o.kind === 'text') o.set({ fill: c, stroke: isLight(c) ? '#111111' : '#ffffff' });
    else if (o.kind === 'marker') o.set({ stroke: markerCol(c) });
    else if (o.kind === 'bubble') { const [b, t] = o.getObjects(); b.set({ stroke: c }); t.set({ fill: c }); o.set({ dirty: true }); }
    else if (o.kind === 'sign') { o.getObjects().forEach(p => p.set({ stroke: c })); o.set({ dirty: true }); }
    else o.set({ stroke: c });
  }
  const colourOf = o => { const p = parts(o)[0]; const c = o.kind === 'text' ? o.fill : p.stroke; if (o.kind !== 'marker') return c; const [r, g, b, a] = rgba(c); return cssOf([r, g, b, Math.min(1, a / 0.38)]); };
  // the size slider: thickness for lines and shapes, size for text, zoom for the magnifier
  function resize(o, n) {
    const u = unit();
    if (o.kind === 'text') o.set({ fontSize: u * 0.012 * (n + 3) / (o.scaleY || 1), strokeWidth: u * 0.012 * (n + 3) * 0.06 / (o.scaleY || 1) });
    else if (o.kind === 'mag') o.set({ zoom: 1 + n * 0.3 });
    else if (o.kind === 'bubble' || o.kind === 'sign') { parts(o).forEach(p => { if (p.type !== 'textbox') p.set({ strokeWidth: u * 0.004 * n / (o.scaleX || 1) }); }); o.set({ dirty: true }); }
    else o.set({ strokeWidth: u * 0.004 * n * (o.kind === 'marker' ? 3 : 1) / (o.scaleX || 1) });
  }
  function sizeOf(o) {
    const u = unit();
    if (o.kind === 'mag') return Math.round(((o.zoom || 2) - 1) / 0.3);
    const p = parts(o).find(x => x.type !== 'textbox') || o;
    const n = o.kind === 'text' ? o.fontSize * (o.scaleY || 1) / (u * 0.012) - 3 : p.strokeWidth * (o.scaleX || 1) / (u * 0.004 * (o.kind === 'marker' ? 3 : 1));
    return Math.max(1, Math.min(10, Math.round(n)));
  }

  // ---- undo / redo: every change is a snapshot of everything (small: numbers and the added things) ----
  function snap() {
    return JSON.stringify({ box: box(), tf: tf(), sel: sel(), angle: E.angle, ratio: E.ratio, pv: E.pv, ph: E.ph, adj: E.adj, filter: E.filter, famt: E.famt, geo,
      objs: fc ? fc.toObject(['kind', 'zoom']).objects : [] });
  }
  function commit(now) {
    if (quiet || !ci) return;
    clearTimeout(commitT);
    const go = () => { const s = snap(); if (hist[at] === s) return; hist.length = at + 1; hist.push(s); at = hist.length - 1; if (hist.length > 80) { hist.shift(); at--; } updUndo(); autosave(); };
    now ? go() : commitT = setTimeout(go, 40);
  }
  function updUndo() { $e('[data-x=undo]').disabled = at <= 0; $e('[data-x=redo]').disabled = at >= hist.length - 1; }
  async function restore(i, loading) {
    const o = JSON.parse(hist[i]); at = i; quiet++;
    let k = 1;
    const ob = o.box, nb = box();
    if (ob && (Math.abs(ob.w - nb.w) > 0.5 || Math.abs(ob.h - nb.h) > 0.5)) {   // made on another screen (or turned phone): fit it to this one
      k = Math.min(nb.w / ob.w, nb.h / ob.h);
      const g = mMul(mMove(nb.w / 2, nb.h / 2), mMul(mSize(k), mMove(-ob.w / 2, -ob.h / 2)));
      const fitR = r => { const [x, y] = mAt(g, [r.x, r.y]); return { x, y, w: r.w * k, h: r.h * k }; };
      o.tf = canvasToTf(mMul(g, picToCanvas(o.tf, W, H)), W, H); o.sel = fitR(o.sel);
      if (o.geo) o.geo = { pic: mMul(g, o.geo.pic), sel: fitR(o.geo.sel) };
    }
    try {
      ci.$setTransform(o.tf); cs.aspectRatio = NaN; cs.$change(o.sel.x, o.sel.y, o.sel.w, o.sel.h);
      Object.assign(E, { angle: o.angle, ratio: o.ratio, pv: o.pv, ph: o.ph, adj: o.adj, filter: o.filter, famt: o.famt });
      applyRatio(false);
      fc.discardActiveObject(); fc.clear();
      const objs = await F.util.enlivenObjects(o.objs);
      objs.forEach(x => { if (k !== 1) F.util.addTransformToObject(x, mSize(k)); fc.add(x); });   // added things are in frame units: same scale
      geo = o.geo || { pic: pic2c(), sel: sel() };
    } finally { quiet--; }
    updUndo(); opts(); if (showsView()) showView();
    if (!loading) autosave();
  }
  $e('[data-x=undo]').onclick = () => { settle(); if (at > 0) restore(at - 1); };
  $e('[data-x=redo]').onclick = () => { if (at < hist.length - 1) restore(at + 1); };
  function onKey(e) {
    if (e.target.closest && e.target.closest('input,textarea')) return;
    const o = active();
    if ((e.key === 'Delete' || e.key === 'Backspace') && o && !o.isEditing) { fc.remove(o); fc.discardActiveObject(); fc.requestRenderAll(); }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'z') { e.preventDefault(); e.shiftKey ? $e('[data-x=redo]').click() : $e('[data-x=undo]').click(); }
  }
  document.addEventListener('keydown', onKey);

  // ---- autosave: the recipe and the undo history go to the laptop a moment after each change ----
  let saveT = 0;
  function recipe() {   // the last 20 steps are plenty to undo through, and keep each autosave small
    let from = Math.max(0, hist.length - 20);
    if (at < from) from = Math.max(0, at - 5);
    return { hist: hist.slice(from, from + 20), at: at - from };
  }
  function autosave(now) {
    clearTimeout(saveT);
    const go = () => api('/api/edit/' + src.id, { state: recipe() }).catch(() => {});   // quietly: it's a safety net
    if (now) return go();
    saveT = setTimeout(go, 900);
  }
  function banner() {
    const b = document.createElement('div');
    b.className = 'edbanner';
    b.innerHTML = `Picked up where you left off. Undo goes back further. <button class="btn">Start over</button>`;
    stageEl.appendChild(b);
    b.querySelector('button').onclick = async () => {
      if (!await ask('<h2>Start over?</h2><p>This forgets the edits and opens the original picture.</p>', [['yes', 'Start over']])) return;
      await api('/api/edit/' + src.id, null, 'DELETE').catch(() => {});
      clearTimeout(saveT); close(); openEditor(src, onSave);
    };
    setTimeout(() => b.classList.add('gone'), 7000);
    setTimeout(() => b.remove(), 7600);
  }

  // ---- copy & paste edits: the Adjust + filter + perspective settings, to use on the next photo ----
  const copyEdits = () => { try { localStorage.setItem('sakuraEdits', JSON.stringify({ adj: E.adj, filter: E.filter, famt: E.famt, pv: E.pv, ph: E.ph })); toast('Edits copied'); } catch (_) {} };
  const pastedEdits = () => { try { return JSON.parse(localStorage.getItem('sakuraEdits') || 'null'); } catch (_) { return null; } };

  // ---- filter previews: each filter on a small copy of the picture ----
  let thumbsFor = '';
  function filterThumbs() {
    const row = $e('#fthumbs'); if (!row) return;
    const key = JSON.stringify([bg.width, bg.height, E.adj, E.pv, E.ph, sel(), tf()]);
    if (key === thumbsFor) return;
    thumbsFor = key;
    const s = sel(), k = 72 / Math.max(s.w, s.h);
    const small = document.createElement('canvas'); small.width = Math.max(1, Math.round(s.w * k)); small.height = Math.max(1, Math.round(s.h * k));
    const sg = small.getContext('2d'); sg.fillStyle = '#fff'; sg.fillRect(0, 0, small.width, small.height);
    sg.setTransform(...mMul(mSize(k), mMul(mMove(-s.x, -s.y), pic2c()))); sg.drawImage(img, 0, 0, W, H); sg.setTransform(1, 0, 0, 1, 0, 0);
    row.querySelectorAll('canvas[data-f]').forEach(c => {
      c.width = small.width; c.height = small.height;
      const g = c.getContext('2d'); g.drawImage(small, 0, 0);
      const d = g.getImageData(0, 0, c.width, c.height); D().develop(d, E.adj, c.dataset.f, 100); g.putImageData(d, 0, 0);
    });
  }

  // ---- the options under the picture ----
  const swatches = cur => {
    const now = cssOf(rgba(cur));
    return `<div class="edcolors">${ED_COLORS.map(c => `<button data-col="${c}" class="${c === now ? 'on' : ''}" style="background:${c}" aria-label="colour"></button>`).join('')}
    <button class="edring ${ED_COLORS.includes(now) ? '' : 'on'}" data-x="more" style="--c:${esc(now)}" aria-label="More colours"></button></div>`;
  };
  const slider = (v, label, id = 'edsize', min = 1, max = 10) => `<label class="edslide"><span>${label}</span><input type="range" id="${id}" min="${min}" max="${max}" step="1" value="${v}"><b class="edval">${v}</b></label>`;
  const chip = (attr, v, cur, label) => `<button class="btn ${cur === v ? 'on' : ''}" ${attr}="${v}">${label}</button>`;
  const ring = (a, v) => {   // the round buttons on Adjust: a ring that fills with the value, like the iPhone
    const r = 17, C = 2 * Math.PI * r, part = Math.abs(v) / Math.max(Math.abs(a.min), a.max);
    return `<button class="adjbtn ${E.param === a.key ? 'on' : ''}" data-p="${a.key}"><svg viewBox="0 0 40 40"><circle cx="20" cy="20" r="${r}" class="rbase"/>
      ${v ? `<circle cx="20" cy="20" r="${r}" class="rval" stroke-dasharray="${C * part} ${C}" transform="translate(20 20) rotate(-90)${v < 0 ? ' scale(1 -1)' : ''} translate(-20 -20)"/>` : ''}
      <text x="20" y="24.5" text-anchor="middle">${v ? Math.round(v) : '·'}</text></svg><span>${a.label}</span></button>`;
  };
  function opts() {
    if (!ci) return;
    const o = $e('#edopts'), a = E.tab === 'markup' && active();
    m.el.querySelectorAll('.edtools [data-tab]').forEach(b => b.classList.toggle('on', b.dataset.tab === E.tab));
    const view = showsView();
    cropEl.classList.toggle('off', view); viewEl.hidden = !view;   // hidden, but keeps its size so the maths still works
    if (fc) {
      fc.isDrawingMode = E.tab === 'markup' && (E.sub === 'pen' || E.sub === 'marker');
      if (fc.isDrawingMode) {
        fc.freeDrawingBrush = fc.freeDrawingBrush || new F.PencilBrush(fc);
        fc.freeDrawingBrush.color = E.sub === 'marker' ? markerCol(E.color) : E.color;
        fc.freeDrawingBrush.width = lineW();
        fc.freeDrawingBrush.strokeLineCap = E.sub === 'marker' ? 'square' : 'round';
      }
      fc.getObjects().forEach(applyMode);
      fc.defaultCursor = E.sub === 'eraser' ? 'crosshair' : E.sub === 'text' ? 'text' : 'default';
    }
    const subs = [['pen', 'pen', 'Pen'], ['marker', 'marker', 'Highlight'], ['eraser', 'eraser', 'Eraser'], ['select', 'pointer', 'Select'], ['text', 'text', 'Text'], ['add', 'plus', 'Add']];
    const subRow = `<div class="edsubs">${subs.map(([k, n, t]) => `<button data-sub="${k}" class="${E.sub === k ? 'on' : ''}">${ic(n)}<span>${t}</span></button>`).join('')}</div>`;
    let h = '';
    if (E.tab === 'crop') {
      h = `<div class="edrow"><button class="btn" data-turn>${ic('turn')} Turn</button><button class="btn" data-flip>${ic('flip')} Flip</button><button class="btn" data-reset>Reset</button></div>
        <div class="edscroll">${ED_RATIOS.map(([k, l]) => chip('data-ratio', k, E.ratio, l)).join('')}</div>
        <div class="seg mini edseg">${[['angle', 'Straighten'], ['pv', 'Vertical'], ['ph', 'Horizontal']].map(([k, l]) => `<button data-st="${k}" class="${E.straight === k ? 'on' : ''}">${l}</button>`).join('')}</div>
        ${E.straight === 'angle' ? `<label class="edslide"><span class="edhint">${ic('turnL')}</span><input type="range" id="edang" min="-15" max="15" step="0.5" value="${E.angle}"><span class="edhint">${ic('turn')}</span></label>`
          : slider(E[E.straight], E.straight === 'pv' ? 'Vertical' : 'Horizontal', 'edpersp', -100, 100)}`;
    } else if (E.tab === 'adjust') {
      const A = D().ADJUST, cur = A.find(x => x.key === E.param);
      h = `<div class="edscroll adjrow">${A.map(x => ring(x, E.adj[x.key] || 0)).join('')}</div>
        ${slider(Math.round(E.adj[cur.key] || 0), cur.label, 'edadj', cur.min, cur.max)}
        <div class="edrow">${pastedEdits() ? '<button class="btn" data-x="paste">Paste edits</button>' : ''}<button class="btn" data-x="copy">Copy edits</button><button class="btn" data-x="resetadj">Reset</button></div>`;
    } else if (E.tab === 'filters') {
      h = `<div class="edscroll fthumbs" id="fthumbs">${D().FILTERS.map(f => `<button class="fthumb ${E.filter === f.key ? 'on' : ''}" data-f="${f.key}"><canvas data-f="${f.key}"></canvas><span>${f.label}</span></button>`).join('')}</div>
        ${E.filter === 'none' ? '<span class="edhint">Pick a look. Tap it again to change how strong it is.</span>' : slider(E.famt, 'Strength', 'edfamt', 0, 100)}`;
    } else if (a) {   // Markup, something selected: change it
      const k = a.kind;
      h = `${subRow}<div class="edrow">${k === 'mag' ? '<span class="edhint">Move it over what you want to show bigger</span>' : swatches(colourOf(a))}</div>
        ${slider(sizeOf(a), k === 'text' ? 'Size' : k === 'mag' ? 'Zoom' : 'Thickness')}
        <div class="edrow">${k === 'text' || k === 'bubble' ? `<button class="btn" data-act="edit">${ic('edit')} Words</button>` : ''}
          <button class="btn" data-act="dup" aria-label="Copy">${ic('copy')}</button><button class="btn" data-act="del" aria-label="Delete">${ic('trash')}</button>
          <button class="btn on" data-act="done">${ic('check')} Done</button></div>`;
    } else {
      const hint = { eraser: 'Swipe over anything you added to rub it out', select: 'Tap something you added to move it, resize it, turn it, or change it', text: 'Tap where the text goes' }[E.sub];
      h = `${subRow}<div class="edrow">${E.sub === 'eraser' || E.sub === 'select' ? `<span class="edhint">${hint}</span>` : swatches(E.color)}</div>
        ${E.sub === 'pen' || E.sub === 'marker' || E.sub === 'text' ? slider(E.size, E.sub === 'text' ? 'Size' : 'Thickness') : ''}
        ${E.sub === 'text' ? `<span class="edhint">${hint}</span>` : ''}`;
    }
    o.innerHTML = h;
    wireOpts(o, a);
    if (E.tab === 'filters') filterThumbs();
  }
  function wireOpts(o, a) {
    const q = s => o.querySelector(s), qa = s => o.querySelectorAll(s);
    const redraw = () => { if (showsView()) showView(); };
    // crop
    if (q('[data-turn]')) q('[data-turn]').onclick = turn;
    if (q('[data-flip]')) q('[data-flip]').onclick = flip;
    if (q('[data-reset]')) q('[data-reset]').onclick = () => {
      Object.assign(E, { ratio: 'free', angle: 0, pv: 0, ph: 0 }); ci.$resetTransform(); fitAll(); opts(); redraw(); commit();
    };
    qa('[data-ratio]').forEach(b => b.onclick = () => { E.ratio = b.dataset.ratio; applyRatio(); opts(); redraw(); commit(); });
    qa('[data-st]').forEach(b => b.onclick = () => { E.straight = b.dataset.st; opts(); redraw(); });
    const ang = q('#edang');
    if (ang) {
      ang.oninput = () => {   // turn around the middle of the frame, then zoom just enough to fill it
        const s = sel(), c = [s.x + s.w / 2, s.y + s.h / 2], d = (+ang.value - E.angle) * Math.PI / 180;
        E.angle = +ang.value;
        setPic(mMul(mMul(mMove(c[0], c[1]), mMul(mTurn(d), mMove(-c[0], -c[1]))), pic2c()));
        coverFrame();
      };
      ang.onchange = () => commit();
    }
    const live = (id, set, then) => { const r = q(id); if (!r) return; const val = r.parentElement.querySelector('.edval');
      r.oninput = () => { set(+r.value); if (val) val.textContent = r.value; if (then) then(); else if (showsView()) showView(true); };
      r.onchange = () => { commit(); if (E.tab === 'adjust') opts(); redraw(); }; };
    live('#edpersp', v => { E[E.straight] = v; });
    // adjust
    qa('[data-p]').forEach(b => b.onclick = () => { E.param = b.dataset.p; opts(); });
    live('#edadj', v => { E.adj = { ...E.adj, [E.param]: v }; });
    if (q('[data-x=copy]')) q('[data-x=copy]').onclick = copyEdits;
    if (q('[data-x=paste]')) q('[data-x=paste]').onclick = () => { const p = pastedEdits(); if (!p) return; Object.assign(E, p); opts(); redraw(); commit(); toast('Edits pasted'); };
    if (q('[data-x=resetadj]')) q('[data-x=resetadj]').onclick = () => { E.adj = {}; opts(); redraw(); commit(); };
    // filters
    qa('.fthumb').forEach(b => b.onclick = () => { if (E.filter !== b.dataset.f) { E.filter = b.dataset.f; E.famt = 100; opts(); redraw(); commit(); } });
    live('#edfamt', v => { E.famt = v; });
    // markup
    qa('[data-sub]').forEach(b => b.onclick = async () => {
      if (b.dataset.sub === 'add') {
        const k = await addSheet();
        if (k) addThing(k);
        return;
      }
      E.sub = b.dataset.sub; fc.discardActiveObject(); fc.requestRenderAll(); opts();
    });
    const setColor = c => { if (a) { recolour(a, c); fc.requestRenderAll(); commit(); } else E.color = c; opts(); };
    qa('[data-col]').forEach(b => b.onclick = () => setColor(b.dataset.col));
    if (q('[data-x=more]')) q('[data-x=more]').onclick = async () => {
      const c = await colourSheet(a ? colourOf(a) : E.color, true);
      if (!c) return;
      if (c.pick) {   // the eyedropper: the next tap on the picture takes its colour
        toast('Tap the photo to take a colour');
        const target = a, drawing = fc.isDrawingMode;
        fc.isDrawingMode = false;
        picking = p => {
          picking = null; fc.isDrawingMode = drawing;
          const k = bg.width / sel().w, x = Math.max(0, Math.min(bg.width - 1, Math.round(p.x * k))), y = Math.max(0, Math.min(bg.height - 1, Math.round(p.y * k)));
          const d = bg.getContext('2d').getImageData(x, y, 1, 1).data;
          const col = cssOf([d[0], d[1], d[2], c.alpha]); rememberColour(col);
          if (target) { fc.setActiveObject(target); recolour(target, col); fc.requestRenderAll(); commit(); } else E.color = col;
          opts();
        };
        return;
      }
      setColor(c);
    };
    const sz = q('#edsize');
    if (sz) {
      const val = sz.parentElement.querySelector('.edval');
      sz.oninput = () => { if (val) val.textContent = sz.value; if (a) { resize(a, +sz.value); a.setCoords(); fc.requestRenderAll(); } else { E.size = +sz.value; if (fc.isDrawingMode) fc.freeDrawingBrush.width = lineW(); } };
      sz.onchange = () => { if (a) commit(); };
    }
    const act = k => q(`[data-act=${k}]`);
    if (act('done')) act('done').onclick = () => { fc.discardActiveObject(); fc.requestRenderAll(); opts(); };
    if (act('del')) act('del').onclick = () => { fc.remove(a); fc.discardActiveObject(); fc.requestRenderAll(); };
    if (act('dup')) act('dup').onclick = async () => { const c = await a.clone(['kind', 'zoom']); c.set({ left: a.left + unit() * 0.05, top: a.top + unit() * 0.05 }); fc.add(c); fc.setActiveObject(c); fc.requestRenderAll(); };
    if (act('edit')) act('edit').onclick = async () => {
      if (a.kind === 'bubble') {
        const t = await askText(a.getObjects()[1].text); if (!t) return;
        const nb = makeBubble(t, a.getObjects()[1].fill, 0, 0, unit());
        nb.set({ left: a.left, top: a.top, angle: a.angle, scaleX: a.scaleX, scaleY: a.scaleY, flipX: a.flipX });
        quiet++; fc.remove(a); quiet--; fc.add(nb); fc.setActiveObject(nb); fc.requestRenderAll(); return;
      }
      const t = await askText(a.text); if (t) { a.set({ text: t }); a.setCoords(); fc.requestRenderAll(); commit(); }
    };
  }
  m.el.querySelectorAll('.edtools [data-tab]').forEach(b => b.onclick = () => {
    if (!ci) return;
    if (E.tab === 'crop') follow();
    E.tab = b.dataset.tab;
    if (E.tab === 'crop') E.straight = 'angle';
    if (fc) { fc.discardActiveObject(); }
    opts();
    if (showsView()) showView();
  });

  $e('[data-x=save]').onclick = () => guard(async () => {
    if (!ci) return;
    settle();
    if (at <= 0) return close();   // nothing changed
    // full print quality takes a few seconds on a phone: say so straight away, and let the screen show it
    const wait = modal(`${bigIcon('check')}<h2 style="text-align:center">Saving…</h2><div class="spin"></div><p class="hint" style="text-align:center">Making it sharp enough to print.</p>`);
    await new Promise(r => requestAnimationFrame(() => setTimeout(r, 30)));
    try { await saveNow(); } finally { wait.close(); }
  });
  async function saveNow() {
    follow();
    fc.discardActiveObject();
    const s = sel(), k = Math.min(ED_FULL / Math.max(s.w, s.h), 1 / scaleOf());   // never bigger than the photo really is
    const out = await renderFull(document.createElement('canvas'), k);
    if (fc.getObjects().length) {
      if (viewEl.hidden) { fit = 1; fc.setDimensions({ width: s.w, height: s.h }); fc.setViewportTransform([1, 0, 0, 1, 0, 0]); }
      if (fc.getObjects().some(o => o.kind === 'mag')) {   // magnifiers show the finished picture, before anything is drawn on it
        const copy = document.createElement('canvas'); copy.width = out.width; copy.height = out.height; copy.getContext('2d').drawImage(out, 0, 0);
        paintMagnifiers(out.getContext('2d'), copy, k);
      }
      out.getContext('2d').drawImage(fc.toCanvasElement(k / fit), 0, 0, out.width, out.height);   // drawn fresh at full size, so it's sharp
    }
    const blob = await new Promise(r => out.toBlob(r, 'image/jpeg', 0.92));
    if (!blob) return toast('Couldn\'t save the picture', true);
    const name = src.name.replace(/\.[^.]+$/, '').replace(/ \(edited\)$/, '') + ' (edited).jpg';
    const files = await new Promise(res => upload([new File([blob], name, { type: 'image/jpeg' })], res));
    if (!files.length) return;
    clearTimeout(saveT);   // the finished picture remembers its recipe, so it can be edited again from the original
    await api('/api/edit/' + src.id, { state: recipe(), result: files[0].id }).catch(() => {});
    close();
    onSave(files[0]);
  }
}
