// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// First-time setup, on the computer: it opens by itself, sets up everything (the printer, how its paper goes in,
// both sides, phones and PINs, print from any app) and explains everything, then a tour of every function. It
// never gets stuck without saying why. Phones get their own short tour, once.
import http from 'node:http';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
export const needsPrinter = true;
export const restoreAfter = true;   // it runs as a fresh install

const TOUR = ['Documents', 'Photos', 'Text', 'Scan', 'Camera', 'Edit', 'Print again', 'Save paper', 'Both sides', 'Print from any app',
  'Phones', 'Printer care', 'Printers & drivers', 'Notifications'];

export default async function (t) {
  await t.resetSettings({ setup: {}, styles: {}, welcome: false });
  const p = await t.launch();
  await p.go(t.base + '/');
  await p.waitFor('.wizard', 15000);
  t.ok(/#\/setup/.test(await p.eval(`location.hash`)), 'the first time, setup opens by itself');
  const step = () => p.eval(`document.querySelector('.wizard').dataset.step`);
  const next = async () => { const s = await step(); await p.tap('.wizard [data-wz=next]'); for (let e = 0; e < 5000 && await step() === s; e += 100) await p.sleep(100); };
  const shot = async n => p.shot(`${t.shots}/setup-${n}.png`);

  t.ok(await step() === 'welcome', 'it starts with a welcome'); await shot('welcome');
  await next();

  // the printer
  t.ok(await step() === 'printer', 'then the printer');
  const printers = await p.eval(`S.info.printers`);
  const listed = await p.eval(`[...document.querySelectorAll('.wizard [data-wz-printer]')].map(e => e.dataset.wzPrinter)`);
  t.ok(printers.every(x => listed.includes(x)), `every printer is listed (${listed.join(', ')})`);
  t.ok(await p.eval(`!!document.querySelector('.wizard [data-go=addprinter], .wizard [data-wz=drivers]')`), 'with a way to add a printer or get its driver');
  await p.eval(`document.querySelector('.wizard [data-wz-printer]').click(); 0`); await shot('printer');
  await next();

  // what it can do: never a tick while it's still looking
  t.ok(await step() === 'abilities', 'then what the printer can do');
  const lying = await p.eval(`[...document.querySelectorAll('.wizard .wz-cap')].some(c => /Looking/.test(c.textContent) && c.querySelector('.ok'))`);
  t.ok(!lying, 'nothing says "looking" with a tick next to it');
  let scan = '';
  for (let e = 0; e < 40000; e += 500) { scan = await p.eval(`(document.querySelector('.wizard .wz-cap[data-cap=scan]') || {}).textContent || ''`); if (!/Looking/.test(scan)) break; await p.sleep(500); }
  t.ok(scan && !/Looking/.test(scan), `the scanner answer arrives by itself (${scan.replace(/\s+/g, ' ').trim()})`);
  await shot('abilities');
  await next();

  // how the paper goes in: Next says why it waits, then works
  t.ok(await step() === 'paper', 'then how the paper goes in');
  t.ok(await p.eval(`document.querySelector('.wizard [data-wz=next]').disabled`), 'Next waits for a choice');
  t.ok(await p.eval(`(() => { const h = document.querySelector('.wizard .wz-hint'); return !!h && getComputedStyle(h).display !== 'none' && /Pick/.test(h.textContent); })()`), 'and says so: pick the one that looks like yours');
  // two questions: where the blank paper goes in, and which way pages come out (both change the page order)
  await p.eval(`document.querySelector('.wizard [data-style=rear]').click(); 0`); await p.sleep(200);
  t.ok(await p.eval(`document.querySelector('.wizard [data-wz=next]').disabled && /Pick/.test(document.querySelector('.wizard .wz-hint').textContent)`), 'one answer isn\'t enough: it still says what\'s missing');
  await p.eval(`document.querySelector('.wizard [data-face=up]').click(); 0`); await p.sleep(200);
  t.ok(!await p.eval(`document.querySelector('.wizard [data-wz=next]').disabled`), 'with both answers, Next works');
  await shot('paper');
  await next();

  // both sides: explained with the pictures; the 2-sheet setup offered, or later
  t.ok(await step() === 'sides', 'then both sides');
  t.ok(await p.eval(`!!document.querySelector('.wizard .guide, .wizard .wz-auto')`), 'explained with pictures (or that the printer does it by itself)');
  t.ok(await p.eval(`!!document.querySelector('.wizard [data-wz=calib]') || !!document.querySelector('.wizard .wz-auto')`), 'the 2-sheet setup is offered (when printing both sides by hand)');
  await shot('sides');
  await next();

  // phones: the same phone access control as everywhere, and a first PIN
  t.ok(await step() === 'phones', 'then phones');
  t.ok(await p.eval(`!!document.querySelector('.wizard .phoneaccess')`), 'with the phone access control');
  t.ok(/http:\/\//.test(await p.eval(`document.querySelector('.wizard').textContent`)), 'and the address to open on a phone');
  await p.eval(`document.querySelector('.wizard #wzPinLabel').value = 'Mom'; document.querySelector('.wizard #wzPin').value = '482915'; 0`);
  await p.tap('.wizard [data-wz=addpin]');
  let pins = '';
  for (let e = 0; e < 3000 && !/Mom/.test(pins); e += 200) { await p.sleep(200); pins = await p.eval(`(document.querySelector('.wizard .wz-pins') || {}).textContent || ''`); }
  t.ok(/Mom/.test(pins), 'a PIN for Mom is added');
  await shot('phones');
  await next();

  // print from any app, and how double-sided prints from phones are finished
  t.ok(await step() === 'anyapp', 'then print from any app');
  const any = await p.eval(`document.querySelector('.wizard').textContent`);
  t.ok(/Print the other side/.test(any) && /notification/i.test(any) && await p.eval(`!!document.querySelector('.wizard [data-tog=airprint]')`), 'explained, including the notification that prints the other side, with its switch');
  await shot('anyapp');
  await next();

  // the tour: every function
  t.ok(await step() === 'tour', 'then a tour of everything');
  const seen = [];
  for (let i = 0; i < 30 && await step() === 'tour'; i++) {
    seen.push(await p.eval(`document.querySelector('.wizard .tour-slide h2').textContent`));
    if (i === 3) await shot('tour');
    await next();
  }
  const missing = TOUR.filter(w => !seen.some(s => s.includes(w)));
  t.ok(missing.length === 0, `the tour shows every function${missing.length ? ', missing: ' + missing.join(', ') : ''} (${seen.length} slides)`);

  t.ok(await step() === 'done', 'and it ends');
  await p.tap('.wizard [data-wz=finish]');
  await p.waitFor('#top'); await p.sleep(500);
  t.ok(!await p.eval(`!!document.querySelector('.wizard')`) && await p.eval(`location.hash`) === '#/', 'finishing goes home');
  const saved = JSON.parse(readFileSync(join(t.work, 'config', 'sakuraprint', 'settings.json'), 'utf8'));
  t.ok(saved.welcome === true && saved.styles[listed[0]] === 'rear' && saved.pins.length === 1, 'everything chosen is kept');
  await p.go(t.base + '/'); await p.sleep(1500);
  t.ok(!await p.eval(`!!document.querySelector('.wizard')`), 'next time, no setup');
  await p.go(t.base + '/#/settings'); await p.waitFor('[data-wz=again]');
  await p.tap('[data-wz=again]'); await p.waitFor('.wizard');
  t.ok(await step() === 'welcome', 'Settings can run setup again');
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();

  // a phone, the first time: a short tour of what it can do, once
  await t.resetSettings({ requirePin: false });
  const port = new URL(t.base).port;
  // a phone: the browser reaches this computer under another name, so the server treats it as a phone
  const ph = await t.launch({ args: ['--host-resolver-rules=MAP sakura-phone.test 127.0.0.1'] });
  await ph.go(`http://sakura-phone.test:${port}/`);
  await ph.waitFor('.tour', 15000);
  const slides = [];
  for (let i = 0; i < 20 && await ph.eval(`!!document.querySelector('.tour')`); i++) {
    slides.push(await ph.eval(`document.querySelector('.tour .tour-slide h2').textContent`));
    await ph.eval(`document.querySelector('.tour [data-wz=next]').click(); 0`); await ph.sleep(250);
  }
  t.ok(slides.length >= 5 && ['Documents', 'Photos', 'Scan', 'Print from any app'].every(w => slides.some(s => s.includes(w))), `a phone gets a short tour (${slides.join(', ')})`);
  await ph.go(`http://sakura-phone.test:${port}/`); await ph.sleep(2000);
  t.ok(!await ph.eval(`!!document.querySelector('.tour')`), 'once');
  await ph.close();
}
