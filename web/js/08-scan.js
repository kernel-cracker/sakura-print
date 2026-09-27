// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 08: Scan and Copy.
'use strict';

// ---------- scan ----------
async function scan() {
  const s = S.scan;
  const pages = S.scans.length ? `<div class="grid">${S.scans.map((f, i) => `<div class="ph"><img src="/api/thumb/${f.id}" alt=""><button class="rm" data-rm="${i}">${ic('x')}</button>
      <button class="edit" data-edit="${i}" title="Crop, straighten, draw">${ic('edit')}</button></div>`).join('')}</div>`
    : `<div class="empty">${S.info.canScan ? 'Put the paper face down on the glass, then press Scan. Or use the phone\'s camera.' : 'Use the phone\'s camera: it finds the page and straightens it.'}</div>`;
  render(`${pageHead('scan', 'Scan pages, then save or print them.')}
    <button class="bigchoice cont" id="camScan"><span class="bic">${icon('camera')}</span><b>Scan with the camera</b><small>finds the page and straightens it</small></button>
    ${S.info.canScan ? `<div class="card"><h2>Scanner settings</h2>
      <div class="field"><span class="lbl">Quality</span>${seg('dpi', [[150, 'Quick'], [300, 'Normal'], [600, 'Detailed', 'slow']], s.dpi)}</div>
      <div class="field"><span class="lbl">Colour</span>${seg('colour', [[1, 'Colour'], [0, 'Black & white']], s.colour ? 1 : 0)}</div>
      <div class="field" id="scnr">${S.scanners ? '' : '<span class="hint">Looking for the scanner…</span>'}</div></div>` : ''}
    <div class="card"><h2>Pages (${S.scans.length})</h2>${pages}</div>`);
  $('#camScan').onclick = () => camScan(files => { S.scans.push(...files); scan(); });
  $('#app').querySelectorAll('[data-seg]').forEach(g => g.onclick = e => {
    const b = e.target.closest('button'); if (!b) return;
    if (g.dataset.seg === 'dpi') s.dpi = Number(b.dataset.v); else s.colour = b.dataset.v === '1';
    segPick(g, b);
  });
  $('#app').querySelectorAll('[data-rm]').forEach(b => b.onclick = () => { S.scans.splice(+b.dataset.rm, 1); scan(); });
  $('#app').querySelectorAll('[data-edit]').forEach(b => b.onclick = () => openEditor(S.scans[+b.dataset.edit], nf => { S.scans[+b.dataset.edit] = nf; scan(); }));
  const has = S.scans.length > 0;
  const bar = actions(`${S.info.canScan ? `<button class="btn main" id="doScan">${icon('scan')} Scan${has ? ' another page' : ''}</button>` : ''}
    ${has ? `<button class="btn ${S.info.canScan ? '' : 'main'}" id="save">Save</button><button class="btn" id="toPrint">Print</button>` : ''}`);
  if (!S.info.canScan && !has) { bar.remove(); document.body.classList.remove('has-actions'); }
  const ds = bar.querySelector('#doScan'); if (ds) ds.onclick = () => guard(doScan);
  if (has) {
    bar.querySelector('#save').onclick = () => guard(saveScans);
    bar.querySelector('#toPrint').onclick = () => { S.docs.push(...S.scans); S.scans = []; go('docs'); setTimeout(() => openWork('docs'), 60); };
  }
  const showScanners = list => {
    const box = $('#scnr'); if (!box) return;
    if (!list.length) box.innerHTML = `<span class="pill bad">No scanner found</span><p class="hint">Is the printer on and on the same WiFi? <a href="#" id="rescan">Look again</a></p>`;
    else {
      if (!s.device || !list.find(x => x.device === s.device)) s.device = list[0].device;
      box.innerHTML = list.length === 1 ? `<span class="pill ok">${ic('check')} ${esc(list[0].name)}</span>`
        : `<label>Scanner</label><select class="big" id="dev">${list.map(x => `<option value="${esc(x.device)}" ${x.device === s.device ? 'selected' : ''}>${esc(x.name)}</option>`).join('')}</select>`;
      const dv = $('#dev'); if (dv) dv.onchange = () => s.device = dv.value;
    }
    const rs = $('#rescan'); if (rs) rs.onclick = async e => {
      e.preventDefault(); box.innerHTML = '<span class="hint">Looking for the scanner…</span>';
      try { S.scanners = await api('/api/scanners?refresh=1'); } catch (err) { return oops(err); }
      showScanners(S.scanners);
    };
  };
  if (!S.info.canScan) return;
  if (S.scanners) showScanners(S.scanners);   // known already: draw it straight away, no "looking…" flash
  try { S.scanners = await api('/api/scanners'); showScanners(S.scanners); } catch (e) { oops(e); }
}

