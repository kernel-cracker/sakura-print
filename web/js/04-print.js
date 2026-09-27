// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 04: Photos, the arrange-it-yourself layout, THE print screen, paper helpers and printing.
'use strict';

// ---------- photos ----------
function swapFile(list, i, nf) {
  const old = list[i]; list[i] = nf;
  const p = S.pho;
  (p.placed || []).forEach(x => { if (x.id === old.id) x.id = nf.id; });
  if (p.known) p.known.add(nf.id);
}

// ---------- the arrange-it-yourself editor ----------
// Positions are fractions of the page (0..1), so they mean the same thing on a phone and on paper.
const SNAP = 0.015;
function snapTo(v, targets) { for (const t of targets) if (Math.abs(v - t) < SNAP) return { v: t, hit: t }; return { v, hit: null }; }

function autoPlace(p, ids) {   // a tidy starting grid, only for photos the editor hasn't seen yet
  p.placed = p.placed || [];
  p.known = p.known || new Set();
  const todo = ids.filter(id => !p.known.has(id));
  todo.forEach(id => p.known.add(id));
  const cols = todo.length > 4 ? 3 : todo.length > 1 ? 2 : 1, rows = Math.ceil(todo.length / cols) || 1;
  const pad = 0.05, gap = 0.03, cw = (1 - 2 * pad - (cols - 1) * gap) / cols, chh = Math.min(0.42, (1 - 2 * pad - (rows - 1) * gap) / rows);
  todo.forEach((id, i) => p.placed.push({ id, page: p.page || 0, x: pad + (i % cols) * (cw + gap), y: pad + Math.floor(i / cols) * (chh + gap), w: cw, h: chh, rot: 0, whole: false }));
}

