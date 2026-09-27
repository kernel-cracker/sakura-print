// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// The picture guide for turning the paper over (double-sided printing by hand) is actually on the screen, with its
// pictures, everywhere it's shown: Printer care, setup, and the flip after side one. (Found: a style meant for the
// photo layout's snap lines, also called "guide", had been hiding it.)
export const needsPrinter = true;
export const restoreAfter = true;   // it switches to a fresh install part way

const visible = `(() => { const g = document.querySelector('.modal .guide, .wizard .guide'); if (!g) return 'no guide';
  const b = g.getBoundingClientRect(), art = g.querySelector('.gstep svg, .gstep .flipanim, .gstep .gart *');
  const ab = art ? art.getBoundingClientRect() : { width: 0, height: 0 };
  return getComputedStyle(g).display !== 'none' && b.width > 150 && b.height > 150 && ab.width > 50 && ab.height > 50 ? 'visible' : 'hidden: ' + Math.round(b.width) + 'x' + Math.round(b.height); })()`;

export default async function (t) {
  const p = await t.launch();
  // Printer care → How both-sides printing works
  await p.go(t.base + '/#/printer'); await p.waitFor('#howDuplex', 15000);
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  await p.tap('#howDuplex'); await p.sleep(600);
  t.ok(await p.eval(visible) === 'visible', `Printer care: the guide's pictures are on the screen (${await p.eval(visible)})`);
  await p.shot(t.shots + '/guide-care.png');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);

  // setup's both-sides step
  await t.resetSettings({ welcome: false, setup: {} });   // a fresh install, which gets the first-time setup
  await p.go(t.base + '/'); await p.waitFor('.wizard', 15000);
  await p.eval(`WZ.step = 'sides'; WZ.style = 'rear'; WZ.manual = true; setupWizard(); 0`); await p.sleep(600);
  t.ok(await p.eval(visible) === 'visible', `setup: the guide's pictures are on the screen (${await p.eval(visible)})`);

  // the flip itself, after side one
  await p.eval(`followJob({ id: 'x', state: 'flip', sheets: 2, rotate: false }); 0`); await p.sleep(600);
  t.ok(await p.eval(visible) === 'visible', `after side one: the guide's pictures are on the screen (${await p.eval(visible)})`);
  await p.shot(t.shots + '/guide-flip.png');
  t.ok(!p.log.errors.filter(e => !/\/api\/job\/x/.test(e)).length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
