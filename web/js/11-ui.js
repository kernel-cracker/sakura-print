// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 11: Popups, drag & drop, little animations.
'use strict';

// ---------- modals ----------
function modal(html, after) {
  const el = document.createElement('div');
  el.className = 'modal';
  el.innerHTML = `<div class="box">${html}</div>`;
  document.body.appendChild(el);
  const m = {
    el,
    close: () => el.remove(),
    set: (h, cb) => { el.querySelector('.box').innerHTML = h; if (cb) cb(); },
  };
  el.addEventListener('click', e => { if (e.target.closest('[data-close]')) m.close(); });
  if (after) after();
  return m;
}

// ask shows a question with buttons, resolves with the chosen value (or null for cancel)
function ask(html, choices) {
  return new Promise(res => {
    const m = modal(`${html}<div class="foot"><button class="btn" data-c="">Cancel</button>${choices.map(([v, l, s], i) =>
      `<button class="btn ${i === choices.length - 1 ? 'main' : ''}" data-c="${esc(v)}">${l}${s ? `<br><small style="font-weight:400">${s}</small>` : ''}</button>`).join('')}</div>`);
    m.el.querySelectorAll('[data-c]').forEach(b => b.onclick = () => { m.close(); res(b.dataset.c || null); });
  });
}

function showPin() {
  if ($('#pinBox')) return;
  const m = modal(`<div id="pinBox">${bigIcon('lock')}<h2 style="text-align:center">Enter the PIN</h2>
    <p class="sub" style="text-align:center">It's shown in Sakura Print on the computer, under Settings.</p>
    <input class="big" id="pinIn" inputmode="numeric" maxlength="8" autocomplete="one-time-code" style="text-align:center;font-size:30px;letter-spacing:12px">
    <div class="foot"><button class="btn main wide" id="pinGo">Unlock</button></div></div>`);
  const go = async () => {
    try { await api('/api/login', { pin: $('#pinIn').value }); burst($('#pinBox .bigicon')); m.close(); boot(); }
    catch (e) { toast(e.message === 'locked' ? 'Try again' : e.message, true); $('#pinIn').value = ''; }
  };
  $('#pinGo').onclick = go;
  $('#pinIn').onkeydown = e => { if (e.key === 'Enter') go(); };
  setTimeout(() => $('#pinIn').focus(), 50);
}

// ---------- drag & drop files onto the window ----------
let dragDepth = 0;
document.addEventListener('dragenter', e => { if (!e.dataTransfer.types.includes('Files')) return; dragDepth++;
  if (!$('.drop')) { const d = document.createElement('div'); d.className = 'drop'; d.textContent = 'Drop to add'; document.body.appendChild(d); } });