function editor(box) {
  const p = S.pho;
  p.page = p.page || 0;
  autoPlace(p, S.photos.map(f => f.id));
  const [pw, ph] = paperMM(p.paper);
  const off = S.photos.filter(f => !p.placed.some(x => x.id === f.id));
  const pages = Math.max(1, ...p.placed.map(x => x.page + 1), p.page + 1);
  box.innerHTML = `<h2>Arrange on the page</h2>
    <div class="ptabs">${Array.from({ length: pages }, (_, i) => `<button data-pg="${i}" class="${i === p.page ? 'on' : ''}">Page ${i + 1}</button>`).join('')}
      <button data-pg="new" title="Add a page">${ic('plus')}</button></div>
    <div class="stagewrap"><div class="stage" id="stage" style="aspect-ratio:${pw}/${ph}"><i class="snapline gv"></i><i class="snapline gh"></i></div></div>
    <div class="etools" id="etools"><span class="hint" style="margin:0">Tap a photo to change it. Drag to move, pull the corner to resize.</span></div>
    ${off.length ? `<div class="offtray"><span class="lbl">Not on any page</span><div class="offrow">${off.map(f => `<button class="offph" data-put="${f.id}" title="Put it on this page">
      <img src="/api/thumb/${f.id}" alt=""><span>${ic('plus')} put back</span></button>`).join('')}</div></div>` : ''}`;
  const stage = box.querySelector('#stage');
  box.querySelectorAll('[data-pg]').forEach(b => b.onclick = () => {
    p.page = b.dataset.pg === 'new' ? pages : +b.dataset.pg; p.sel = null; editor(box);
  });
  const here = p.placed.filter(x => x.page === p.page);
  here.forEach(it => {
    const el = document.createElement('div');
    el.className = 'pl' + (p.sel === it ? ' sel' : '');
    el.innerHTML = `<div class="clip"><img src="/api/thumb/${it.id}" draggable="false" alt=""></div><i class="hnd"></i>`;
    stage.appendChild(el);
    it.el = el;
    el.style.zIndex = p.placed.indexOf(it) + 1;
    place(it);
    const img = el.querySelector('img');
    img.onload = () => { it.ar = img.naturalWidth / img.naturalHeight; place(it); };
    el.addEventListener('pointerdown', e => grab(e, it, e.target.classList.contains('hnd') ? 'size' : 'move'));
  });
  if (!here.length) stage.insertAdjacentHTML('beforeend', `<div class="emptypage">Empty page.<br><button class="btn" id="bringAll">Put the other photos here</button></div>`);
  box.querySelectorAll('[data-put]').forEach(b => b.onclick = () => {
    p.placed.push({ id: b.dataset.put, page: p.page, x: 0.25, y: 0.25, w: 0.5, h: 0.35, rot: 0, whole: false });
    p.sel = p.placed[p.placed.length - 1]; editor(box);
  });
  const ba = box.querySelector('#bringAll'); if (ba) ba.onclick = () => { p.placed.forEach(x => x.page = p.page); editor(box); };
  tools();

  function place(it) {
    const e = it.el; if (!e) return;
    e.style.left = it.x * 100 + '%'; e.style.top = it.y * 100 + '%';
    e.style.width = it.w * 100 + '%'; e.style.height = it.h * 100 + '%';
    const img = e.querySelector('img'), side = it.rot % 180 === 90;
    const W = e.clientWidth || 1, H = e.clientHeight || 1;
    // turned photos: size the picture to the frame's swapped shape, then rotate it into place
    img.style.width = (side ? H : W) + 'px'; img.style.height = (side ? W : H) + 'px';
    img.style.objectFit = it.whole ? 'contain' : 'cover';
    img.style.transform = `translate(-50%, -50%) rotate(${it.rot}deg)`;
  }

  function grab(e, it, how) {
    e.preventDefault(); e.stopPropagation();
    p.sel = it; stage.querySelectorAll('.pl').forEach(x => x.classList.toggle('sel', x === it.el)); tools();
    const R = stage.getBoundingClientRect(), sx = e.clientX, sy = e.clientY, s0 = { ...it };
    // bring to front with z-index instead of moving the element (moving it would drop the finger's grip)
    p.placed.splice(p.placed.indexOf(it), 1); p.placed.push(it);
    p.placed.forEach((o, i) => { if (o.el) o.el.style.zIndex = i + 1; });
    it.el.setPointerCapture(e.pointerId); it.el.classList.add('active');
    const gv = stage.querySelector('.gv'), gh = stage.querySelector('.gh');
    let raf = 0;
    const move = ev => {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => {
        const dx = (ev.clientX - sx) / R.width, dy = (ev.clientY - sy) / R.height;
        gv.style.display = gh.style.display = 'none';
        if (how === 'move') {
          const others = p.placed.filter(o => o !== it && o.page === it.page);
          const xs = [0, 0.5, 1, ...others.flatMap(o => [o.x, o.x + o.w, o.x + o.w / 2])];
          const ys = [0, 0.5, 1, ...others.flatMap(o => [o.y, o.y + o.h, o.y + o.h / 2])];
          let x = s0.x + dx, y = s0.y + dy;
          // snap the left edge, the middle or the right edge, whichever is closest to a line
          for (const [off, k] of [[0, 'l'], [s0.w / 2, 'c'], [s0.w, 'r']]) { const r = snapTo(x + off, xs); if (r.hit !== null) { x = r.v - off; gv.style.left = r.hit * 100 + '%'; gv.style.display = 'block'; break; } }
          for (const [off] of [[0], [s0.h / 2], [s0.h]]) { const r = snapTo(y + off, ys); if (r.hit !== null) { y = r.v - off; gh.style.top = r.hit * 100 + '%'; gh.style.display = 'block'; break; } }
          it.x = Math.min(1 - 0.04, Math.max(-it.w + 0.04, x));
          it.y = Math.min(1 - 0.04, Math.max(-it.h + 0.04, y));
        } else {
          // resize from the corner, keeping the photo's shape so it never looks stretched
          const ar = (it.ar ? (it.rot % 180 === 90 ? 1 / it.ar : it.ar) : s0.w * R.width / (s0.h * R.height));
          let w = Math.max(0.08, s0.w + dx);
          let h = (w * R.width / ar) / R.height;
          if (h < 0.06) { h = 0.06; w = h * R.height * ar / R.width; }
          it.w = Math.min(w, 1.2); it.h = Math.min(h, 1.2);
        }
        place(it);
      });
    };
    const up = () => {
      cancelAnimationFrame(raf);
      it.el.removeEventListener('pointermove', move); it.el.removeEventListener('pointerup', up); it.el.removeEventListener('pointercancel', up);
      it.el.classList.remove('active'); gv.style.display = gh.style.display = 'none';
    };
    it.el.addEventListener('pointermove', move); it.el.addEventListener('pointerup', up); it.el.addEventListener('pointercancel', up);
  }

  function tools() {
    const t = box.querySelector('#etools'); const it = p.sel;
    if (!it || it.page !== p.page) { t.innerHTML = '<span class="hint" style="margin:0">Tap a photo to change it. Drag to move, pull the corner to resize.</span>'; return; }
    t.innerHTML = `<button class="btn" data-t="rot">${ic('turn')} Turn</button><button class="btn" data-t="whole">${it.whole ? ic('fill') + ' Fill frame' : ic('whole') + ' Whole photo'}</button>
      <button class="btn" data-t="big">${ic('expand')} Fill page</button><button class="btn danger" data-t="del">${ic('x')} Take off</button>`;
    t.querySelectorAll('[data-t]').forEach(b => b.onclick = () => {
      const k = b.dataset.t;
      if (k === 'rot') {   // turn around the middle, swapping width and height
        const cx = it.x + it.w / 2, cy = it.y + it.h / 2, R = stage.getBoundingClientRect();
        const nw = it.h * R.height / R.width, nh = it.w * R.width / R.height;
        it.rot = (it.rot + 90) % 360; it.w = nw; it.h = nh; it.x = cx - nw / 2; it.y = cy - nh / 2;
      } else if (k === 'whole') it.whole = !it.whole;
      else if (k === 'big') { Object.assign(it, { x: 0.04, y: 0.04, w: 0.92, h: 0.92 }); }
      else if (k === 'del') { p.placed.splice(p.placed.indexOf(it), 1); p.sel = null; return editor(box); }
      place(it); tools();
    });
  }
}

