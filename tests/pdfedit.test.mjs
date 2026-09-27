// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Edit a page inside a PDF from the print screen and see it come back.
import { readFileSync } from 'node:fs';
export const needsPrinter = true;

export default async function (t) {
  const p = await t.launch();
  await p.go(t.base + '/#/docs'); await p.waitFor('[data-src=pc]');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  // add the test PDF the way "From this device" does, then open the print screen
  const b64 = readFileSync(t.fixture('twopage.pdf')).toString('base64');
  await p.eval(`(async () => { const bin = Uint8Array.from(atob(${JSON.stringify(b64)}), c => c.charCodeAt(0));
    const fd = new FormData(); fd.append('files', new File([bin], 'two pages.pdf', { type: 'application/pdf' }));
    const f = (await (await fetch('/api/upload', { method: 'POST', body: fd })).json()).files[0];
    await fetch('/api/edit/nothing').catch(() => {}); S.docs.push(f); openWork('docs'); })()`);
  await p.waitFor('.pvpage img'); await p.idle(600);
  const before = await p.eval(`document.querySelector('.pvpage img').src`);
  await p.tap('.pvacts [data-x=edit]', 0, false);
  await p.waitFor('.ed cropper-canvas', 20000);
  await p.eval(`new Promise(r => { const t = setInterval(() => { if (!document.querySelector('#edspin')) { clearInterval(t); r(); } }, 50); })`);
  await p.tap('.edtools [data-tab=markup]', 0, false); await p.sleep(400);
  const b = await p.eval(`(() => { const r = document.querySelector('#edbg').getBoundingClientRect(); return { x: r.x, y: r.y, w: r.width, h: r.height }; })()`);
  await p.send('Input.dispatchMouseEvent', { type: 'mousePressed', x: b.x + b.w * .2, y: b.y + b.h * .3, button: 'left', clickCount: 1 });
  for (let i = 1; i <= 10; i++) { await p.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: b.x + b.w * (.2 + i * .06), y: b.y + b.h * .3, button: 'left', buttons: 1 }); await p.sleep(16); }
  await p.send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: b.x + b.w * .8, y: b.y + b.h * .3, button: 'left', clickCount: 1 });
  await p.tap('.edtop [data-x=save]', 0, false);
  // wait for the result itself (the edited copy in the print list), not for what's on screen: several messages can be
  // open at once ("Saving…", "Adding 1 file…"), and reading the wrong one made this test check too early on a busy
  // computer. Fails if the edited copy hasn't arrived after 20 seconds.
  const arrived = await p.eval(`new Promise(r => { const t0 = Date.now(); const t = setInterval(() => {
    const ok = S.docs[0] && S.docs[0].name.includes('(edited)') && !document.querySelector('.ed');
    if (ok || Date.now() - t0 > 20000) { clearInterval(t); r(ok ? Date.now() - t0 : -1); } }, 50); })`);
  await p.idle(800);
  const after = await p.eval(`document.querySelector('.pvpage img').src`);
  if (arrived < 0) {   // say what happened, so a failure can be understood
    const why = await p.eval(`JSON.stringify({ docs: S.docs.map(d => d.name), editor: !!document.querySelector('.ed'),
      modals: [...document.querySelectorAll('.modal')].map(m => m.innerText.replace(/\\s+/g, ' ').slice(0, 80)) })`);
    console.log('    what happened: ' + why);
    console.log('    requests: ' + p.log.requests.filter(r => /api\/(upload|replacepage|pageimage|edit)/.test(r.url)).map(r => `${r.method} ${r.url.replace(/^.*\/api/, '/api')} ${r.status} ${r.ms}ms`).join(', '));
    console.log('    errors: ' + (p.log.errors.join(' | ') || 'none'));
  }
  t.ok(before !== after, 'the edited page went back into the PDF and the preview was rebuilt');
  t.ok(await p.eval(`S.docs[0].name.includes('(edited)')`), 'the print list now has the edited copy of the PDF');
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
