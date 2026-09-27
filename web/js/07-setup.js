// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 07: Setting up: the first-time setup (the wizard), the tours, and each printer's questions.
'use strict';

// ---------- first-time setup: which kind of printer, which way pages come out ----------
// what the printer can do, in plain words (the first setup page)
function capsList(printer, manual) {
  const c = S.caps[printer] || {}, auto = c.canAuto && !manual;
  const row = (on, ico, title, sub) => `<li class="${on ? 'yes' : 'no'}"><span class="cic">${icon(ico)}</span><div><b>${title}</b>${sub ? `<small>${sub}</small>` : ''}</div>
    <span class="cmark">${on ? ic('check') : ic('x')}</span></li>`;
  return `<ul class="caps">
    ${row(c.colour, 'photo', c.colour ? 'Prints in colour' : 'Black & white only', c.colour ? 'and in black & white' : '')}
    ${row(true, 'copy', auto ? 'Prints both sides by itself' : 'Prints both sides with your help',
      auto ? 'Turn on "Both" and it does the rest' : 'It prints one side, you turn the paper over, it prints the other. The app shows you how.')}
    ${c.scanner ? row(true, 'scan', 'Scans and copies', 'Found on your WiFi')
      : c.scanKnown ? row(false, 'scan', 'No scanner found', 'So Scan and Copy are hidden. If it can scan, turn it on and open the app again.')
      : `<li class="pending"><span class="cic">${icon('scan')}</span><div><b>Looking for a scanner…</b><small>Scan and Copy show up if one is found</small></div><span class="cmark"><span class="spin small"></span></span></li>`}
  </ul>
  ${c.canAuto ? `<button class="linkbtn" data-x="duplex">${manual ? 'It can print both sides by itself after all' : 'It can\'t really print both sides by itself'}</button>` : ''}`;
}