function paperMM(v) {   // mirror of the server's paper sizes, just for the editor's page shape
  const n = String(v || 'A4').toLowerCase().replace(/_[bs]$/, '').replace(/\.borderless$/, '').replace(/^br(?=[a-z0-9])/, '');
  const t = { a4: [210, 297], letter: [215.9, 279.4], legal: [215.9, 355.6], a5: [148, 210], a6: [105, 148], b5: [182, 257], postc4x6: [101.6, 152.4],
    '4x6': [101.6, 152.4], photol: [89, 127], '3.5x5': [89, 127], photo2l: [127, 178], '5x7': [127, 178], postcard: [100, 148] };
  return t[n] || [210, 297];
}

function photoSpec() {
  const p = S.pho;
  const o = printOptions({ paper: p.paper, colour: p.colour, quality: p.quality, media: p.media });
  if (p.layout === 'custom') {
    const placed = (p.placed || []).filter(x => S.photos.some(f => f.id === x.id))
      .map(({ id, page, x, y, w, h, rot, whole }) => ({ id, page, x, y, w, h, rot, whole }));
    // drop empty pages in between, so page numbers stay 0,1,2...
    const used = [...new Set(placed.map(x => x.page))].sort((a, b) => a - b);
    placed.forEach(x => x.page = used.indexOf(x.page));
    return { printer: S.printer, mode: 'photos', fit: 'custom', items: S.photos.map(f => ({ id: f.id })), placed, copies: p.copies, options: o };
  }
  const per = p.layout.startsWith('fi') ? 1 : Number(p.layout);
  return { printer: S.printer, mode: 'photos', items: S.photos.map(f => ({ id: f.id })), copies: p.copies,
    perSheet: per, fit: p.layout === 'fit1' ? 'fit' : 'fill', options: o };
}

// ---------- preview & print ----------
// ---------- starting a print: three big "where from?" buttons ----------
// documents: any file (PDFs, pictures of any kind, Word, Excel, PowerPoint, text…); the computer turns it into
// something printable. Photos: any kind of picture, including iPhone HEIC and WebP.
const ACCEPT = { docs: '', photos: 'image/*,.heic,.heif,.avif,.webp' };
function sourceButtons(kind) {
  return `<div class="bigchoices">
    <button class="bigchoice" data-src="phone"><span class="bic">${icon('upload')}</span><b>From this device</b><small>${kind === 'photos' ? 'your photo gallery' : 'PDFs, pictures, files'}</small></button>
    <button class="bigchoice" data-src="cam"><span class="bic">${icon('camera')}</span><b>${kind === 'photos' ? 'Take a picture' : 'Scan with the camera'}</b><small>${kind === 'photos' ? 'with the camera' : 'finds the page and straightens it'}</small></button>
    <button class="bigchoice" data-src="pc"><span class="bic">${icon('folder')}</span><b>From the computer</b><small>search all its folders</small></button></div>`;
}
function wireSources(root, kind, add) {
  root.querySelectorAll('[data-src]').forEach(b => b.onclick = () => {
    const s = b.dataset.src;
    if (s === 'pc') pickLocal(kind === 'photos' ? 'image' : 'doc', add);
    else if (s === 'cam' && kind !== 'photos') camScan(add);   // documents: the page scanner, not a plain photo
    else pickFiles(s === 'cam' ? 'image/*' : ACCEPT[kind], add, s === 'cam');
  });
}
function addFiles(kind, fs) {
  const list = kind === 'photos' ? S.photos : S.docs;
  list.push(...(kind === 'photos' ? fs.filter(f => f.kind === 'image') : fs));
  return list.length;
}
function startScreen(kind) {
  const list = kind === 'photos' ? S.photos : S.docs, noun = kind === 'photos' ? 'photo' : 'file';
  render(`${pageHead(kind, kind === 'photos' ? 'Where are the photos?' : 'Where is it? PDFs and pictures both work.', kind === 'photos' ? 'Print photos' : 'Print documents')}
    ${list.length ? `<button class="bigchoice cont" id="cont"><span class="bic">▶</span><b>Continue</b><small>${list.length} ${noun}${list.length > 1 ? 's' : ''} ready to print</small></button>` : ''}
    ${sourceButtons(kind)}
    ${list.length ? '<p style="text-align:center;margin-top:18px"><button class="btn" id="over">Start over</button></p>' : ''}`);
  wireSources($('#app'), kind, fs => { if (addFiles(kind, fs)) openWork(kind); });
  const c = $('#cont'); if (c) c.onclick = () => openWork(kind);
  const o = $('#over'); if (o) o.onclick = () => { list.length = 0; if (kind === 'photos') { S.pho.placed = []; S.pho.known = new Set(); } startScreen(kind); };
}

