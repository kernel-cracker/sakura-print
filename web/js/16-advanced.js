// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 16: Advanced: what's going on under the hood, for fixing problems (the computer only).
'use strict';

async function advanced() {
  if (!S.info.local) {
    render(`${pageHead('advanced', 'Under the hood')}<div class="card"><p style="margin:0">This is only on the computer itself.</p></div>`);
    return;
  }
  render(`${pageHead('advanced', 'Under the hood: for fixing problems')}<div class="spin"></div>`);
  let d;
  try { d = await api('/api/debug'); } catch (e) { return oops(e); }
  const row = (k, v) => `<div class="adv-row"><span>${esc(k)}</span><b>${v}</b></div>`;
  const yes = v => v ? `<span class="pill ok">yes</span>` : `<span class="pill bad">no</span>`;
  const driverWord = { driver: 'maker\'s driver', driverless: 'driverless', broken: 'driver missing' };
  const ap = d.AirPrint || {}, ph = d.Phones || {};
  render(`${pageHead('advanced', 'Under the hood: for fixing problems')}
    <div class="card adv-section"><h2>This copy of Sakura Print</h2>
      ${row('Version', esc(d.Version))}${row('Built', esc(d.Build))}${row('Go', esc(d.Go) + ' · ' + esc(d.OS))}${row('Running for', esc(d.Uptime))}
      ${row('Settings', `<code>${esc(d.Paths.settings)}</code>`)}${row('Data', `<code>${esc(d.Paths.data)}</code>`)}</div>
    <div class="card adv-section"><h2>This computer</h2>
      ${row('System', esc(d.Distro || 'unknown'))}${row('Linux', esc(d.Kernel))}${row('Installs with', esc(d.Install.kind || 'nothing known'))}</div>
    <div class="card adv-section"><h2>CUPS and print queues</h2>
      ${row('CUPS', esc(d.CUPSServer))}${row('Usual printer', esc(d.Default || 'none'))}
      ${(d.Queues || []).map(q => `<div class="adv-queue"><b>${esc(q.Name)}</b><small>${esc(q.Model)}</small>
        <span class="pill ${q.Driver === 'broken' ? 'bad' : q.Driver === 'driver' ? 'ok' : ''}">${driverWord[q.Driver] || q.Driver}</span>
        <span class="pill ${q.State === 'stopped' ? 'bad' : ''}">${esc(q.State)}${q.Reasons && q.Reasons !== 'none' ? ': ' + esc(q.Reasons) : ''}</span>
        <code>${esc(q.URI)}</code></div>`).join('') || '<p class="sub">No print queues.</p>'}
      <button class="btn" data-go="addprinter">Printers & drivers</button>
      <button class="btn" id="advdoctor">Check drivers (doctor)</button><pre class="adv-pre" id="advdoc" hidden></pre></div>
    <div class="card adv-section"><h2>Scanners</h2>
      ${!d.ScannersKnown ? '<p class="sub">Not looked for yet.</p>' : (d.Scanners || []).map(s => row(s.name, `<code>${esc(s.device)}</code>`)).join('') || '<p class="sub">None found.</p>'}</div>
    <div class="card adv-section"><h2>Print from any app</h2>
      ${row('Switched on', yes(ap.switchedOn))}${row('Taking prints now', yes(ap.accepting))}${row('Bonjour (avahi) installed', yes(ap.bonjourTool))}
      ${row('Announced on the WiFi', yes(ap.announcing))}${row('Prints waiting to be allowed', esc(String(ap.waitingToBeAllowed ?? 0)))}</div>
    <div class="card adv-section"><h2>Phones</h2>
      ${row('Access', esc(phonesLine(ph.access)))}${row('PINs', esc(String(ph.pins)))}${row('Logged in', esc(String(ph.loggedIn)))}
      ${row('Allowed to print from any app', esc(String(ph.allowedToPrintFromAnyApp)))}</div>
    <div class="card adv-section"><h2>Tools</h2><div class="adv-tools">${Object.entries(d.Tools || {}).sort().map(([t, ok]) =>
      `<span class="pill ${ok ? 'ok' : 'bad'}">${esc(t)}</span>`).join('')}</div></div>
    <div class="card adv-section"><h2>What the server has been doing</h2>
      <ol class="adv-log">${(d.Log || []).slice(-80).reverse().map(l => `<li>${esc(l)}</li>`).join('') || '<li>Nothing yet.</li>'}</ol></div>
    <div class="card adv-section"><h2>Help with a problem</h2>
      <p class="sub">The report says all of the above in text, to paste into a bug report. It never has PINs, logins or phones in it, and WiFi addresses are blanked out.</p>
      <div class="row"><button class="btn main" id="advreport">${ic('copy')} Copy the report</button><a class="btn" href="/api/debug/report" download="sakura-print-report.txt">Save it as a file</a>
      <button class="btn" id="advrefresh">${ic('turn')} Look at the printers again</button></div></div>`);
  $('#advreport').onclick = () => guard(async () => {
    const txt = await (await fetch('/api/debug/report')).text();
    try { await navigator.clipboard.writeText(txt); toast('Copied'); }
    catch (_) { const m = modal(`<h2>The report</h2><pre class="adv-pre">${esc(txt)}</pre><div class="foot"><button class="btn main" data-close>Close</button></div>`); }
  });
  $('#advrefresh').onclick = () => guard(async () => { await api('/api/debug/refresh', {}); toast('Looked again'); advanced(); });
  $('#advdoctor').onclick = () => guard(async () => {
    const pre = $('#advdoc'); pre.hidden = false; pre.textContent = 'Checking… (this takes a few seconds)';
    pre.textContent = await (await fetch('/api/debug/doctor')).text();
  });
}
