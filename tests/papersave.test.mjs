// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Paper savers on the print screen: pages per sheet, booklet, leaving out blank pages.
import { readFileSync } from 'node:fs';
export const needsPrinter = true;

export default async function (t) {
  const p = await t.launch();
  await p.go(t.base + '/#/docs'); await p.waitFor('[data-src=pc]');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  const b64 = readFileSync(t.fixture('withblank.pdf')).toString('base64');   // page, blank page, page
  await p.eval(`(async () => { const bin = Uint8Array.from(atob(${JSON.stringify(b64)}), c => c.charCodeAt(0));
    const fd = new FormData(); fd.append('files', new File([bin], 'three pages.pdf', { type: 'application/pdf' }));
    const f = (await (await fetch('/api/upload', { method: 'POST', body: fd })).json()).files[0];
    Object.assign(S.doc, { sides: 'single', layout: '', skipBlank: false, copies: 1, cover: 1 });
    S.docs = [f]; openWork('docs'); })()`);
  await p.waitFor('.pvpage img'); await p.idle(600);
  const pages = () => p.eval(`document.querySelectorAll('.pvpage').length`);
  const title = () => p.eval(`document.querySelector('#pvtitle').textContent`);
  t.ok(await pages() === 3, 'the PDF shows its 3 pages');
  await p.tap('.pvacts [data-x=set]', 0, false); await p.sleep(400);
  const change = async (sel, val) => { await p.select(sel, val); await p.sleep(1500); await p.idle(500); };
  const toggle = async sel => { await p.eval(`document.querySelector('${sel}').click(); 0`); await p.sleep(1500); await p.idle(500); };

  await toggle('#pvskip');
  t.ok(await pages() === 2 && /left out 1 blank page/.test(await p.eval(`document.querySelector('#pvnote').textContent`)), 'Leave out blank pages: the blank one is gone, and it says so');
  await change('#pvnup', '2up');
  t.ok(await pages() === 1, `2 pages per sheet: the 2 pages fit on 1 sheet (${await title()})`);
  const img = await p.eval(`(async () => { const i = document.querySelector('.pvpage img'); await i.decode(); return { w: i.naturalWidth, h: i.naturalHeight }; })()`);
  t.ok(img.h > img.w, 'the sheet is an upright A4');
  await change('#pvnup', '4up');
  t.ok(await pages() === 1, '4 pages per sheet: 1 sheet');
  await toggle('#pvskip');
  await change('#pvnup', 'booklet');
  t.ok(await p.eval(`S.doc.sides`) === 'duplex' && await p.eval(`document.querySelector('#pvsides [data-v=duplex]').classList.contains('on')`), 'Booklet switches on both sides by itself');
  t.ok(await pages() === 2 && /1 sheet/.test(await title()), `a 3-page booklet is one sheet, printed on both sides (${await title()})`);
  await p.eval(`document.querySelector('.pv [data-x=set]').click(); 0`); await p.sleep(300);
  await p.eval(`document.querySelector('.pvacts [data-x=edit]').click(); 0`); await p.sleep(400);
  t.ok(/can't be edited one by one/.test(await p.eval(`[...document.querySelectorAll('.toast')].map(x => x.textContent).join(' ')`)), 'Edit this page explains why it can\'t, with pages sharing sheets');
  await change('#pvnup', '');
  t.ok(await pages() === 4, 'back to 1 per sheet: the 3 pages, plus a blank so both sides line up');
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
