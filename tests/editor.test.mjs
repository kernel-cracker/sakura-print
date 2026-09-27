// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// The photo editor: every tab, checked in the saved picture where possible.
import { readFileSync, readdirSync, writeFileSync } from 'node:fs';

export default async function (t) {
  const p = await t.launch();
  await p.go(t.base + '/#/'); await p.waitFor('.ftile');
  await p.eval(`document.querySelectorAll('.modal').forEach(m => m.remove()); try { localStorage.clear(); } catch (_) {} 0`);
  const b64 = readFileSync(t.fixture('quad.jpg')).toString('base64');
  await p.eval(`(async () => {
    const bin = Uint8Array.from(atob(${JSON.stringify(b64)}), c => c.charCodeAt(0));
    const fd = new FormData(); fd.append('files', new File([bin], 'quad.jpg', { type: 'image/jpeg' }));
    window.__f = (await (await fetch('/api/upload', { method: 'POST', body: fd })).json()).files[0];
    window.__open = async () => { window.__saved = null; await fetch('/api/edit/' + window.__f.id, { method: 'DELETE' }); openEditor(window.__f, nf => { window.__saved = nf; }); };
    window.__px = async pts => { const bmp = await createImageBitmap(await (await fetch('/api/file/' + __saved.id)).blob());
      const c = new OffscreenCanvas(bmp.width, bmp.height), g = c.getContext('2d'); g.drawImage(bmp, 0, 0);
      return { w: bmp.width, h: bmp.height, px: pts.map(([fx, fy]) => [...g.getImageData(Math.floor(fx * (bmp.width - 1)), Math.floor(fy * (bmp.height - 1)), 1, 1).data].slice(0, 3)) }; };
  })()`);
  const colour = ([r, g, b]) => r > 180 && g < 90 && b < 90 ? 'red' : g > 140 && r < 90 && b < 110 ? 'green' : b > 170 && r < 90 && g < 130 ? 'blue' : r > 200 && g > 170 && b < 90 ? 'yellow' : r > 225 && g > 225 && b > 225 ? 'white' : r < 70 && g < 70 && b < 70 ? 'black' : `rgb(${r},${g},${b})`;
  const fc = e => p.eval(`(() => { const fc = document.querySelector('#edview').fc; return ${e}; })()`);
  async function open() { await p.eval('__open()'); await p.waitFor('.ed cropper-canvas'); await p.eval(`new Promise(r => { const t = setInterval(() => { if (!document.querySelector('#edspin')) { clearInterval(t); r(); } }, 50); })`); await p.sleep(300); }
  const tab = async t => { await p.tap(`.edtools [data-tab=${t}]`, 0, false); await p.sleep(400); };
  const sub = async s => { await p.tap(`.edsubs [data-sub=${s}]`, 0, false); await p.sleep(250); };
  const tap = async s => {   // scroll it into view inside its row first, like a thumb would, then tap
    await p.eval(`(() => { const e = document.querySelector(${JSON.stringify(s)}); if (e) e.scrollIntoView({ block: 'nearest', inline: 'center' }); })()`);
    await p.sleep(80); await p.tap(s, 0, false); await p.sleep(300); };
  const setRange = (sel, v) => p.eval(`(() => { const r = document.querySelector('${sel}'); r.value = ${v}; r.dispatchEvent(new Event('input')); r.dispatchEvent(new Event('change')); })()`).then(() => p.sleep(500));
  async function save() { await p.tap('.edtop [data-x=save]', 0, false); await p.eval(`new Promise(r => { const t = setInterval(() => { if (window.__saved) { clearInterval(t); r(); } }, 50); })`); }
  const area = () => p.eval(`(() => { const b = document.querySelector('#edbg').getBoundingClientRect(); return { x: b.x, y: b.y, w: b.width, h: b.height }; })()`);
  async function drag(from, to, steps = 10) {
    const a = await area(), at = ([fx, fy]) => ({ x: a.x + fx * a.w, y: a.y + fy * a.h });
    await p.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [at(from)] });
    for (let i = 1; i <= steps; i++) { await p.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [at([from[0] + (to[0] - from[0]) * i / steps, from[1] + (to[1] - from[1]) * i / steps])] }); await p.sleep(16); }
    await p.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] }); await p.sleep(250);
  }
  const px = pts => p.eval(`__px(${JSON.stringify(pts)})`);
  const C4 = [[0.1, 0.1], [0.9, 0.1], [0.1, 0.9], [0.9, 0.9]];
  const corners = r => r.px.slice(0, 4).map(colour).join(' ');

  // ---- crop ----
  await open(); await tap('.edopts [data-flip]'); await save();
  let r = await px(C4); t.ok(corners(r) === 'green red yellow blue', `Flip mirrors left-right (${corners(r)})`);
  await open(); await tap('.edopts [data-turn]'); await save();
  r = await px(C4); t.ok(r.w === 1200 && r.h === 1600 && corners(r) === 'blue red yellow green', `Turn (${r.w}x${r.h} ${corners(r)})`);
  await open(); await tap('.edopts [data-ratio=sq]'); await save();
  r = await px([]); t.ok(Math.abs(r.w - r.h) <= 2, `Square crop (${r.w}x${r.h})`);
  await open(); await tap('.edopts [data-st=pv]');
  t.ok(await p.eval(`!document.querySelector('#edview').hidden`), 'Vertical perspective shows the finished picture while adjusting');
  await setRange('#edpersp', 70); await save();
  r = await px([[0.02, 0.02], [0.98, 0.02], [0.02, 0.98], [0.98, 0.98], [0.5, 0.02], [0.5, 0.98]]);
  t.ok(r.px.every(c => colour(c) !== 'white'), `perspective leaves no empty corners (${r.px.map(colour).join(' ')})`);
  t.ok(r.w === 1600 && r.h === 1200, 'perspective keeps full size');

  // ---- adjust ----
  await open(); await tab('adjust'); await tap('.adjbtn[data-p=exposure]'); await setRange('#edadj', 60);
  t.ok(await p.eval(`document.querySelector('.adjbtn[data-p=exposure] text').textContent`) === '60', 'the Exposure ring shows its value');
  await save(); r = await px([[0.25, 0.75]]);
  t.ok(r.px[0][1] > 90, `Exposure +60 brightens (blue corner's green ${r.px[0][1]}, was 60)`);
  await open(); await tab('adjust'); await tap('.adjbtn[data-p=saturation]'); await setRange('#edadj', -100); await save();
  r = await px([[0.25, 0.25]]); t.ok(Math.max(...r.px[0]) - Math.min(...r.px[0]) < 12, `Saturation -100 is grey (${r.px[0]})`);

  // ---- filters ----
  await open(); await tab('filters');
  t.ok(await p.eval(`[...document.querySelectorAll('.fthumb canvas')].every(c => c.width > 10)`), 'every filter has a preview');
  await tap('.fthumb[data-f=mono]'); await save();
  r = await px([[0.25, 0.25], [0.75, 0.75]]); t.ok(r.px.every(c => Math.max(...c) - Math.min(...c) < 8), `Mono filter is grey (${r.px.join(' | ')})`);
  await open(); await tab('filters'); await tap('.fthumb[data-f=noir]'); await setRange('#edfamt', 0);
  await p.eval(`document.querySelector('.edtop [data-x=undo]').disabled`);
  await tap('.fthumb[data-f=vivid]'); await save();
  r = await px([[0.25, 0.25]]); t.ok(r.px[0][0] > 200 && r.px[0][1] < 40, `Vivid makes red more red (${r.px[0]})`);

  // ---- markup ----
  await open(); await tab('markup');
  t.ok(await p.eval(`document.querySelector('.edsubs [data-sub=pen]').classList.contains('on')`), 'Markup starts on the pen');
  await drag([0.6, 0.25], [0.9, 0.25]);
  t.ok(await fc(`fc.getObjects().length === 1 && fc.getObjects()[0].kind === 'pen'`), 'pen draws');
  await drag([0.7, 0.2], [0.8, 0.35]);
  t.ok(await fc(`fc.getObjects().length === 2 && !fc.getActiveObject()`), 'drawing over a drawing makes a new stroke, never grabs the old one');
  await sub('text');
  const a0 = await area();
  await p.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: a0.x + a0.w * 0.75, y: a0.y + a0.h * 0.25 }] });
  await p.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] }); await p.sleep(300);
  t.ok(await p.eval(`!!document.querySelector('#edtxt')`), 'Text: tapping on top of a drawing adds text instead of selecting the drawing');
  await p.eval(`document.querySelector('#edtxt').value = 'HI'; 0`); await tap('[data-c="1"]');
  t.ok(await fc(`fc.getActiveObject() && fc.getActiveObject().kind === 'text'`) && await p.eval(`document.querySelector('.edsubs [data-sub=select]').classList.contains('on')`), 'new text is selected, and the Select tool is shown as active');
  await tap('.edopts [data-act=done]');
  await sub('eraser');
  await drag([0.55, 0.25], [0.95, 0.25]);
  t.ok(await fc(`fc.getObjects().filter(o => o.kind === 'pen').length`) < 2, 'Eraser rubs out what it swipes over');
  // colour sheet
  await sub('pen');
  await tap('.edopts [data-x=more]');
  t.ok(await p.eval(`document.querySelectorAll('.cgrid button').length`) === 108, 'colour grid has 108 colours');
  await tap('.cgrid button:nth-child(40)');
  await setRange('#copa', 50);
  await tap('.chead [data-x=done]');
  const penCol = await fc(`fc.freeDrawingBrush.color`);
  t.ok(/rgba\(.*0\.5\)/.test(penCol), `see-through colour reaches the pen (${penCol})`);
  await tap('.edopts [data-x=more]'); await tap('[data-tab=spec]');
  t.ok(await p.eval(`!!document.querySelector('#cspec')`), 'Spectrum tab opens');
  await tap('.crow [data-x=pick]');
  const a1 = await area();
  await p.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: a1.x + a1.w * 0.25, y: a1.y + a1.h * 0.75 }] });
  await p.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] }); await p.sleep(300);
  const picked = await fc(`fc.freeDrawingBrush.color`);
  t.ok(/^(#1e3|#1f3|#1d3|#1c3|rgba\((2\d|3\d),\s*(5\d|6\d),\s*(2[01]\d))/.test(picked) || /^rgba\(\d+,\d+,2\d\d/.test(picked), `eyedropper takes the blue from the photo (${picked})`);
  // add things
  for (const k of ['bubble', 'star', 'mag']) {
    await tap('.edsubs [data-sub=add]'); await tap(`.addgrid [data-k=${k}]`);
    if (k === 'bubble') { await p.waitFor('#edtxt'); await p.eval(`document.querySelector('#edtxt').value = 'Sign here!'; 0`); await tap('[data-c="1"]'); }
    t.ok(await fc(`fc.getActiveObject() && fc.getActiveObject().kind === '${k}'`), `Add ${k}`);
    await tap('.edopts [data-act=done]');
  }
  await tap('.edsubs [data-sub=add]'); await tap('.addgrid [data-k=sign]');
  const pad = await p.eval(`(() => { const b = document.querySelector('#pad').getBoundingClientRect(); return { x: b.x, y: b.y, w: b.width, h: b.height }; })()`);
  await p.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: pad.x + pad.w * 0.2, y: pad.y + pad.h * 0.6 }] });
  for (let i = 1; i <= 10; i++) { await p.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: pad.x + pad.w * (0.2 + i * 0.06), y: pad.y + pad.h * (0.6 - Math.sin(i) * 0.2) }] }); await p.sleep(16); }
  await p.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  // a person lifts the finger from the pad and then reaches for the button; Chrome's touch emulator drops a
  // tap that comes within ~0.4 s of a drag in this spot (double-tap detection), so pause like a person would
  await p.sleep(800); await tap('[data-x=use]');
  t.ok(await fc(`fc.getActiveObject() && fc.getActiveObject().kind === 'sign'`), 'Signature is added');
  t.ok(await p.eval(`!!localStorage.getItem('sakuraSignature')`), 'and remembered on this phone');
  await tap('.edopts [data-col="#1e63d6"]');
  t.ok(await fc(`fc.getActiveObject().getObjects().every(o => o.stroke === '#1e63d6')`), 'signature can be recoloured');
  await tap('.edopts [data-act=done]');
  await p.shot(t.shots + '/v3-markup-all.png');
  // undo across tabs: change exposure, undo -> back to 0
  await tab('adjust'); await tap('.adjbtn[data-p=exposure]'); await setRange('#edadj', 40);
  await tap('.edtop [data-x=undo]'); await p.sleep(400);
  t.ok(await p.eval(`document.querySelector('.adjbtn[data-p=exposure] text').textContent`) === '·', 'undo takes back an Adjust change');
  await tap('.edtop [data-x=redo]'); await p.sleep(400);
  await tab('markup');
  await save();
  r = await px([[0.5, 0.5]]); t.ok(r.w === 1600 && r.h === 1200, 'saves at full size with everything on it');
  const b = await p.eval(`(async () => { const b = await (await fetch('/api/file/' + __saved.id)).blob(); const a = new Uint8Array(await b.arrayBuffer()); let s = ''; for (const x of a) s += String.fromCharCode(x); return btoa(s); })()`);
  writeFileSync(t.shots + '/v3-saved.jpg', Buffer.from(b, 'base64'));
  // copy & paste edits
  await open(); await tab('adjust'); await tap('.adjbtn[data-p=warmth]'); await setRange('#edadj', 50); await tap('.edopts [data-x=copy]');
  await tap('.edtop [data-x=cancel]'); await open(); await tab('adjust');
  t.ok(await p.eval(`!!document.querySelector('.edopts [data-x=paste]')`), 'Paste edits appears after copying');
  await tap('.edopts [data-x=paste]');
  t.ok(await p.eval(`document.querySelector('.adjbtn[data-p=warmth] text').textContent`) === '50', 'Paste edits brings the warmth over');
  await tap('.edtop [data-x=cancel]');
  t.ok(!p.log.errors.length, 'no JavaScript errors ' + p.log.errors.join(' | '));
  await p.close();
}