// ---------- THE screen: your pages big, three buttons under them, everything else in a side drawer ----------
function pageLabel(kind, r) {
  if (!r) return '';
  const nm = i => esc(((kind === 'photos' ? S.photos : S.docs)[i] || {}).name || '');
  return { cover: () => `Cover for ${nm(r.item)}`, blank: () => 'Left empty on purpose, so the next file starts on a new sheet',
    page: () => `Page ${r.page} of ${nm(r.item)}`, image: () => kind === 'text' ? 'Your text' : nm(r.item),
    photos: () => r.items.length === 1 ? nm(r.items[0]) : `${r.items.length} photos` }[r.k]();
}

async function openWork(kind) {
  const cfg = { docs: S.doc, photos: S.pho, text: S.txt }[kind];
  const list = kind === 'docs' ? S.docs : kind === 'photos' ? S.photos : null;
  const specOf = { docs: docSpec, photos: photoSpec, text: textSpec }[kind];
  const m = modal(`<div class="pv">
      <div class="pvtop"><button class="back" data-x="close" aria-label="Back">‹</button><b id="pvtitle">Print</b>
        <button class="back" data-x="set" aria-label="Settings">${ic('gear')}</button></div>
      <div class="pvbody"><div class="pvpages"><div class="gtrack" id="pvtrack"></div><div class="pvbusy" id="pvbusy"><div class="spin"></div></div>
          <div class="pvnote" id="pvnote"></div>
          <div class="pvacts"><button class="btn" data-x="edit">${ic('edit')} Edit this page</button>${list ? `<button class="btn" data-x="add">${ic('plus')} Add more</button>` : ''}
            <button class="btn" data-x="set">${ic('gear')} Settings</button></div></div>
        <div class="pvset" id="pvset"></div></div>
      <div class="pvbar"><div class="pvsum" id="pvsum"></div><button class="btn main" id="pvprint">${icon('go')} Print</button></div></div>`);
  m.el.classList.add('pvmodal');
  const $m = s => m.el.querySelector(s), pv = $m('.pv'), track = $m('#pvtrack');
  let seq = 0, cur = null, timer = 0, dirty = true, busy = false, want = 0;
  const at = () => Math.round(track.scrollLeft / (track.clientWidth || 1));

  async function rebuild() {
    clearTimeout(timer);
    const my = ++seq; busy = true;
    const slow = setTimeout(() => { if (my === seq) $m('#pvbusy').style.display = 'grid'; }, 250);   // only show a spinner if it's actually slow
    try {
      const spec = await specOf();
      if (!spec) { m.close(); return; }
      spec.preview = true; spec.copies = 1;              // the screen only needs small pictures and one copy
      const b = await api('/api/build', spec);
      if (my !== seq) return;               // a newer change already started another preview
      cur = b; dirty = false; show(b);
      return true;
    } catch (e) { if (my === seq) oops(e); return false; }
    finally { clearTimeout(slow); if (my === seq) { busy = false; $m('#pvbusy').style.display = 'none'; } }
  }
  const later = () => { dirty = true; want = at(); clearTimeout(timer); timer = setTimeout(rebuild, 350); };
  const now = () => { want = at(); return rebuild(); };

  // show the new preview. Pages already on screen keep their old picture until the new one has
  // loaded, then swap: replacing them all at once made every setting change blank the pages like a reload.
  function show(b) {
    const n = Math.min(b.pages, 60);
    title(b);
    const figs = [...track.children];
    figs.slice(n).forEach(f => f.remove());
    for (let i = 0; i < n; i++) {
      const url = `/api/preview/${b.id}/${i + 1}`, near = Math.abs(i - want) <= 2;
      const cap = `${i + 1} / ${b.pages}${b.map ? ` · ${pageLabel(kind, b.map[i % b.perCopy])}` : ''}`;
      let f = figs[i];
      if (!f) {
        f = document.createElement('figure');
        f.className = 'pvpage';
        f.innerHTML = `<img alt="page ${i + 1}" ${near ? '' : 'loading="lazy"'}><figcaption></figcaption>`;
        f.querySelector('img').src = url;
        track.appendChild(f);
      } else if (near) {
        const img = f.querySelector('img'), next = new Image();
        next.src = url;
        next.decode().catch(() => {}).then(() => { if (cur === b) img.src = url; });
      } else f.querySelector('img').src = url;   // off screen: nobody sees it change
      f.querySelector('figcaption').innerHTML = cap;
    }
    $m('#pvnote').innerHTML = (b.warnings || []).map(x => `${ic('turn')} ${esc(x)}`).join('<br>') + (b.pages > n ? `<br>Showing the first ${n} pages.` : '');
    if (!figs.length) requestAnimationFrame(() => { track.scrollLeft = Math.min(want, n - 1) * track.clientWidth; });
    else if (at() > n - 1) track.scrollLeft = (n - 1) * track.clientWidth;
    summary();
  }
  function title(b) {
    if (!b) return;
    $m('#pvtitle').innerHTML = `Print <small>(${b.pages} page${b.pages > 1 ? 's' : ''}${b.duplex || b.autoDuplex ? `, ${b.sheets} sheet${b.sheets > 1 ? 's' : ''}` : ''}${cfg.copies > 1 ? ` × ${cfg.copies}` : ''})</small>`;
  }
  function summary() {
    const mo = mediaOpt(), po = key(['PageSize']);
    const media = mo ? nicer(cfg.media || mo.default) : '';
    $m('#pvsum').innerHTML = `<span class="pdot"></span><div><b>${esc(S.pname || S.printer)}</b><br><small>${[media, po ? paperName(cfg.paper || po.default) : ''].filter(Boolean).join(', ')}${cfg.copies > 1 ? ` · ${cfg.copies} copies` : ''}</small></div>`;
  }

  // edit whatever page you're looking at: a photo, a picture, a scan, or a page inside a PDF
  async function editHere() {
    if (!cur) return;
    const r = cur.map && cur.map[at() % cur.perCopy];
    if (!r) return toast(cfg.layout ? 'With several pages on each sheet, pages can\'t be edited one by one. Set Pages per sheet to 1 first.' : 'This page can\'t be edited');
    if (r.k === 'cover') return toast('That\'s a cover page. Change how it looks in Settings.');
    if (r.k === 'blank') return toast('This page stays empty on purpose, so the next file starts on a new sheet.');
    if (kind === 'text') return openEditor(S.txt.files[r.item], nf => { S.txt.files[r.item] = nf; now(); });
    if (r.k === 'image') return openEditor(S.docs[r.item], nf => { nf.range = S.docs[r.item].range; S.docs[r.item] = nf; now(); });
    if (r.k === 'photos') {
      const edit = i => openEditor(S.photos[i], nf => { swapFile(S.photos, i, nf); now(); });
      if (r.items.length === 1) return edit(r.items[0]);
      const q = modal(`<h2>Which photo?</h2><div class="grid">${r.items.map(i => `<button class="ph pick" data-i="${i}"><img src="/api/thumb/${S.photos[i].id}" alt=""></button>`).join('')}</div>
        <div class="foot"><button class="btn" data-close>Cancel</button></div>`);
      q.el.querySelectorAll('[data-i]').forEach(b => b.onclick = () => { q.close(); edit(+b.dataset.i); });
      return;
    }
    // a page inside a PDF: turn it into a picture, edit it, then put it back into (a copy of) the PDF
    const f = S.docs[r.item], wait = modal('<h2 style="text-align:center">Opening the page…</h2><div class="spin"></div>');
    try {
      const img = await api('/api/pageimage', { file: f.id, page: r.page });
      wait.close();
      openEditor(img, async nf => {
        const w2 = modal('<h2 style="text-align:center">Putting the page back…</h2><div class="spin"></div>');
        try { const nd = await api('/api/replacepage', { file: f.id, page: r.page, image: nf.id }); nd.range = f.range; S.docs[r.item] = nd; w2.close(); now(); }
        catch (e) { w2.close(); oops(e); }
      });
    } catch (e) { wait.close(); oops(e); }
  }

  // the settings drawer: everything in one place
  function settings() {
    const po = key(['PageSize']), mo = mediaOpt(), qo = qualityOpt(), co = colourOpt();
    const row = (label, ctl) => `<div class="pvrow"><span>${label}</span>${ctl}</div>`;
    const sel = (id, values, curV, lab) => `<select id="${id}">${values.map(v => `<option value="${esc(v)}" ${v === curV ? 'selected' : ''}>${esc(lab ? lab(v) : nicer(v))}</option>`).join('')}</select>`;
    const segm = (id, items, curV) => `<div class="seg mini" id="${id}">${items.map(([v, l]) => `<button data-v="${v}" class="${String(v) === String(curV) ? 'on' : ''}">${l}</button>`).join('')}</div>`;
    const tog = (id, on) => `<div class="toggle ${on ? 'on' : ''}" id="${id}"><i></i></div>`;
    let h = `<div class="pvdrawhead"><b>Settings</b><button class="btn" data-x="set">Done</button></div><div class="pvprinter">${ic('printer')} ${esc(S.pname || S.printer)}</div>`;
    if (list) {
      h += `<div class="pvhead">${kind === 'photos' ? 'Photos' : 'Files'} (${list.length})</div><ul class="pvfiles">${list.map((f, i) => `<li>
        <img src="/api/thumb/${f.id}" alt=""><div class="meta"><div class="name">${esc(f.name)}</div>
        ${kind === 'docs' && f.kind === 'pdf' && f.pages > 1 ? `<input class="pages" data-pages="${i}" placeholder="all ${f.pages} pages" value="${esc(f.range || '')}">` : `<small>${f.kind === 'pdf' ? f.pages + ' page' + (f.pages > 1 ? 's' : '') : 'picture'}</small>`}</div>
        <button class="icon-btn" data-mv="${i}" data-d="-1" ${i ? '' : 'disabled'}>${ic('up')}</button><button class="icon-btn" data-mv="${i}" data-d="1" ${i < list.length - 1 ? '' : 'disabled'}>${ic('down')}</button>
        <button class="icon-btn" data-rm="${i}">${ic('x')}</button></li>`).join('')}</ul>`;
    }
    if (kind === 'photos') {
      h += `<div class="pvhead">Layout</div>` + row('On each page', sel('pvlayout', ['fill1', 'fit1', '2', '4', '9', 'custom'], cfg.layout,
        v => ({ fill1: '1 photo, fill the page', fit1: '1 photo, whole photo', 2: '2 photos', 4: '4 photos', 9: '9 small photos', custom: 'Arrange myself' }[v])));
      if (cfg.layout === 'custom') h += `<div class="pvrow"><button class="btn wide" id="pvarrange">${ic('move')} Arrange the photos</button></div>`;
    }
    if (kind === 'text') {
      h += `<div class="pvhead">Text</div>` + row('Size', segm('pvtsize', [[11, 'S'], [13, 'M'], [16, 'L'], [20, 'XL']], cfg.size)) + row('Big title', tog('pvthead', cfg.heading))
        + `<div class="pvrow"><button class="btn wide" id="pvtedit">${ic('edit')} Change the text</button></div>`;
    }
    h += `<div class="pvhead">Paper & printing</div>`;
    if (mo) h += row('Media type', sel('pvmedia', mo.values, cfg.media || mo.default));
    if (po) h += row('Paper size', sel('pvpaper', paperChoices(po), borderTwin(po, cfg.paper, false) || cfg.paper, v => paperName(v).replace(' · no border', '')));
    if (kind === 'photos' && po && cfg.paper && borderTwin(po, cfg.paper, true) && borderTwin(po, cfg.paper, false)) h += row('Borderless', tog('pvborder', isBorderless(cfg.paper)));
    h += row('Copies', `<div class="stepper" id="pvcopies"><button data-d="-1">−</button><span>${cfg.copies}</span><button data-d="1">+</button></div>`);
    if (co) h += row('Colour', segm('pvcolour', [['colour', 'Colour'], ['mono', 'B & W']], cfg.colour));
    if (qo) h += row('Quality', sel('pvquality', qo.values, cfg.quality || qo.default));
    if (kind !== 'photos') h += row('Sides', segm('pvsides', [['single', 'One'], ['duplex', 'Both']], cfg.sides));
    if (kind === 'docs') {
      h += row('Fit to paper', tog('pvfit', cfg.fitPage));
      h += `<div class="pvhead">Save paper</div>` + row('Pages per sheet', sel('pvnup', ['', '2up', '4up', 'booklet'], cfg.layout,
        v => ({ '': '1', '2up': '2', '4up': '4', booklet: 'Booklet (fold in half)' }[v])));
      h += row('Leave out blank pages', tog('pvskip', cfg.skipBlank));
      h += `<div class="pvhead">Cover page</div>` + row('Cover', sel('pvcover', ['1', '2', '3'], String(cfg.cover), v => ({ 1: 'No cover', 2: 'With the file name', 3: 'Just pretty' }[v])));
      if (cfg.cover > 1) {
        h += row('Decoration', sel('pvstyle', ['1', '2', '3', '4', '5'], String(cfg.style), v => ({ 1: 'Flowers + shapes', 2: 'Flowers', 3: 'Shapes', 4: 'Simple', 5: 'Surprise me' }[v])));
        if (cfg.colour !== 'mono') h += `<div class="pvrow col">${swatches(cfg.palette < 0 ? S.info.theme : cfg.palette)}</div>`;
        h += row('White background', tog('pvwhite', cfg.whiteBg || cfg.colour === 'mono'));
      }
    }
    const extras = opts().filter(o => !handled().includes(o.key) && o.values.length > 1 && (!mo || o.key !== mo.key));
    if (extras.length) h += `<details class="more pvmore"><summary>More printer settings</summary>${extras.map(o => row(esc(o.label), sel('x_' + o.key, o.values, (cfg.extra || {})[o.key] ?? o.default))).join('')}</details>`;
    $m('#pvset').innerHTML = h;
    wireSettings();
  }
  function wireSettings() {
    const po = key(['PageSize']);
    const on = (s, ev, fn) => m.el.querySelectorAll(s).forEach(el => el.addEventListener(ev, fn));
    const redo = () => { settings(); later(); summary(); };
    const soft = () => { settings(); summary(); title(cur); };   // printer-only settings: the picture doesn't change
    on('#pvset [data-x=set]', 'click', () => pv.classList.remove('setopen'));
    on('#pvmedia', 'change', e => { cfg.media = e.target.value; if (kind === 'photos') cfg.glossy = /gloss/i.test(cfg.media); soft(); });
    on('#pvpaper', 'change', e => { const b = isBorderless(cfg.paper) && borderTwin(po, e.target.value, true); cfg.paper = b || e.target.value; redo(); });
    on('#pvborder', 'click', () => { cfg.paper = borderTwin(po, cfg.paper, !isBorderless(cfg.paper)) || cfg.paper; redo(); });
    on('#pvquality', 'change', e => { cfg.quality = e.target.value; soft(); });
    on('#pvfit', 'click', () => { cfg.fitPage = !cfg.fitPage; redo(); });
    on('#pvnup', 'change', e => { cfg.layout = e.target.value; if (cfg.layout === 'booklet') cfg.sides = 'duplex'; redo(); });   // a booklet is always both sides
    on('#pvskip', 'click', () => { cfg.skipBlank = !cfg.skipBlank; redo(); });
    on('#pvthead', 'click', () => { cfg.heading = !cfg.heading; redo(); });
    on('#pvwhite', 'click', () => { cfg.whiteBg = !cfg.whiteBg; redo(); });
    on('#pvcopies', 'click', e => { const b = e.target.closest('button'); if (!b) return; cfg.copies = Math.min(99, Math.max(1, cfg.copies + Number(b.dataset.d))); soft(); });
    on('#pvcolour button', 'click', e => { cfg.colour = e.currentTarget.dataset.v; S.docColourSet = true; (kind === 'docs' && cfg.cover > 1) ? redo() : soft(); });   // only covers change colour on screen
    on('#pvsides button', 'click', e => { cfg.sides = e.currentTarget.dataset.v; redo(); });
    on('#pvtsize button', 'click', e => { cfg.size = Number(e.currentTarget.dataset.v); redo(); });
    on('#pvlayout', 'change', e => { cfg.layout = e.target.value; redo(); if (cfg.layout === 'custom') arrange(); });
    on('#pvarrange', 'click', () => arrange());
    on('#pvcover', 'change', e => { cfg.cover = Number(e.target.value); redo(); });
    on('#pvstyle', 'change', e => { cfg.style = Number(e.target.value); redo(); });
    on('#pvset [data-pal]', 'click', e => { cfg.palette = Number(e.currentTarget.dataset.pal); redo(); });
    on('#pvset [id^=x_]', 'change', e => { cfg.extra = cfg.extra || {}; cfg.extra[e.target.id.slice(2)] = e.target.value; soft(); });
    on('#pvtedit', 'click', () => { m.close(); go('text'); });
    on('[data-pages]', 'change', e => { list[+e.target.dataset.pages].range = e.target.value.trim(); later(); });
    on('[data-mv]', 'click', e => {
      const i = +e.currentTarget.dataset.mv, j = i + Number(e.currentTarget.dataset.d);
      [list[i], list[j]] = [list[j], list[i]]; redo();
    });
    on('[data-rm]', 'click', e => {
      const [gone] = list.splice(+e.currentTarget.dataset.rm, 1);
      if (kind === 'photos') S.pho.placed = (S.pho.placed || []).filter(x => x.id !== gone.id);
      if (!list.length) { m.close(); route(); return; }
      redo();
    });
  }
  function arrange() {
    const a = modal(`<div id="arr"></div><div class="foot"><button class="btn main" id="arrdone">Done</button></div>`);
    a.el.classList.add('arrmodal');
    editor(a.el.querySelector('#arr'));
    a.el.querySelector('#arrdone').onclick = () => { a.close(); later(); };
  }
  function addMore() {
    const q = modal(`<h2>Add more</h2>${sourceButtons(kind)}<div class="foot"><button class="btn" data-close>Cancel</button></div>`);
    wireSources(q.el, kind, fs => { if (addFiles(kind, fs)) { settings(); later(); } });
    q.el.querySelectorAll('[data-src]').forEach(b => b.addEventListener('click', () => q.close()));
  }

  m.el.querySelectorAll('[data-x=set]').forEach(b => b.onclick = () => pv.classList.toggle('setopen'));
  $m('[data-x=edit]').onclick = () => editHere();
  const ad = $m('[data-x=add]'); if (ad) ad.onclick = addMore;
  $m('[data-x=close]').onclick = () => { clearTimeout(timer); seq++; m.close(); route(); };
  $m('#pvprint').onclick = () => guard(async () => {
    // a setting was just changed: print what you now see, and never an older preview if the new one failed
    if ((dirty || busy || !cur) && !(await rebuild())) return;   // the preview must be valid (it catches wrong page numbers)
    const spec = await specOf(); if (!spec) return;             // now the real thing: full quality, all copies
    let b;
    try { b = await api('/api/build', spec); } catch (e) { return oops(e); }
    m.close(); route();
    await startPrint(b);
  });
  settings(); summary(); rebuild();
}

