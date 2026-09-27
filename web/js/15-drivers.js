// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 15: Printers & drivers: find printers, and get each one the best driver (docs/design/drivers.md).
'use strict';

const DRV_STATE = {
  driver: ['ok', 'Set up with its maker\'s driver'],
  driverless: ['', 'Set up without a driver: can be slower'],
  broken: ['bad', 'Its driver is missing: it can\'t print'],
  '': ['bad', 'Not set up yet'],
};

async function printers() {
  if (!S.info.local) {
    render(`${pageHead('addprinter', 'Setting up printers')}<div class="card"><p style="margin:0">Printers are set up on the computer itself: open Sakura Print there.</p></div>`);
    return;
  }
  render(`${pageHead('addprinter', 'Looking for printers on the WiFi and USB…')}<div class="spin"></div>`);
  let r;
  try { r = await api('/api/drivers'); } catch (e) { return oops(e); }
  const list = r.devices;
  render(`${pageHead('addprinter', list.length ? 'Your printers, and the best driver for each.' : 'No printers found.')}
    ${list.map((d, i) => {
      const [cls, txt] = DRV_STATE[d.queue ? d.queueDriver : ''];
      return `<div class="card drvcard"><div class="drvrow">${bigIcon('printer')}<div class="drvname"><b>${esc(d.name)}</b>
        <span class="pill ${cls}">${txt}</span>${d.queue ? `<small class="sub">${esc(d.queueModel || d.queue)}</small>` : ''}</div></div>
        <button class="btn ${d.queueDriver === 'driver' ? '' : 'main'} wide" data-dev="${i}">${d.queueDriver === 'driver' ? 'Change driver' : d.queueDriver === 'broken' ? 'Get its driver again' : d.queue ? 'Get its maker\'s driver' : 'Set it up'}</button></div>`;
    }).join('')}
    ${list.length ? '' : `<div class="card"><p style="margin:0">Is the printer switched on, and on the same WiFi (or plugged in with USB)? Then look again.</p></div>`}
    <div class="row" style="gap:8px"><button class="btn wide" id="again">${ic('turn')} Look again</button><button class="btn wide" id="updates">Check for driver updates</button></div>
    <p class="hint">Drivers come from the printer's maker or from ${esc(DRV_SYS[r.system.kind] || 'your system')}. Sakura Print asks for the computer's password before installing anything.</p>`);
  $('#again').onclick = printers;
  $('#updates').onclick = () => guard(checkDriverUpdates);
  $('#app').querySelectorAll('[data-dev]').forEach(b => b.onclick = () => drvPlan(list[+b.dataset.dev]));
}

const DRV_SYS = { apt: 'Debian/Ubuntu', dnf: 'Fedora', zypper: 'openSUSE', pacman: 'Arch', ostree: 'Fedora Atomic', nixos: 'NixOS', pkcon: 'your system' };

