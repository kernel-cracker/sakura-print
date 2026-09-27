// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 99: Starting the app (loaded last: the routes point at screens from the other files).
'use strict';

// ---------- routes: which screen each #/address shows ----------
const routes = { '': home, docs: () => startScreen('docs'), photos: () => startScreen('photos'), text: textView, scan, copy,
  printer: care, settings, again, addprinter: printers, advanced, setup: setupWizard };

// ---------- start ----------
async function boot() {
  try { S.info = await api('/api/info'); } catch (e) { if (e.message !== 'locked') render(`<div class="card"><h2>Can't reach Sakura Print</h2><p>${esc(e.message)}</p></div>`); return; }
  applyTheme(S.info.theme);
  S.printer = S.info.default || S.info.printers[0] || '';
  let later = false;
  try { later = sessionStorage.getItem('sakuraSetupLater') === '1'; } catch (_) {}
  // the computer, the first time: setup, which also finds and sets up the printer
  if (S.info.local && !S.info.welcome && !later) { if (location.hash !== '#/setup') location.hash = '#/setup'; else route(); return; }
  if (!S.printer) {   // nothing set up yet: the computer sets one up right here; a phone is told to ask the computer
    if (S.info.local) { if (location.hash !== '#/addprinter') location.hash = '#/addprinter'; else route(); }
    else render(`<h1>No printer set up yet</h1><div class="card"><p>Open Sakura Print on the computer: it finds your printer and sets it up.</p></div>`);
    return;
  }
  // the printer's state for the chip, while its options load; the home screen reuses the answer
  S.firstStatus = api('/api/printer?printer=' + encodeURIComponent(S.printer));
  S.firstStatus.then(st => { S.pname = prettyPrinter(st); S.pstate = stateClass(st); paintChip(); }).catch(() => {});
  await loadOptions(S.printer).catch(oops);
  route();
  let toured = false;
  try { toured = localStorage.getItem('sakuraPhoneTour') === '1'; } catch (_) { toured = true; }
  if (!S.info.local && !toured) setTimeout(phoneTour, 400);
  // a printer set up since: its two questions (the computer only; setup already asked them for the first one)
  else if (S.info.local && !S.setupDone[S.printer]) setTimeout(() => onboarding(S.printer), 450);
}
boot();