// ---------- paper helpers ----------
const paperBase = v => String(v).replace(/\.Borderless$/i, '').replace(/_[BS]$/, '').replace(/^Br(?=[A-Z0-9])/, '');   // "A4" and "BrA4_B" are the same size
const isBorderless = v => /_B$|\.Borderless$/i.test(v);
function paperChoices(po) {               // one entry per size; "no border" is a switch, not a separate size
  const seen = new Map();
  for (const v of po.values) {
    const b = paperBase(v);
    if (!seen.has(b) || (isBorderless(seen.get(b)) && !isBorderless(v))) seen.set(b, v);
  }
  return [...seen.values()];
}
const borderTwin = (po, v, wantB) => po.values.find(x => paperBase(x) === paperBase(v) && isBorderless(x) === wantB);
function a4Of(po) { return po.values.find(v => paperBase(v) === 'A4' && !isBorderless(v)) || po.default; }

async function printSpec(spec) {
  const m = modal('<h2>Getting it ready…</h2><div class="spin"></div>');
  try { const b = await api('/api/build', spec); m.close(); startPrint(b); }
  catch (e) { m.close(); oops(e); }
}

// asks the one-time "which way do pages come out" question if needed, then prints
async function startPrint(b, test) {
  if (!S.setupDone[b.printer]) {
    if (!await onboarding(b.printer)) return;
    if (b.duplex && !test) toast('Tip: in Printer care, do the both-sides setup once, so double-sided always comes out right.');
  }
  let j;
  try { j = await api('/api/print', { build: b.id, test: !!test }); } catch (e) { return oops(e); }
  followJob(j, test);
}