async function drvPlan(d) {
  render(`${pageHead('addprinter', esc(d.name))}<div class="spin"></div><p class="hint" style="text-align:center">Checking which drivers there are…</p>`);
  let p;
  try { p = await api('/api/drivers/plan?key=' + encodeURIComponent(d.key)); } catch (e) { return oops(e); }
  const best = p.routes.find(r => r.id === p.best);
  const others = p.routes.filter(r => r.state !== 'skipped' && r.id !== p.best);
  const skipped = p.routes.filter(r => r.state === 'skipped');
  const card = (r, main, label) => `<div class="card drvroute ${main ? 'best' : ''}" data-route="${r.id}">
      ${main ? `<span class="lbl">${label || 'Best for this printer'}</span>` : ''}<h2>${esc(r.title)}</h2><p class="sub">${esc(r.detail || '')}</p>
      ${r.aur ? `<div class="aurlist">${r.aur.map((a, i) => `<label class="aurpkg"><input type="radio" name="aur-${r.id}" value="${esc(a.name)}" ${i ? '' : 'checked'}>
        <span><b>${esc(a.name)}</b> ${esc(a.version)}<small>${a.votes} vote${a.votes === 1 ? '' : 's'} · by ${esc(a.maintainer)} · updated ${esc(new Date(a.updated * 1000).toLocaleDateString())}${a.licence ? ' · licence: ' + esc(a.licence) : ''}</small></span></label>`).join('')}</div>` : ''}
      ${r.restart ? '<p class="hint">Takes effect after the computer restarts.</p>' : ''}
      <button class="btn ${main ? 'main' : ''} wide" data-go-route="${r.id}">${r.state === 'ready' ? 'Set it up' : r.id === 'guided' ? 'Open the website' : 'Get it'}</button></div>`;
  const scanReady = (p.scan || []).find(r => r.state === 'ready');
  const scanBest = (p.scan || []).find(r => r.id === p.scanBest && r.state === 'available');
  const scanOthers = (p.scan || []).filter(r => r.state === 'available' && r !== scanBest);
  const scanSkipped = (p.scan || []).filter(r => r.state === 'skipped');
  const scanning = scanReady ? `<div class="card"><h2>${ic('scan')} Scanner</h2><p class="sub" style="margin:0">${ic('check')} Scanning works. ${esc(scanReady.detail)}</p></div>`
    : scanBest ? `<div class="sect">Scanner</div>${card(scanBest, true, 'Best for scanning')}${scanOthers.map(r => card(r, false)).join('')}`
    : '';
  render(`${pageHead('addprinter', esc(d.name))}
    ${best ? card(best, true) : '<div class="card"><p style="margin:0">No way to set this printer up was found here.</p></div>'}
    ${others.length ? `<div class="sect">Other ways</div>${others.map(r => card(r, false)).join('')}` : ''}
    ${scanning}
    ${skipped.length + scanSkipped.length ? `<details class="card drvwhy"><summary>Why not the others</summary><ul>${skipped.concat(scanReady ? [] : scanSkipped).map(r => `<li><b>${esc(r.title)}</b>: ${esc(r.why)}</li>`).join('')}</ul></details>` : ''}
    <button class="btn wide" id="back">Back to the printers</button>`);
  $('#back').onclick = printers;
  $('#app').querySelectorAll('[data-go-route]').forEach(b => b.onclick = () => guard(async () => {
    const id = b.dataset.goRoute, choice = ($(`input[name=aur-${id}]:checked`) || {}).value || '';
    if (id.endsWith('aur') && !await ask(`<h2>A community package</h2><p>AUR packages are made by volunteers and nobody checks them. A terminal window opens where
      you can read the recipe (PKGBUILD) before it installs: only go ahead if it downloads from ${esc(d.make)}.</p>`, [['yes', 'Open the terminal']])) return;
    let j;
    try { j = await api('/api/drivers/run', { key: d.key, route: id, choice }); } catch (e) { return oops(e); }
    followDriverJob(j, d);
  }));
}

