// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 09: Printer care.
'use strict';

// ---------- printer care ----------
async function care() {
  render(`${pageHead('printer', 'Checking your printer…')}<div class="spin"></div>`);
  let st;
  try { st = await api('/api/printer?printer=' + encodeURIComponent(S.printer)); } catch (e) { return oops(e); }
  const state = { ready: ['ok', 'Ready'], printing: ['', 'Printing'], stopped: ['bad', 'Stopped'] }[st.state] || ['', 'Unknown'];
  const known = st.markers.filter(m => m.level >= 0);
  const ink = known.length ? `<div class="ink">${known.map(m => `<div class="tank"><div><i style="height:${m.level}%;background:${esc(m.color || '#999')}"></i></div>${m.level}%<br>${esc(m.name.replace(/ ?(ink|cartridge)/gi, ''))}</div>`).join('')}</div>`
    : `<p class="sub" style="margin:0">This printer doesn't tell the computer its ink levels. Check the ink windows on the front of the printer.</p>`;
  const cmdName = { Clean: ['Clean the print head', 'Fixes faded or streaky prints. Uses a little ink.'], PrintSelfTestPage: ['Print a test page', 'Shows if all colours print properly.'] };
  render(`${pageHead('printer', esc((st.model || st.name).replace(/\s*CUPS$/, '')))}
    <div class="card"><h2>Status</h2><span class="pill ${state[0]}">${state[1]}</span>
      ${st.reasons ? `<p class="hint">${esc(st.reasons)}</p>` : ''}
      <p style="margin:12px 0 0">${st.jobs.length ? `${st.jobs.length} job${st.jobs.length > 1 ? 's' : ''} waiting.` : 'Nothing waiting to print.'}</p>
      ${st.jobs.length ? '<button class="btn danger" id="cancelAll" style="margin-top:10px">Cancel everything waiting</button>' : ''}</div>
    <div class="card"><h2>Printer type</h2><div class="row" style="align-items:center">
      <div style="width:150px">${S.style[S.printer] ? ART[S.style[S.printer]]() : ''}</div>
      <div style="flex:1"><p style="margin:0 0 8px">${S.style[S.printer] === 'rear' ? 'Paper stands up at the back' : S.style[S.printer] === 'tray' ? 'Paper goes in a drawer at the front' : 'Not set yet'}</p>
      <button class="btn" id="setType">Change</button></div></div>
      ${S.style[S.printer] ? `<button class="btn wide" id="howDuplex" style="margin-top:12px">${ic('book')} How both-sides printing works</button>` : ''}</div>
    <div class="card"><h2>Ink</h2>${ink}</div>
    ${S.info.local ? `<div class="card"><h2>Driver</h2><p class="sub" style="margin:0 0 10px">${/driverless|everywhere/i.test(st.model || '')
      ? 'Printing without a driver. The maker\'s driver is often faster and sharper on inkjets.' : esc((st.model || '').replace(/\s*CUPS$/, '')) + ': the maker\'s driver.'}</p>
      <button class="btn wide" id="drivers">${ic('printer')} Printers & drivers</button></div>` : ''}
    ${st.commands.length ? `<div class="card"><h2>Maintenance</h2>${st.commands.map(c => `<div class="field"><button class="btn wide" data-cmd="${c}">${cmdName[c][0]}</button><p class="hint">${cmdName[c][1]}</p></div>`).join('')}</div>` : ''}
    <div class="card"><h2>Both-sides setup</h2>
      <p class="sub" style="margin-bottom:10px">${st.profile ? ic('check') + ' Set up. Run it again if double-sided pages ever come out mixed up.' : 'Not set up yet. Do this once so double-sided printing comes out in order. Uses 2 sheets.'}</p>
      <button class="btn main wide" id="calib">Start the 2-sheet setup</button>
      ${st.profile ? '<button class="btn wide" id="forget" style="margin-top:8px">Forget this printer\'s setup</button>' : ''}</div>`);
  const ca = $('#cancelAll'); if (ca) ca.onclick = async () => { if (await ask('<h2>Cancel everything waiting?</h2>', [['yes', 'Yes, cancel']])) { await api('/api/maintenance', { printer: S.printer, action: 'cancel' }).catch(oops); care(); } };
  $('#app').querySelectorAll('[data-cmd]').forEach(b => b.onclick = async () => {
    const c = b.dataset.cmd;
    if (!await ask(`<h2>${cmdName[c][0]}?</h2><p>${cmdName[c][1]}</p>`, [['yes', 'Yes, do it']])) return;
    try { await api('/api/maintenance', { printer: S.printer, action: c }); toast('Sent to the printer'); } catch (e) { oops(e); }
  });
  $('#calib').onclick = calibration;
  const dr = $('#drivers'); if (dr) dr.onclick = () => { location.hash = '#/addprinter'; };
  $('#setType').onclick = async () => { if (await onboarding(S.printer)) care(); };
  const hd = $('#howDuplex'); if (hd) hd.onclick = () => { const m = modal(`<h2>Printing on both sides</h2>${guide(S.style[S.printer], 1, !!(st.profile && st.profile.rot))}<div class="foot"><button class="btn main" data-close>Got it</button></div>`); wireGuide(m.el); };
  const fg = $('#forget'); if (fg) fg.onclick = async () => { await api('/api/profile', { printer: S.printer, forget: true }).catch(oops); S.calibrated[S.printer] = false; care(); };
}

