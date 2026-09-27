// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// The print screen's settings come from the printer's driver, whatever its maker: paper type, quality and colour
// are found by what they do (not by one maker's option names), and every choice is in the driver's own words.
import { readFileSync } from 'node:fs';
export const needsPrinter = true;

export default async function (t) {
  const p = await t.launch();
  await p.go(t.base + '/#/docs'); await p.waitFor('[data-src=pc]');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  const b64 = readFileSync(t.fixture('twopage.pdf')).toString('base64');
  await p.eval(`(async () => { const bin = Uint8Array.from(atob(${JSON.stringify(b64)}), c => c.charCodeAt(0));
    const fd = new FormData(); fd.append('files', new File([bin], 'opts.pdf', { type: 'application/pdf' }));
    const f = (await (await fetch('/api/upload', { method: 'POST', body: fd })).json()).files[0];
    S.docs.push(f); openWork('docs'); })()`);
  await p.waitFor('.pvpage img'); await p.idle(400);
  await p.tap('[data-x=set]', 0, false); await p.sleep(500);
  const labels = sel => p.eval(`[...document.querySelectorAll('${sel} option')].map(o => o.textContent)`);
  const roles = await p.eval(`S.roles[S.printer]`);
  const ppd = await p.eval(`S.labels[S.printer]`);
  t.ok(roles && roles.colour && roles.quality && roles.mediaType, `the driver's colour, quality and paper type options are found (${JSON.stringify(roles)})`);
  const media = await labels('#pvmedia'), quality = await labels('#pvquality');
  const mediaWords = Object.values(ppd[roles.mediaType] || {}), qualityWords = Object.values(ppd[roles.quality] || {});
  t.ok(mediaWords.length > 0 && media.some(m => mediaWords.includes(m)), `paper types in the driver's own words (${media.slice(0, 4).join(', ')})`);
  t.ok(qualityWords.length > 0 && quality.some(q => qualityWords.includes(q)), `quality in the driver's own words (${quality.slice(0, 4).join(', ')})`);
  t.ok(await p.eval(`!!document.querySelector('#pvcolour')`), 'and colour');
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