function followDriverJob(j, d) {
  const m = modal('');
  let last = '', timer = null;
  const answer = async body => { try { j = await api('/api/drivers/job/' + j.id, body); draw(); } catch (e) { oops(e); } };
  const draw = () => {
    const key = j.state + (j.ask ? j.ask.kind : '') + j.step;
    if (key === last) return; last = key;
    if (j.state === 'asking' && j.ask) {
      const a = j.ask;
      if (a.kind === 'licence') {
        m.set(`<h2>${esc(a.title)}</h2><p class="sub">Read it, then agree to install. Nothing is installed if you don't.</p>
          <pre class="licence">${esc(a.text)}</pre>
          <div class="foot"><button class="btn" data-a="no">I don't agree</button><button class="btn main" data-a="yes">I agree</button></div>`);
      } else if (a.kind === 'download') {
        m.set(`<h2>${esc(a.title)}</h2><p>${esc(a.text)}</p><div class="spin"></div>
          <div class="foot"><button class="btn" data-a="no">Stop</button><button class="btn" data-a="site">${ic('turn')} Open the website again</button>
          <label class="btn main">${ic('folder')} Choose the file<input type="file" accept=".deb,.rpm,.ppd,.gz" hidden id="drvfile"></label></div>`);
        const fi = m.el.querySelector('#drvfile');
        fi.onchange = async () => {
          if (!fi.files[0]) return;
          const fd = new FormData(); fd.append('file', fi.files[0]);
          try { const r = await fetch('/api/drivers/job/' + j.id + '/file', { method: 'POST', body: fd }); j = await r.json(); draw(); } catch (e) { oops(e); }
        };
      } else {
        const [no, yes] = { app: ['Stop', 'Check again'], plugin: ['Skip', 'Install HP\'s plugin'] }[a.kind] || ['Cancel', 'Use it'];
        m.set(`<h2>${esc(a.title)}</h2><p>${esc(a.text)}</p>
          <div class="foot"><button class="btn" data-a="no">${no}</button><button class="btn main" data-a="yes">${yes}</button></div>`);
      }
      m.el.querySelectorAll('[data-a]').forEach(b => b.onclick = () => {
        if (b.dataset.a === 'site') { window.open(a.site, '_blank'); return; }
        answer({ ok: b.dataset.a === 'yes' });
      });
    } else if (j.state === 'working') {
      m.set(`${bigIcon('printer')}<h2 style="text-align:center">${esc(j.step || 'Working…')}</h2><div class="spin"></div>
        <p class="hint" style="text-align:center">If a password window opens, it's the computer's own: type the password you log in with.</p>`);
    } else if (j.state === 'done' && j.scanner) {
      clearInterval(timer);
      m.set(`${bigIcon('check')}<h2 style="text-align:center">Scanning works</h2><p style="text-align:center" class="sub">Found as ${esc(j.queue)}.</p>
        <div class="foot"><button class="btn main" data-close>OK</button></div>`);
      burst(m.el.querySelector('.bigicon'));
      S.info.canScan = true;
      m.el.querySelector('[data-close]').addEventListener('click', () => drvPlan(d));
    } else if (j.state === 'done') {
      clearInterval(timer);
      m.set(`${bigIcon('check')}<h2 style="text-align:center">${esc(d.name)} is ready</h2>
        <p style="text-align:center" class="sub">${j.restart ? 'Restart the computer to finish.' : 'Now the two quick setup questions, so every instruction matches your printer.'}</p>
        ${j.note ? `<p class="hint" style="text-align:center"><b>${esc(j.note)}</b></p>` : ''}
        <div class="foot"><button class="btn main" id="drvnext">${j.restart ? 'OK' : 'Continue'}</button></div>`);
      burst(m.el.querySelector('.bigicon'));
      m.el.querySelector('#drvnext').onclick = async () => {
        m.close();
        try { S.info = await api('/api/info'); } catch (_) {}
        if (j.queue && S.info.printers.includes(j.queue)) {
          S.printer = j.queue; S.pname = ''; paintChip();
          await loadOptions(S.printer).catch(() => {});
          if (S.wzActive && WZ) { WZ.printer = j.queue; WZ.step = 'printer'; location.hash = '#/setup'; return; }   // back to setup, which asks the rest
          if (!j.restart) await onboarding(S.printer);
        }
        location.hash = S.wzActive ? '#/setup' : '#/';
      };
    } else if (j.state === 'failed') {
      clearInterval(timer);
      m.set(`<h2>Didn't work</h2><p>${esc(j.error)}</p>
        ${j.log && j.log.length ? `<details><summary>What happened</summary><ul class="drvlog">${j.log.map(l => `<li>${esc(l)}</li>`).join('')}</ul></details>` : ''}
        <div class="foot"><button class="btn main" id="drvother">Try another way</button></div>`);
      m.el.querySelector('#drvother').onclick = () => { m.close(); drvPlan(d); };
    }
  };
  draw();
  timer = setInterval(async () => {
    if (!document.body.contains(m.el)) return clearInterval(timer);
    try { j = await api('/api/drivers/job/' + j.id); draw(); } catch (_) {}
  }, 1000);
}

async function checkDriverUpdates() {
  const w = modal('<h2 style="text-align:center">Asking the makers…</h2><div class="spin"></div>');
  let ups;
  try { ups = await api('/api/drivers/updates'); } catch (e) { w.close(); return oops(e); } finally { w.close(); }
  if (!ups.length) return toast('All drivers are up to date');
  const m = modal(`<h2>Driver updates</h2>${ups.map(u => `<p><b>${esc(u.name)}</b>: ${esc(u.package)} ${esc(u.installed)} → ${esc(u.available)}</p>`).join('')}
    <p class="hint">Updating shows the maker's licence again if it changed, then asks for the computer's password.</p>
    <div class="foot"><button class="btn" data-close>Later</button><button class="btn main" id="doup">Update</button></div>`);
  m.el.querySelector('#doup').onclick = async () => {
    m.close();
    const u = ups[0];   // one at a time: the rest show up when this is checked again
    let r;
    try { r = await api('/api/drivers'); } catch (e) { return oops(e); }
    const d = r.devices.find(x => x.key === u.device);
    if (!d) return toast('The printer isn\'t reachable right now', true);
    try { followDriverJob(await api('/api/drivers/run', { key: d.key, route: u.route || 'maker' }), d); } catch (e) { oops(e); }
  };
}