async function calibration() {
  const ok = await ask(`<h2>Both-sides setup</h2><p>This prints big numbers <b>1–4</b> on 2 sheets. Halfway, you'll flip the paper <b>like turning a page in a book</b>. Always flip it that same way afterwards.</p>
    <p class="hint">Load at least 2 sheets of plain paper.</p>`, [['go', 'Start']]);
  if (!ok) return;
  try { const b = await api('/api/test', { printer: S.printer }); startPrint(b, true); } catch (e) { oops(e); }
}

function testQuestions(m, j) {
  const q = { back: 0, top: 0, up: 1 };
  const draw = () => m.set(`<h2>How did it come out?</h2><p class="sub">Pick up the stack exactly as it came out.</p>
    <div class="field"><span class="lbl">What number is on the back of the sheet with <b>1</b>?</span>${seg('back', [[2, '2'], [3, '3'], [4, '4'], [1, '1 again'], [0, 'Nothing']], q.back)}</div>
    <div class="field"><span class="lbl">Which number is on top of the stack, facing you?</span>${seg('top', [[1, '1'], [2, '2'], [3, '3'], [4, '4']], q.top)}</div>
    <div class="field"><span class="lbl">Turn the sheet with 1 over. Where is the ${FLOWER} on the back?</span>
      <div class="choices small"><button class="choice ${q.up === 1 ? 'on' : ''}" data-up="1">${backPic(false)}<b>At the top</b></button>
      <button class="choice ${q.up === 0 ? 'on' : ''}" data-up="0">${backPic(true)}<b>At the bottom</b></button></div></div>
    <div class="foot"><button class="btn main" id="ans" ${q.top ? '' : 'disabled'}>Done</button></div>`, () => {
    m.el.querySelectorAll('[data-seg]').forEach(g => g.onclick = e => { const b = e.target.closest('button'); if (!b) return; q[g.dataset.seg] = Number(b.dataset.v); draw(); });
    m.el.querySelectorAll('[data-up]').forEach(b => b.onclick = () => { q.up = Number(b.dataset.up); draw(); });
    $('#ans').onclick = async () => {
      try {
        const r = await api('/api/profile', { printer: S.printer, answer: true, back: q.back, top: q.top, upsideDown: q.up === 0 });
        const msg = { perfect: ['check', 'Perfect!', 'Double-sided printing is set up. You\'re done!'],
          updated: ['wrench', 'Fixed it!', 'Run the setup once more to double check.'],
          noflip: ['turn', 'The paper wasn\'t flipped', 'Both sides got the same kind of page. Try again and flip the stack like a book page.'],
          unknown: ['help', 'Hmm, that\'s odd', 'Try the setup again, being careful with the flip.'] }[r.result] || ['check', 'Saved', ''];
        m.set(`${bigIcon(msg[0])}<h2 style="text-align:center">${msg[1]}</h2><p style="text-align:center">${msg[2]}</p>
          <div class="foot">${r.result === 'perfect' ? '' : '<button class="btn" id="again">Run it again</button>'}<button class="btn main" data-close>OK</button></div>`);
        const ag = $('#again'); if (ag) ag.onclick = () => { m.close(); calibration(); };
        if (r.result === 'perfect') burst(m.el.querySelector('.bigicon'));
        S.calibrated[S.printer] = true;
      } catch (e) { oops(e); }
    };
  });
  draw();
}
