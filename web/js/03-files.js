// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 03: Adding files (phone, camera, computer) and the shared option widgets.
'use strict';

// ---------- shared: adding files ----------
function pickFiles(accept, onDone, camera) {
  const inp = document.createElement('input');
  inp.type = 'file'; inp.multiple = !camera; inp.accept = accept;
  if (camera) inp.setAttribute('capture', 'environment');
  inp.onchange = () => upload([...inp.files], onDone);
  inp.click();
}

function upload(list, onDone) {
  if (!list.length) return;
  const fd = new FormData();
  list.forEach(f => fd.append('files', f));
  const m = modal(`<h2>Adding ${list.length} file${list.length > 1 ? 's' : ''}…</h2><div class="bar"><i id="upbar"></i></div>`);
  const x = new XMLHttpRequest();
  x.open('POST', '/api/upload');
  x.upload.onprogress = e => { if (e.lengthComputable) $('#upbar').style.width = (e.loaded / e.total * 100) + '%'; };
  x.onload = () => {
    m.close();
    if (x.status === 401) return showPin();
    let d = {}; try { d = JSON.parse(x.responseText); } catch (_) {}
    if (x.status !== 200) return toast(d.error || 'upload failed', true);
    (d.errors || []).forEach(e => toast(e, true));
    onDone(d.files || []);
  };
  x.onerror = () => { m.close(); toast('upload failed, is the laptop still on?', true); };
  x.send(fd);
}

// pick files that are anywhere on the computer: search box, newest first, folder shown under each name
function pickLocal(kind, onDone) {
  const photos = kind === 'image';
  const picked = new Map();          // path -> true, kept while you search
  let filter = photos ? 'image' : 'doc', q = '', timer = 0, seq = 0;
  const m = modal(`<h2>On this computer</h2>
    <div class="findbar"><input class="big" id="findq" type="search" placeholder="Search by name or folder" autocomplete="off">
      ${photos ? '' : `<div class="seg mini" id="findkind"><button data-v="doc" class="on">All</button><button data-v="pdf">PDFs</button><button data-v="image">Pictures</button><button data-v="office">Office</button></div>`}</div>
    <div class="findlist" id="findlist"><div class="spin"></div></div>
    <div class="foot"><button class="btn" data-close>Cancel</button><button class="btn main" id="addLocal" disabled>Add</button></div>`);
  m.el.classList.add('findmodal');
  const $f = sel => m.el.querySelector(sel);
  const when = t => {
    const d = new Date(t * 1000), now = new Date(), days = Math.floor((now - d) / 864e5);
    if (d.toDateString() === now.toDateString()) return 'Today ' + d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
    if (days < 2) return 'Yesterday';
    return d.toLocaleDateString([], { day: 'numeric', month: 'short', year: d.getFullYear() === now.getFullYear() ? undefined : 'numeric' });
  };
  const size = b => b > 1048576 ? (b / 1048576).toFixed(1) + ' MB' : Math.max(1, Math.round(b / 1024)) + ' KB';
  const count = () => { const n = picked.size, b = $f('#addLocal'); b.disabled = !n; b.textContent = n ? `Add ${n}` : 'Add'; };
  async function load() {
    const my = ++seq;
    let d;
    try { d = await api(`/api/local?kind=${filter}&q=${encodeURIComponent(q)}`); } catch (e) { return oops(e); }
    if (my !== seq || !document.body.contains(m.el)) return;
    const list = $f('#findlist');
    if (!d.ready) { list.innerHTML = '<div class="spin"></div><p class="hint" style="text-align:center">Looking through the computer for the first time…</p>'; setTimeout(load, 900); return; }
    if (!d.files.length) { list.innerHTML = `<p class="empty">${q ? 'Nothing matches that. Try a shorter word.' : 'No files found.'}</p>`; return; }
    list.innerHTML = d.files.map(f => `<label class="findrow"><input type="checkbox" value="${esc(f.path)}" ${picked.has(f.path) ? 'checked' : ''}>
      <span class="findic">${icon(f.kind === 'image' ? 'photo' : f.kind === 'office' ? 'text2' : 'doc')}</span>
      <span class="findtxt"><b>${esc(f.name)}</b><small>${esc(f.dir)} · ${when(f.mtime)} · ${size(f.size)}</small></span></label>`).join('')
      + (d.total > d.files.length ? `<p class="hint" style="text-align:center">Showing the newest ${d.files.length} of ${d.total}. Search to find the rest.</p>` : '');
    list.querySelectorAll('input').forEach(c => c.onchange = () => { c.checked ? picked.set(c.value, true) : picked.delete(c.value); count(); });
    if (d.busy) setTimeout(() => { if (my === seq) load(); }, 2500);   // still finding more: refresh once it's done
  }
  $f('#findq').oninput = e => { q = e.target.value; clearTimeout(timer); timer = setTimeout(load, 180); };
  const fk = $f('#findkind');
  if (fk) fk.onclick = e => { const b = e.target.closest('button'); if (!b) return; filter = b.dataset.v; fk.querySelectorAll('button').forEach(x => x.classList.toggle('on', x === b)); load(); };
  $f('#addLocal').onclick = async () => {
    try {
      const d = await api('/api/local/add', { paths: [...picked.keys()] });
      (d.errors || []).forEach(e => toast(e, true));
      m.close(); onDone(d.files || []);
    } catch (e) { oops(e); }
  };
  load();
  setTimeout(() => { const i = $f('#findq'); if (i && matchMedia('(pointer: fine)').matches) i.focus(); }, 60);
}

