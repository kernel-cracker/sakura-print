// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Print again: recent prints are listed, and one tap rebuilds the same job (never really printed in the test).
import { mkdirSync, copyFileSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
export const needsPrinter = true;

export default async function (t) {
  // a kept print, as the app leaves it after printing: 2 pages, both sides, 3 copies
  const id = 'a1b2c3d4e5f60718', dir = join(t.dataDir, 'history', id);
  rmSync(join(t.dataDir, 'history'), { recursive: true, force: true });
  mkdirSync(dir, { recursive: true });
  copyFileSync(t.fixture('twopage.pdf'), join(dir, 'job.pdf'));
  copyFileSync(t.fixture('quad.jpg'), join(dir, 'thumb.jpg'));
  writeFileSync(join(dir, 'meta.json'), JSON.stringify({ id, when: new Date().toISOString(), title: 'Maths homework', pages: 2, copies: 3, sides: 'duplex', mode: 'docs', printer: 'nope', options: {} }));

  const p = await t.launch();
  await p.go(t.base + '/#/'); await p.waitFor('.ftile');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); window.startPrint = b => { window.__printed = b; }; 0`);   // never really print
  // the tile (the sidebar has a Print again button too, hidden on phones)
  t.ok(await p.eval(`!!document.querySelector('.ftile[data-go=again]')`), 'the home screen has a "Print again" tile');
  await p.tap('.ftile[data-go=again]', 0, false);
  await p.waitFor('.agrow');
  const row = await p.eval(`document.querySelector('.agrow').textContent.replace(/\\s+/g, ' ')`);
  t.ok(/Maths homework/.test(row) && /Today/.test(row) && /2 pages · both sides · 3 copies/.test(row), `a recent print shows what and when (${row.trim().slice(0, 80)})`);
  // the picture loads a moment after its row appears (under load, more than a moment): wait for it to load, or fail
  // after 10 seconds; checking at once made this test fail now and then on a busy computer
  const pic = await p.eval(`new Promise(r => { const i = document.querySelector('.agrow img'); const t0 = Date.now();
    const done = () => r({ ok: i.naturalWidth > 0, ms: Date.now() - t0 });
    if (i.complete) return done(); i.onload = done; i.onerror = () => r({ ok: false, ms: Date.now() - t0 }); setTimeout(() => r({ ok: i.naturalWidth > 0, ms: 10000 }), 10000); })`);
  t.ok(pic.ok, `with a picture of its first page (loaded in ${pic.ms} ms)`);

  await p.tap('[data-again]', 0, false);
  await p.waitFor('.modal [data-c=yes]');
  t.ok(/Print it again/.test(await p.eval(`document.querySelector('.modal:last-child').textContent`)), 'it asks first, so a mis-tap doesn\'t waste paper');
  await p.tap('.modal:last-child [data-c=yes]', 0, false);
  await p.eval(`new Promise(r => { const t = setInterval(() => { if (window.__printed) { clearInterval(t); r(); } }, 50); })`);
  const b = await p.eval('window.__printed');
  t.ok(b.pages === 6 && b.perCopy === 2, `the same job is rebuilt: 2 pages x 3 copies (${b.pages} pages)`);
  t.ok(b.duplex || b.autoDuplex, 'on both sides, like the first time');

  await p.go(t.base + '/#/again'); await p.waitFor('.agrow');
  await p.tap('[data-change]', 0, false);
  await p.waitFor('.pvpage img', 15000);
  t.ok(await p.eval(`S.docs.length === 1 && S.doc.copies === 3 && S.doc.sides === 'duplex'`), '"Change settings" opens the print screen with the same settings');
  await p.eval(`document.querySelector('.pv [data-x=close]').click(); 0`); await p.sleep(300);

  await p.go(t.base + '/#/again'); await p.waitFor('.agrow');
  await p.tap('#agclear', 0, false); await p.waitFor('.modal [data-c=yes]'); await p.tap('.modal:last-child [data-c=yes]', 0, false);
  await p.waitFor('.empty');
  t.ok(/Nothing printed yet/.test(await p.eval(`document.querySelector('.empty').textContent`)), 'Clear empties the list');
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
