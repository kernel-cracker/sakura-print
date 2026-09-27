// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Pages of PDFs keep their original: edit a page, put it back, edit it again, edit another page.
import { readFileSync, readdirSync, writeFileSync } from 'node:fs';

export default async function (t) {
  const p = await t.launch();
  await p.go(t.base + '/#/'); await p.waitFor('.ftile');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  const b64 = readFileSync(t.fixture('twopage.pdf')).toString('base64');
  await p.eval(`(async () => {
    const bin = Uint8Array.from(atob(${JSON.stringify(b64)}), c => c.charCodeAt(0));
    const fd = new FormData(); fd.append('files', new File([bin], 'two.pdf', { type: 'application/pdf' }));
    window.__pdf = (await (await fetch('/api/upload', { method: 'POST', body: fd })).json()).files[0];
    // what "Edit this page" does in the app: page -> picture -> editor -> finished picture back into (a copy of) the PDF
    window.__editPage = async (page, fresh) => { window.__next = null;
      const img = await api('/api/pageimage', { file: __pdf.id, page });
      if (fresh) await fetch('/api/edit/' + img.id, { method: 'DELETE' });   // forget edits other tests made to this page
      openEditor(img, async nf => { window.__next = await api('/api/replacepage', { file: __pdf.id, page, image: nf.id }); }); };
  })()`);
  const ready = () => p.eval(`new Promise(r => { const t = setInterval(() => { if (document.querySelector('.ed cropper-canvas') && !document.querySelector('#edspin')) { clearInterval(t); r(); } }, 50); })`).then(() => p.sleep(800));
  const fc = e => p.eval(`(() => { const fc = document.querySelector('#edview').fc; return ${e}; })()`);
  async function draw() {
    await p.eval(`document.querySelector('.edtools [data-tab=markup]').click(); 0`); await p.sleep(400);
    const a = await p.eval(`(() => { const b = document.querySelector('#edbg').getBoundingClientRect(); return { x: b.x, y: b.y, w: b.width, h: b.height }; })()`);
    await p.send('Input.dispatchMouseEvent', { type: 'mousePressed', x: a.x + a.w * .3, y: a.y + a.h * .6, button: 'left', buttons: 1, clickCount: 1 });
    for (let i = 1; i <= 10; i++) { await p.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: a.x + a.w * (.3 + i * .04), y: a.y + a.h * .6, button: 'left', buttons: 1 }); await p.sleep(16); }
    await p.send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: a.x + a.w * .7, y: a.y + a.h * .6, button: 'left', clickCount: 1 }); await p.sleep(400);
  }
  const done = async () => { await p.eval(`document.querySelector('.edtop [data-x=save]').click(); 0`); await p.eval(`new Promise(r => { const t = setInterval(() => { if (window.__next) { clearInterval(t); r(); } }, 50); })`); await p.eval(`window.__pdf = window.__next; 0`); await p.sleep(300); };

  await p.eval('__editPage(1, true)'); await ready(); await draw(); await done();
  t.ok(await p.eval(`__pdf.name.includes('(edited)')`), 'page 1 edited and put back into a new copy of the PDF');
  await p.eval('__editPage(1)'); await ready();
  t.ok(await p.eval(`!!document.querySelector('.edbanner')`), 'Edit page 1 of the new copy: "picked up where you left off"');
  await p.eval(`document.querySelector('.edtools [data-tab=markup]').click(); 0`); await p.sleep(400);
  t.ok(await fc(`fc.getObjects().length === 1 && fc.getObjects()[0].kind === 'pen'`), 'the drawing is back as something you can move');
  // edit page 2 of that copy, then check page 1 again in the newest copy
  await p.eval(`document.querySelector('.edtop [data-x=cancel]').click(); 0`); await p.sleep(300);
  await p.eval('__editPage(2, true)'); await ready();
  t.ok(!(await p.eval(`!!document.querySelector('.edbanner')`)), 'page 2 was never edited, so it opens fresh');
  await draw(); await done();
  await p.eval('__editPage(1)'); await ready();
  await p.eval(`document.querySelector('.edtools [data-tab=markup]').click(); 0`); await p.sleep(400);
  t.ok(await fc(`fc.getObjects().length === 1`), 'after editing page 2, page 1 still opens from its original with its drawing');
  await p.eval(`document.querySelector('.edtop [data-x=cancel]').click(); 0`); await p.sleep(300);
  await p.eval('__editPage(2)'); await ready();
  await p.eval(`document.querySelector('.edtools [data-tab=markup]').click(); 0`); await p.sleep(400);
  t.ok(await fc(`fc.getObjects().length === 1`), 'and page 2 opens from its original too');
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
