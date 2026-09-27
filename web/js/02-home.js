// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 02: The home screen.
'use strict';

// ---------- home ----------
const PRINTER_ART = `<svg viewBox="0 0 120 100" aria-hidden="true">
  <rect x="36" y="4" width="48" height="34" rx="4" fill="#fff" stroke="var(--deep)" stroke-width="2.5"/>
  <path d="M44 14h32M44 21h26M44 28h30" stroke="var(--acc)" stroke-width="3" stroke-linecap="round"/>
  <rect x="10" y="30" width="100" height="44" rx="14" fill="var(--deep)"/>
  <rect x="10" y="30" width="100" height="12" rx="6" fill="rgba(255,255,255,.14)"/>
  <circle cx="94" cy="50" r="4" fill="#7ee0a3"/>
  <rect x="28" y="60" width="64" height="7" rx="3.5" fill="rgba(0,0,0,.28)"/>
  <path d="M32 64h56v26a5 5 0 0 1-5 5H37a5 5 0 0 1-5-5z" fill="#fff" stroke="var(--deep)" stroke-width="2.5"/>
  <g transform="translate(60 80)" fill="var(--acc)"><circle cy="-5" r="4"/><circle cx="4.8" cy="-1.5" r="4"/><circle cx="3" cy="4" r="4"/><circle cx="-3" cy="4" r="4"/><circle cx="-4.8" cy="-1.5" r="4"/><circle r="2.4" fill="var(--deep)"/></g>
</svg>`;

function home() {
  let ti = 0;
  const tile = k => `<button class="ftile" data-go="${k}" style="${grad(k)};--i:${ti++}"><span class="fic">${icon(F[k].ic)}</span>
    <span class="ft"><b>${F[k].t}</b><small>${F[k].s}</small></span></button>`;
  const h = new Date().getHours();
  const hello = h < 12 ? 'Good morning' : h < 17 ? 'Good afternoon' : 'Good evening';
  const phones = S.info.local ? `<div class="card home-phones"><div id="homepa"></div>
      ${(S.info.urls || []).length ? `<p class="sub" style="margin:12px 0 6px">On a phone on the same WiFi, open:</p>${S.info.urls.map(u => `<div class="url">${esc(u)}</div>`).join('')}` : ''}
      ${!S.info.requirePin ? '' : (S.info.pins || []).length
        ? `<p class="hint">It asks for a PIN once. PINs are set up for: ${S.info.pins.map(p => esc(p.label)).join(', ')}.</p>`
        : `<p class="hint">Phones need a PIN first.</p><button class="btn main" data-go="settings">Add a PIN</button>`}</div>` : '';
  render(`<div class="greet">${hello}</div>
    <div class="hero-card" data-go="printer" role="button">
      <div class="printer-art">${PRINTER_ART}</div>
      <div class="pinfo"><div class="plabel">Your printer</div><div class="pname" id="pname">${esc(S.pname || S.printer)}</div>
        <div class="pstate"><span class="dot" id="pdot"></span><span id="pstate">Checking…</span></div></div>
      <div class="chev">›</div></div>
    <div class="sect">Print</div>
    <div class="ftiles">${tile('docs')}${tile('photos')}${tile('text')}${tile('again')}</div>
    ${S.info.canScan ? `<div class="sect">Scan</div><div class="ftiles">${tile('scan')}${tile('copy')}</div>` : ''}
    ${phones}`);
  if (S.info.local) phoneAccess($('#homepa'));
  flipWaiting();
  heldWaiting();
  if (!S.printer) return;
  const status = S.firstStatus || api('/api/printer?printer=' + encodeURIComponent(S.printer));
  S.firstStatus = null;   // next time the home screen opens, ask again
  status.then(st => {
    const n = $('#pname'), d = $('#pdot'), t = $('#pstate');
    S.pname = prettyPrinter(st); S.pstate = stateClass(st); paintChip();
    if (!n) return;
    n.textContent = S.pname;
    const [cls, txt] = st.state === 'ready' ? ['ok', st.jobs.length ? `Ready · ${st.jobs.length} waiting` : 'Ready to print']
      : st.state === 'printing' ? ['busy', 'Printing…'] : st.state === 'stopped' ? ['bad', 'Needs attention'] : ['', 'Status unknown'];
    d.className = 'dot ' + cls; t.textContent = txt;
    const hc = $('.hero-card'); if (hc) hc.classList.toggle('busy', cls === 'busy');
  }).catch(() => { const t = $('#pstate'); if (t) t.textContent = 'Can\'t check right now'; });
}

// prints sent from another app's Print button that are waiting for the paper to be turned over
function flipWaiting() {
  api('/api/jobs/waiting').then(list => {
    const g = $('.greet'); if (!g || !list.length || $('.flipwait')) return;
    const div = document.createElement('div');
    div.className = 'card flipwait';
    div.innerHTML = list.map(j => `<div class="fwrow">${ic('printer')}<span><b>${esc(j.title || 'A print')}</b> is waiting: flip the paper for the other side.</span>
      <button class="btn main" data-flip="${esc(j.id)}">Flip now</button></div>`).join('');
    g.after(div);
    div.querySelectorAll('[data-flip]').forEach(b => b.onclick = () => { const j = list.find(x => x.id === b.dataset.flip); div.remove(); followJob(j); });
  }).catch(() => {});
}

// prints from phones that haven't logged in with a PIN (a guest, or a neighbour): shown with a picture, to allow or not
function heldWaiting() {
  api('/api/airprint/waiting').then(list => {
    const g = $('.greet'); if (!g || !list.length || $('.heldwait')) return;
    const div = document.createElement('div');
    div.className = 'card heldwait';
    div.innerHTML = `<h2>${ic('printer')} ${list.length > 1 ? list.length + ' prints are' : 'A print is'} waiting for you</h2>
      <p class="hint" style="margin-top:0">From a phone that hasn't logged in to Sakura Print. Only print it if you know who sent it.</p>
      ${list.map(j => `<div class="hwrow" data-id="${j.id}">
        ${j.file ? `<img src="/api/thumb/${encodeURIComponent(j.file)}" alt="">` : '<span class="hwimg"></span>'}
        <div class="hwtext"><b>${esc(j.title)}</b><small>${esc(j.from)}${j.pages ? ` · ${j.pages} page${j.pages > 1 ? 's' : ''}` : ''}</small></div>
        <div class="hwbtns"><button class="btn" data-act="refuse">Don't print</button><button class="btn main" data-act="allow">Print</button>
          <button class="btn" data-act="trust">Print, and always allow this phone</button></div></div>`).join('')}`;
    g.after(div);
    div.querySelectorAll('[data-act]').forEach(b => b.onclick = () => guard(async () => {
      const row = b.closest('.hwrow');
      try { await api('/api/airprint/waiting/' + row.dataset.id, { action: b.dataset.act }); }
      catch (e) { oops(e); }
      toast(b.dataset.act === 'refuse' ? 'Not printed' : b.dataset.act === 'trust' ? 'Printing. That phone can print by itself from now on' : 'Printing');
      row.remove();
      if (!div.querySelector('.hwrow')) div.remove();
    }));
  }).catch(() => {});
}