async function doScan() {
  const m = modal(bigIcon('scan') + '<h2 style="text-align:center">Scanning…</h2><div class="spin"></div><p class="hint" style="text-align:center">This can take up to a minute.</p>');
  try { const f = await api('/api/scan', { device: S.scan.device, dpi: S.scan.dpi, colour: S.scan.colour }); S.scans.push(f); m.close(); scan(); }
  catch (e) { m.close(); oops(e); }
}

async function saveScans() {
  const how = await ask('<h2>Save as…</h2>', [['pdf', 'One PDF', 'best for documents'], ['images', 'Pictures', 'one PNG per page']]);
  if (!how) return;
  try {
    const d = await api('/api/scan/save', { ids: S.scans.map(f => f.id), format: how });
    modal(`${bigIcon('check')}<h2>Saved!</h2><p>On the computer in <b>${esc(d.folder)}</b>.</p>
      <p class="sub">Want it on this device too?</p>${d.saved.map(x => `<a class="btn wide" style="margin-bottom:8px" href="${x.url}">${ic('download')} ${esc(x.name)}</a>`).join('')}
      <div class="foot"><button class="btn" data-close>Keep scanning</button><button class="btn main" id="newScan">Done</button></div>`);
    $('#newScan').onclick = () => { S.scans = []; document.querySelector('.modal').remove(); scan(); };
  } catch (e) { oops(e); }
}

// ---------- copy ----------
function copy() {
  const c = S.cp;
  render(`${pageHead('copy', 'Put the paper face down on the glass, then press Copy.')}
    <div class="card"><div class="field"><span class="lbl">Colour</span>${seg('colour', [[0, 'Black & white'], [1, 'Colour']], c.colour ? 1 : 0)}</div>
      <div class="field"><span class="lbl">Copies</span>${stepper('copies', c.copies)}</div></div>`);
  $('#app').querySelector('[data-seg]').onclick = e => { const b = e.target.closest('button'); if (b) { c.colour = b.dataset.v === '1'; segPick(e.currentTarget, b); } };
  $('#app').querySelector('[data-step]').onclick = e => { const b = e.target.closest('button'); if (!b) return;
    c.copies = Math.min(99, Math.max(1, c.copies + Number(b.dataset.d))); $('[data-step] span').textContent = c.copies; };
  const cbar = actions(`<button class="btn" id="fixCopy">${ic('edit')} Preview & fix</button><button class="btn main" id="doCopy">${icon('copy')} Copy</button>`);
  const printCopy = async f => {
    const o = printOptions({ colour: c.colour ? 'colour' : 'mono' });
    const b = await api('/api/build', { printer: S.printer, mode: 'copy', items: [{ id: f.id }], copies: c.copies, options: o });
    await startPrint(b);
  };
  cbar.querySelector('#fixCopy').onclick = () => guard(async () => {
    const m = modal(bigIcon('scan') + '<h2 style="text-align:center">Scanning…</h2><div class="spin"></div>');
    try {
      const f = await api('/api/scan', { device: S.scan.device, dpi: 300, colour: c.colour });
      m.close();
      openEditor(f, nf => { printCopy(nf).catch(oops); });   // not guarded: the editor's Done tap is still finishing
    } catch (e) { m.close(); oops(e); }
  });
  cbar.querySelector('#doCopy').onclick = () => guard(async () => {
    const m = modal(bigIcon('scan') + '<h2 style="text-align:center">Scanning…</h2><div class="spin"></div>');
    try {
      const f = await api('/api/scan', { device: S.scan.device, dpi: 300, colour: c.colour });
      m.set('<h2 style="text-align:center">Getting the copy ready…</h2><div class="spin"></div>');
      const o = printOptions({ colour: c.colour ? 'colour' : 'mono' });
      const b = await api('/api/build', { printer: S.printer, mode: 'copy', items: [{ id: f.id }], copies: c.copies, options: o });
      m.close(); await startPrint(b);
    } catch (e) { m.close(); oops(e); }
  });
}