function followJob(j, test) {
  const m = modal('');
  let last = '';
  const draw = () => {
    if (j.state === last) return; last = j.state;
    if (typeof refreshNotices === 'function') refreshNotices();   // the bell follows the print (a flip done is one thing less)
    if (j.state === 'printing1' || j.state === 'printing2') {
      m.set(`${bigIcon('printer')}<h2 style="text-align:center">${j.state === 'printing2' ? 'Printing the other side…' : `Printing${j.sheets > 1 ? ' ' + j.sheets + ' sheets' : ''}…`}</h2>
        <div class="spin"></div><p class="hint" style="text-align:center">You can leave this open.</p>
        <div class="foot"><button class="btn danger" id="stop">Stop printing</button></div>`);
      $('#stop').onclick = async () => { if (await ask('<h2>Stop printing?</h2><p>This print is stopped. Anything else waiting for the printer carries on.</p>', [['yes', 'Yes, stop']])) api('/api/job/' + j.id + '/cancel', {}).catch(oops); };
    } else if (j.state === 'flip') {
      m.set(`<h2>Time to flip the paper</h2>
        ${guide(S.style[S.printer] || 'tray', 2, j.rotate)}
        <div class="countcheck">You should have <b>${j.sheets} sheet${j.sheets > 1 ? 's' : ''}</b>. Count them, and flick through the edges so they don't stick.
          <span>Fewer? The printer grabbed two at once. Tap Cancel and print again.</span></div>
        <div class="foot"><button class="btn" id="laterFlip">Later</button><button class="btn" id="stopFlip">Don't print the other side</button><button class="btn main" id="side2">Print the other side</button></div>
        <p class="hint" style="text-align:center">Later: it waits in the notifications (the bell) until you're ready.</p>`);
      wireGuide(m.el, 2);
      $('#side2').onclick = () => guard(async () => { $('#side2').disabled = true; try { j = await api('/api/job/' + j.id + '/side2', {}); draw(); } catch (e) { oops(e); const b = $('#side2'); if (b) b.disabled = false; } });
      $('#laterFlip').onclick = () => { clearInterval(t); m.close(); if (typeof refreshNotices === 'function') refreshNotices(); };
      $('#stopFlip').onclick = async () => { if (await ask('<h2>Don\'t print the other side?</h2><p>The pages printed so far stay as they are.</p>', [['yes', 'Yes, stop here']])) api('/api/job/' + j.id + '/cancel', {}).then(v => { j = v; draw(); }).catch(oops); };
    } else if (j.state === 'canceled') {
      clearInterval(t); m.close(); toast('Stopped');
    } else if (j.state === 'done') {
      clearInterval(t);
      if (test) return testQuestions(m, j);
      m.set(`${bigIcon('check')}<h2 style="text-align:center">All done!</h2><p style="text-align:center" class="sub">Your pages are ready.</p>
        <div class="foot"><button class="btn main" data-close>OK</button></div>`);
      burst(m.el.querySelector('.bigicon'));
    }
  };
  draw();
  const t = setInterval(async () => {
    if (!document.body.contains(m.el)) return clearInterval(t);
    try { j = await api('/api/job/' + j.id); draw(); } catch (_) {}
  }, 2000);
}