function onboarding(printer) {
  return new Promise(async res => {
    try { await loadOptions(printer, true); } catch (e) { oops(e); return res(false); }
    const caps = S.caps[printer] || {};
    const pick = { style: S.style[printer] || '', face: '', manual: caps.canAuto && !caps.autoDuplex };
    const flip = () => !caps.canAuto || pick.manual;   // the tray pictures only matter for turning the paper by hand
    const total = () => flip() ? 3 : 2;
    const m = modal('');
    const card = (k, v, art, title, extra = '') => `<button class="choice ${pick[k] === v ? 'on' : ''}" data-k="${k}" data-v="${v}">${art}<b>${title}</b>${extra}</button>`;
    const step0 = () => m.set(`<div class="obstep">Step 1 of ${total()}</div><h2>Here's what your printer can do</h2>
      <p class="sub">${esc(S.pname || printer)}</p>${capsList(printer, pick.manual)}
      <div class="foot"><button class="btn" data-x="cancel">Later</button><button class="btn main" data-x="next0">Next</button></div>`, wireStep);
    const step1 = () => m.set(`<div class="obstep">Step 2 of 3</div><h2>Which one looks like your printer?</h2>
      <p class="sub">Look at where you put the blank paper.</p>
      <div class="choices">${card('style', 'tray', ART.tray(), 'Drawer at the front', legend('tray'))}${card('style', 'rear', ART.rear(), 'Paper stands up at the back', legend('rear'))}</div>
      <div class="foot"><button class="btn" data-x="back0">Back</button><button class="btn main" data-x="next" ${pick.style ? '' : 'disabled'}>Next</button></div>`, wireStep);
    const step2 = () => m.set(`<div class="obstep">Step ${total()} of ${total()}</div><h2>When a page comes out, what do you see?</h2>
      <p class="sub">Print anything once if you're not sure.</p>
      <div class="choices">${card('face', 'up', ART.faceUp(), 'I can see the printing')}${card('face', 'down', ART.faceDown(), 'I see a blank back')}</div>
      <div class="foot"><button class="btn" data-x="back">Back</button><button class="btn main" data-x="done" ${pick.face ? '' : 'disabled'}>Done</button></div>`, wireStep);
    function wireStep() {
      m.el.querySelectorAll('.choice').forEach(c => c.onclick = () => {
        pick[c.dataset.k] = c.dataset.v;
        m.el.querySelectorAll(`.choice[data-k="${c.dataset.k}"]`).forEach(x => x.classList.toggle('on', x === c));
        const nb = m.el.querySelector('[data-x=next],[data-x=done]'); if (nb) nb.disabled = false;
      });
      m.el.querySelectorAll('[data-x]').forEach(b => b.onclick = async () => {
        const x = b.dataset.x;
        if (x === 'cancel') { m.close(); return res(false); }
        if (x === 'duplex') { pick.manual = !pick.manual; return step0(); }
        if (x === 'back0') return step0();
        if (x === 'back') return flip() ? step1() : step0();
        if (x === 'next0') return flip() ? step1() : step2();
        if (x === 'next') return step2();
        try {
          await api('/api/profile', { printer, style: flip() ? pick.style : '', faceUp: pick.face === 'up', setup: true, duplex: caps.canAuto ? (pick.manual ? 'manual' : 'auto') : '' });
          if (flip()) S.style[printer] = pick.style;
          S.calibrated[printer] = true; S.setupDone[printer] = true;
          await loadOptions(printer, true).catch(() => {});
          m.set(`${bigIcon('check')}<h2 style="text-align:center">All set!</h2>
            ${flip() ? `<p class="sub" style="text-align:center">Here's how printing on both sides works. Swipe to see each step.</p>${guide(pick.style)}`
              : '<p class="sub" style="text-align:center">To print on both sides, just pick "Both". Your printer turns the paper over by itself.</p>'}
            <div class="foot"><button class="btn main" id="obok">Got it</button></div>`, () => { if (flip()) wireGuide(m.el); burst(m.el.querySelector('.bigicon')); });
          m.el.querySelector('#obok').onclick = () => { m.close(); res(true); };
        } catch (e) { oops(e); }
      });
    }
    step0();
  });
}


// ---------- the first-time setup (on the computer): everything set up and explained, then a tour ----------
const WZ_STEPS = ['welcome', 'printer', 'abilities', 'paper', 'sides', 'phones', 'anyapp', 'tour', 'done'];
let WZ = null;   // the wizard's answers, kept while it's open (also while a driver is being installed in between)

// the tour: every function, where to find it, in a sentence
const TOUR_ALL = [
  ['doc', 'Documents', 'PDFs and pictures from this computer or your phone. Pick pages, copies, colour, paper, one or both sides.', 'Print → Documents'],
  ['photo', 'Photos', '1, 2, 4 or 9 to a page, edge to edge, or arrange them yourself on the page.', 'Print → Photos'],
  ['text2', 'Text', 'Type or paste anything, like a recipe or a letter, in any language, and print it.', 'Print → Text'],
  ['scan', 'Scan and copy', 'Scan pages into a PDF or pictures, save them, or print them straight away as a copy.', 'Scan'],
  ['camera', 'Camera scanner', 'No scanner? Take a photo of a page: it\'s found, straightened and cleaned up like a scan.', 'Print → Documents → Scan with the camera'],
  ['edit', 'Edit any page', 'Crop, straighten, brighten, filters, and write or draw on any photo, scan or page of a PDF. Saved as you go.', 'Edit this page, on the print screen'],
  ['redo', 'Print again', 'Everything you print is kept for a month: one tap prints it again, the same way.', 'Home → Print again'],
  ['doc', 'Save paper', '2 or 4 pages on a sheet, booklets, and leaving out blank pages.', 'The print screen → Settings → Save paper'],
  ['copy', 'Both sides', 'Printers that can\'t print both sides by themselves: it prints one side, shows you how to turn the paper over, prints the other.', 'Pick "Both" when printing'],
  ['phone', 'Print from any app', 'On phones, the normal Print button in any app (Photos, WhatsApp, Safari…) prints here, with the printer\'s own options.', 'Settings → Phones'],
  ['phone', 'Phones and PINs', 'Open Sakura Print on a phone on the same WiFi. Everyone gets their own PIN.', 'Settings → Phones'],
  ['printer', 'Printer care', 'Status, ink, cleaning the print head, a test page, and the 2-sheet setup for both sides.', 'Printer'],
  ['plus', 'Printers & drivers', 'Add a printer or get its maker\'s driver, which on small inkjets prints faster and sharper.', 'Settings → Printers & drivers'],
  ['info', 'Notifications', 'The bell: turning paper over, out of paper, low ink, prints waiting, new drivers. A tap deals with it.', 'The bell, at the top'],
  ['info', 'Advanced', 'What\'s going on under the hood, and a report to copy if something goes wrong.', 'Settings → Advanced'],
];
const TOUR_PHONE = ['Documents', 'Photos', 'Camera scanner', 'Scan and copy', 'Print from any app', 'Print again', 'Notifications'];

const tourSlide = ([ico, title, text, where], i, n) => `<div class="tour-slide">${bigIcon(ico)}<h2>${esc(title)}</h2><p>${esc(text)}</p>
  <p class="tour-where">${ic('info')} ${esc(where)}</p><div class="tour-dots">${Array.from({ length: n }, (_, k) => `<i class="${k === i ? 'on' : ''}"></i>`).join('')}</div></div>`;

function setupWizard() {
  if (!S.info.local) { render(`<div class="card"><p style="margin:0">Setting up is done on the computer.</p></div>`); return; }
  if (!WZ) WZ = { step: 'welcome', printer: S.printer || S.info.default || (S.info.printers || [])[0] || '', style: '', face: '', manual: false, tour: 0, pins: null };
  S.wzActive = true;
  const at = WZ_STEPS.indexOf(WZ.step);
  const dots = `<div class="wz-dots">${WZ_STEPS.slice(0, -1).map((s, i) => `<i class="${i < at ? 'done' : i === at ? 'on' : ''}"></i>`).join('')}</div>`;
  const nav = (nextLabel = 'Next', disabled = false, back = true) => `<div class="wz-nav">${back ? '<button class="btn" data-wz="back">Back</button>' : '<span></span>'}
    <button class="btn main" data-wz="next" ${disabled ? 'disabled' : ''}>${nextLabel}</button></div>`;
  const body = {
    welcome: () => `${bigIcon('printer')}<h1>Welcome to Sakura Print</h1>
      <p class="wz-lead">Let's get your printer ready, and show you everything it can do. It takes about two minutes, and you can change anything later in Settings.</p>
      ${nav('Let\'s start', false, false)}<button class="linkbtn" data-wz="skip">I'll do this later</button>`,
    printer: () => {
      const ps = S.info.printers || [];
      return `<h1>Your printer</h1><p class="wz-lead">${ps.length ? 'Pick the printer you use most.' : 'No printer is set up on this computer yet.'}</p>
        <div class="wz-list">${ps.map(p => `<button class="wz-printer choice-row ${p === WZ.printer ? 'on' : ''}" data-wz-printer="${esc(p)}">${icon('printer')}<span>${esc(p)}</span>${p === WZ.printer ? ic('check') : ''}</button>`).join('')}</div>
        <button class="btn wide" data-wz="drivers">${ic('plus')} ${ps.length ? 'Add another printer, or get the maker\'s driver' : 'Find my printer and set it up'}</button>
        <p class="hint">Sakura Print looks on the WiFi and USB, and gets the printer's own driver from its maker when there is one.</p>
        ${nav('Next', !WZ.printer)}${!WZ.printer ? '<p class="wz-hint">Pick a printer to go on.</p>' : ''}`;
    },
    abilities: () => {
      const c = S.caps[WZ.printer] || {}, auto = c.canAuto && !WZ.manual;
      const cap = (k, state, ico, title, sub) => `<li class="wz-cap ${state}" data-cap="${k}"><span class="cic">${icon(ico)}</span><div><b>${title}</b><small>${sub}</small></div>
        <span class="cmark">${state === 'pending' ? '<span class="spin small"></span>' : state === 'yes' ? `<span class="ok">${ic('check')}</span>` : `<span class="no">${ic('x')}</span>`}</span></li>`;
      return `<h1>What it can do</h1><p class="wz-lead">${esc(S.pname || WZ.printer)}</p><ul class="caps">
        ${cap('colour', c.colour ? 'yes' : 'no', 'photo', c.colour ? 'Prints in colour' : 'Black & white only', c.colour ? 'and in black & white' : 'Colour pages print in grey')}
        ${cap('sides', 'yes', 'copy', auto ? 'Prints both sides by itself' : 'Prints both sides with your help', auto ? 'Pick "Both" and it does the rest' : 'It prints one side, you turn the paper over as shown, it prints the other')}
        ${c.scanner ? cap('scan', 'yes', 'scan', 'Scans and copies', 'Found on the WiFi')
          : c.scanKnown ? cap('scan', 'no', 'scan', 'No scanner found', 'Scan and Copy are hidden. If it can scan, switch it on: it\'s found by itself')
          : cap('scan', 'pending', 'scan', 'Looking for a scanner…', 'This takes a few seconds')}</ul>
        ${c.canAuto ? `<button class="linkbtn" data-wz="manual">${WZ.manual ? 'It can print both sides by itself after all' : 'It can\'t really print both sides by itself'}</button>` : ''}
        ${nav()}`;
    },
    paper: () => {
      const choice = (k, v, art, title, extra = '') => `<button class="choice ${WZ[k] === v ? 'on' : ''}" data-${k}="${v}">${art}<b>${title}</b>${extra}</button>`;
      const missing = !WZ.style && !WZ.face ? 'Pick the one that looks like yours, in both questions.' : !WZ.style ? 'Pick where the blank paper goes in.' : !WZ.face ? 'Pick what you see when a page comes out.' : '';
      return `<h1>How the paper goes in and out</h1><p class="wz-lead">So every picture in the app matches your printer, and pages come out in order.</p>
        <h2>Where does the blank paper go?</h2><div class="choices">${choice('style', 'tray', ART.tray(), 'A drawer at the front', legend('tray'))}${choice('style', 'rear', ART.rear(), 'Standing up at the back', legend('rear'))}</div>
        <h2>When a page comes out, what do you see?</h2><p class="hint">Print anything once if you're not sure.</p>
        <div class="choices">${choice('face', 'up', ART.faceUp(), 'The printing')}${choice('face', 'down', ART.faceDown(), 'A blank back')}</div>
        ${nav('Next', !!missing)}<p class="wz-hint" ${missing ? '' : 'hidden'}>${missing}</p>`;
    },
    sides: () => {
      const c = S.caps[WZ.printer] || {}, auto = c.canAuto && !WZ.manual;
      return `<h1>Printing on both sides</h1>${auto
        ? `<div class="wz-auto">${bigIcon('copy')}<p class="wz-lead">Your printer turns the paper over by itself. When printing, pick <b>Both</b>: that's all.</p></div>`
        : `<p class="wz-lead">Your printer prints one side; then you turn the stack over as these pictures show, and it prints the other. Swipe through the steps.</p>
          ${guide(WZ.style || 'tray')}
          <div class="wz-box"><b>Do the 2-sheet setup once</b><p>It prints the numbers 1 to 4 on 2 sheets and asks how they came out, so double-sided pages always come out in order. Load 2 sheets of plain paper.</p>
          <button class="btn" data-wz="calib">Do the 2-sheet setup now</button><p class="hint">Or later: Printer → Both-sides setup.</p></div>`}
        ${nav()}`;
    },
    phones: () => `<h1>Phones</h1><p class="wz-lead">Open Sakura Print on any phone on the same WiFi: it works like an app, and prints from any app too.</p>
      <div class="wz-box"><b>On a phone, open:</b>${(S.info.urls || []).map(u => `<div class="url">${esc(u)}</div>`).join('') || '<p>No WiFi connection found right now.</p>'}
        <p class="hint">Then Share → Add to Home Screen (iPhone), or ⋮ → Add to Home screen (Android), to make it an app.</p></div>
      <div id="wzpa"></div>
      <div class="wz-box"><b>PINs</b><p>Each person gets their own PIN, asked once on their phone. Write it down before adding it: it's stored scrambled, so it can't be shown again.</p>
        <div class="wz-pins">${(WZ.pins || S.info.pins || []).map(p => `<span class="pill">${esc(p.label)}</span>`).join('') || '<span class="hint">No PINs yet.</span>'}</div>
        <div class="row"><input class="big" id="wzPinLabel" placeholder="Who is it for? (Mom)" maxlength="20" style="flex:1 1 140px">
        <input class="big" id="wzPin" placeholder="4 to 8 numbers" inputmode="numeric" maxlength="8" style="flex:1 1 120px">
        <button class="btn main" data-wz="addpin">Add PIN</button></div></div>
      ${nav()}`,
    anyapp: () => `<h1>Print from any app</h1>
      <p class="wz-lead">Phones find this printer by themselves in the normal Print button of any app: Photos, WhatsApp, Safari, Files, Mail…</p>
      <ul class="wz-points"><li>${ic('phone')}<span><b>iPhone:</b> Share → Print. <b>Android:</b> ⋮ → Print. Pick <b>Sakura Print</b>.</span></li>
        <li>${ic('printer')}<span>The phone offers the printer's own paper sizes, paper types, quality, colour and both sides, from its driver.</span></li>
        <li>${ic('copy')}<span><b>Both sides from a phone:</b> side one prints, then this computer shows a <b>notification</b> with a <b>Print the other side</b> button, and the phone's print queue says to turn the paper over. Turn it over as shown, then tap it. The bell has it too.</span></li>
        <li>${ic('info')}<span>Phones that logged in here with a PIN print straight away. Prints from other phones wait on the home screen until you allow them.</span></li></ul>
      <div class="field">${toggle('airprint', WZ.airprint !== false, 'Print from any app')}</div>
      ${WZ.bonjour === false ? '<p class="wz-hint">Phones can\'t find it yet: avahi (Bonjour) isn\'t installed. Run the installer again.</p>' : ''}
      ${nav()}`,
    tour: () => { const i = WZ.tour; return `<h1>Everything it can do</h1>${tourSlide(TOUR_ALL[i], i, TOUR_ALL.length)}${nav(i === TOUR_ALL.length - 1 ? 'Done' : 'Next')}`; },
    done: () => `${bigIcon('check')}<h1>All set</h1><p class="wz-lead">Your printer is ready. Setup is always in Settings if you want to go through it again.</p>
      <div class="wz-nav"><button class="btn" data-wz="back">Back</button><button class="btn main" data-wz="finish">Start printing</button></div>`,
  };
  render(`<div class="wizard" data-step="${WZ.step}">${dots}<div class="wz-body">${body[WZ.step]()}</div></div>`);
  const w = $('.wizard'), q = s => w.querySelector(s);
  if (WZ.step === 'sides' && q('.guide')) wireGuide(w);
  if (WZ.step === 'phones') phoneAccess(q('#wzpa'));
  if (WZ.step === 'done') burst(q('.bigicon'));
  if (WZ.step === 'abilities') {
    const c = S.caps[WZ.printer] || {};
    if (!c.scanKnown && !c.scanner) setTimeout(async () => { if (WZ && WZ.step === 'abilities' && location.hash === '#/setup') { await loadOptions(WZ.printer, true).catch(() => {}); if (WZ && WZ.step === 'abilities') setupWizard(); } }, 2000);
  }
  if (WZ.step === 'anyapp' && WZ.airprint === undefined) api('/api/settings').then(r => { WZ.airprint = r.airprint; WZ.bonjour = r.bonjour; if (WZ.step === 'anyapp') setupWizard(); }).catch(() => {});
  const go = step => { WZ.step = step; setupWizard(); window.scrollTo(0, 0); };
  w.querySelectorAll('[data-wz-printer]').forEach(b => b.onclick = () => guard(async () => {
    WZ.printer = b.dataset.wzPrinter; S.printer = WZ.printer; S.pname = '';
    await api('/api/settings', { printer: WZ.printer }).catch(oops);
    await loadOptions(WZ.printer, true).catch(() => {});
    WZ.style = S.style[WZ.printer] || WZ.style; paintChip(); setupWizard();
  }));
  w.querySelectorAll('[data-style],[data-face]').forEach(b => b.onclick = () => {
    if (b.dataset.style) WZ.style = b.dataset.style; else WZ.face = b.dataset.face;
    setupWizard();
  });
  const tog = q('[data-tog=airprint]');
  if (tog) tog.onclick = () => guard(async () => { WZ.airprint = !(WZ.airprint !== false); await api('/api/settings', { airPrint: WZ.airprint }).catch(oops); setupWizard(); });
  w.querySelectorAll('[data-wz]').forEach(b => b.onclick = () => guard(async () => {
    const x = b.dataset.wz, i = WZ_STEPS.indexOf(WZ.step);
    if (x === 'skip') { try { sessionStorage.setItem('sakuraSetupLater', '1'); } catch (_) {} WZ = null; S.wzActive = false; location.hash = '#/'; return; }
    if (x === 'drivers') { location.hash = '#/addprinter'; return; }
    if (x === 'manual') { WZ.manual = !WZ.manual; return setupWizard(); }
    if (x === 'calib') { S.printer = WZ.printer; return calibration(); }
    if (x === 'addpin') {
      try {
        await api('/api/settings', { addPin: { label: q('#wzPinLabel').value, pin: q('#wzPin').value.trim() } });
        WZ.pins = (await api('/api/settings')).pins; S.info.pins = WZ.pins; toast('PIN added'); setupWizard();
      } catch (e) { oops(e); }
      return;
    }
    if (x === 'back') {
      if (WZ.step === 'tour' && WZ.tour > 0) { WZ.tour--; return setupWizard(); }
      return go(WZ_STEPS[Math.max(0, i - 1)]);
    }
    if (x === 'finish') {
      await api('/api/settings', { welcome: true }).catch(oops);
      S.info.welcome = true; WZ = null; S.wzActive = false; location.hash = '#/'; return;
    }
    // next
    if (WZ.step === 'printer') await loadOptions(WZ.printer, true).catch(() => {});
    if (WZ.step === 'abilities' && !WZ.style) WZ.style = S.style[WZ.printer] || '';
    if (WZ.step === 'paper') {
      const c = S.caps[WZ.printer] || {};
      try {
        await api('/api/profile', { printer: WZ.printer, style: WZ.style, faceUp: WZ.face === 'up', setup: true, duplex: c.canAuto ? (WZ.manual ? 'manual' : 'auto') : '' });
        S.style[WZ.printer] = WZ.style; S.setupDone[WZ.printer] = true; S.calibrated[WZ.printer] = true;
        await loadOptions(WZ.printer, true).catch(() => {});
      } catch (e) { return oops(e); }
    }
    if (WZ.step === 'tour' && WZ.tour < TOUR_ALL.length - 1) { WZ.tour++; return setupWizard(); }
    go(WZ_STEPS[Math.min(WZ_STEPS.length - 1, i + 1)]);
  }));
}

// a phone, the first time: a short tour of what it can do there
function phoneTour() {
  const slides = TOUR_PHONE.map(t => TOUR_ALL.find(s => s[1] === t)).filter(Boolean);
  let i = 0;
  const m = modal('');
  m.el.classList.add('tour');
  const draw = () => {
    m.set(`${tourSlide(slides[i], i, slides.length)}<div class="wz-nav"><button class="btn" data-wz="skip">Skip</button>
      <button class="btn main" data-wz="next">${i === slides.length - 1 ? 'Start' : 'Next'}</button></div>`);
    m.el.querySelectorAll('[data-wz]').forEach(b => b.onclick = () => {
      if (b.dataset.wz === 'next' && i < slides.length - 1) { i++; return draw(); }
      try { localStorage.setItem('sakuraPhoneTour', '1'); } catch (_) {}
      m.close();
    });
  };
  draw();
}