document.addEventListener('dragleave', () => { if (--dragDepth <= 0) { dragDepth = 0; const d = $('.drop'); if (d) d.remove(); } });
document.addEventListener('dragover', e => e.preventDefault());
document.addEventListener('drop', e => {
  e.preventDefault(); dragDepth = 0; const d = $('.drop'); if (d) d.remove();
  const fs = [...e.dataTransfer.files]; if (!fs.length) return;
  const r = location.hash.replace(/^#\/?/, '');
  const kind = r === 'photos' ? 'photos' : 'docs';
  upload(fs, x => { if (addFiles(kind, x)) { if (r !== kind) go(kind); setTimeout(() => openWork(kind), 60); } });
});

// ---------- little animations ----------
document.addEventListener('pointerdown', e => {
  const b = e.target.closest('.btn, .ftile, .seg button, .choice, #tabs button');
  if (!b || b.disabled) return;
  const r = b.getBoundingClientRect(), d = Math.max(r.width, r.height) * 2.2;
  const s = document.createElement('span');
  s.className = 'rip';
  s.style.cssText = `width:${d}px;height:${d}px;left:${e.clientX - r.left - d / 2}px;top:${e.clientY - r.top - d / 2}px`;
  b.appendChild(s);
  setTimeout(() => s.remove(), 500);
});

function burst(from) {   // a little puff of sakura petals
  if (!from || matchMedia('(prefers-reduced-motion: reduce)').matches) return;
  const r = from.getBoundingClientRect(), cx = r.left + r.width / 2, cy = r.top + r.height / 2;
  for (let i = 0; i < 14; i++) {
    const p = document.createElement('i');
    const a = (i / 14) * Math.PI * 2 + Math.random() * .4, dist = 70 + Math.random() * 70;
    p.className = 'petal';
    p.style.cssText = `left:${cx}px;top:${cy}px;--x:${Math.cos(a) * dist}px;--y:${Math.sin(a) * dist + 30}px;--r:${Math.random() * 540 - 270}deg;animation-delay:${Math.random() * 60}ms`;
    document.body.appendChild(p);
    setTimeout(() => p.remove(), 1100);
  }
}

const prettyPrinter = st => (st.model || st.name || '').replace(/\s*(CUPS|- IPP Everywhere|, driverless.*)$/i, '').trim() || st.name;

// ---------- the printer chip (in the header): which printer, how it is, and switching ----------
function paintChip() {
  const c = $('#pchip'); if (!c) return;
  c.querySelector('.pname').textContent = S.pname || S.printer || '';
  c.setAttribute('aria-label', 'Printer: ' + (S.pname || S.printer || 'none') + (S.pstate === 'bad' ? ' (needs attention)' : ''));
  c.title = S.pname || S.printer || '';
  c.querySelector('.dot').className = 'dot ' + (S.pstate || '');
}
async function printerSheet() {
  const list = S.info.printers || [];
  const m = modal(`<h2>Printer</h2>
    <div class="plist">${list.map(p => `<button class="pchoice ${p === S.printer ? 'on' : ''}" data-printer="${esc(p)}">${icon('printer')}<span>${esc(p === S.printer && S.pname ? S.pname : p)}${p === S.info.default ? '<small>The usual one</small>' : ''}</span>${p === S.printer ? ic('check') : ''}</button>`).join('')}</div>
    ${S.info.local ? '<button class="btn wide" data-go="addprinter" style="margin-top:12px">Add a printer, or change its driver</button>' : ''}
    <div class="foot"><button class="btn" data-close>Close</button></div>`);
  m.el.querySelectorAll('[data-go]').forEach(b => b.addEventListener('click', () => m.close()));
  m.el.querySelectorAll('.pchoice').forEach(b => b.onclick = async () => {
    m.close();
    if (b.dataset.printer === S.printer) return;
    S.printer = b.dataset.printer; S.pname = ''; S.pstate = ''; S.firstStatus = null;
    paintChip();
    await loadOptions(S.printer).catch(oops);
    route();
    api('/api/printer?printer=' + encodeURIComponent(S.printer)).then(st => { S.pname = prettyPrinter(st); S.pstate = stateClass(st); paintChip(); }).catch(() => {});
  });
}
const stateClass = st => st.state === 'ready' ? 'ok' : st.state === 'printing' ? 'busy' : st.state === 'stopped' ? 'bad' : '';
document.addEventListener('click', e => { if (e.target.closest('#pchip')) printerSheet(); });

// ---------- phone access: one control, the same wherever it's shown (home, Settings, setup) ----------
// It shows what's true now: it ticks every second (minutes left) and asks the computer every 20 seconds, so a
// timer running out while the screen is open is shown straight away (it used to keep saying "on").
function phoneAccess(el) {
  let ph = S.info.phones || { state: 'on' }, busy = false;
  const left = () => {
    const ms = new Date(ph.until) - Date.now();
    if (ms < 60000) return 'less than a minute left';
    return `${Math.ceil(ms / 60000)} min left`;
  };
  // the buttons are built again only when something changes: rebuilding them under a finger or a mouse (it used to
  // happen every second) loses the press. The countdown only changes the text.
  let shown = '';
  const draw = () => {
    if (!document.body.contains(el)) return false;
    if (ph.state === 'until' && new Date(ph.until) <= Date.now()) ph = { state: 'off' };   // until the computer says otherwise
    const on = ph.state !== 'off', chip = ph.state === 'on' ? 'always' : ph.state === 'until' ? ph.hours || '' : '';
    const line = on
      ? (ph.state === 'until' ? `Phones can use Sakura Print until ${tillText(ph.until)} (${left()}).` : 'Phones can use Sakura Print.')
      : 'Phones can\'t use Sakura Print right now.';
    const key = `${on}|${chip}`;
    if (key === shown && el.querySelector('.pa-line')) { el.querySelector('.pa-line').textContent = line; return true; }
    shown = key;
    el.innerHTML = `<div class="phoneaccess ${on ? 'on' : 'off'}">
      <div class="pa-head">${icon('phone')}<div class="pa-text"><b>Phones</b><span class="pa-line">${esc(line)}</span></div>
        <button class="switch" role="switch" data-pa="toggle" aria-checked="${on}" aria-label="Phones can use Sakura Print"><i></i></button></div>
      ${on ? `<div class="pa-chips">${[['always', 'Always'], ['1h', 'For 1 hour'], ['3h', 'For 3 hours']].map(([k, l]) =>
        `<button data-pa-chip="${k}" class="${k === chip ? 'on' : ''}">${l}</button>`).join('')}</div>
      <p class="hint">On a shared WiFi (apartments, hostels), let phones in for an hour when someone needs it: it switches itself off again.</p>` : ''}
    </div>`;
    el.querySelector('[data-pa=toggle]').onclick = () => set(on ? 'off' : 'on');
    el.querySelectorAll('[data-pa-chip]').forEach(b => b.onclick = () => set(b.dataset.paChip === 'always' ? 'on' : b.dataset.paChip));
    return true;
  };
  const hoursOf = p => p.state === 'until' ? (new Date(p.until) - Date.now() > 61 * 60000 ? '3h' : '1h') : '';
  async function set(v) {
    if (busy) return; busy = true;
    try { const r = await api('/api/settings', { phones: v }); ph = { ...r.phones, hours: v === '1h' || v === '3h' ? v : hoursOf(r.phones) }; S.info.phones = ph; draw(); }
    catch (e) { oops(e); } finally { busy = false; }
  }
  async function fetchNow() {
    try { const r = await api('/api/settings'); if (r.phones) { ph = { ...r.phones, hours: ph.hours || hoursOf(r.phones) }; S.info.phones = ph; } } catch (_) {}
  }
  ph = { ...ph, hours: hoursOf(ph) };
  draw();
  let n = 0;
  const t = setInterval(async () => {
    n++;
    const expired = ph.state === 'until' && new Date(ph.until) <= Date.now();
    if (expired || n % 20 === 0) await fetchNow();
    if (!draw()) clearInterval(t);
  }, 1000);
}
