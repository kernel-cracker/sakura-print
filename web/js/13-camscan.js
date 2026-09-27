// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 13: Scanning pages with the phone's camera, like the iPhone Notes scanner.
'use strict';

// camScan: take (or choose) photos of pages; each one is found, straightened flat, cleaned up, and added.
// done(files) gets the finished pages as picture files on the computer, ready to print or save as a PDF.
async function camScan(done) {
  try { await loadScript('develop.js', 'SakuraDevelop'); } catch (e) { return oops(e); }
  const D = window.SakuraDevelop;
  const MODES = [['colour', 'Colour'], ['grey', 'Greyscale'], ['bw', 'Black & white']];
  let mode = 'colour';
  try { mode = localStorage.getItem('sakuraScanMode') || 'colour'; } catch (_) {}
  const pages = [];           // { blob, url }
  let shot = null, quad = null;   // the photo being looked at, and its page corners (in the photo's pixels)
  const m = modal(`<div class="ed cam">
      <div class="edtop"><button class="btn" data-x="cancel">Cancel</button><b id="camtitle">Scan pages</b><button class="btn main" data-x="done" disabled>Done</button></div>
      <div class="edstage camstage" id="camstage"><div class="camempty" id="camempty">${bigIcon('camera')}<p>Put the page on a table, with some table showing around it.</p></div>
        <div class="camview" id="camview" hidden><canvas id="camc"></canvas><svg id="camsvg"></svg></div></div>
      <div class="camopts"><div class="seg mini edseg" id="cammode">${MODES.map(([k, l]) => `<button data-mode="${k}" class="${k === mode ? 'on' : ''}">${l}</button>`).join('')}</div>
        <span class="edhint" id="camhint">Drag the corners to the edges of the page</span></div>
      <div class="campages" id="campages"></div>
      <div class="cambar" id="cambar"></div></div>`);
  m.el.classList.add('edmodal', 'cammodal');
  const $c = s => m.el.querySelector(s), view = $c('#camview'), cv = $c('#camc'), svg = $c('#camsvg');
  const close = () => { pages.forEach(p => URL.revokeObjectURL(p.url)); m.close(); };
  $c('[data-x=cancel]').onclick = async () => { if (!pages.length || await ask('<h2>Throw away these pages?</h2>', [['yes', 'Throw away']])) close(); };

  const pick = camera => new Promise(res => {
    const inp = document.createElement('input');
    inp.type = 'file'; inp.accept = 'image/*';
    if (camera) inp.setAttribute('capture', 'environment');
    inp.onchange = () => res(inp.files[0] || null);
    inp.click();
  });
  function bar() {
    $c('#cambar').innerHTML = shot
      ? `<button class="btn" data-x="retake">${ic('undo')} Retake</button><button class="btn main" data-x="use">${ic('check')} Use this page</button>`
      : `<button class="btn" data-x="choose">${ic('photo')} Choose a photo</button><button class="btn main" data-x="take">${ic('camera')} ${pages.length ? 'Next page' : 'Take a picture'}</button>`;
    const on = (k, f) => { const b = $c(`[data-x=${k}]`); if (b) b.onclick = f; };
    on('take', () => load(pick(true)));
    on('choose', () => load(pick(false)));
    on('retake', () => { shot = null; show(); });
    on('use', usePage);
    $c('#camempty').hidden = !!shot; view.hidden = !shot;
    $c('#cammode').parentElement.style.visibility = shot ? 'visible' : 'hidden';
    $c('[data-x=done]').disabled = !pages.length;
    $c('[data-x=done]').textContent = pages.length ? `Done (${pages.length})` : 'Done';
    $c('#campages').innerHTML = pages.map((p, i) => `<div class="campage"><img src="${p.url}" alt="page ${i + 1}"><span>${i + 1}</span><button class="rm" data-rm="${i}" aria-label="Remove">${ic('x')}</button></div>`).join('');
    m.el.querySelectorAll('[data-rm]').forEach(b => b.onclick = () => { const [p] = pages.splice(+b.dataset.rm, 1); URL.revokeObjectURL(p.url); bar(); });
  }
  async function load(filePromise) {
    const f = await filePromise; if (!f) return;
    try { shot = await createImageBitmap(f, { imageOrientation: 'from-image' }); }
    catch (_) {   // a kind of photo this phone's browser can't open (HEIC on Android): the computer converts it
      try {
        const up = await new Promise(res => upload([f], res));
        if (!up.length) return;
        shot = await createImageBitmap(await (await fetch('/api/file/' + up[0].id)).blob(), { imageOrientation: 'from-image' });
      } catch (e) { return toast('Couldn\'t open that photo', true); }
    }
    // find the page on a small copy (fast), then scale the corners back up
    const k = Math.min(1, 400 / Math.max(shot.width, shot.height)), sw = Math.round(shot.width * k), sh = Math.round(shot.height * k);
    const small = document.createElement('canvas'); small.width = sw; small.height = sh;
    const g = small.getContext('2d', { willReadFrequently: true }); g.drawImage(shot, 0, 0, sw, sh);
    const found = D.findPage(g.getImageData(0, 0, sw, sh));
    const i = 0.04;   // nothing found: start from the whole photo, a little inside, and let the corners be dragged
    quad = found ? found.map(([x, y]) => [x / k, y / k]) : [[i, i], [1 - i, i], [1 - i, 1 - i], [i, 1 - i]].map(([x, y]) => [x * shot.width, y * shot.height]);
    $c('#camhint').textContent = found ? 'Found the page. Drag the corners if it\'s not quite right' : 'Drag the corners to the edges of the page';
    show();
  }
  // the photo on screen, with the page outline and four finger-sized corner handles
  let fitK = 1;
  function show() {
    bar();
    if (!shot) return;
    const r = $c('#camstage').getBoundingClientRect();
    fitK = Math.min((r.width - 40) / shot.width, (r.height - 40) / shot.height);
    const w = Math.round(shot.width * fitK), h = Math.round(shot.height * fitK), dpr = Math.min(2, window.devicePixelRatio || 1);
    cv.width = w * dpr; cv.height = h * dpr; cv.style.width = w + 'px'; cv.style.height = h + 'px';
    cv.getContext('2d').drawImage(shot, 0, 0, cv.width, cv.height);
    view.style.width = w + 'px'; view.style.height = h + 'px';
    svg.innerHTML = '';
    svg.setAttribute('viewBox', `0 0 ${w} ${h}`); svg.setAttribute('width', w); svg.setAttribute('height', h);
    outline();
  }
  // the outline and handles are drawn once, then only moved: redrawing them would drop the handle a finger is holding
  function outline() {
    const p = quad.map(([x, y]) => [x * fitK, y * fitK]), W = cv.clientWidth, H = cv.clientHeight;
    if (!svg.querySelector('.camh')) {
      svg.innerHTML = `<path class="camshade" fill="rgba(0,0,0,.45)" fill-rule="evenodd"/><polygon class="camline" fill="rgba(255,204,0,.12)" stroke="#ffcc00" stroke-width="2.5"/>
        ${[0, 1, 2, 3].map(i => `<circle class="camh" data-c="${i}" r="16"/>`).join('')}`;
      svg.querySelectorAll('.camh').forEach(c => c.onpointerdown = e => {
        e.preventDefault(); c.setPointerCapture(e.pointerId);
        const i = +c.dataset.c, box = svg.getBoundingClientRect();
        c.onpointermove = ev => {
          quad[i] = [Math.max(0, Math.min(shot.width, (ev.clientX - box.left) / fitK)), Math.max(0, Math.min(shot.height, (ev.clientY - box.top) / fitK))];
          outline();
        };
        c.onpointerup = c.onpointercancel = () => { c.onpointermove = null; };
      });
    }
    svg.querySelector('.camshade').setAttribute('d', `M0 0H${W}V${H}H0Z M${p.map(q => q.join(' ')).join(' L')} Z`);
    svg.querySelector('.camline').setAttribute('points', p.map(q => q.join(',')).join(' '));
    svg.querySelectorAll('.camh').forEach((c, i) => { c.setAttribute('cx', p[i][0]); c.setAttribute('cy', p[i][1]); });
  }
  async function usePage() {
    if (!shot) return;
    const wait = modal(`<h2 style="text-align:center">Straightening the page…</h2><div class="spin"></div>`);
    try {
      await new Promise(r => requestAnimationFrame(() => setTimeout(r, 30)));
      const size = D.pageSize(quad), out = document.createElement('canvas');
      out.width = size.w; out.height = size.h;
      D.warpQuad(shot, out, quad, 24);
      const g = out.getContext('2d', { willReadFrequently: true }), d = g.getImageData(0, 0, out.width, out.height);
      if (mode === 'bw') D.blackWhite(d);
      else await D.developFast(d, mode === 'grey' ? { saturation: -100 } : {}, 'document', mode === 'grey' ? 100 : 70);
      g.putImageData(d, 0, 0);
      const blob = await new Promise(r => out.toBlob(r, 'image/jpeg', 0.9));
      pages.push({ blob, url: URL.createObjectURL(blob) });
      shot = null; quad = null;
      show();
    } catch (e) { oops(e); } finally { wait.close(); }
  }
  m.el.querySelectorAll('[data-mode]').forEach(b => b.onclick = () => {
    mode = b.dataset.mode; try { localStorage.setItem('sakuraScanMode', mode); } catch (_) {}
    m.el.querySelectorAll('[data-mode]').forEach(x => x.classList.toggle('on', x === b));
  });
  $c('[data-x=done]').onclick = () => guard(async () => {
    const stamp = new Date().toISOString().slice(0, 16).replace('T', ' ').replace(':', '-');
    const list = pages.map((p, i) => new File([p.blob], `Scan ${stamp} page ${i + 1}.jpg`, { type: 'image/jpeg' }));
    const files = await new Promise(res => upload(list, res));
    if (!files.length) return;
    close();
    done(files);
  });
  window.addEventListener('resize', show);
  bar();
}
