// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Editor autosave: edit, cancel, reopen, finish, reopen the finished picture, start over, restart, other screen size.
import { readFileSync, readdirSync, writeFileSync } from 'node:fs';

export default async function (t) {
  const colour = ([r, g, b]) => r > 180 && g < 90 && b < 90 ? 'red' : g > 140 && r < 90 && b < 110 ? 'green' : b > 170 && r < 90 && g < 130 ? 'blue' : r > 200 && g > 170 && b < 90 ? 'yellow' : r < 70 && g < 70 && b < 70 ? 'black' : `rgb(${r},${g},${b})`;
  const b64 = readFileSync(t.fixture('quad.jpg')).toString('base64');
  async function start(opts) {
    const p = await t.launch(opts);
    await p.go(t.base + '/#/'); await p.waitFor('.ftile');
    await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
    await p.eval(`(async () => {
      const bin = Uint8Array.from(atob(${JSON.stringify(b64)}), c => c.charCodeAt(0));
      const fd = new FormData(); fd.append('files', new File([bin], 'quad.jpg', { type: 'image/jpeg' }));
      window.__f = (await (await fetch('/api/upload', { method: 'POST', body: fd })).json()).files[0];
      window.__open = f => { window.__saved = null; openEditor(f || window.__f, nf => { window.__saved = nf; }); };
      window.__px = async pts => { const bmp = await createImageBitmap(await (await fetch('/api/file/' + __saved.id)).blob());
        const c = new OffscreenCanvas(bmp.width, bmp.height), g = c.getContext('2d'); g.drawImage(bmp, 0, 0);
        return { w: bmp.width, h: bmp.height, px: pts.map(([fx, fy]) => [...g.getImageData(Math.floor(fx * (bmp.width - 1)), Math.floor(fy * (bmp.height - 1)), 1, 1).data].slice(0, 3)) }; };
    })()`);
    return p;
  }
  const H = p => ({
    open: async f => { await p.eval(`__open(${f || ''})`); await p.waitFor('.ed cropper-canvas'); await p.eval(`new Promise(r => { const t = setInterval(() => { if (!document.querySelector('#edspin')) { clearInterval(t); r(); } }, 50); })`); await p.sleep(700); },
    tab: async t => { await p.eval(`document.querySelector('.edtools [data-tab=${t}]').click(); 0`); await p.sleep(400); },
    fc: e => p.eval(`(() => { const fc = document.querySelector('#edview').fc; return ${e}; })()`),
    draw: async (from, to) => {
      const a = await p.eval(`(() => { const b = document.querySelector('#edbg').getBoundingClientRect(); return { x: b.x, y: b.y, w: b.width, h: b.height }; })()`);
      const at = ([fx, fy]) => ({ x: a.x + fx * a.w, y: a.y + fy * a.h });
      await p.send('Input.dispatchMouseEvent', { type: 'mousePressed', ...at(from), button: 'left', buttons: 1, clickCount: 1 });
      for (let i = 1; i <= 10; i++) { await p.send('Input.dispatchMouseEvent', { type: 'mouseMoved', ...at([from[0] + (to[0] - from[0]) * i / 10, from[1] + (to[1] - from[1]) * i / 10]), button: 'left', buttons: 1 }); await p.sleep(16); }
      await p.send('Input.dispatchMouseEvent', { type: 'mouseReleased', ...at(to), button: 'left', clickCount: 1 }); await p.sleep(300);
    },
    save: async () => { await p.eval(`document.querySelector('.edtop [data-x=save]').click(); 0`); await p.eval(`new Promise(r => { const t = setInterval(() => { if (window.__saved) { clearInterval(t); r(); } }, 50); })`); await p.sleep(300); },
  });

  // 1. edit, then Cancel
  let p = await start(), h = H(p);
  await h.open();
  await h.tab('markup'); await h.draw([0.6, 0.25], [0.9, 0.25]);
  await h.tab('adjust'); await p.eval(`(() => { document.querySelector('.adjbtn[data-p=warmth]').click(); })()`); await p.sleep(150);
  await p.eval(`(() => { const r = document.querySelector('#edadj'); r.value = 40; r.dispatchEvent(new Event('input')); r.dispatchEvent(new Event('change')); })()`);
  await p.sleep(1500);   // autosave waits about a second
  await p.eval(`document.querySelector('.edtop [data-x=cancel]').click(); 0`); await p.sleep(300);
  const files = readdirSync(t.dataDir + '/edits').filter(f => f.endsWith('.json'));
  t.ok(files.length === 1 && readdirSync(t.dataDir + '/edits/originals').length === 1, `autosave written to the cache folder (${files.join(', ')})`);

  // 2. reopen: everything is back, and Undo goes back to the start
  await h.open();
  t.ok(await p.eval(`!!document.querySelector('.edbanner')`), 'reopening shows "picked up where you left off"');
  await h.tab('markup');
  t.ok(await h.fc(`fc.getObjects().length === 1 && fc.getObjects()[0].kind === 'pen'`), 'the drawing is back');
  await h.tab('adjust');
  t.ok(await p.eval(`document.querySelector('.adjbtn[data-p=warmth] text').textContent`) === '40', 'the Warmth setting is back');
  let undos = 0;
  while (await p.eval(`!document.querySelector('.edtop [data-x=undo]').disabled`) && undos < 10) { await p.eval(`document.querySelector('.edtop [data-x=undo]').click(); 0`); await p.sleep(350); undos++; }
  await h.tab('markup');
  t.ok(undos >= 2 && await h.fc(`fc.getObjects().length === 0`) && await p.eval(`document.querySelector('.edtools [data-tab=adjust]').click(), document.querySelector('.adjbtn[data-p=warmth] text').textContent`) === '·', `Undo goes all the way back to the untouched photo (${undos} steps)`);
  for (let i = 0; i < undos; i++) { await p.eval(`document.querySelector('.edtop [data-x=redo]').click(); 0`); await p.sleep(350); }
  await h.tab('markup');
  t.ok(await h.fc(`fc.getObjects().length === 1`), 'and Redo brings it all back');

  // 3. Done, then open the finished picture
  await h.save();
  const done = await p.eval('window.__saved');
  let r = await p.eval(`__px([[0.75, 0.25], [0.1, 0.1]])`);
  t.ok(colour(r.px[0]) !== 'green' && r.w === 1600, `the finished picture has the drawing on it (${r.px.map(colour)})`);
  await h.open(JSON.stringify(done));
  await h.tab('markup');
  t.ok(await h.fc(`fc.getObjects().length === 1`), 'opening the finished picture brings back the drawing as something you can still move');
  t.ok(await p.eval(`document.querySelector('cropper-image').getAttribute('src').indexOf(${JSON.stringify(done.id)}) < 0`), 'and it edits the original photo, not the flattened copy');
  // 4. start over
  await p.eval(`document.querySelector('.edbanner button').click(); 0`); await p.sleep(300);
  await p.eval(`document.querySelector('.modal:last-child [data-c=yes]').click(); 0`);
  await p.waitFor('.ed cropper-canvas'); await p.sleep(1500);
  await h.tab('markup');
  t.ok(await h.fc(`fc.getObjects().length === 0`) && !(await p.eval(`!!document.querySelector('.edbanner')`)), 'Start over opens the original with nothing on it');
  await p.eval(`document.querySelector('.edtop [data-x=cancel]').click(); 0`);
  // leave an autosave for the next steps
  await h.open(); await h.tab('markup'); await h.draw([0.6, 0.25], [0.9, 0.25]); await p.sleep(1500);
  await p.eval(`document.querySelector('.edtop [data-x=cancel]').click(); 0`);
  await p.close();

  // 5. restart the server; the same photo added again finds its autosave
  await t.restart();
  // 6. ...on a laptop-sized screen this time
  p = await start({ width: 1100, height: 820, mobile: false }); h = H(p);
  await h.open(); await h.tab('markup');
  t.ok(await h.fc(`fc.getObjects().length === 1`), 'after a restart, the same photo added again finds its autosave');
  await h.save();
  r = await p.eval(`__px([[0.75, 0.25], [0.62, 0.25], [0.88, 0.25], [0.75, 0.4], [0.75, 0.1]])`);
  t.ok(r.px.slice(0, 3).every(c => colour(c) === 'red') && r.px.slice(3).every(c => colour(c) === 'green'),
    `a phone-size edit continued on a laptop lands on the same spot of the photo (${r.px.map(colour).join(' ')})`);
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
