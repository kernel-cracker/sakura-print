// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Found by an intermittent failure: draw, then tap Save straight away, on a busy computer. The editor adds a change
// to its history a moment after it's made; Save used to decide "nothing changed" before that, and closed without
// saving the drawing. Here the moment is made long on purpose (as on a busy computer), so it happens every time.
import { readFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';

export default async function (t) {
  const p = await t.launch();
  // a busy computer: the editor's short "in a moment" timers (40 ms history, 900 ms autosave) run 5 seconds late.
  // The real timer is kept, so each case can go back to normal (open() does).
  const busy = () => p.eval(`(() => { window.__st = window.__st || window.setTimeout; const st = window.__st; window.setTimeout = (f, ms, ...a) => st(f, ms === 40 || ms === 900 ? 5000 : ms, ...a); })(); 0`);
  await p.go(t.base + '/#/docs'); await p.waitFor('[data-src=pc]');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  const b64 = readFileSync(t.fixture('twopage.pdf')).toString('base64');
  await p.eval(`(async () => { const bin = Uint8Array.from(atob(${JSON.stringify(b64)}), c => c.charCodeAt(0));
    const fd = new FormData(); fd.append('files', new File([bin], 'quick.pdf', { type: 'application/pdf' }));
    const f = (await (await fetch('/api/upload', { method: 'POST', body: fd })).json()).files[0];
    S.docs.push(f); openWork('docs'); })()`);
  await p.waitFor('.pvpage img'); await p.idle(600);
  await p.tap('.pvacts [data-x=edit]', 0, false);
  await p.waitFor('.ed cropper-canvas', 20000);
  await p.eval(`new Promise(r => { const t = setInterval(() => { if (!document.querySelector('#edspin')) { clearInterval(t); r(); } }, 50); })`);
  await p.tap('.edtools [data-tab=markup]', 0, false);
  await p.eval(`new Promise(r => { const t = setInterval(() => { if (document.querySelector('.ed canvas.upper-canvas')) { clearInterval(t); r(); } }, 50); })`);
  await p.sleep(300);
  await busy();
  const b = await p.eval(`(() => { const r = document.querySelector('#edbg').getBoundingClientRect(); return { x: r.x, y: r.y, w: r.width, h: r.height }; })()`);
  await p.send('Input.dispatchMouseEvent', { type: 'mousePressed', x: b.x + b.w * .2, y: b.y + b.h * .5, button: 'left', clickCount: 1 });
  for (let i = 1; i <= 10; i++) await p.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: b.x + b.w * (.2 + i * .06), y: b.y + b.h * .5, button: 'left', buttons: 1 });
  await p.send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: b.x + b.w * .8, y: b.y + b.h * .5, button: 'left', clickCount: 1 });
  await p.eval(`document.querySelector('.edtop [data-x=save]').click(); 0`);   // straight away
  const ms = await p.eval(`new Promise(r => { const t0 = Date.now(); const t = setInterval(() => {
    const ok = S.docs[0] && S.docs[0].name.includes('(edited)');
    if (ok || Date.now() - t0 > 20000) { clearInterval(t); r(ok ? Date.now() - t0 : -1); } }, 50); })`);
  t.ok(ms >= 0, `drawing, then Save straight away on a busy computer: the drawing is saved (${ms} ms)`);
  await p.idle(800);   // back on the print screen, with the edited copy

  // the same for Undo and for closing: they act on a change made a moment ago too
  // each case starts from a page with no edits yet (earlier cases' autosaves removed), with the drawing tools open
  const open = async () => {
    await p.eval(`window.setTimeout = window.__st || window.setTimeout; 0`);
    rmSync(join(t.dataDir, 'edits'), { recursive: true, force: true });
    await p.tap('.pvacts [data-x=edit]', 0, false);
    await p.waitFor('.ed cropper-canvas', 20000);
    await p.eval(`new Promise(r => { const t = setInterval(() => { if (!document.querySelector('#edspin')) { clearInterval(t); r(); } }, 50); })`);
    await p.tap('.edtools [data-tab=markup]', 0, false);
    const ready = await p.eval(`new Promise(r => { const t0 = Date.now(); const t = setInterval(() => {
      const ok = document.querySelector('.edtools [data-tab=markup]').classList.contains('on') && document.querySelector('.ed canvas.upper-canvas');
      if (ok || Date.now() - t0 > 10000) { clearInterval(t); r(!!ok); } }, 50); })`);
    const fresh = await p.eval(`!document.querySelector('.edbanner') && document.querySelector('.edtop [data-x=undo]').disabled`);
    if (!ready || !fresh) throw new Error(`the editor isn't ready to draw on a fresh page (drawing tools: ${ready}, fresh: ${fresh})`);
    await p.sleep(300);
  };
  const stroke = async y => {
    const b = await p.eval(`(() => { const r = document.querySelector('#edbg').getBoundingClientRect(); return { x: r.x, y: r.y, w: r.width, h: r.height }; })()`);
    await p.send('Input.dispatchMouseEvent', { type: 'mousePressed', x: b.x + b.w * .2, y: b.y + b.h * y, button: 'left', clickCount: 1 });
    for (let i = 1; i <= 10; i++) await p.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: b.x + b.w * (.2 + i * .06), y: b.y + b.h * y, button: 'left', buttons: 1 });
    await p.send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: b.x + b.w * .8, y: b.y + b.h * y, button: 'left', clickCount: 1 });
  };

  await open();
  await stroke(.3); await p.sleep(1200);   // the first line, settled
  t.ok(await p.eval(`!document.querySelector('.edtop [data-x=undo]').disabled`), 'a line was drawn (it can be undone)');
  await busy();
  await stroke(.6);                         // the second, and Undo straight away
  await p.eval(`document.querySelector('.edtop [data-x=undo]').click(); 0`);
  await p.sleep(200);
  t.ok(await p.eval(`!document.querySelector('.edtop [data-x=undo]').disabled`), 'Undo straight after a line takes back that line only (the first is still there to undo)');
  await p.eval(`document.querySelector('.edtop [data-x=cancel]').click(); 0`);
  await p.sleep(500);

  // closing straight after a line: the autosave has it, so it's there next time
  await open();
  await p.eval(`(() => { window.__saved = []; const f = window.fetch; window.fetch = (u, o) => { if (String(u).includes('/api/edit/') && o && o.body) window.__saved.push(String(o.body)); return f(u, o); }; })(); 0`);
  await busy();
  await stroke(.8);
  await p.eval(`document.querySelector('.edtop [data-x=cancel]').click(); 0`);
  await p.sleep(1500);
  // the last autosave's newest history entry has the line in it (the history is JSON inside JSON: decoded, not searched)
  const lastHas = await p.eval(`(() => { const b = window.__saved[window.__saved.length - 1]; if (!b) return 'no autosave at all';
    const h = JSON.parse(b).state.hist; const newest = JSON.parse(h[h.length - 1]);
    return newest.objs.some(o => /^path$/i.test(o.type)) ? 'yes' : 'newest entry has ' + newest.objs.length + ' things, none a line'; })()`);
  t.ok(lastHas === 'yes', `closing straight after drawing: the drawing is in the autosave (${lastHas})`);
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
