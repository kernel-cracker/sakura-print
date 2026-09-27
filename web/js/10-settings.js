// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 10: Settings.
'use strict';

// ---------- settings ----------
async function settings() {
  const s = await api('/api/settings').catch(oops); if (!s) return;
  render(`${pageHead('settings', 'Your printer, phones, and how it looks.')}
    <div class="card"><h2>${ic('printer')} Your printer</h2><p class="sub" style="margin:0 0 10px">${esc(S.pname || S.printer)}</p>
      <span class="lbl">The usual printer</span><select class="big" id="fav" style="margin-bottom:10px"><option value="">Computer's default</option>
      ${S.info.printers.map(p => `<option ${p === s.printer ? 'selected' : ''}>${esc(p)}</option>`).join('')}</select>
      <button class="btn wide" id="resetup">Set up the printer again</button>
      ${S.info.local ? '<button class="btn wide" id="drivers" style="margin-top:8px">Add a printer, or change its driver</button>' : ''}
      <p class="hint">Shows what your printer can do, and asks the two setup questions again.</p></div>
    <div class="card"><h2>Look</h2><span class="lbl">App colour</span>${swatches(s.theme).replace(/<button class="swatch [^"]*" data-pal="10"[\s\S]*?<\/button>/, '')}</div>
    ${s.pins !== undefined ? `<div class="card"><h2>${ic('phone')} Phones</h2>
      <div id="setpa"></div>
      <span class="lbl" style="margin-top:14px">On a phone on the same WiFi, open</span>
      ${(S.info.urls || []).map(u => `<div class="url">${esc(u)}</div>`).join('') || '<p class="sub">No WiFi connection found.</p>'}
      <div class="field" style="margin-top:14px">${toggle('pin', s.requirePin, 'Ask for a PIN on phones')}</div>
      <div class="field">${toggle('airprint', s.airprint, 'Print from any app')}</div>
      <p class="hint">${s.airprint
        ? (s.requirePin
          ? 'Phones find this printer by themselves in any app\'s Print button (Photos, WhatsApp, Safari, Files…). Phones that logged in here with a PIN print straight away; prints from other phones wait on the home screen until you allow them.'
          : 'Phones find this printer by themselves in any app\'s Print button (Photos, WhatsApp, Safari, Files…). PINs are off, so <b>any phone on your WiFi can print</b>.')
        : 'Lets phones print from any app\'s own Print button (iPhone AirPrint, Android printing), without opening Sakura Print.'}
        ${s.airprint && !s.bonjour ? '<br><b>Phones can\'t find it yet:</b> install avahi (run <code>./install.sh</code> again).' : ''}
        ${s.airprint && s.phones.state === 'off' ? '<br><b>Phone access is off</b>, so it\'s hidden until you turn that on.' : ''}</p>
      ${s.airprint && s.requirePin && s.devices.length ? `<span class="lbl">Phones that can print from any app</span>
        <ul class="pins">${s.devices.map(d => `<li><span class="plab">${esc(d.label)}${d.byHand ? ' (allowed by hand)' : ''}</span>
          <small class="sub">${esc(agoText(d.seen))}</small>
          <button class="icon-btn" data-rmphone="${esc(d.id)}" data-label="${esc(d.label)}" title="Remove">${ic('x')}</button></li>`).join('')}</ul>` : ''}
      ${s.requirePin ? `<span class="lbl">PINs that work</span>
        ${s.pins.length ? `<ul class="pins">${s.pins.map(p => `<li><span class="pin">••••</span><span class="plab">${esc(p.label)}</span>
          <button class="icon-btn" data-rmpin="${esc(p.id)}" data-label="${esc(p.label)}" title="Remove">${ic('x')}</button></li>`).join('')}</ul>`
          : '<p class="hint">No PINs yet, so phones can\'t get in. Add one below.</p>'}
        <p class="hint">Everyone can have their own PIN. PINs are stored scrambled, so they can't be shown again: write a new one down before adding it. Removing a PIN logs out only the phones that used it.</p>
        <div class="row" style="margin-top:10px"><input class="big" id="pinLabel" placeholder="Who is it for? (Mom)" maxlength="20" style="flex:1 1 150px">
          <input class="big" id="pinNew" placeholder="4 to 8 numbers" inputmode="numeric" maxlength="8" style="flex:1 1 130px">
          <button class="btn main" id="addPin">Add PIN</button></div>` : '<p class="hint">Anyone on your WiFi can print. Not a good idea on a shared WiFi (apartments, hostels).</p>'}</div>` : ''}
    <div class="card"><h2>${ic('info')} Setup and help</h2>
      ${S.info.local ? '<button class="btn wide" data-wz="again">Run the setup and the tour again</button>' : '<button class="btn wide" id="tourAgain">Show the tour again</button>'}
      ${S.info.local ? '<button class="btn wide" data-go="advanced" style="margin-top:8px">Advanced: what\'s going on under the hood</button>' : ''}</div>
    <div class="card"><h2>About</h2><p style="margin:0">Sakura Print ${esc(S.info.version)}</p>
      <p class="hint">Vibe coded with Claude. Made with love for home printers.</p></div>`);
  $('#app').querySelectorAll('[data-pal]').forEach(b => b.onclick = async () => {
    const t = Number(b.dataset.pal); await api('/api/settings', { theme: t }).catch(oops); S.info.theme = t; applyTheme(t); S.doc.palette = -1; settings();
  });
  const dr = $('#drivers'); if (dr) dr.onclick = () => { location.hash = '#/addprinter'; };
  $('#resetup').onclick = async () => { if (await onboarding(S.printer)) { toast('Saved'); settings(); } };
  $('#fav').onchange = async e => { await api('/api/settings', { printer: e.target.value }).catch(oops); toast('Saved'); };
  if ($('#setpa')) phoneAccess($('#setpa'));
  const again = $('[data-wz=again]'); if (again) again.onclick = () => { WZ = null; try { sessionStorage.removeItem('sakuraSetupLater'); } catch (_) {} location.hash = '#/setup'; };
  const tourAgain = $('#tourAgain'); if (tourAgain) tourAgain.onclick = phoneTour;
  $('#app').querySelectorAll('[data-rmphone]').forEach(b => b.onclick = async () => {
    if (!await ask(`<h2>Remove ${esc(b.dataset.label)}?</h2><p>That phone's prints from other apps will wait for you to allow them, until it logs in here with a PIN again.</p>`, [['yes', 'Remove']])) return;
    await api('/api/settings', { removePhone: b.dataset.rmphone }).catch(oops); settings();
  });
  const at = $('[data-tog=airprint]'); if (at) at.onclick = async () => { await api('/api/settings', { airPrint: !s.airprint }).catch(oops); settings(); };
  const tg = $('[data-tog=pin]'); if (tg) tg.onclick = async () => { await api('/api/settings', { requirePin: !s.requirePin }).catch(oops); settings(); };
  $('#app').querySelectorAll('[data-rmpin]').forEach(b => b.onclick = async () => {
    if (!await ask(`<h2>Remove ${esc(b.dataset.label)}'s PIN?</h2><p>Phones that used it will have to log in again.</p>`, [['yes', 'Remove']])) return;
    await api('/api/settings', { removePin: b.dataset.rmpin }).catch(oops); settings();
  });
  const ap = $('#addPin'); if (ap) ap.onclick = async () => {
    try { await api('/api/settings', { addPin: { label: $('#pinLabel').value, pin: $('#pinNew').value.trim() } }); toast('PIN added'); settings(); } catch (e) { oops(e); }
  };
}