// ---------- shared: option widgets ----------
const seg = (name, items, cur) => `<div class="seg" data-seg="${name}">${items.map(([v, label, small]) =>
  `<button data-v="${esc(v)}" class="${String(v) === String(cur) ? 'on' : ''}">${label}${small ? `<small>${small}</small>` : ''}</button>`).join('')}</div>`;
const stepper = (name, v) => `<div class="stepper" data-step="${name}"><button data-d="-1">−</button><span>${v}</span><button data-d="1">+</button></div>`;
const selectOf = (name, o, cur, label) => `<select class="big" data-sel="${esc(name)}">${o.values.map(v =>
  `<option value="${esc(v)}" ${v === cur ? 'selected' : ''}>${esc(label ? label(v) : nicer(v))}</option>`).join('')}</select>`;
const toggle = (name, on, label) => `<div class="toggle ${on ? 'on' : ''}" data-tog="${name}"><i></i><span>${label}</span></div>`;

function swatches(cur, disabled) {
  return `<div class="swatches">${S.info.palettes.map((p, i) => `<button class="swatch ${i === cur ? 'on' : ''}" data-pal="${i}" ${disabled ? 'disabled' : ''}>
    <div class="dot" style="background:${p.tint};border-color:${p.acc}"></div>${esc(p.name)}</button>`).join('')}
    <button class="swatch ${cur === 10 ? 'on' : ''}" data-pal="10" ${disabled ? 'disabled' : ''}><div class="dot" style="background:conic-gradient(#f2b5c4,#b5c99a,#a9bfe3,#c7b8e6,#f2b5c4);border-color:#fff"></div>Surprise</button></div>`;
}

// segPick highlights the tapped button of a choice row, without redrawing the screen
function segPick(g, b) { g.querySelectorAll('button').forEach(x => x.classList.toggle('on', x === b)); }

// wires up seg/step/toggle/select widgets to an object in S. Choices update in place: redrawing the
// whole screen on every tap made it flash like a reload (and closed the phone keyboard).
function wire(target, rerender) {
  const app = $('#app');
  app.querySelectorAll('[data-seg]').forEach(g => g.onclick = e => {
    const b = e.target.closest('button'); if (!b) return;
    const k = g.dataset.seg; let v = b.dataset.v;
    if (typeof target[k] === 'number') v = Number(v);
    target[k] = v; if (k === 'colour') S.docColourSet = true; segPick(g, b);
  });
  app.querySelectorAll('[data-step]').forEach(g => g.onclick = e => {
    const b = e.target.closest('button'); if (!b) return;
    const k = g.dataset.step;
    target[k] = Math.min(99, Math.max(1, target[k] + Number(b.dataset.d)));
    g.querySelector('span').textContent = target[k];
  });
  app.querySelectorAll('[data-tog]').forEach(g => g.onclick = () => { target[g.dataset.tog] = !target[g.dataset.tog]; g.classList.toggle('on', target[g.dataset.tog]); });
  app.querySelectorAll('[data-sel]').forEach(s => s.onchange = () => {
    const k = s.dataset.sel;
    if (k.startsWith('x:')) target.extra[k.slice(2)] = s.value; else target[k] = s.value;
  });
  app.querySelectorAll('[data-pal]').forEach(b => b.onclick = () => { target.palette = Number(b.dataset.pal); rerender(); });
}

function actions(html) {
  document.querySelectorAll('.actions').forEach(a => a.remove());
  const a = document.createElement('div');
  a.className = 'actions';
  a.innerHTML = `<div class="in">${html}</div>`;
  document.body.appendChild(a);
  document.body.classList.add('has-actions');
  return a;
}


function printOptions(cfg) {
  const o = {};
  if (cfg.paper) o.PageSize = cfg.paper;
  const co = colourOpt(); const cv = colourValue(cfg.colour);
  if (co && cv) o[co.key] = cv;
  const qo = qualityOpt(); if (qo && cfg.quality) o[qo.key] = cfg.quality;
  const mo = mediaOpt(); if (mo && cfg.media) o[mo.key] = cfg.media;
  Object.assign(o, cfg.extra || {});
  return o;
}

function docSpec() {
  const d = S.doc;
  return { printer: S.printer, mode: 'docs', items: S.docs.map(f => ({ id: f.id, pages: f.range || '' })), sides: d.sides,
    copies: d.copies, cover: d.cover, style: d.style, palette: d.palette < 0 ? S.info.theme : d.palette,
    whiteBg: d.whiteBg || d.colour === 'mono', fitPage: d.fitPage, layout: d.layout, skipBlank: d.skipBlank, options: printOptions(d) };
}
