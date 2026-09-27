// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Crop frame handles are big enough for fingers.
import { readFileSync, readdirSync, writeFileSync } from 'node:fs';

export default async function (t) {
  const p = await t.launch();
  await p.go(`${t.base}/#/`); await p.waitFor('.ftile');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); 0`);
  const b64 = readFileSync(t.fixture('quad.jpg')).toString('base64');
  await p.eval(`(async () => { const bin = Uint8Array.from(atob(${JSON.stringify(b64)}), c => c.charCodeAt(0));
    const fd = new FormData(); fd.append('files', new File([bin], 'quad.jpg', { type: 'image/jpeg' }));
    window.__f = (await (await fetch('/api/upload', { method: 'POST', body: fd })).json()).files[0]; await fetch('/api/edit/' + __f.id, { method: 'DELETE' }); openEditor(__f, () => {}); })()`);
  await p.eval(`new Promise(r => { const t = setInterval(() => { if (document.querySelector('.ed cropper-canvas') && !document.querySelector('#edspin')) { clearInterval(t); r(); } }, 50); })`);
  await p.sleep(400);
  const sizes = await p.eval(`JSON.stringify([...document.querySelectorAll('cropper-handle[action$=-resize]')].map(h => { const r = h.getBoundingClientRect(); return h.getAttribute('action').replace('-resize', '') + ':' + Math.round(r.width) + 'x' + Math.round(r.height); }))`);
  t.ok(['nw', 'ne', 'se', 'sw'].every(d => sizes.includes(`"${d}:44x44"`)), 'corner handles are 44x44');
  const sel = () => p.eval(`(() => { const s = document.querySelector('cropper-selection'), c = document.querySelector('cropper-canvas').getBoundingClientRect(); return { x: s.x, y: s.y, w: s.width, h: s.height, cx: c.x, cy: c.y }; })()`);
  async function finger(x0, y0, x1, y1) {
    await p.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: x0, y: y0 }] });
    for (let i = 1; i <= 10; i++) { await p.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: x0 + (x1 - x0) * i / 10, y: y0 + (y1 - y0) * i / 10 }] }); await p.sleep(16); }
    await p.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] }); await p.sleep(250);
  }
  for (const [corner, fx, fy, dx, dy] of [['top-left', 0, 0, 14, 14], ['bottom-right', 1, 1, -14, -14]]) {
    const s0 = await sel();
    const x = s0.cx + s0.x + fx * s0.w + dx, y = s0.cy + s0.y + fy * s0.h + dy;   // 14 px inside the corner
    await finger(x, y, x + (fx ? -40 : 40), y + (fy ? -40 : 40));
    const s1 = await sel();
    t.ok(Math.abs((s0.w - s1.w) - 40) < 3 && Math.abs((s0.h - s1.h) - 40) < 3, `${corner}: a finger 14 px inside the corner resizes the frame (w ${s0.w.toFixed(0)} -> ${s1.w.toFixed(0)}, h ${s0.h.toFixed(0)} -> ${s1.h.toFixed(0)})`);
  }
  { // an edge grip
    const s0 = await sel(), x = s0.cx + s0.x + s0.w / 2, y = s0.cy + s0.y - 10;   // 10 px outside the top edge
    await finger(x, y, x, y + 30);
    const s1 = await sel();
    t.ok(Math.abs((s0.h - s1.h) - 30) < 3 && Math.abs(s0.w - s1.w) < 1, `top edge: grabbed 10 px outside it, only the height changes (h ${s0.h.toFixed(0)} -> ${s1.h.toFixed(0)})`);
  }
  { // the middle still moves the whole frame
    const s0 = await sel(), x = s0.cx + s0.x + s0.w / 2, y = s0.cy + s0.y + s0.h / 2;
    await finger(x, y, x + 20, y + 10);
    const s1 = await sel();
    t.ok(Math.abs(s1.w - s0.w) < 1 && Math.abs(s1.x - s0.x - 20) < 3, `dragging the middle still moves the frame (x ${s0.x.toFixed(0)} -> ${s1.x.toFixed(0)})`);
  }
  await p.shot(t.shots + '/crop-handles.png');
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
