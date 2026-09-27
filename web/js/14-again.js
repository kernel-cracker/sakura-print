// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 14: Print again: the recent prints, one tap to print them the same way.
'use strict';

function agoText(iso) {
  const d = new Date(iso), now = new Date(), days = Math.round((new Date(now.toDateString()) - new Date(d.toDateString())) / 864e5);
  const time = d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
  if (days === 0) return 'Today ' + time;
  if (days === 1) return 'Yesterday ' + time;
  if (days < 7) return d.toLocaleDateString([], { weekday: 'long' }) + ' ' + time;
  return d.toLocaleDateString([], { day: 'numeric', month: 'short' });
}
const howText = e => [`${e.pages} page${e.pages > 1 ? 's' : ''}`, e.sides === 'duplex' ? 'both sides' : '', e.copies > 1 ? `${e.copies} copies` : ''].filter(Boolean).join(' · ');

async function again() {
  render(`${pageHead('again', 'Everything you print is here, ready to print again.')}<div id="agl"><div class="spin"></div></div>`);
  let list;
  try { list = await api('/api/history'); } catch (e) { return oops(e); }
  const box = $('#agl'); if (!box) return;
  if (!list.length) { box.innerHTML = '<div class="empty">Nothing printed yet. Whatever you print shows up here.</div>'; return; }
  box.innerHTML = list.map(e => `<div class="card agrow"><img src="/api/history/${e.id}/thumb" alt="" loading="lazy">
      <div class="agmeta"><b>${esc(e.title)}</b><small>${agoText(e.when)}<br>${howText(e)}</small>
        <div class="agbtns"><button class="btn main" data-again="${e.id}">${ic('printer')} Print again</button>
          <button class="btn" data-change="${e.id}">Change settings</button></div></div>
      <button class="icon-btn agx" data-forget="${e.id}" aria-label="Remove">${ic('x')}</button></div>`).join('')
    + '<p style="text-align:center;margin-top:14px"><button class="btn" id="agclear">Clear the list</button></p>';
  const entry = id => list.find(e => e.id === id);
  box.querySelectorAll('[data-again]').forEach(b => b.onclick = () => guard(async () => {
    const e = entry(b.dataset.again);
    if (!await ask(`<h2>Print it again?</h2><p><b>${esc(e.title)}</b><br>${howText(e)}</p>`, [['yes', 'Print']])) return;
    const r = await api(`/api/history/${e.id}/file`, {}).catch(oops); if (!r) return;
    const printer = S.info.printers.includes(e.printer) ? e.printer : S.printer;
    await printSpec({ printer, mode: 'docs', items: [{ id: r.file.id }], sides: e.sides, copies: e.copies, cover: 1, join: true, fitPage: false,
      options: e.options || {}, again: true });
  }));
  box.querySelectorAll('[data-change]').forEach(b => b.onclick = () => guard(async () => {
    const e = entry(b.dataset.change);
    const r = await api(`/api/history/${e.id}/file`, {}).catch(oops); if (!r) return;
    S.docs = [r.file];
    Object.assign(S.doc, { sides: e.sides, copies: e.copies, cover: 1, fitPage: false });
    go('docs'); setTimeout(() => openWork('docs'), 60);
  }));
  box.querySelectorAll('[data-forget]').forEach(b => b.onclick = async () => {
    await api('/api/history/' + b.dataset.forget, null, 'DELETE').catch(oops); again();
  });
  $('#agclear').onclick = async () => {
    if (!await ask('<h2>Clear the list?</h2><p>The printed pages stay printed; they just won\'t be here to print again.</p>', [['yes', 'Clear']])) return;
    await api('/api/history', null, 'DELETE').catch(oops); again();
  };
}
